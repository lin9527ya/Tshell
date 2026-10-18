package app

import (
	"reflect"
	"testing"
)

// 端点选择器的接线（多传输健康探测注入）：默认顺序轮转，注入选择器后优先健康端点，
// 选择器返回空串时回退顺序轮转（避免全不健康时原地卡住）。

func withEndpoints(t *testing.T, list []string) {
	t.Helper()
	// 先触发一次真实初始化（从 config 填充），之后再覆盖为测试列表：
	// initEndpoints 只会执行一次，不会把 config.Host 追加进测试数据。
	CurrentEndpoint()
	old := endpoints
	oldIdx := epIdx
	oldSel := endpointSelector
	endpoints, epIdx, endpointSelector = append([]string{}, list...), 0, nil
	t.Cleanup(func() {
		endpoints, epIdx, endpointSelector = old, oldIdx, oldSel
	})
}

func TestAdvanceEndpointDefaultRoundRobin(t *testing.T) {
	withEndpoints(t, []string{"tcp://a", "https://b", "dns://c"})
	if got := AdvanceEndpoint(); got != "https://b" {
		t.Fatalf("默认应顺序轮转，实际 %q", got)
	}
	if got := AdvanceEndpoint(); got != "dns://c" {
		t.Fatalf("默认应顺序轮转，实际 %q", got)
	}
	if got := AdvanceEndpoint(); got != "tcp://a" {
		t.Fatalf("应回环到第一个，实际 %q", got)
	}
}

func TestAdvanceEndpointUsesSelector(t *testing.T) {
	withEndpoints(t, []string{"tcp://a", "https://b", "dns://c"})
	SetEndpointSelector(func(current string, all []string) string {
		// 跳过 https://b（模拟它被健康探测标记为不可用）
		if current == "tcp://a" {
			return "dns://c"
		}
		return ""
	})
	if got := AdvanceEndpoint(); got != "dns://c" {
		t.Fatalf("应采纳选择器结果，实际 %q", got)
	}
	// 选择器返回空 → 回退顺序轮转
	if got := AdvanceEndpoint(); got != "tcp://a" {
		t.Fatalf("选择器空结果应回退顺序轮转，实际 %q", got)
	}
}

func TestAllEndpointsReturnsCopy(t *testing.T) {
	withEndpoints(t, []string{"tcp://a", "https://b"})
	got := AllEndpoints()
	if !reflect.DeepEqual(got, []string{"tcp://a", "https://b"}) {
		t.Fatalf("AllEndpoints 不符: %v", got)
	}
	got[0] = "mutated"
	if endpoints[0] != "tcp://a" {
		t.Fatal("AllEndpoints 必须返回副本")
	}
}

func TestSetCurrentEndpoint(t *testing.T) {
	withEndpoints(t, []string{"tcp://a", "https://b", "dns://c"})
	SetCurrentEndpoint("dns://c")
	if got := CurrentEndpoint(); got != "dns://c" {
		t.Fatalf("当前端点应切到 dns://c，实际 %q", got)
	}
	// 不存在的端点不应改变当前值
	SetCurrentEndpoint("nope://x")
	if got := CurrentEndpoint(); got != "dns://c" {
		t.Fatalf("不存在的端点不应改变当前值，实际 %q", got)
	}
}
