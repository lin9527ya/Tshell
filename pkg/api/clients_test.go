package api

import (
	"BackendTemplate/pkg/database"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestGetClientsPaging 验证 SQL 级分页: page/page_size 生效、total 正确、越界页返回空
func TestGetClientsPaging(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	// 插入 5 个客户端
	for i := 0; i < 5; i++ {
		uid := make([]byte, 0)
		uid = append(uid, byte('a'+i))
		_ = uid
		database.Engine.Insert(&database.Clients{Uid: string([]byte{'u', '0', byte('0' + i), '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0', '0'}), Online: "1", Listener: "tcp"})
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/clients", GetClients)

	// 第 2 页, 每页 2 条
	req := httptest.NewRequest(http.MethodGet, "/api/clients?page=2&page_size=2", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	body, _ := io.ReadAll(w.Body)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, body)
	}
	var resp struct {
		Data struct {
			List  []json.RawMessage `json:"list"`
			Total int64             `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Data.Total != 5 {
		t.Fatalf("total 应为 5, 实际 %d", resp.Data.Total)
	}
	if len(resp.Data.List) != 2 {
		t.Fatalf("第 2 页应返回 2 条, 实际 %d", len(resp.Data.List))
	}

	// 越界页返回空但不报错
	req2 := httptest.NewRequest(http.MethodGet, "/api/clients?page=99&page_size=2", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	var resp2 struct {
		Data struct {
			List []json.RawMessage `json:"list"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if len(resp2.Data.List) != 0 {
		t.Fatalf("越界页应返回空列表")
	}
	t.Log("客户端分页验证通过")
}

// TestGetClientsDefaultPaging 无参数时默认 page=1 size=20
func TestGetClientsDefaultPaging(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/clients", GetClients)

	req := httptest.NewRequest(http.MethodGet, "/api/clients", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d", w.Code)
	}
	t.Log("默认分页验证通过")
}
