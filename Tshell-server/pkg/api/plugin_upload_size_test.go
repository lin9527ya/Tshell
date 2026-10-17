package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	pkgerrors "BackendTemplate/pkg/errors"

	"github.com/gin-gonic/gin"
)

// TestPluginUploadOverGlobalLimit 12MB 的插件上传必须成功。
// 全局请求体上限是 10MB，/api/plugin/upload 在 main.go 里被按路径放宽到 50MB+1MB；
// 修复前这个上传会在 c.FormFile 阶段就以 "http: request body too large" 失败，
// handler 里的 50MB 检查根本走不到。
func TestPluginUploadOverGlobalLimit(t *testing.T) {
	pluginDir := setupPluginTest(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// 与 main.go 的配置一致：默认 10MB，插件上传按路径放宽
	router.Use(pkgerrors.MaxBodyBytesMiddlewareWithLimits(10<<20, map[string]int64{
		"/api/plugin/upload": MaxPluginUploadBytes + 1<<20,
	}))
	router.POST("/api/plugin/upload", UploadPlugin)

	content := bytes.Repeat([]byte("A"), 12<<20) // 12MB > 全局 10MB
	w := httptest.NewRecorder()
	router.ServeHTTP(w, uploadRequest(t, "/api/plugin/upload", "big.exe", content))

	if w.Code != http.StatusOK {
		t.Fatalf("12MB 上传应成功，实际 HTTP %d: %s", w.Code, w.Body.String())
	}
	info, err := os.Stat(filepath.Join(pluginDir, "big.exe"))
	if err != nil {
		t.Fatalf("文件未落盘: %v", err)
	}
	if info.Size() != int64(len(content)) {
		t.Fatalf("落盘大小不符: %d != %d", info.Size(), len(content))
	}
}

// TestPluginUploadOverLimitReportsRealReason 超过请求体上限时要给出能定位原因的错误，
// 而不是含混的"获取上传文件失败"（更不能像 /bin/execute 原来那样报成 No file uploaded）
func TestPluginUploadOverLimitReportsRealReason(t *testing.T) {
	setupPluginTest(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	// 把覆盖值压到 2MB，用 3MB 请求体触发（避免测试里真造 50MB 数据）
	router.Use(pkgerrors.MaxBodyBytesMiddlewareWithLimits(10<<20, map[string]int64{
		"/api/plugin/upload": 2 << 20,
	}))
	router.POST("/api/plugin/upload", UploadPlugin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, uploadRequest(t, "/api/plugin/upload", "big.exe", bytes.Repeat([]byte("A"), 3<<20)))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("超限上传应 400，实际 HTTP %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("获取上传文件失败")) {
		t.Fatalf("错误文案未指出真实原因: %s", body)
	}
}
