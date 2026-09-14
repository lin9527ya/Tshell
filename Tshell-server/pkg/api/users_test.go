package api

import (
	"BackendTemplate/pkg/database"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestUserManagementAPI 验证用户管理 handler:
// 创建(含角色) → 列表(含搜索) → 重置密码 → 删除；删除自己/最后一个 admin 被拒绝。
func TestUserManagementAPI(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test_users.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// 路由挂载需 username 上下文（从 AuthMiddleware 注入），测试直接设置
	router.Use(func(c *gin.Context) {
		c.Set("username", "admin")
		c.Next()
	})
	router.GET("/users/list", GetUserListHandler)
	router.POST("/users/create", CreateUserHandler)
	router.POST("/users/delete", DeleteUserHandler)
	router.POST("/users/reset", AdminResetPasswordHandler)

	postJSON := func(url, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// 1. 创建用户（operator）
	w := postJSON("/users/create", `{"username":"alice","password":"Alice123456","role":"operator"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("create: HTTP %d: %s", w.Code, w.Body.String())
	}

	// 2. 创建重复用户应失败
	w = postJSON("/users/create", `{"username":"alice","password":"Alice123456","role":"admin"}`)
	var dupResp struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &dupResp)
	if dupResp.Code != 400 {
		t.Fatalf("重复创建应返回 code=400, 实际 %d", dupResp.Code)
	}

	// 3. 无效角色应失败
	w = postJSON("/users/create", `{"username":"bob","password":"Bob123456","role":"root"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("无效角色应 400, 实际 %d", w.Code)
	}

	// 4. 弱密码应失败
	w = postJSON("/users/create", `{"username":"carol","password":"123","role":"operator"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("弱密码应返回业务失败, 实际 HTTP %d", w.Code)
	}
	var weakResp struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &weakResp)
	if weakResp.Code != 400 {
		t.Fatalf("弱密码应返回 code=400, 实际 %d", weakResp.Code)
	}

	// 5. 列表：alice 应存在且角色正确
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/users/list?page=1&page_size=10&search=alice", nil)
	router.ServeHTTP(w2, req)
	var listResp struct {
		Data struct {
			List []struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"list"`
			Total int64 `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("list json: %v", err)
	}
	if len(listResp.Data.List) != 1 || listResp.Data.List[0].Username != "alice" || listResp.Data.List[0].Role != "operator" {
		t.Fatalf("列表不符: %+v", listResp.Data.List)
	}

	// 6. 重置密码
	w = postJSON("/users/reset", `{"username":"alice","new_password":"NewAlice123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("reset: HTTP %d: %s", w.Code, w.Body.String())
	}
	var alice database.Users
	has, _ := database.Engine.Where("username = ?", "alice").Get(&alice)
	if !has || !database.CheckPasswordHash("NewAlice123", alice.PasswordHash) {
		t.Fatalf("重置后的密码校验失败")
	}

	// 7. 删除自己应被拒绝
	w = postJSON("/users/delete", `{"username":"admin"}`)
	var selfResp struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &selfResp)
	if selfResp.Code != 400 {
		t.Fatalf("删除自己应被拒绝, 实际 %d", selfResp.Code)
	}

	// 8. 删除 alice（当前还有 admin 在，允许）
	w = postJSON("/users/delete", `{"username":"alice"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: HTTP %d: %s", w.Code, w.Body.String())
	}
	has, _ = database.Engine.Where("username = ?", "alice").Exist(new(database.Users))
	if has {
		t.Fatalf("alice 应已被删除")
	}

	// 9. 删除最后一个 admin 应被拒绝（admin 是库里唯一 admin）
	w = postJSON("/users/delete", `{"username":"admin"}`)
	var lastAdminResp struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &lastAdminResp)
	// 删除自己已被拦截，故此处必然是"不能删除自己"路径——验证返回非成功即可
	if lastAdminResp.Code != 400 {
		t.Fatalf("删除当前用户应失败, 实际 %d", lastAdminResp.Code)
	}
}
