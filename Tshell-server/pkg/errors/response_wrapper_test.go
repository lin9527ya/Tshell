package errors

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ResponseWrapper())
	r.GET("/old-status", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{"list": []int{1, 2}, "total": 2}})
	})
	r.GET("/old-status-message", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 200, "message": "操作成功"})
	})
	r.GET("/old-status-extra", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 200, "success_count": 3, "failed_count": 0})
	})
	r.GET("/raw-array", func(c *gin.Context) {
		c.JSON(http.StatusOK, []string{"a", "b"})
	})
	r.GET("/unified", func(c *gin.Context) {
		RespondWithSuccess(c, gin.H{"x": 1})
	})
	r.GET("/plain-text", func(c *gin.Context) {
		c.String(http.StatusOK, "hello")
	})
	return r
}

func TestResponseWrapper(t *testing.T) {
	r := setupRouter()

	check := func(path string, wantSuccess bool, wantCode float64, wantData interface{}) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)

		if !json.Valid(w.Body.Bytes()) {
			t.Fatalf("%s: 响应不是合法 JSON: %s", path, w.Body.String())
		}
		var resp ResponseFormat
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: 解析失败: %v body=%s", path, err, w.Body.String())
		}
		if resp.Success != wantSuccess {
			t.Errorf("%s: success=%v, want %v (body=%s)", path, resp.Success, wantSuccess, w.Body.String())
		}
		if resp.Code != ErrorCode(wantCode) {
			t.Errorf("%s: code=%v, want %v", path, resp.Code, wantCode)
		}
		// 顶层不应再有 status 字段
		var m map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &m)
		if _, has := m["status"]; has {
			t.Errorf("%s: 顶层仍残留 status 字段", path)
		}
		_ = wantData
	}

	check("/old-status", true, 200, nil)
	check("/old-status-message", true, 200, nil)
	check("/old-status-extra", true, 200, nil)
	check("/raw-array", true, 200, nil)
	// 统一契约成功码为 ErrCodeSuccess=0（与错误码区分，前端按 HTTP 状态/业务码判断）
	check("/unified", true, 0, nil)

	// 具体校验 data 内容
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/old-status", nil)
	r.ServeHTTP(w, req)
	var resp ResponseFormat
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("old-status: data 应为对象, got %T", resp.Data)
	}
	if data["total"].(float64) != 2 {
		t.Errorf("old-status: data.total=%v, want 2", data["total"])
	}

	// 额外字段并入 data
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/old-status-extra", nil)
	r.ServeHTTP(w, req)
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, ok = resp.Data.(map[string]interface{})
	if !ok || data["success_count"].(float64) != 3 {
		t.Errorf("old-status-extra: data 应含 success_count=3, got %v", resp.Data)
	}

	// 非 JSON 透传
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/plain-text", nil)
	r.ServeHTTP(w, req)
	if w.Body.String() != "hello" {
		t.Errorf("plain-text: body=%q, want hello", w.Body.String())
	}
}
