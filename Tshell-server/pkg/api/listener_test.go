package api

import (
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/connection/tcp"
	"BackendTemplate/pkg/database"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// stubLocalConn 只实现 connWithLocalAddr，用于验证归属判定中的端口比对。
type stubLocalConn struct{ local net.Addr }

func (s stubLocalConn) LocalAddr() net.Addr { return s.local }

// TestConnOnListener 覆盖归属判定的关键语义：通配监听地址与连接本端具体 IP
// 整串不等，只能按端口判定；地址不含端口（oss/dns 复用 listen_address 字段）
// 或取不到本端地址时无法判定，须回退为 true（保持原有按类型清理的语义）。
func TestConnOnListener(t *testing.T) {
	tests := []struct {
		name     string
		listener string
		local    net.Addr
		want     bool
		why      string
	}{
		{
			name:     "通配监听_连接本端为具体IP_端口一致_判定归属",
			listener: "0.0.0.0:8081",
			local:    &net.TCPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 8081},
			want:     true,
			why:      "0.0.0.0 与 192.168.1.5 整串不等，只能按端口判定",
		},
		{
			name:     "同类型另一监听器_端口不同_不归属",
			listener: "0.0.0.0:8081",
			local:    &net.TCPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 8082},
			want:     false,
			why:      "本修复要拦的场景：关 8081 不得误断 8082 上的客户端",
		},
		{
			name:     "IPv6通配_端口一致_判定归属",
			listener: "[::]:8081",
			local:    &net.TCPAddr{IP: net.IPv6loopback, Port: 8081},
			want:     true,
		},
		{
			name:     "监听地址为域名_端口一致_判定归属",
			listener: "c2.example.com:8081",
			local:    &net.TCPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 8081},
			want:     true,
		},
		{
			name:     "地址非hostport_无法判定_回退按类型",
			listener: "oss:AK:SK:bucket",
			local:    &net.TCPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 8081},
			want:     true,
			why:      "oss/dns 复用 listen_address 承载凭证与域配置，取不到端口",
		},
		{
			name:     "本端地址不含端口_无法判定_回退按类型",
			listener: "0.0.0.0:8081",
			local:    &net.UnixAddr{Name: "/tmp/tshell.sock", Net: "unix"},
			want:     true,
		},
		{
			name:     "本端地址为空_无法判定_回退按类型",
			listener: "0.0.0.0:8081",
			local:    nil,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := connOnListener(stubLocalConn{local: tt.local}, tt.listener)
			if got != tt.want {
				t.Fatalf("connOnListener(listener=%q, local=%v) = %v, want %v (%s)",
					tt.listener, tt.local, got, tt.want, tt.why)
			}
		})
	}
}

// stubNetConn 实现 net.Conn：仅 LocalAddr 参与归属判定，Close 记录连接是否被关闭。
type stubNetConn struct {
	local  net.Addr
	closed bool
}

func (s *stubNetConn) Read([]byte) (int, error)    { return 0, io.EOF }
func (s *stubNetConn) Write(b []byte) (int, error) { return len(b), nil }
func (s *stubNetConn) Close() error                { s.closed = true; return nil }
func (s *stubNetConn) LocalAddr() net.Addr         { return s.local }
func (s *stubNetConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 40000}
}
func (s *stubNetConn) SetDeadline(time.Time) error      { return nil }
func (s *stubNetConn) SetReadDeadline(time.Time) error  { return nil }
func (s *stubNetConn) SetWriteDeadline(time.Time) error { return nil }

// TestCleanupClientsForListenerScopedByAddress 验证按地址归属清理：
// 同类型两个 tcp 监听器（8081/8082）时，关闭 8081 只能断掉 8081 上的客户端，
// 8082 上的客户端必须原样保留（连接不关、映射不删、DB 不标离线）。
// 修复前按类型枚举，8082 的客户端会被一并断掉并标记离线。
func TestCleanupClientsForListenerScopedByAddress(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	const uidSame, uidOther = "uid-on-8081", "uid-on-8082"
	database.Engine.Insert(&database.Clients{Uid: uidSame, Online: "1", Listener: "tcp"})
	database.Engine.Insert(&database.Clients{Uid: uidOther, Online: "1", Listener: "tcp"})

	connSame := &stubNetConn{local: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 8081}}
	connOther := &stubNetConn{local: &net.TCPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 8082}}

	tcp.Mutex.Lock()
	tcp.TCPClientManger[uidSame] = connSame
	tcp.TCPClientManger[uidOther] = connOther
	tcp.Mutex.Unlock()
	defer func() {
		tcp.Mutex.Lock()
		delete(tcp.TCPClientManger, uidSame)
		delete(tcp.TCPClientManger, uidOther)
		tcp.Mutex.Unlock()
	}()

	connection.MuClientListenerType.Lock()
	connection.ClientListenerType[uidSame] = "tcp"
	connection.ClientListenerType[uidOther] = "tcp"
	connection.MuClientListenerType.Unlock()
	defer func() {
		connection.MuClientListenerType.Lock()
		delete(connection.ClientListenerType, uidSame)
		delete(connection.ClientListenerType, uidOther)
		connection.MuClientListenerType.Unlock()
	}()

	// 关闭监听 0.0.0.0:8081（通配地址，连接本端是具体 IP）
	cleanupClientsForListener("tcp", "0.0.0.0:8081")

	// 属于本监听器的客户端：应被断开并清理
	if !connSame.closed {
		t.Error("8081 上的客户端连接应被关闭")
	}
	if _, ok := tcp.TCPClientManger[uidSame]; ok {
		t.Error("8081 上的客户端应从连接管理器移除")
	}
	var cSame database.Clients
	database.Engine.Where("uid = ?", uidSame).Get(&cSame)
	if cSame.Online != "2" {
		t.Errorf("8081 上的客户端应标记离线, Online=%s", cSame.Online)
	}

	// 同类型另一监听器的客户端：必须原样保留
	if connOther.closed {
		t.Error("同类型另一监听器(8082)上的客户端被误断连接")
	}
	if _, ok := tcp.TCPClientManger[uidOther]; !ok {
		t.Error("同类型另一监听器(8082)上的客户端被误清出连接管理器")
	}
	if _, ok := connection.ClientListenerType[uidOther]; !ok {
		t.Error("同类型另一监听器(8082)上的客户端监听类型映射被误删")
	}
	var cOther database.Clients
	database.Engine.Where("uid = ?", uidOther).Get(&cOther)
	if cOther.Online != "1" {
		t.Errorf("同类型另一监听器(8082)上的客户端被误标离线, Online=%s", cOther.Online)
	}
}
