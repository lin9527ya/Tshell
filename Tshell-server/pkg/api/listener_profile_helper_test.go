package api

import (
	"path/filepath"
	"testing"

	"BackendTemplate/pkg/database"
)

// setupTestDatabase 建一个临时库（供需要查监听器的用例使用）
func setupTestDatabase(t *testing.T) {
	t.Helper()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(t.TempDir(), "listener-profile-test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() {
		if database.Engine != nil {
			database.Engine.Close()
		}
	})
}

// insertListener 插一条监听器记录（profile 为空表示 default）
func insertListener(t *testing.T, name, addr, profile string) {
	t.Helper()
	if _, err := database.Engine.Insert(&database.Listener{
		Type: "http", ListenAddress: addr, ConnectAddress: addr,
		Status: 2, HeartbeatTimeout: "30", HeartbeatInterval: "10",
		Note: name, Profile: profile,
	}); err != nil {
		t.Fatalf("插入监听器 %s 失败: %v", name, err)
	}
}
