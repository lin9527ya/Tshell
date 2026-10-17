package dns

import (
	"net"
	"testing"
)

// TestStartTwoInstancesAreIndependent 两个 DNS 监听器各自持有域名/前缀/端口/客户端表，
// 互不覆盖。原实现全是包级状态：后启动的域名直接覆盖先启动的，两者还会共享客户端表。
func TestStartTwoInstancesAreIndependent(t *testing.T) {
	a, err := Start("key-a", "127.0.0.1:0", "a.example.", "pa")
	if err != nil {
		t.Fatalf("启动 A 失败: %v", err)
	}
	defer StopListener("key-a")
	b, err := Start("key-b", "127.0.0.1:0", "b.example", "pb")
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

	l, err := Start("key-dup", addr, "dup.example.", "")
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
	if _, err := Start("key-x", "127.0.0.1:0", "x.example.", ""); err != nil {
		t.Fatalf("启动 X 失败: %v", err)
	}
	b, err := Start("key-y", "127.0.0.1:0", "y.example.", "")
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
