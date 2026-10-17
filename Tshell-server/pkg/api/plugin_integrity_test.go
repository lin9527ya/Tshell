package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"BackendTemplate/pkg/database"

	"github.com/gin-gonic/gin"
)

// setupPluginTest 准备临时 DB 与临时插件目录
func setupPluginTest(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() { database.Engine.Close() })

	pluginDir := filepath.Join(tmpDir, "Plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TSHELL_PLUGIN_DIR", pluginDir)
	return pluginDir
}

// uploadRequest 构造 multipart 上传请求
func uploadRequest(t *testing.T, url, filename string, content []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, err := mw.CreateFormFile("plugin", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, url, body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// TestPluginUploadValidatesExtension 扩展名白名单（挡明显放错的文件，不是安全边界）
func TestPluginUploadValidatesExtension(t *testing.T) {
	pluginDir := setupPluginTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/plugin/upload", UploadPlugin)

	cases := []struct {
		name       string
		filename   string
		wantStatus int
	}{
		{"exe 允许", "tool.exe", http.StatusOK},
		{"bin 允许（插件目录里的 fscan.bin/god.bin 就是这种）", "god.bin", http.StatusOK},
		{"dll 允许", "asm.dll", http.StatusOK},
		{"txt 拒绝", "notes.txt", http.StatusBadRequest},
		{"图片拒绝", "logo.png", http.StatusBadRequest},
		{"无扩展名拒绝", "payload", http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, uploadRequest(t, "/api/plugin/upload", tc.filename, []byte("data")))
			if w.Code != tc.wantStatus {
				t.Fatalf("HTTP %d（期望 %d）: %s", w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}

	if _, err := os.Stat(filepath.Join(pluginDir, "notes.txt")); err == nil {
		t.Fatal("被拒绝的文件仍然落盘了")
	}
}

// TestPluginUploadStoresMetadata 上传要落 SHA256 与平台推断（下载校验的基准）
func TestPluginUploadStoresMetadata(t *testing.T) {
	pluginDir := setupPluginTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/plugin/upload", UploadPlugin)

	content := []byte("MZ-fake-pe")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, uploadRequest(t, "/api/plugin/upload", "tool.exe", content))
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}

	var row database.Plugin
	has, err := database.Engine.Where("name = ?", "tool.exe").Get(&row)
	if err != nil || !has {
		t.Fatalf("上传未写库: has=%v err=%v", has, err)
	}
	want := sha256.Sum256(content)
	if row.FileHash != hex.EncodeToString(want[:]) {
		t.Errorf("入库哈希与内容不符: %s", row.FileHash)
	}
	if row.Os != "windows" {
		t.Errorf("exe 的平台推断应为 windows，实际 %q", row.Os)
	}
	// Type 列是执行模式，推断不出来就不该瞎猜
	if row.Type != "" {
		t.Errorf("Type 不应被扩展名污染，实际 %q", row.Type)
	}

	// 内容真的落盘
	onDisk, err := os.ReadFile(filepath.Join(pluginDir, "tool.exe"))
	if err != nil || !bytes.Equal(onDisk, content) {
		t.Fatalf("落盘内容不符: %v %q", err, onDisk)
	}
}

// TestPluginDownloadIntegrity 下载以 DB 的 SHA256 为基准；篡改必须拒绝
func TestPluginDownloadIntegrity(t *testing.T) {
	pluginDir := setupPluginTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/plugin/download/:pluginName", DownloadPlugin)

	content := []byte("original-bytes")
	if err := os.WriteFile(filepath.Join(pluginDir, "god.bin"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if _, err := database.Engine.Insert(&database.Plugin{
		Name: "god.bin", FileName: "god.bin", FileHash: hex.EncodeToString(sum[:]),
		FileSize: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}

	// 1) 未被篡改：正常下载，且 Content-Length 与内容一致
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/plugin/download/god.bin", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("正常下载失败 HTTP %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), content) {
		t.Fatalf("下载内容不符: %q", w.Body.Bytes())
	}

	// 2) 被篡改：403
	if err := os.WriteFile(filepath.Join(pluginDir, "god.bin"), []byte("tampered-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/plugin/download/god.bin", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("篡改后应 403，实际 HTTP %d: %s", w.Code, w.Body.String())
	}
}

// TestPluginDownloadWithoutMetadata 没有基准哈希的历史文件仍可下载，
// 但不能因为"查不到元数据"就把校验静默跳过到连文件都发不出去（Content-Length 必须正确）
func TestPluginDownloadWithoutMetadata(t *testing.T) {
	pluginDir := setupPluginTest(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/plugin/download/:pluginName", DownloadPlugin)

	content := []byte("legacy-plugin-content")
	if err := os.WriteFile(filepath.Join(pluginDir, "legacy.bin"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/plugin/download/legacy.bin", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), content) {
		t.Fatalf("无元数据时下载内容为空或错误: %q", w.Body.Bytes())
	}

	// 文件不存在 → 404
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/plugin/download/nope.bin", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("不存在的插件应 404，实际 %d", w.Code)
	}

	// 非法文件名 → 400（路径穿越防护仍生效）
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/plugin/download/..%2F..%2Fetc%2Fpasswd", nil))
	if w.Code == http.StatusOK || strings.Contains(w.Body.String(), "root:") {
		t.Fatalf("路径穿越未被拦截: HTTP %d %s", w.Code, w.Body.String())
	}
}
