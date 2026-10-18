package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"BackendTemplate/pkg/backup"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

// 备份接口只暴露"备份的查看/创建/下载/导入/删除"；恢复必须走 CLI。
// 这里守两件事：接口可用，以及不能借接口读到备份目录之外的文件。

func setupBackupAPI(t *testing.T) *backup.Manager {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "database.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("建库失败: %v", err)
	}
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE clients (id INTEGER PRIMARY KEY, uid TEXT)`,
		`CREATE TABLE listener (id INTEGER PRIMARY KEY)`,
		`INSERT INTO clients (uid) VALUES ('uid-1')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("建表/写入失败: %v", err)
		}
	}
	db.Close()

	m := backup.New(backup.Options{DBPath: dbPath, Dir: filepath.Join(dir, "backups"), Keep: 3})
	old := BackupManager
	SetBackupManager(m)
	t.Cleanup(func() { BackupManager = old })
	return m
}

func backupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Set("username", u)
		}
		c.Next()
	})
	r.GET("/backup/list", ListBackups)
	r.POST("/backup/create", CreateBackup)
	r.GET("/backup/download/:name", DownloadBackup)
	r.DELETE("/backup/:name", DeleteBackup)
	r.POST("/backup/import", ImportBackup)
	return r
}

func TestBackupListCreateDelete(t *testing.T) {
	setupBackupAPI(t)
	r := backupRouter()

	// 初始为空
	w := doJSON(t, r, http.MethodGet, "/backup/list", "admin", "")
	if w.Code != http.StatusOK {
		t.Fatalf("列表状态码 = %d", w.Code)
	}
	var empty struct {
		Data struct {
			List []backup.Info `json:"list"`
			Dir  string        `json:"dir"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &empty)
	if len(empty.Data.List) != 0 || empty.Data.Dir == "" {
		t.Fatalf("初始列表不符: %+v", empty.Data)
	}

	// 创建
	w = doJSON(t, r, http.MethodPost, "/backup/create", "alice", `{"note":"drill"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("创建状态码 = %d（%s）", w.Code, w.Body.String())
	}
	var created struct {
		Data backup.Info `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	if !created.Data.Verified || created.Data.Size == 0 || created.Data.Note != "drill" {
		t.Fatalf("创建的备份元信息不符: %+v", created.Data)
	}

	// 列表可见且已校验
	w = doJSON(t, r, http.MethodGet, "/backup/list", "admin", "")
	var listResp struct {
		Data struct {
			List []backup.Info `json:"list"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Data.List) != 1 || !listResp.Data.List[0].Verified {
		t.Fatalf("列表不符: %+v", listResp.Data.List)
	}

	// 下载：内容与磁盘一致
	w = doJSON(t, r, http.MethodGet, "/backup/download/"+created.Data.Name, "admin", "")
	if w.Code != http.StatusOK {
		t.Fatalf("下载状态码 = %d", w.Code)
	}
	onDisk, err := os.ReadFile(created.Data.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(w.Body.Bytes(), onDisk) {
		t.Fatal("下载内容与备份文件不一致")
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, created.Data.Name) {
		t.Fatalf("下载响应缺少文件名: %q", cd)
	}

	// 删除
	w = doJSON(t, r, http.MethodDelete, "/backup/"+created.Data.Name, "admin", "")
	if w.Code != http.StatusOK {
		t.Fatalf("删除状态码 = %d", w.Code)
	}
	if _, err := os.Stat(created.Data.Path); !os.IsNotExist(err) {
		t.Fatal("删除后文件仍在")
	}
}

// TestBackupEndpointsRejectTraversal 备份名不能越出备份目录
func TestBackupEndpointsRejectTraversal(t *testing.T) {
	m := setupBackupAPI(t)
	r := backupRouter()

	// 在备份目录外放一个"敏感文件"，确认它读不到、删不掉
	outside := filepath.Join(filepath.Dir(m.Options().Dir), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"..%2Fsecret.txt", "../secret.txt", "secret.txt"} {
		if w := doJSON(t, r, http.MethodGet, "/backup/download/"+name, "admin", ""); w.Code == http.StatusOK {
			t.Errorf("下载 %q 不应成功", name)
		}
		if w := doJSON(t, r, http.MethodDelete, "/backup/"+name, "admin", ""); w.Code == http.StatusOK {
			t.Errorf("删除 %q 不应成功", name)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("备份目录外的文件被动了: %v", err)
	}
}

// TestBackupImportRejectsGarbage 导入前必须校验，坏文件不落盘
func TestBackupImportRejectsGarbage(t *testing.T) {
	m := setupBackupAPI(t)
	r := backupRouter()

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	fw, _ := mw.CreateFormFile("file", "not-a-backup.db")
	fw.Write([]byte("definitely not sqlite"))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/backup/import", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-User", "admin")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("导入垃圾文件状态码 = %d，期望 400（%s）", w.Code, w.Body.String())
	}
	entries, _ := os.ReadDir(m.Options().Dir)
	for _, e := range entries {
		t.Fatalf("校验失败的文件不应落盘: %s", e.Name())
	}
}

// TestBackupImportSanitizesFilename 上传的文件名只取基名并规整，不能用来做路径
func TestBackupImportSanitizesFilename(t *testing.T) {
	cases := map[string]string{
		"../../../etc/passwd":            "tshell-backup-passwd.db", // 只取基名
		"backup.db":                      "tshell-backup-backup.db",
		"tshell-backup-20260101-0000.db": "tshell-backup-20260101-0000.db",
	}
	for in, want := range cases {
		if got := sanitizeBackupName(in, "tshell-backup"); got != want {
			t.Errorf("sanitizeBackupName(%q) = %q，期望 %q", in, got, want)
		}
		if strings.ContainsAny(want, `/\`) {
			t.Errorf("规整后的名字仍含路径分隔符: %q", want)
		}
	}
	// 空名字用时间戳兜底，不落出 "tshell-backup-.db" 这种没法辨认的名字
	fallback := sanitizeBackupName("", "tshell-backup")
	if fallback == "tshell-backup-.db" || !strings.HasPrefix(fallback, "tshell-backup-2") || !strings.HasSuffix(fallback, ".db") {
		t.Errorf("空文件名未按时间戳兜底: %q", fallback)
	}
}

// TestBackupUnavailableWhenDisabled 未启用备份功能时接口明确报不可用
func TestBackupUnavailableWhenDisabled(t *testing.T) {
	old := BackupManager
	BackupManager = nil
	t.Cleanup(func() { BackupManager = old })

	r := backupRouter()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/backup/list"},
		{http.MethodPost, "/backup/create"},
	} {
		w := doJSON(t, r, tc.method, tc.path, "admin", "")
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s 状态码 = %d，期望 503", tc.method, tc.path, w.Code)
		}
	}
}
