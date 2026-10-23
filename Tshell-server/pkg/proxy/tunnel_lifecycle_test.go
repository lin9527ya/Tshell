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
	// 服务端现在绑定通配地址 ":port+1000"（对外通告地址与 bind 分离），
	// 这里也用通配地址占位，确保在 Windows 上能真实触发绑定失败。
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
