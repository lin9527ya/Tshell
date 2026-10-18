package api

import (
	"BackendTemplate/pkg/database"
	"net"
	"path/filepath"
	"testing"
)

func setupDNSTestDB(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() {
		if database.Engine != nil {
			database.Engine.Close()
		}
	})
}

// TestDNSListenerBindFailureMarksStopped UDP 端口被占用时状态必须落回已停止。
// 原实现把 ListenAndServe 放在 goroutine 里：绑定失败只打一行日志，库里仍是"运行中"，
// 而前端的"监听中"与 dashboard 计数都以此为据。
func TestDNSListenerBindFailureMarksStopped(t *testing.T) {
	setupDNSTestDB(t)

	blocker, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占位失败: %v", err)
	}
	defer blocker.Close()

	addr := blocker.LocalAddr().String() + ":dns.example." // addr:port:domain
	database.Engine.Insert(&database.Listener{
		Type: "dns", ListenAddress: addr, ConnectAddress: addr,
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})

	handleOpenPort("dns", addr, testVKey, testSalt, "")

	if got := listenerStatus(t, addr); got != 2 {
		t.Errorf("绑定失败后状态应回置为 2（已停止），实际 %d", got)
	}
}

// TestDNSListenerAddressTooFewParts 地址段数不足时应干净失败：
// 原实现直接取 tmp[1]，段数不足会 panic（靠外层 recover 兜住，日志里只留一条 panic 记录）。
func TestDNSListenerAddressTooFewParts(t *testing.T) {
	setupDNSTestDB(t)

	const addr = "onlyone" // 无端口
	database.Engine.Insert(&database.Listener{
		Type: "dns", ListenAddress: addr, ConnectAddress: addr,
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})

	handleOpenPort("dns", addr, testVKey, testSalt, "") // 不应 panic

	if got := listenerStatus(t, addr); got != 2 {
		t.Errorf("地址格式非法时状态应回置为 2（已停止），实际 %d", got)
	}
}
