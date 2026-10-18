package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"BackendTemplate/pkg/database"

	"github.com/gin-gonic/gin"
)

func setupTaskTest(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() { database.Engine.Close() })
}

// TestTaskLifecycle 任务一等对象：建行 → 回显归属（最早一条）→ 终态幂等 → 聚合视图。
func TestTaskLifecycle(t *testing.T) {
	setupTaskTest(t)

	batchID, rows := createTaskRows([]string{"uid-1", "uid-2"}, "alice", "batch", "whoami")
	if batchID == "" || len(rows) != 2 {
		t.Fatalf("建任务失败: batch=%q rows=%d", batchID, len(rows))
	}
	if rows[0].Operator != "alice" || rows[0].Status != taskStatusDispatched {
		t.Fatalf("任务字段不符: %+v", rows[0])
	}

	// 回包归属：uid-1 的最早一条 dispatched → completed，且带输出
	OnTaskOutput("uid-1", "uid-1-output")
	var t1 database.Task
	if has, _ := database.Engine.ID(rows[0].Id).Get(&t1); !has || t1.Status != taskStatusCompleted {
		t.Fatalf("uid-1 任务未完成: %+v", t1)
	}
	if t1.Result != "uid-1-output" || t1.FinishedAt.IsZero() {
		t.Fatalf("完成态字段不符: %+v", t1)
	}
	// uid-2 仍在 dispatched
	var t2 database.Task
	if has, _ := database.Engine.ID(rows[1].Id).Get(&t2); !has || t2.Status != taskStatusDispatched {
		t.Fatalf("uid-2 不应被顺带完成: %+v", t2)
	}

	// 终态幂等：已 completed 不再被覆盖
	finishTask(rows[0].Id, taskStatusFailed, "", "late error")
	database.Engine.ID(rows[0].Id).Get(&t1)
	if t1.Status != taskStatusCompleted || t1.Result != "uid-1-output" {
		t.Fatalf("终态被覆盖: %+v", t1)
	}

	// 无 dispatched 任务时回显不应报错/不写库
	OnTaskOutput("uid-404", "x")

	// 聚合视图：一组两 uid，uid-2 未完成 → running；完成后 → done
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/client/tasks", ListTasks)
	getTasks := func() []TaskRecord {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/client/tasks", nil))
		var resp struct {
			Data []TaskRecord `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json: %v body=%s", err, w.Body.String())
		}
		return resp.Data
	}
	list := getTasks()
	if len(list) != 1 || len(list[0].UIDs) != 2 || list[0].Status != "running" {
		t.Fatalf("聚合视图不符: %+v", list)
	}
	if list[0].Results["uid-1"] != "uid-1-output" {
		t.Fatalf("聚合结果未带输出: %+v", list[0].Results)
	}

	OnTaskOutput("uid-2", "uid-2-output")
	if list = getTasks(); len(list) != 1 || list[0].Status != "done" {
		t.Fatalf("全部完成后应为 done: %+v", list)
	}
}

// TestTaskCancel 只有 dispatched 可取消；重复取消返回 409。
func TestTaskCancel(t *testing.T) {
	setupTaskTest(t)
	_, rows := createTaskRows([]string{"uid-1"}, "bob", "console", "pwd")
	if len(rows) != 1 {
		t.Fatal("建任务失败")
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/tasks/:id/cancel", CancelTask)
	cancel := func() int {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost,
			"/api/tasks/"+itoa(rows[0].Id)+"/cancel", nil))
		return w.Code
	}
	if code := cancel(); code != http.StatusOK {
		t.Fatalf("首次取消应 200，实际 %d", code)
	}
	if code := cancel(); code != http.StatusConflict {
		t.Fatalf("重复取消应 409，实际 %d", code)
	}
	var got database.Task
	database.Engine.ID(rows[0].Id).Get(&got)
	if got.Status != taskStatusCancelled {
		t.Fatalf("状态应为 cancelled: %+v", got)
	}
}

// TestTaskRowsFilter 明细查询按 uid/status 过滤并分页。
func TestTaskRowsFilter(t *testing.T) {
	setupTaskTest(t)
	createTaskRows([]string{"uid-1"}, "alice", "console", "cmd1")
	createTaskRows([]string{"uid-2"}, "alice", "console", "cmd2")
	rows := []*database.Task{}
	_, r2 := createTaskRows([]string{"uid-1"}, "alice", "batch", "cmd3")
	rows = append(rows, r2...)
	finishTask(r2[0].Id, taskStatusCompleted, "ok", "")
	_ = time.Now()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/tasks", ListTaskRows)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/tasks?uid=uid-1&status=completed", nil))
	var resp struct {
		Data struct {
			Total int64           `json:"total"`
			List  []database.Task `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v body=%s", err, w.Body.String())
	}
	if resp.Data.Total != 1 || len(resp.Data.List) != 1 || resp.Data.List[0].Command != "cmd3" {
		t.Fatalf("过滤结果不符: %+v", resp.Data)
	}
}

func itoa(v int64) string {
	// 避免引入 strconv 只为一个调用（测试可读性优先）
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
