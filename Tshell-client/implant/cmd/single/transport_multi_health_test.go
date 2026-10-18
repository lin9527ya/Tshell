//go:build transport_multi

package main

import (
	"net"
	"testing"
)

// 主动健康探测的验收点：不健康端点被跳过、未知端点不回避、活动端点连续失败会
// 触发主动断开、不可 TCP 探测的传输不被误判。

func TestHealthTrackerNextHealthy(t *testing.T) {
	h := newHealthTracker()
	eps := []string{"tcp://a:1", "https://b", "dns://c"}

	// 未探测过 → 都健康
	if got := h.nextHealthy("tcp://a:1", eps); got != "https://b" {
		t.Fatalf("应选下一个健康端点，实际 %q", got)
	}

	// https 连续失败 → 跳过它
	h.record("https://b", false)
	h.record("https://b", false)
	if h.isHealthy("https://b") {
		t.Fatal("连续失败两次应判定为不健康")
	}
	if got := h.nextHealthy("tcp://a:1", eps); got != "dns://c" {
		t.Fatalf("应跳过不健康端点，实际 %q", got)
	}

	// 全部不健康 → 返回空串（调用方回退顺序轮转，而不是原地卡住）
	h.record("dns://c", false)
	h.record("dns://c", false)
	h.record("tcp://a:1", false)
	h.record("tcp://a:1", false)
	if got := h.nextHealthy("tcp://a:1", eps); got != "" {
		t.Fatalf("全不健康时应返回空串，实际 %q", got)
	}

	// 一次成功即恢复健康
	h.record("https://b", true)
	if !h.isHealthy("https://b") {
		t.Fatal("成功后应恢复健康")
	}
}

func TestHealthProbeAndActiveCallback(t *testing.T) {
	h := newHealthTracker()
	eps := []string{"tcp://a:1", "https://b"}
	reachable := map[string]bool{"tcp://a:1": false, "https://b": true}
	h.probeFn = func(ep string) bool { return reachable[ep] }

	var forced []string
	h.onActiveUnhealthy = func(ep string) { forced = append(forced, ep) }
	h.setActive("tcp://a:1")

	// 第一次探测：活动端点失败 1 次，未达阈值
	h.probeAll(eps)
	if len(forced) != 0 {
		t.Fatalf("单次失败不应触发主动断开: %v", forced)
	}
	// 第二次：达到阈值 → 回调一次
	h.probeAll(eps)
	if len(forced) != 1 || forced[0] != "tcp://a:1" {
		t.Fatalf("活动端点连续失败应触发断开回调，实际 %v", forced)
	}
	// 非活动端点始终健康
	if !h.isHealthy("https://b") {
		t.Fatal("探测通过的端点应健康")
	}
	// 活动端点恢复后不再回调
	reachable["tcp://a:1"] = true
	h.probeAll(eps)
	if len(forced) != 1 {
		t.Fatalf("恢复后不应再回调: %v", forced)
	}
}

// TestProbeEndpoint 真实探测：本机监听端口可达、关闭端口不可达、
// 不可 TCP 探测的传输（KCP/DNS/OSS）不误判
func TestProbeEndpoint(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	alive := "tcp://" + ln.Addr().String()
	if !probeEndpoint(alive) {
		t.Fatalf("监听中的端点应探测为可达: %s", alive)
	}
	dead := "tcp://127.0.0.1:1"
	if probeEndpoint(dead) {
		t.Fatalf("关闭的端口不应探测为可达: %s", dead)
	}
	for _, ep := range []string{"dns://1.1.1.1", "oss://bucket.example.com", "kcp://127.0.0.1:1", "kcp_forward://127.0.0.1:1"} {
		if !probeEndpoint(ep) {
			t.Errorf("不可 TCP 探测的端点应视为健康: %s", ep)
		}
	}
	// 带路径的端点：去掉路径后按显式端口探测（本机 :1 必然拒绝）
	if probeEndpoint("https://127.0.0.1:1/api") {
		t.Error("不可达端点不应探测为可达")
	}
}

// TestConnectSubstitutesUnhealthyEndpoint 请求连接一个不健康端点时，
// 应自动改用健康端点（runloop 持有的 host 可能是探测前的旧值）
func TestConnectSubstitutesUnhealthyEndpoint(t *testing.T) {
	tcpFake := &fakeTransport{}
	httpsFake := &fakeTransport{}
	withRegistry(t, map[string]func() transportClient{
		"tcp":   func() transportClient { return tcpFake },
		"https": func() transportClient { return httpsFake },
	})

	m := newMultiTransport()
	m.health.endpointsFn = func() []string { return []string{"tcp://dead:1", "https://cdn/api"} }
	// 把 tcp 端点标记为不健康
	m.health.record("tcp://dead:1", false)
	m.health.record("tcp://dead:1", false)

	if err := m.Connect("tcp://dead:1"); err != nil {
		t.Fatalf("应改为连接健康端点: %v", err)
	}
	if !httpsFake.IsConnected() || tcpFake.IsConnected() {
		t.Fatalf("应使用 https 端点: tcp=%v https=%v", tcpFake.IsConnected(), httpsFake.IsConnected())
	}
	if m.ActiveScheme() != "https" {
		t.Fatalf("活动传输应为 https，实际 %s", m.ActiveScheme())
	}
}
