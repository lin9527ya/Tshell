//go:build transport_multi

package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"implant/core/reconnect"
)

// 多传输调度器的验收点：按 scheme 选实现、切换时关掉旧连接、
// 未连接时明确报错、runloop 能通过 Underlying() 看到实际传输类型。

type fakeTransport struct {
	mu        sync.Mutex
	connected bool
	addr      string
	closed    bool
	failWith  error
	recvData  []byte
}

func (f *fakeTransport) Connect(host string) error {
	if f.failWith != nil {
		return f.failWith
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected = true
	f.addr = host
	return nil
}
func (f *fakeTransport) Recv() ([]byte, error)     { return f.recvData, nil }
func (f *fakeTransport) SendResult(b []byte) error { return nil }
func (f *fakeTransport) Close() error              { f.mu.Lock(); f.closed = true; f.mu.Unlock(); return nil }
func (f *fakeTransport) IsConnected() bool         { f.mu.Lock(); defer f.mu.Unlock(); return f.connected }
func (f *fakeTransport) ReconnectMgr() *reconnect.ReconnectManager {
	return reconnect.NewReconnectManager()
}

func withRegistry(t *testing.T, ctor map[string]func() transportClient) {
	t.Helper()
	oldReg := transportRegistry
	oldMulti := newMultiTransportFn
	oldListener := listenerType
	transportRegistry = map[string]func() transportClient{}
	for k, v := range ctor {
		transportRegistry[k] = v
	}
	t.Cleanup(func() {
		transportRegistry = oldReg
		newMultiTransportFn = oldMulti
		listenerType = oldListener
	})
}

func TestSplitEndpoint(t *testing.T) {
	cases := []struct{ in, scheme, addr string }{
		{"tcp://10.0.0.5:443", "tcp", "10.0.0.5:443"},
		{"https://cdn.example.com/api", "https", "cdn.example.com/api"},
		{"ws://cdn.example.com/ws", "websocket", "cdn.example.com/ws"},
		{"WEBSOCKET://h/ws", "websocket", "h/ws"},
		{"dns://1.1.1.1", "dns", "1.1.1.1"},
	}
	for _, c := range cases {
		scheme, addr, err := splitEndpoint(c.in)
		if err != nil || scheme != c.scheme || addr != c.addr {
			t.Errorf("splitEndpoint(%q) = (%q,%q,%v)，期望 (%q,%q)", c.in, scheme, addr, err, c.scheme, c.addr)
		}
	}
	for _, bad := range []string{"", "10.0.0.5:443", "tcp://", "://h:1"} {
		if _, _, err := splitEndpoint(bad); err == nil {
			t.Errorf("splitEndpoint(%q) 应报错", bad)
		}
	}
}

func TestMultiTransportDispatchesByScheme(t *testing.T) {
	tcpFake := &fakeTransport{}
	httpsFake := &fakeTransport{}
	withRegistry(t, map[string]func() transportClient{
		"tcp":   func() transportClient { return tcpFake },
		"https": func() transportClient { return httpsFake },
	})

	m := newMultiTransport()
	if err := m.Connect("tcp://a:1"); err != nil {
		t.Fatalf("tcp 连接失败: %v", err)
	}
	if !tcpFake.IsConnected() || tcpFake.addr != "a:1" {
		t.Fatalf("tcp 实现未收到连接: %+v", tcpFake)
	}
	if listenerType != "tcp" {
		t.Fatalf("listenerType = %q，期望 tcp", listenerType)
	}
	if m.Underlying() != tcpFake {
		t.Fatal("Underlying 应返回实际 tcp 传输")
	}

	// 切换到 https：旧连接必须被关闭
	if err := m.Connect("https://cdn.example.com/api"); err != nil {
		t.Fatalf("https 连接失败: %v", err)
	}
	if !httpsFake.IsConnected() || httpsFake.addr != "cdn.example.com/api" {
		t.Fatalf("https 实现未收到连接: %+v", httpsFake)
	}
	if !tcpFake.closed {
		t.Fatal("切换传输时旧连接应被关闭")
	}
	if listenerType != "https" || m.ActiveScheme() != "https" {
		t.Fatalf("切换后类型不符: global=%q active=%q", listenerType, m.ActiveScheme())
	}
}

func TestMultiTransportUnknownScheme(t *testing.T) {
	withRegistry(t, map[string]func() transportClient{})
	m := newMultiTransport()
	err := m.Connect("smtp://x")
	if err == nil {
		t.Fatal("未知传输应报错")
	}
	if !strings.Contains(err.Error(), "未知传输类型") {
		t.Fatalf("错误信息应说明未知传输: %v", err)
	}
}

func TestMultiTransportNotConnected(t *testing.T) {
	withRegistry(t, map[string]func() transportClient{})
	m := newMultiTransport()
	if _, err := m.Recv(); err == nil {
		t.Fatal("未连接时 Recv 应报错")
	}
	if err := m.SendResult([]byte("x")); err == nil {
		t.Fatal("未连接时 SendResult 应报错")
	}
	if m.IsConnected() {
		t.Fatal("未连接时 IsConnected 应为 false")
	}
	if m.Underlying() != nil {
		t.Fatal("未连接时 Underlying 应为 nil")
	}
}

// TestFailedConnectKeepsSchedulerUnconnected 连接失败时调度器不能装作已连接，
// 否则 runloop 会进入 Recv 循环并拿到 nil 传输
func TestFailedConnectKeepsSchedulerUnconnected(t *testing.T) {
	failing := &fakeTransport{failWith: errors.New("connection refused")}
	withRegistry(t, map[string]func() transportClient{
		"tcp": func() transportClient { return failing },
	})
	m := newMultiTransport()
	if err := m.Connect("tcp://dead:1"); err == nil {
		t.Fatal("连接失败应返回错误")
	}
	if m.IsConnected() || m.Underlying() != nil {
		t.Fatal("连接失败后调度器仍报告已连接")
	}
}

// TestMultiTransportUsesSharedReconnectMgr 重连退避跨传输共享：
// 切换传输不应把退避重置成零，否则连续失败时会疯狂重试
// statsEqual 粗略比较统计（用于判定状态是否变化）
func statsEqual(a, b map[string]interface{}) bool {
	return len(a) == len(b) && fmtStats(a) == fmtStats(b)
}

func fmtStats(m map[string]interface{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(fmt.Sprint(m[k]))
		b.WriteString(";")
	}
	return b.String()
}

func TestMultiTransportUsesSharedReconnectMgr(t *testing.T) {
	withRegistry(t, map[string]func() transportClient{
		"tcp": func() transportClient { return &fakeTransport{} },
	})
	m := newMultiTransport()
	first := m.ReconnectMgr()
	_ = m.Connect("tcp://a:1")
	if m.ReconnectMgr() != first {
		t.Fatal("重连管理器应在切换传输间保持同一个实例")
	}
	// 失败状态跨传输保留（不因切换传输被重置）——用统计里的失败计数判定，
	// 不用退避时长：后者带抖动，比较会偶发失败
	before := m.ReconnectMgr().GetStats()
	m.ReconnectMgr().OnReconnectFailed()
	after := m.ReconnectMgr().GetStats()
	if statsEqual(before, after) {
		t.Fatal("切换传输后重连统计被重置（退避状态应跨传输共享）")
	}
}
