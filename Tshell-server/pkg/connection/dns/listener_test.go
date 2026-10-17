package dns

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

var (
	keyA  = bytes.Repeat([]byte{0xA1}, 32)
	saltA = bytes.Repeat([]byte{0xA2}, 16)
	keyB  = bytes.Repeat([]byte{0xB1}, 32)
	saltB = bytes.Repeat([]byte{0xB2}, 16)
)

// TestStartTwoInstancesAreIndependent 两个 DNS 监听器各自持有域名/前缀/端口/客户端表，
// 互不覆盖。原实现全是包级状态：后启动的域名直接覆盖先启动的，两者还会共享客户端表。
func TestStartTwoInstancesAreIndependent(t *testing.T) {
	a, err := Start("key-a", "127.0.0.1:0", "a.example.", "pa", keyA, saltA)
	if err != nil {
		t.Fatalf("启动 A 失败: %v", err)
	}
	defer StopListener("key-a")
	b, err := Start("key-b", "127.0.0.1:0", "b.example", "pb", keyB, saltB)
	if err != nil {
		t.Fatalf("启动 B 失败: %v", err)
	}
	defer StopListener("key-b")

	if a.listenerDomain != "a.example." || b.listenerDomain != "b.example." {
		t.Errorf("域名互相覆盖: A=%q B=%q", a.listenerDomain, b.listenerDomain)
	}
	if a.dnsLabelPrefix != "pa" || b.dnsLabelPrefix != "pb" {
		t.Errorf("标签前缀互相覆盖: A=%q B=%q", a.dnsLabelPrefix, b.dnsLabelPrefix)
	}
	// 密钥必须各用各的：按类型查库只能取到任意一行，多实例下会串
	if !bytes.Equal(a.masterKey, keyA) || !bytes.Equal(a.salt, saltA) {
		t.Errorf("A 未持有自己的密钥")
	}
	if !bytes.Equal(b.masterKey, keyB) || !bytes.Equal(b.salt, saltB) {
		t.Errorf("B 未持有自己的密钥")
	}
	if a.packetConn.LocalAddr().String() == b.packetConn.LocalAddr().String() {
		t.Error("两个实例应各自绑定独立的 UDP 端口")
	}

	listenersMu.Lock()
	_, hasA := listeners["key-a"]
	_, hasB := listeners["key-b"]
	listenersMu.Unlock()
	if !hasA || !hasB {
		t.Errorf("两个实例都应登记在 listeners 中: A=%v B=%v", hasA, hasB)
	}
}

// TestStartBindFailureReturnsError 端口被占用时 Start 必须同步返回错误
// （原实现把 ListenAndServe 放在 goroutine 里，失败只打日志）。
func TestStartBindFailureReturnsError(t *testing.T) {
	blocker, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("占位失败: %v", err)
	}
	defer blocker.Close()
	addr := blocker.LocalAddr().String()

	l, err := Start("key-dup", addr, "dup.example.", "", keyA, saltA)
	if err == nil {
		StopListener("key-dup")
		t.Fatalf("端口被占用时应返回错误，实际拿到实例 %v", l)
	}
	listenersMu.Lock()
	_, registered := listeners["key-dup"]
	listenersMu.Unlock()
	if registered {
		t.Error("启动失败的实例不应登记")
	}
}

// TestStopListenerIsolatesAndIsIdempotent 按地址停止只影响对应实例，重复停止不 panic。
func TestStopListenerIsolatesAndIsIdempotent(t *testing.T) {
	if _, err := Start("key-x", "127.0.0.1:0", "x.example.", "", keyA, saltA); err != nil {
		t.Fatalf("启动 X 失败: %v", err)
	}
	b, err := Start("key-y", "127.0.0.1:0", "y.example.", "", keyB, saltB)
	if err != nil {
		t.Fatalf("启动 Y 失败: %v", err)
	}
	defer StopListener("key-y")

	StopListener("key-x")
	StopListener("key-x") // 幂等

	listenersMu.Lock()
	_, hasX := listeners["key-x"]
	_, hasY := listeners["key-y"]
	listenersMu.Unlock()
	if hasX {
		t.Error("X 应已被注销")
	}
	if !hasY {
		t.Error("停 X 不应影响 Y")
	}
	if b.stopped.Load() {
		t.Error("停 X 不应把 Y 标记为停止")
	}
}

// TestHandleRequestRoutedToCorrectInstance 真实 UDP 查询级别的验证：
// 每个实例只应答自己域名的查询。原实现域名是包级的，后启动的实例会覆盖前一个的域名，
// 两个监听器会对同一批查询都给出应答（或都不应答）。
func TestHandleRequestRoutedToCorrectInstance(t *testing.T) {
	a, err := Start("key-a", "127.0.0.1:0", "a.example.", "", keyA, saltA)
	if err != nil {
		t.Fatalf("启动 A 失败: %v", err)
	}
	defer StopListener("key-a")
	b, err := Start("key-b", "127.0.0.1:0", "b.example.", "", keyB, saltB)
	if err != nil {
		t.Fatalf("启动 B 失败: %v", err)
	}
	defer StopListener("key-b")

	addrA := a.packetConn.LocalAddr().String()
	addrB := b.packetConn.LocalAddr().String()
	uid := "0123456789abcdef0123456789abcdef"

	// 让 uid 命中各自实例的客户端缓存（steady-state 路径）。否则 updateClientOnline
	// 会走 DB 兜底分支，而本测试不初始化数据库。
	for _, l := range []*Listener{a, b} {
		l.dnsClientsLock.Lock()
		l.dnsClients[uid] = &dnsClientInfo{uid: uid, listener: "dns", ip: "127.0.0.1", lastSeen: time.Now(), lastDBWrite: time.Now()}
		l.dnsClientsLock.Unlock()
	}

	query := func(addr, name string) string {
		t.Helper()
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn(name), dns.TypeTXT)
		c := new(dns.Client)
		c.Timeout = 2 * time.Second
		resp, _, err := c.Exchange(m, addr)
		if err != nil {
			t.Fatalf("查询 %s @ %s 失败: %v", name, addr, err)
		}
		for _, rr := range resp.Answer {
			if txt, ok := rr.(*dns.TXT); ok {
				return strings.Join(txt.Txt, "")
			}
		}
		return ""
	}

	// 各自的域名 → 各自的实例应答（hb 端点固定回 "ok"）
	if got := query(addrA, uid+".hb.0.a.example."); got != "ok" {
		t.Errorf("A 应应答自己的域名，实际 TXT=%q", got)
	}
	if got := query(addrB, uid+".hb.0.b.example."); got != "ok" {
		t.Errorf("B 应应答自己的域名，实际 TXT=%q", got)
	}
	// 交叉查询 → 不应答（域名不匹配，路由不到任何处理分支）
	if got := query(addrA, uid+".hb.0.b.example."); got != "" {
		t.Errorf("A 不应应答 B 的域名，实际 TXT=%q", got)
	}
	if got := query(addrB, uid+".hb.0.a.example."); got != "" {
		t.Errorf("B 不应应答 A 的域名，实际 TXT=%q", got)
	}
}
