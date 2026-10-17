package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"BackendTemplate/pkg/database"

	"github.com/gin-gonic/gin"
)

// executeBinRequest 构造 /api/bin/execute 的 multipart 请求
func executeBinRequest(t *testing.T, uid, mode, args string) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("file", "payload.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("payload-bytes")); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"uid": uid, "mode": mode, "args": args} {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/bin/execute", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func setupExecuteBinTest(t *testing.T) *gin.Engine {
	t.Helper()
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() { database.Engine.Close() })

	if _, err := database.Engine.Insert(&database.Clients{Uid: "known-uid-0001", Online: "1"}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/bin/execute", ExecuteBin)
	return router
}

// TestExecuteBinValidatesInput 参数校验：原来 uid 不校验、mode 靠 switch 兜底
func TestExecuteBinValidatesInput(t *testing.T) {
	router := setupExecuteBinTest(t)

	cases := []struct {
		name       string
		uid        string
		mode       string
		wantStatus int
		wantBody   string
	}{
		{"缺少 uid", "", "binary", http.StatusBadRequest, "缺少 uid"},
		{"uid 不存在", "deadbeef", "binary", http.StatusNotFound, "客户端不存在"},
		{"mode 非法", "known-uid-0001", "not-a-mode", http.StatusBadRequest, "Invalid execution mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, executeBinRequest(t, tc.uid, tc.mode, ""))
			if w.Code != tc.wantStatus {
				t.Fatalf("HTTP %d（期望 %d）: %s", w.Code, tc.wantStatus, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Fatalf("响应缺少 %q: %s", tc.wantBody, w.Body.String())
			}
		})
	}
}

// TestExecuteBinReportsDeliveryFailure 载荷没发出去就不能报成功。
// 客户端在 DB 里存在但没有活跃连接（getListenerType 查不到）→ 原来一律 200，
// 前端显示"命令已发送"，实际什么都没下发。
func TestExecuteBinReportsDeliveryFailure(t *testing.T) {
	router := setupExecuteBinTest(t)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, executeBinRequest(t, "known-uid-0001", "binary", "arg1"))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("投递失败应返回 503，实际 HTTP %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "命令下发失败") {
		t.Fatalf("响应未说明下发失败: %s", w.Body.String())
	}
}

// TestExecuteBinAcceptsAllFrontendModes 白名单必须覆盖前端全部 7 个模式，
// 否则会把正常执行误判成非法模式
func TestExecuteBinAcceptsAllFrontendModes(t *testing.T) {
	router := setupExecuteBinTest(t)

	// 与 PluginsView.vue 的 executionModes 一致
	for _, mode := range []string{
		"execute-assembly", "inline-bin", "shellcode-inject",
		"script", "binary", "bof", "exec-file",
	} {
		t.Run(mode, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, executeBinRequest(t, "known-uid-0001", mode, ""))
			if strings.Contains(w.Body.String(), "Invalid execution mode") {
				t.Fatalf("模式 %s 被白名单误拒: %s", mode, w.Body.String())
			}
		})
	}
}
