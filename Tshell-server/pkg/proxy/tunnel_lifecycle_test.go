package proxy

import (
	"BackendTemplate/pkg/database"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
)

func setupProxyTestDB(t *testing.T) {
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

// TestResetTunnelStatusOnStartup 重启后必须把"运行中"的隧道置停并返回这些记录——
// 否则界面一直显示运行中而实际没有监听（点停止还会报"不存在或未运行"）。
func TestResetTunnelStatusOnStartup(t *testing.T) {
	setupProxyTestDB(t)

	database.Engine.Insert(&database.TunnelProxy{ClientUID: "u1", Protocol: "tcp", ServerPort: 19101, Status: 1})
	database.Engine.Insert(&database.TunnelProxy{ClientUID: "u2", Protocol: "tcp", ServerPort: 19102, Status: 0})

	stopped := ResetTunnelStatusOnStartup()
	if len(stopped) != 1 || stopped[0].ServerPort != 19101 {
		t.Fatalf("应只返回重启前运行中的记录，实际 %+v", stopped)
	}
	running, _ := database.Engine.Where("status = ?", 1).Count(&database.TunnelProxy{})
	if running != 0 {
		t.Errorf("所有隧道都应被置停，实际仍有 %d 条运行中", running)
	}
	// 幂等：再次调用不应再返回记录
	if again := ResetTunnelStatusOnStartup(); len(again) != 0 {
		t.Errorf("再次调用不应有记录，实际 %d 条", len(again))
	}
}

// TestReverseSocksServerBindFailure 绑定改为同步后，端口被占用必须返回错误且不注册监听器
// （原实现在 goroutine 内绑定，失败只打日志，启动接口仍报成功）。
func TestReverseSocksServerBindFailure(t *testing.T) {
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占位失败: %v", err)
	}
	defer blocker.Close()
	addr := blocker.Addr().String()

	ln, err := ReverseSocksServer(addr, "127.0.0.1:0", GenerateRandomPSK(), "op", "p")
	if err == nil {
		ln.Close()
		t.Fatal("端口被占用时应返回错误")
	}
	MuSocks5Server.Lock()
	_, registered := Socks5Server[addr]
	MuSocks5Server.Unlock()
	if registered {
		t.Error("绑定失败不应注册监听器")
	}
}

// TestStartTunnelFailsWhenAgentPortBusy agent 端口（服务端口 + 1000）被占用时，
// StartTunnel 必须返回错误且不留下内存实例与端口占位。
func TestStartTunnelFailsWhenAgentPortBusy(t *testing.T) {
	setupProxyTestDB(t)

	const port = 19111
	blocker, err := net.Listen("tcp", fmt.Sprintf(":%d", port+1000))
	if err != nil {
		t.Fatalf("占位 agent 端口失败: %v", err)
	}
	defer blocker.Close()

	database.Engine.Insert(&database.Clients{Uid: "uid-x", Online: "1", Listener: "tcp"})
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:19110", ConnectAddress: "127.0.0.1:19110",
		Status: 1, VKey: strings.Repeat("a", 32), EncryptionSalt: strings.Repeat("b", 16),
	})
	tun := database.TunnelProxy{ClientUID: "uid-x", Protocol: "socks5", ServerPort: port, Username: "op", Password: "p", Status: 0}
	database.Engine.Insert(&tun)

	InitTunnelManager()
	if err := GlobalTunnelManager.StartTunnel(tun.ID); err == nil {
		_ = GlobalTunnelManager.StopTunnel(tun.ID)
		t.Fatal("agent 端口被占用时 StartTunnel 应返回错误")
	}
	GlobalTunnelManager.mu.RLock()
	_, exists := GlobalTunnelManager.tunnels[tun.ID]
	_, portReserved := GlobalTunnelManager.listeners[port]
	GlobalTunnelManager.mu.RUnlock()
	if exists || portReserved {
		t.Errorf("启动失败不应留实例或端口占位: instance=%v reserved=%v", exists, portReserved)
	}
}
