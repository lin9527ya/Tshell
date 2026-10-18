package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"BackendTemplate/pkg/collab"
	"BackendTemplate/pkg/database"

	"github.com/gin-gonic/gin"
)

// setupCollabTest 每个用例一套干净的注册表（Global 是包级变量，用例间必须隔离）。
func setupCollabTest(t *testing.T) {
	t.Helper()
	old := collab.Global
	collab.Global = collab.NewRegistry()
	t.Cleanup(func() { collab.Global = old })
}

// collabRouter 模拟 protected 组：中间件只做 AuthMiddleware 的用户信息注入，
// 用 X-Test-User 头代替真实 JWT（认证本身不在本用例范围内）。
func collabRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Set("username", u)
			c.Set("user_id", u)
		}
		c.Next()
	})
	r.GET("/collab/operators", ListOperators)
	r.GET("/collab/locks", ListSessionLocks)
	r.POST("/collab/locks/:uid", AcquireSessionLock)
	r.DELETE("/collab/locks/:uid", ReleaseSessionLock)
	r.POST("/client/shell/sendcommand", SendCommands)
	r.POST("/client/batch-command", BatchCommand)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path, user, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body == "" {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("响应不是 JSON: %v (%s)", err, w.Body.String())
	}
	return m
}

// TestOperatorPresenceAPI 在线列表来自 AuthMiddleware 的 Touch（这里直接喂 registry）。
func TestOperatorPresenceAPI(t *testing.T) {
	setupCollabTest(t)
	r := collabRouter()

	collab.Global.Touch("alice", "admin", "10.0.0.1")
	collab.Global.Touch("bob", "operator", "10.0.0.2")

	w := doJSON(t, r, http.MethodGet, "/collab/operators", "alice", "")
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 200", w.Code)
	}
	var resp struct {
		Data []collab.Operator `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0].Username != "alice" || resp.Data[0].Role != "admin" ||
		resp.Data[0].LastIP != "10.0.0.1" || resp.Data[1].Username != "bob" {
		t.Fatalf("在线列表不符: %+v", resp.Data)
	}
}

// TestSessionLockAPI 抢锁 200 → 他人抢锁 409（带持有人）→ 非持有人释放 404 → 持有人释放 200。
func TestSessionLockAPI(t *testing.T) {
	setupCollabTest(t)
	r := collabRouter()

	if w := doJSON(t, r, http.MethodPost, "/collab/locks/uid-1", "alice", `{"reason":"排查"}`); w.Code != http.StatusOK {
		t.Fatalf("首次抢锁状态码 = %d，期望 200（%s）", w.Code, w.Body.String())
	}
	// 同一人重复抢 = 续期，仍 200
	if w := doJSON(t, r, http.MethodPost, "/collab/locks/uid-1", "alice", ""); w.Code != http.StatusOK {
		t.Fatalf("同持有人续期状态码 = %d，期望 200", w.Code)
	}

	w := doJSON(t, r, http.MethodPost, "/collab/locks/uid-1", "bob", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("他人抢锁状态码 = %d，期望 409", w.Code)
	}
	if got := decodeBody(t, w)["locked_by"]; got != "alice" {
		t.Fatalf("冲突响应未指明持有人: %v", got)
	}

	// 锁列表可见
	var locks struct {
		Data []collab.Lock `json:"data"`
	}
	lw := doJSON(t, r, http.MethodGet, "/collab/locks", "bob", "")
	json.Unmarshal(lw.Body.Bytes(), &locks)
	if len(locks.Data) != 1 || locks.Data[0].UID != "uid-1" || locks.Data[0].Owner != "alice" {
		t.Fatalf("锁列表不符: %+v", locks.Data)
	}

	if w := doJSON(t, r, http.MethodDelete, "/collab/locks/uid-1", "bob", ""); w.Code != http.StatusNotFound {
		t.Fatalf("非持有人释放状态码 = %d，期望 404", w.Code)
	}
	if w := doJSON(t, r, http.MethodDelete, "/collab/locks/uid-1", "alice", ""); w.Code != http.StatusOK {
		t.Fatalf("持有人释放状态码 = %d，期望 200", w.Code)
	}
	if h := collab.Global.Holder("uid-1"); h != "" {
		t.Fatalf("释放后仍有持有人: %q", h)
	}
}

// TestSessionLockBlocksWrites 锁生效面：控制台命令 / 批量下发被他人锁定时 409，
// 持有人自己不受影响（软锁：无人持锁时行为完全不变）。
func TestSessionLockBlocksWrites(t *testing.T) {
	setupCollabTest(t)
	setupTaskTest(t) // SendCommands/BatchCommand 会写 shell 历史与任务表
	r := collabRouter()

	collab.Global.Touch("alice", "admin", "")
	collab.Global.Touch("bob", "operator", "")
	if _, ok, _ := collab.Global.Acquire("uid-1", "alice", ""); !ok {
		t.Fatal("alice 抢锁失败")
	}

	// 他人下命令 → 409，且不产生任何任务行
	w := doJSON(t, r, http.MethodPost, "/client/shell/sendcommand", "bob", `{"uid":"uid-1","command":"whoami"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("被锁定会话下发状态码 = %d，期望 409（%s）", w.Code, w.Body.String())
	}
	if got := decodeBody(t, w)["locked_by"]; got != "alice" {
		t.Fatalf("409 未指明持有人: %v", got)
	}
	var n int64
	if _, err := database.Engine.Where("uid = ?", "uid-1").Count(&database.Task{}); err != nil {
		t.Fatalf("统计任务失败: %v", err)
	} else if n, _ = database.Engine.Where("uid = ?", "uid-1").Count(&database.Task{}); n != 0 {
		t.Fatalf("被拒绝的请求不应建任务，实际 %d 条", n)
	}

	// 批量里只要有一台被占 → 整批拒绝，且不建任何任务
	w = doJSON(t, r, http.MethodPost, "/client/batch-command", "bob", `{"uids":["uid-2","uid-1"],"command":"whoami"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("批量下发状态码 = %d，期望 409（%s）", w.Code, w.Body.String())
	}
	if got := decodeBody(t, w)["locked_uid"]; got != "uid-1" {
		t.Fatalf("批量冲突未指明被占 uid: %v", got)
	}
	if n, _ = database.Engine.Count(&database.Task{}); n != 0 {
		t.Fatalf("整批拒绝时不应建任务，实际 %d 条", n)
	}

	// 持有人自己不受锁影响
	w = doJSON(t, r, http.MethodPost, "/client/shell/sendcommand", "alice", `{"uid":"uid-1","command":"whoami"}`)
	if w.Code == http.StatusConflict {
		t.Fatalf("持有人自己被拒: %s", w.Body.String())
	}
	// 未被锁定的会话，任何人都不受影响
	w = doJSON(t, r, http.MethodPost, "/client/shell/sendcommand", "bob", `{"uid":"uid-9","command":"whoami"}`)
	if w.Code == http.StatusConflict {
		t.Fatalf("未锁定会话被拒: %s", w.Body.String())
	}
}

// TestPresenceWiredIntoAuthMiddleware 源码级守卫：在线状态的事实来源是"已认证请求"，
// 若 AuthMiddleware 不再 Touch，在线列表会永远为空（编译与其余用例都发现不了）。
func TestPresenceWiredIntoAuthMiddleware(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("users.go"))
	if err != nil {
		t.Fatalf("读取 users.go 失败: %v", err)
	}
	if !strings.Contains(string(src), "collab.Global.Touch(") {
		t.Error("AuthMiddleware 未调用 collab.Global.Touch：在线状态将永远为空")
	}
}
