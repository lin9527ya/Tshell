package commands

import "testing"

// TestStopHTTPTunnelAgentOnlyStopsOne 同一客户端多个 HTTP/TCP 隧道按 Agent 地址隔离：
// 停止一个不能影响另一个（旧实现使用单一全局 agent，第二个会顶掉第一个）。
func TestStopHTTPTunnelAgentOnlyStopsOne(t *testing.T) {
	a := &HTTPTunnelAgent{ServerAddress: "a:2001", Running: true, StopChan: make(chan bool, 1)}
	b := &HTTPTunnelAgent{ServerAddress: "b:2002", Running: true, StopChan: make(chan bool, 1)}

	httpTunnelMutex.Lock()
	saved := httpTunnelAgents
	httpTunnelAgents = map[string]*HTTPTunnelAgent{
		a.ServerAddress: a,
		b.ServerAddress: b,
	}
	httpTunnelMutex.Unlock()
	t.Cleanup(func() {
		httpTunnelMutex.Lock()
		httpTunnelAgents = saved
		httpTunnelMutex.Unlock()
	})

	stopHTTPTunnelAgent(a.ServerAddress)

	if a.Running {
		t.Fatal("被停止的 agent 仍为 Running")
	}
	if !b.Running {
		t.Fatal("另一个 agent 不应被停止")
	}
	httpTunnelMutex.RLock()
	_, aExists := httpTunnelAgents[a.ServerAddress]
	_, bExists := httpTunnelAgents[b.ServerAddress]
	httpTunnelMutex.RUnlock()
	if aExists {
		t.Fatal("被停止的 agent 未从 map 移除")
	}
	if !bExists {
		t.Fatal("另一个 agent 不应从 map 移除")
	}
}
