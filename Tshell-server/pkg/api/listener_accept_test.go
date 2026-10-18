package api

import (
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"net"
	"testing"
	"time"
)

// TestAcceptLoopAbnormalExitClearsHandleAndStatus Accept 循环异常退出时（socket 直接失效、
// 未经 handleClosePort），必须摘除句柄并回置状态：否则界面显示"监听中"、dashboard 计数
// 虚高，而端口已不再接受连接。
// 正常关闭路径不受影响——handleClosePort 会先摘句柄，收尾逻辑据"句柄是否仍是自己的"
// 判断出正常退出。
func TestAcceptLoopAbnormalExitClearsHandleAndStatus(t *testing.T) {
	setupMaskTestDB(t)

	const addr = "127.0.0.1:18082"
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: addr, ConnectAddress: addr,
		Status: 2, VKey: testVKey, EncryptionSalt: testSalt,
	})

	// tcp 的 handleOpenPort 内部是 Accept 循环（阻塞），生产代码以 go 调用
	go handleOpenPort("tcp", addr, testVKey, testSalt, "")

	var lis net.Listener
	for i := 0; i < 300; i++ {
		connection.MuClientListenerType.Lock()
		lis = connection.TCPServer[addr]
		connection.MuClientListenerType.Unlock()
		if lis != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if lis == nil {
		t.Fatal("监听器句柄未就绪")
	}
	// 模拟运行中
	database.Engine.Where("listen_address = ?", addr).Update(&database.Listener{Status: 1})

	// 绕过 handleClosePort 直接关掉底层 socket：Accept 返回错误，而句柄仍留在 map 里
	_ = lis.Close()

	deadline := time.Now().Add(3 * time.Second)
	var left bool
	status := 0
	for time.Now().Before(deadline) {
		connection.MuClientListenerType.Lock()
		_, left = connection.TCPServer[addr]
		connection.MuClientListenerType.Unlock()
		status = listenerStatus(t, addr)
		if !left && status == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if left {
		t.Error("Accept 循环异常退出后句柄仍留在 map 中（会被当成运行中）")
	}
	if status != 2 {
		t.Errorf("Accept 循环异常退出后状态应回置为 2（已停止），实际 %d", status)
	}
}

// TestAcceptLoopNormalCloseDoesNotRollback 正常关闭（经 handleClosePort）时句柄已被
// 关闭路径摘除，收尾逻辑不应把它当作异常（不重复摘除、不额外回置）。
// 这里验证关闭后句柄已摘除且状态由调用方决定的值保持不变。
func TestAcceptLoopNormalCloseDoesNotRollback(t *testing.T) {
	setupMaskTestDB(t)

	const addr = "127.0.0.1:18081"
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: addr, ConnectAddress: addr,
		Status: 1, VKey: testVKey, EncryptionSalt: testSalt,
	})

	go handleOpenPort("tcp", addr, testVKey, testSalt, "")
	for i := 0; i < 300; i++ {
		connection.MuClientListenerType.Lock()
		_, ok := connection.TCPServer[addr]
		connection.MuClientListenerType.Unlock()
		if ok {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := handleClosePort("tcp", addr); err != nil {
		t.Fatalf("正常关闭不应报错: %v", err)
	}
	// 给 Accept 循环一点时间观察到关闭
	time.Sleep(100 * time.Millisecond)

	connection.MuClientListenerType.Lock()
	_, left := connection.TCPServer[addr]
	connection.MuClientListenerType.Unlock()
	if left {
		t.Error("正常关闭后句柄应已摘除")
	}
	if got := listenerStatus(t, addr); got != 1 {
		// handleClosePort 本身不改状态（由调用方负责），这里应保持 1
		t.Errorf("handleClosePort 不应自行改动状态，实际 %d", got)
	}
}
