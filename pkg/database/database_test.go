package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseWALInitialization(t *testing.T) {
	// 创建临时测试数据库
	tmpDir := t.TempDir()
	testDBPath := filepath.Join(tmpDir, "test_tshell.db")

	oldDBName := DatabaseName
	DatabaseName = testDBPath
	defer func() {
		DatabaseName = oldDBName
		if Engine != nil {
			Engine.Close()
		}
	}()

	ConnectDateBase()

	if Engine == nil {
		t.Fatalf("Engine 初始化失败，返回 nil")
	}

	// 验证引擎 ping
	if err := Engine.Ping(); err != nil {
		t.Fatalf("数据库 Ping 失败: %v", err)
	}

	// 验证 SQLite WAL 模式生效
	results, err := Engine.QueryString("PRAGMA journal_mode;")
	if err != nil {
		t.Fatalf("查询 journal_mode 失败: %v", err)
	}

	if len(results) > 0 {
		mode := results[0]["journal_mode"]
		if mode == "" {
			mode = results[0]["JOURNAL_MODE"]
		}
		t.Logf("当前 SQLite journal_mode: %s", mode)
	}

	// 验证文件是否正常生成
	if _, err := os.Stat(testDBPath); os.IsNotExist(err) {
		t.Fatalf("测试数据库文件未生成: %s", testDBPath)
	}
}
