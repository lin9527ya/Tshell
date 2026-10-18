package config

import (
	"strings"
	"testing"
)

// 多传输占位符不能参与"未 patch 拒绝启动"检查：单传输载荷本就不会替换它，
// 算进去会让所有普通载荷一启动就退出（本次实测踩到并修复的回归）。

func TestMultiEndpointsNotInResidueCheck(t *testing.T) {
	residues := PlaceholderResidues()
	// 测试里没有 patch，安全敏感项应仍然命中（说明检查本身有效）
	if len(residues) == 0 {
		t.Fatal("未 patch 的配置应检出残留")
	}
	for _, r := range residues {
		if strings.Contains(r, MultiEndpointsPlaceholderPrefix) {
			t.Fatalf("多传输占位符不应参与 fail-closed 检查: %q", r)
		}
	}
}

func TestMultiEndpointsActiveAndList(t *testing.T) {
	old := MultiEndpoints
	t.Cleanup(func() { MultiEndpoints = old })

	if MultiEndpointsActive() {
		t.Fatal("默认（未 patch）状态应为 inactive")
	}
	if got := MultiEndpointsList(); got != nil {
		t.Fatalf("未 patch 时应返回 nil: %v", got)
	}

	// 模拟服务端 patch：逗号分隔 + 空格填充到定长
	MultiEndpoints = "tcp://10.0.0.5:443,https://cdn.example.com/api" +
		strings.Repeat(" ", 512-len("tcp://10.0.0.5:443,https://cdn.example.com/api"))
	if !MultiEndpointsActive() {
		t.Fatal("patch 后应为 active")
	}
	list := MultiEndpointsList()
	if len(list) != 2 || list[0] != "tcp://10.0.0.5:443" || list[1] != "https://cdn.example.com/api" {
		t.Fatalf("端点解析不符: %v", list)
	}
}
