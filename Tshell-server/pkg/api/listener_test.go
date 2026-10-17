package api

import (
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/connection/tcp"
	"BackendTemplate/pkg/database"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// stubAddrs 只实现 connAddrs，用于验证归属判定中的端口比对。
type stubAddrs struct{ local, remote net.Addr }

func (s stubAddrs) LocalAddr() net.Addr  { return s.local }
func (s stubAddrs) RemoteAddr() net.Addr { return s.remote }

// TestConnOnListener 覆盖归属判定的关键语义。
// 反向监听比对本端端口：监听地址常绑通配地址，而 Accept 到的连接其 LocalAddr 是具体 IP，
// 整串比较必然不匹配。正向监听比对远端端口：连接由本机拨出，监听地址即拨号目标，
// 本端只是临时端口（若误按本端比对会全部判为不归属，关闭正向监听器时客户端不被清理）。
// 地址不含端口（oss/dns 复用 listen_address 字段）或取不到地址时无法判定，
// 须回退为 true（保持原有按类型清理的语义）。
func TestConnOnListener(t *testing.T) {
	loopback := &net.TCPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 54321}

	tests := []struct {
		name         string
		listenerType string
		listener     string
		local        net.Addr
		remote       net.Addr
		want         bool
		why          string
	}{
		// 反向：连接由本机 Accept，比对本端端口
		{
			name: "反向_通配监听_本端为具体IP_端口一致_判定归属", listenerType: "tcp", listener: "0.0.0.0:8081",
			local:  &net.TCPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 8081},
			remote: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 40001}, want: true,
			why: "0.0.0.0 与 192.168.1.5 整串不等，只能按端口判定",
		},
		{
			name: "反向_同类型另一监听器_本端端口不同_不归属", listenerType: "tcp", listener: "0.0.0.0:8081",
			local:  &net.TCPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 8082},
			remote: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 9), Port: 40001}, want: false,
			why: "本修复要拦的场景：关 8081 不得误断 8082 上的客户端",
		},
		{
			name: "反向_IPv6通配_端口一致_判定归属", listenerType: "kcp", listener: "[::]:8081",
			local: &net.TCPAddr{IP: net.IPv6loopback, Port: 8081}, want: true,
		},
		{
			name: "反向_监听地址为域名_端口一致_判定归属", listenerType: "websocket", listener: "c2.example.com:8081",
			local: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 7), Port: 8081}, want: true,
		},

		// 正向：连接由本机拨出，监听地址即拨号目标，比对远端端口
		{
			name: "正向_远端为拨号目标_端口一致_判定归属", listenerType: "tcp_forward", listener: "10.0.0.5:8081",
			local: loopback, remote: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 5), Port: 8081}, want: true,
			why: "本端是临时端口，若误按本端比对会判为不归属",
		},
		{
			name: "正向_远端端口不同_不归属", listenerType: "tcp_forward", listener: "10.0.0.5:8081",
			local: loopback, remote: &net.TCPAddr{IP: net.IPv4(10, 0, 0, 5), Port: 8082}, want: false,
		},
		{
			name: "正向_远端为域名解析后IP_按端口判定归属", listenerType: "ws_forward", listener: "client.example.com:8081",
			local: loopback, remote: &net.TCPAddr{IP: net.IPv4(203, 0, 113, 9), Port: 8081}, want: true,
			why: "远端是域名解析后的 IP，整串比较会不匹配，只能按端口",
		},

		// 无法判定 → 保持原有按类型语义
		{
			name: "地址非hostport_无法判定_回退按类型", listenerType: "tcp", listener: "oss:AK:SK:bucket",
			local: loopback, want: true,
		},
		{
			name: "本端地址不含端口_无法判定_回退按类型", listenerType: "tcp", listener: "0.0.0.0:8081",
			local: &net.UnixAddr{Name: "/tmp/tshell.sock", Net: "unix"}, want: true,
		},
		{
			name: "本端地址为空_无法判定_回退按类型", listenerType: "tcp", listener: "0.0.0.0:8081", want: true,
		},
		{
			name: "正向远端地址为空_无法判定_回退按类型", listenerType: "tcp_forward", listener: "10.0.0.5:8081", want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := connOnListener(stubAddrs{local: tt.local, remote: tt.remote}, tt.listenerType, tt.listener)
			if got != tt.want {
				t.Fatalf("connOnListener(type=%s, listener=%q, local=%v, remote=%v) = %v, want %v (%s)",
					tt.listenerType, tt.listener, tt.local, tt.remote, got, tt.want, tt.why)
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

// TestHandleOpenPortRejectsDefaultKeyMarksStopped 启动失败必须把状态回置为已停止：
// 原实现只在日志里拒绝，库里仍留着 Status=1，前端显示"监听中"、dashboard 计数虚高。
func TestHandleOpenPortRejectsDefaultKeyMarksStopped(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	const addr = "0.0.0.0:18098"
	const defaultVKey, defaultSalt = "initial_master_key_12345678901", "initial_salt_123"
	database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: addr, ConnectAddress: "c2.example.com:18098",
		Status: 1, VKey: defaultVKey, EncryptionSalt: defaultSalt,
	})

	handleOpenPort("tcp", addr, defaultVKey, defaultSalt)

	var lis database.Listener
	if has, _ := database.Engine.Where("listen_address = ?", addr).Get(&lis); !has {
		t.Fatal("监听器记录应仍在库中")
	}
	if lis.Status != 2 {
		t.Errorf("启动失败后状态应为 2（已停止），实际 %d", lis.Status)
	}
	connection.MuClientListenerType.Lock()
	_, hasHandle := connection.TCPServer[addr]
	connection.MuClientListenerType.Unlock()
	if hasHandle {
		t.Error("密钥被拒时不应注册任何监听句柄")
	}
}

// TestAddListenerRejectsDefaultKeyBeforeInsert 默认/占位符密钥必须在入库前就被拒：
// 原实现先 Insert(Status:1) 再异步启动，被 J4 拒绝后库里会留下永远起不来的记录。
func TestAddListenerRejectsDefaultKeyBeforeInsert(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	const addr = "0.0.0.0:18099"
	body := `{"type":"tcp","listenAddress":"` + addr + `","connectAddress":"c2.example.com:18099",` +
		`"vkey":"initial_master_key_12345678901","encryptionSalt":"initial_salt_123"}`

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/listener/add", AddListener)

	req := httptest.NewRequest(http.MethodPost, "/listener/add", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var resp struct {
		Status int    `json:"status"`
		Data   string `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应解析失败: %v (body=%s)", err, w.Body.String())
	}
	if resp.Status != 400 {
		t.Errorf("默认密钥应返回 status=400，实际 %d (body=%s)", resp.Status, w.Body.String())
	}

	var count int64
	count, _ = database.Engine.Where("listen_address = ?", addr).Count(&database.Listener{})
	if count != 0 {
		t.Errorf("被拒的监听器不应入库，实际有 %d 条记录", count)
	}
}

// TestHTTPBindFailureClearsHandleAndStatus 端口被占用导致 HTTP 监听起不来时，
// 必须同时摘掉 connection.HttpServer 里的句柄并把状态回置为已停止。
// 原实现只在 goroutine 里打一行日志：句柄留在 map 中会让 listenerHandleAlive 探活为 true
// （"显示监听中、实际没在听"），且 DB 状态永远停在 1。
func TestHTTPBindFailureClearsHandleAndStatus(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	// 先占住一个端口，让后续 bind 必然失败
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占位监听失败: %v", err)
	}
	defer blocker.Close()
	addr := blocker.Addr().String()

	const vkey, salt = "httpBindFailVKey0123456789abcd", "httpBindFailSalt"
	database.Engine.Insert(&database.Listener{
		Type: "http", ListenAddress: addr, ConnectAddress: addr,
		Status: 1, VKey: vkey, EncryptionSalt: salt,
	})

	handleOpenPort("http", addr, vkey, salt)

	// ListenAndServe 在 goroutine 中失败，轮询等待状态收敛
	deadline := time.Now().Add(3 * time.Second)
	var status int
	var handleLeft bool
	for time.Now().Before(deadline) {
		var lis database.Listener
		database.Engine.Where("listen_address = ?", addr).Get(&lis)
		status = lis.Status
		connection.MuClientListenerType.Lock()
		_, handleLeft = connection.HttpServer[addr]
		connection.MuClientListenerType.Unlock()
		if status == 2 && !handleLeft {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if handleLeft {
		t.Error("bind 失败后 connection.HttpServer 中仍残留句柄（listenerHandleAlive 会误判为运行中）")
	}
	if status != 2 {
		t.Errorf("bind 失败后状态应回置为 2（已停止），实际 %d", status)
	}
}
