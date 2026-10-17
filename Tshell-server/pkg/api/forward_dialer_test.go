package api

import (
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// closeFunc 便于构造 io.Closer
type closeFunc func() error

func (f closeFunc) Close() error { return f() }

// TestRunForwardDialerRetriesWithBackoffAndStops 拨号失败要按退避重试；
// 停止信号在等待退避期间也必须立即生效（关闭监听器不能干等满一个退避周期）。
func TestRunForwardDialerRetriesWithBackoffAndStops(t *testing.T) {
	stopCh := make(chan bool, 1)
	var mu sync.Mutex
	dials := 0
	dial := func() (*forwardSession, error) {
		mu.Lock()
		dials++
		mu.Unlock()
		return nil, fmt.Errorf("目标不可达")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runForwardDialerWith(stopCh, 5*time.Millisecond, 20*time.Millisecond,
			"tcp_forward", "127.0.0.1:1", dial)
	}()

	time.Sleep(80 * time.Millisecond)
	stopCh <- true

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("停止信号未能让拨号循环退出")
	}

	mu.Lock()
	n := dials
	mu.Unlock()
	if n < 2 {
		t.Errorf("拨号失败后应重试，实际只拨号 %d 次", n)
	}
}

// TestRunForwardDialerReconnectsAfterSessionEnds 会话结束（掉线）后应重连，
// 且会话运行期间必须登记在 forwardSessions 中——handleClosePort 依赖它主动断开
// 正在阻塞的会话。
func TestRunForwardDialerReconnectsAfterSessionEnds(t *testing.T) {
	const target = "127.0.0.1:65001"
	stopCh := make(chan bool, 1)
	var mu sync.Mutex
	runs := 0

	dial := func() (*forwardSession, error) {
		s := &forwardSession{closer: closeFunc(func() error { return nil })}
		s.run = func() {
			forwardSessionsMu.Lock()
			_, registered := forwardSessions[target]
			forwardSessionsMu.Unlock()
			if !registered {
				t.Error("会话运行期间未登记在 forwardSessions 中（关闭监听器将无法断开它）")
			}
			mu.Lock()
			runs++
			mu.Unlock()
		}
		return s, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runForwardDialerWith(stopCh, 5*time.Millisecond, 20*time.Millisecond,
			"tcp_forward", target, dial)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := runs
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	stopCh <- true

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("停止信号未能让拨号循环退出")
	}

	mu.Lock()
	n := runs
	mu.Unlock()
	if n < 2 {
		t.Errorf("会话结束后应重连，实际只运行 %d 次", n)
	}
	forwardSessionsMu.Lock()
	_, left := forwardSessions[target]
	forwardSessionsMu.Unlock()
	if left {
		t.Error("循环退出后 forwardSessions 不应残留会话")
	}
}

// TestCloseForwardSessionClosesActiveSession 关闭监听器必须真正断开活跃会话：
// run 阻塞在协议读循环里，只发停止信号不会让它返回。
func TestCloseForwardSessionClosesActiveSession(t *testing.T) {
	const target = "127.0.0.1:65002"
	closed := make(chan struct{})
	sess := &forwardSession{
		closer: closeFunc(func() error { close(closed); return nil }),
		run:    func() {},
	}

	forwardSessionsMu.Lock()
	forwardSessions[target] = sess
	forwardSessionsMu.Unlock()

	closeForwardSession(target)

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("活跃会话未被关闭")
	}
	forwardSessionsMu.Lock()
	_, left := forwardSessions[target]
	forwardSessionsMu.Unlock()
	if left {
		t.Error("关闭后应从 forwardSessions 中摘除")
	}
}

// TestCloseForwardSessionNoopWhenIdle 无活跃会话时不应 panic
func TestCloseForwardSessionNoopWhenIdle(t *testing.T) {
	closeForwardSession("127.0.0.1:65003")
}

// TestIsForwardListenerType 正向类型判定：J4 豁免与 handleOpenPort/ClosePort 分支都依赖它
func TestIsForwardListenerType(t *testing.T) {
	forward := []string{"tcp_forward", "kcp_forward", "ws_forward", "http_forward", "https_forward"}
	for _, ty := range forward {
		if !isForwardListenerType(ty) {
			t.Errorf("%s 应判定为正向监听", ty)
		}
	}
	reverse := []string{"tcp", "kcp", "websocket", "http", "https", "dns", "oss", ""}
	for _, ty := range reverse {
		if isForwardListenerType(ty) {
			t.Errorf("%s 不应判定为正向监听", ty)
		}
	}
}

// TestForwardListenerOpenCloseLifecycle 端到端接线：正向监听器"开启"启动拨号循环、
// "关闭"停止它；listenerHandleAlive 据此探活（RestartListener 依赖探活避免句柄缺失时
// 对同一个 gin.Context 二次写响应）。
func TestForwardListenerOpenCloseLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	// connect_address 指向必然拒绝连接的端口：拨号失败即进入退避重试，不影响断言
	const addr = "127.0.0.1:1"
	database.Engine.Insert(&database.Listener{
		Type: "tcp_forward", ListenAddress: addr, ConnectAddress: addr,
		Status: 2, HeartbeatTimeout: "3", HeartbeatInterval: "10",
	})

	handleOpenPort("tcp_forward", addr, "", "")

	if !listenerHandleAlive("tcp_forward", addr) {
		t.Fatal("开启后 listenerHandleAlive 应为 true（拨号循环已启动）")
	}
	connection.MuClientListenerType.Lock()
	_, hasStop := connection.StopChan[addr]
	connection.MuClientListenerType.Unlock()
	if !hasStop {
		t.Error("开启后应登记停止通道")
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	handleClosePort("tcp_forward", addr, c)

	if listenerHandleAlive("tcp_forward", addr) {
		t.Error("关闭后 listenerHandleAlive 应为 false")
	}
	connection.MuClientListenerType.Lock()
	_, hasStop = connection.StopChan[addr]
	connection.MuClientListenerType.Unlock()
	if hasStop {
		t.Error("关闭后应摘除停止通道")
	}
}
