package api

import (
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"path/filepath"
	"testing"
)

func setupOSSTestDB(t *testing.T) {
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

func listenerStatus(t *testing.T, addr string) int {
	t.Helper()
	var lis database.Listener
	database.Engine.Where("listen_address = ?", addr).Get(&lis)
	return lis.Status
}

// TestOSSListenerInitFailureMarksStopped OSS 客户端初始化失败必须把状态落回已停止。
// 原实现把 InitClient 放在轮询 goroutine 里且忽略其错误：DB 停在"运行中"、
// 停止通道无人消费，循环里的 List(nil) 还会 panic（被 recover 兜住，但监听器实际没在跑）。
func TestOSSListenerInitFailureMarksStopped(t *testing.T) {
	setupOSSTestDB(t)

	// 四段齐全但参数非法（桶名不允许大写）→ oss.InitClient 返回错误。
	// 注意 endpoint 自身不能含冒号（地址按冒号切分），故用桶名触发校验失败。
	const addr = "oss-cn-test.aliyuncs.com:ak:sk:BAD-BUCKET"
	database.Engine.Insert(&database.Listener{
		Type: "oss", ListenAddress: addr, ConnectAddress: addr,
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})

	handleOpenPort("oss", addr, testVKey, testSalt, "")

	if got := listenerStatus(t, addr); got != 2 {
		t.Errorf("初始化失败后状态应回置为 2（已停止），实际 %d", got)
	}
	connection.MuClientListenerType.Lock()
	_, hasStop := connection.StopChan[addr]
	connection.MuClientListenerType.Unlock()
	if hasStop {
		t.Error("初始化失败时不应登记停止通道（原实现登记后无人消费）")
	}
}

// TestOSSListenerAddressTooFewParts OSS 地址段数不足时应干净失败：
// 原实现直接取 tmp[3]，段数不足会 panic（靠外层 recover 兜住，日志里只留一条 panic 记录）。
func TestOSSListenerAddressTooFewParts(t *testing.T) {
	setupOSSTestDB(t)

	const addr = "only:two" // 不足 4 段
	database.Engine.Insert(&database.Listener{
		Type: "oss", ListenAddress: addr, ConnectAddress: addr,
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})

	handleOpenPort("oss", addr, testVKey, testSalt, "") // 不应 panic

	if got := listenerStatus(t, addr); got != 2 {
		t.Errorf("地址格式非法时状态应回置为 2（已停止），实际 %d", got)
	}
}
