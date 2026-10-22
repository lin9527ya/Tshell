package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"BackendTemplate/pkg/database"

	"github.com/gin-gonic/gin"
)

// setupPluginExecuteTest 建临时 DB + 插件目录，挂 /api/plugin/execute。
func setupPluginExecuteTest(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "plugin-exec.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() { database.Engine.Close() })

	if _, err := database.Engine.Insert(&database.Clients{Uid: "known-uid-0001", Online: "1"}); err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(tmpDir, "Plugins")
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TSHELL_PLUGIN_DIR", pluginDir)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/plugin/execute", ExecutePlugin)
	return router, pluginDir
}

func pluginExecuteRequest(t *testing.T, name, uid, mode, args string) *http.Request {
	t.Helper()
	body, err := json.Marshal(ExecutePluginRequest{Name: name, Uid: uid, Mode: mode, Args: args})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/plugin/execute", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestExecutePluginRejectsMissingOrInvalid 文件不存在 / 插件名非法 / 模式非法都要明确拒绝。
func TestExecutePluginRejectsMissingOrInvalid(t *testing.T) {
	router, pluginDir := setupPluginExecuteTest(t)
	if err := os.WriteFile(filepath.Join(pluginDir, "tool.bin"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		plugin     string
		uid        string
		mode       string
		wantStatus int
		wantBody   string
	}{
		{"插件不存在", "nope.bin", "known-uid-0001", "binary", http.StatusNotFound, "插件不存在"},
		{"插件名带路径", "../escape.bin", "known-uid-0001", "binary", http.StatusBadRequest, "插件名非法"},
		{"缺少 uid", "tool.bin", "", "binary", http.StatusBadRequest, "缺少 uid"},
		{"模式非法", "tool.bin", "known-uid-0001", "not-a-mode", http.StatusBadRequest, "Invalid execution mode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, pluginExecuteRequest(t, tc.plugin, tc.uid, tc.mode, ""))
			if w.Code != tc.wantStatus {
				t.Fatalf("HTTP %d（期望 %d）: %s", w.Code, tc.wantStatus, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.wantBody) {
				t.Fatalf("响应缺少 %q: %s", tc.wantBody, w.Body.String())
			}
		})
	}
}

// TestExecutePluginDeliversFromRepo 有效请求走与 /bin/execute 相同的下发逻辑：
// 没有活跃连接时返回 503（说明真的尝试了下发，而不是只校验完就报成功）。
func TestExecutePluginDeliversFromRepo(t *testing.T) {
	router, pluginDir := setupPluginExecuteTest(t)
	if err := os.WriteFile(filepath.Join(pluginDir, "tool.bin"), []byte("payload-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, pluginExecuteRequest(t, "tool.bin", "known-uid-0001", "binary", "a b"))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("投递失败应 503，实际 HTTP %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "命令下发失败") {
		t.Fatalf("响应未说明下发失败: %s", w.Body.String())
	}
}

// TestExecutePluginPlatformMismatch 平台矩阵与 /bin/execute 同一套：
// Linux 客户端选 execute-assembly 必须在服务端拦下。
func TestExecutePluginPlatformMismatch(t *testing.T) {
	router, pluginDir := setupPluginExecuteTest(t)
	if err := os.WriteFile(filepath.Join(pluginDir, "asm.exe"), []byte("MZ...."), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Engine.Exec("UPDATE clients SET os = ? WHERE uid = ?", "linux", "known-uid-0001"); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, pluginExecuteRequest(t, "asm.exe", "known-uid-0001", "execute-assembly", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("平台不匹配应 400，实际 HTTP %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "不支持客户端系统") {
		t.Fatalf("错误文案未说明平台不匹配: %s", w.Body.String())
	}
}

// TestExecutePluginRejectsTamperedFile DB 有基准哈希时，磁盘文件被改动必须拒绝执行。
func TestExecutePluginRejectsTamperedFile(t *testing.T) {
	router, pluginDir := setupPluginExecuteTest(t)
	content := []byte("original-bytes")
	if err := os.WriteFile(filepath.Join(pluginDir, "tool.bin"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if _, err := database.Plugins().Insert(&database.Plugin{
		Name: "tool.bin", FileName: "tool.bin",
		FileHash: hex.EncodeToString(sum[:]), FileSize: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}

	// 篡改磁盘文件
	if err := os.WriteFile(filepath.Join(pluginDir, "tool.bin"), []byte("tampered-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, pluginExecuteRequest(t, "tool.bin", "known-uid-0001", "binary", ""))
	if w.Code != http.StatusForbidden {
		t.Fatalf("被篡改文件应 403，实际 HTTP %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "完整性校验失败") {
		t.Fatalf("错误文案未说明完整性失败: %s", w.Body.String())
	}
}
