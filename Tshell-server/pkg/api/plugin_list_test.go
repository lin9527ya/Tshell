package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"BackendTemplate/pkg/database"

	"github.com/gin-gonic/gin"
)

func writePluginFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPluginsListReconcilesWithFilesystem 列表以文件系统为准：
// 未入库的文件要出现并补录元数据，文件已不在的 DB 行不能再显示（幽灵条目）。
func TestPluginsListReconcilesWithFilesystem(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	pluginDir := filepath.Join(tmpDir, "Plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TSHELL_PLUGIN_DIR", pluginDir)

	writePluginFile(t, pluginDir, "fscan.exe", "MZ-on-disk")
	writePluginFile(t, pluginDir, "god.bin", "shellcode-bytes")

	// DB：一条有对应文件，一条是文件已被手工删除的孤儿行
	if _, err := database.Engine.Insert(&database.Plugin{
		Name: "fscan.exe", Os: "windows", FileName: "fscan.exe",
		FileHash: strings.Repeat("a", 64), FileSize: 10, UploadTime: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Engine.Insert(&database.Plugin{
		Name: "ghost.exe", Os: "windows", FileName: "ghost.exe",
		FileHash: strings.Repeat("b", 64), FileSize: 1, UploadTime: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/plugin/list", GetPlugins)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/plugin/list", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []PluginInfo `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}

	got := map[string]PluginInfo{}
	for _, p := range resp.Data {
		got[p.Name] = p
	}

	if len(resp.Data) != 2 {
		t.Fatalf("列表应有 2 个磁盘上的插件，实际 %d: %+v", len(resp.Data), resp.Data)
	}
	if _, ok := got["ghost.exe"]; ok {
		t.Fatal("文件已不存在的 DB 行仍然出现在列表里（幽灵条目）")
	}
	god, ok := got["god.bin"]
	if !ok {
		t.Fatal("未入库的磁盘文件 god.bin 没有出现在列表里")
	}
	if god.Type != "bin" {
		t.Errorf("god.bin 类型应为 bin，实际 %q", god.Type)
	}
	// 回填后 Description 应带上真实哈希（不是占位 "-"）
	if !strings.Contains(god.Description, "Hash:") || strings.Contains(god.Description, "Hash:-") {
		t.Errorf("god.bin 未回填哈希: %q", god.Description)
	}
	if fs, ok := got["fscan.exe"]; !ok {
		t.Fatal("已入库且有文件的 fscan.exe 丢失")
	} else if !strings.Contains(fs.Description, "OS:windows") {
		t.Errorf("fscan.exe 未使用 DB 元数据: %q", fs.Description)
	}

	// 回填要真的写库：否则每次列表都要重算 SHA256，下载校验也拿不到基准哈希
	n, err := database.Engine.Count(&database.Plugin{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("回填后 DB 应有 3 行（fscan/ghost/god.bin），实际 %d", n)
	}
	var backfilled database.Plugin
	has, err := database.Engine.Where("name = ?", "god.bin").Get(&backfilled)
	if err != nil || !has {
		t.Fatalf("god.bin 未入库: has=%v err=%v", has, err)
	}
	if len(backfilled.FileHash) != 64 {
		t.Fatalf("回填的哈希不是 SHA256: %q", backfilled.FileHash)
	}
}

// TestPluginDetailsMatchesList 详情与列表的口径要一致（类型推断、平台/哈希展示）
func TestPluginDetailsMatchesList(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	pluginDir := filepath.Join(tmpDir, "Plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TSHELL_PLUGIN_DIR", pluginDir)
	writePluginFile(t, pluginDir, "fscan.exe", "MZ")

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/plugin/details/:pluginName", GetPluginDetails)
	router.GET("/api/plugin/list", GetPlugins)

	// 先列表（触发回填），再查详情
	wList := httptest.NewRecorder()
	router.ServeHTTP(wList, httptest.NewRequest(http.MethodGet, "/api/plugin/list", nil))

	wDet := httptest.NewRecorder()
	router.ServeHTTP(wDet, httptest.NewRequest(http.MethodGet, "/api/plugin/details/fscan.exe", nil))
	if wDet.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", wDet.Code, wDet.Body.String())
	}
	var det struct {
		Data PluginInfo `json:"data"`
	}
	if err := json.Unmarshal(wDet.Body.Bytes(), &det); err != nil {
		t.Fatalf("json: %v", err)
	}
	if det.Data.Type != "exe" {
		t.Errorf("详情类型应为 exe，实际 %q", det.Data.Type)
	}
	if !strings.Contains(det.Data.Description, "OS:windows") {
		t.Errorf("详情未带平台信息: %q", det.Data.Description)
	}
	if strings.Contains(det.Data.Description, "Hash:-") {
		t.Errorf("详情未带哈希: %q", det.Data.Description)
	}
}
