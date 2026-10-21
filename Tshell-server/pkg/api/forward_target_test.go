package api

import (
	"BackendTemplate/pkg/encrypt"
	"testing"
)

// TestIsUnspecifiedForwardTarget 通配/空地址不能作为正向拨号目标：
// 载荷生成时填的是客户端 bind 地址（0.0.0.0），拨过去只会连到服务端本机。
func TestIsUnspecifiedForwardTarget(t *testing.T) {
	cases := []struct {
		addr string
		want bool
		why  string
	}{
		{"", true, "空地址不可拨"},
		{"0.0.0.0:80", true, "IPv4 通配"},
		{"[::]:80", true, "IPv6 通配"},
		{"0.0.0.0", true, "无端口通配"},
		{"[::]", true, "无端口 IPv6 通配"},
		{"127.0.0.1:8080", false, "回环可用于本机联调"},
		{"192.168.1.5:80", false, "具体 IPv4"},
		{"client.example.com:443", false, "域名"},
	}
	for _, c := range cases {
		if got := isUnspecifiedForwardTarget(c.addr); got != c.want {
			t.Errorf("isUnspecifiedForwardTarget(%q) = %v，期望 %v（%s）", c.addr, got, c.want, c.why)
		}
	}
}

// TestResolveForwardKeyConfigPrefersRecordKeys 同协议多条正向监听器时，
// 拨号必须用记录自己的密钥，不能按类型取库里第一条。
func TestResolveForwardKeyConfigPrefersRecordKeys(t *testing.T) {
	const vkey = "0123456789abcdef0123456789abcdef" // 32 字节
	const salt = "0123456789abcdef"                 // 16 字节

	mk, s := resolveForwardKeyConfig("tcp_forward", vkey, salt)
	if string(mk) != vkey || string(s) != salt {
		t.Fatalf("应优先使用记录自己的密钥，实际 mk=%q salt=%q", mk, s)
	}
	defMK, defSalt := encrypt.GetListenerKeyConfigForForward("tcp_forward")
	if string(mk) == string(defMK) || string(s) == string(defSalt) {
		t.Fatal("记录配了自定义密钥却仍用了类型默认值")
	}
}

// TestResolveForwardKeyConfigFallsBackForLegacyRecord 历史记录可能没存密钥，
// 此时保持原有行为：回落到按类型的正向密钥配置（与旧实现同一来源）。
func TestResolveForwardKeyConfigFallsBackForLegacyRecord(t *testing.T) {
	wantMK, wantSalt := encrypt.GetListenerKeyConfigForForward("tcp_forward")
	mk, s := resolveForwardKeyConfig("tcp_forward", "", "")
	if string(mk) != string(wantMK) || string(s) != string(wantSalt) {
		t.Fatalf("空密钥记录应回落到按类型的配置，实际 mk=%q salt=%q", mk, s)
	}
}

// TestResolveForwardConnectKeysExplicitAndFallback 手动连接：显式密钥优先；
// 没有显式密钥且库里查不到记录时返回 nil，保持按类型回落的既有行为。
func TestResolveForwardConnectKeysExplicitAndFallback(t *testing.T) {
	const vkey = "abcdefabcdefabcdefabcdefabcdefab" // 32 字节
	const salt = "abcdefabcdefabcd"                 // 16 字节

	mk, s := resolveForwardConnectKeys("203.0.113.77:65000", vkey, salt)
	if string(mk) != vkey || string(s) != salt {
		t.Fatalf("显式密钥应优先，实际 mk=%q salt=%q", mk, s)
	}

	mk, s = resolveForwardConnectKeys("203.0.113.77:65000", "", "")
	if mk != nil || s != nil {
		t.Fatalf("无显式密钥且无记录时应返回 nil 交给类型回落，实际 mk=%v salt=%v", mk, s)
	}
}
