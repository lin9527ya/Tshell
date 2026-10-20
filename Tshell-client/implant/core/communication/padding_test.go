package communication

import (
	"strings"
	"testing"
)

// ===== 流量体积整形（阶段二）=====
//
// 要守住的两条：
//  1. **定长**：同一载荷长度的请求必须得到同一填充长度（否则等于把定长指纹
//     换成随机长度指纹，对长度分析毫无帮助 —— 这正是改造前的问题）
//  2. **形态**：填充值必须是标准 base64 字符集（真实 Chrome 的 X-Client-Data
//     是 base64 blob，随机小写串反而不像浏览器）

func TestPaddingValueIsDeterministicPerPayload(t *testing.T) {
	// 同一载荷长度反复取值必须一致
	for _, payload := range []int{0, 1, 100, 400, 900, 1500, 3000, 10000} {
		first := PaddingValueFor(payload)
		for i := 0; i < 20; i++ {
			if got := PaddingValueFor(payload); got != first {
				t.Fatalf("payload=%d 的填充长度不稳定: %d vs %d（定长目标没达成）",
					payload, first, got)
			}
		}
	}
}

func TestPaddingValueMeetsMinimum(t *testing.T) {
	for payload := 0; payload <= 10000; payload += 37 {
		if got := PaddingValueFor(payload); got < MinPadLen {
			t.Fatalf("payload=%d 的填充 %d 小于下限 %d", payload, got, MinPadLen)
		}
	}
}

// 载荷 + 填充应落在同一个桶里（这是"定长分桶"的核心承诺）
func TestPaddingHitsBucket(t *testing.T) {
	for _, payload := range []int{100, 400, 500, 900, 1900, 3900, 7900} {
		pad := PaddingValueFor(payload)
		total := payload + pad
		hit := false
		for _, b := range PadBuckets {
			if total == b {
				hit = true
				break
			}
		}
		if !hit {
			t.Errorf("payload=%d + pad=%d = %d 未落在任何桶 %v 上",
				payload, pad, total, PadBuckets)
		}
	}
}

// 超过最大桶的载荷：退化为最小填充（不无限放大）
func TestPaddingCapsForHugePayload(t *testing.T) {
	for _, payload := range []int{8192, 20000, 100000} {
		if got := PaddingValueFor(payload); got != MinPadLen {
			t.Errorf("payload=%d 超过最大桶时应退化为 %d，实际 %d", payload, MinPadLen, got)
		}
	}
}

// 填充值长度必须精确等于请求值（长度整形的前提）
func TestBuildPaddingExactLength(t *testing.T) {
	for _, n := range []int{1, 40, 100, 512, 4096} {
		if got := BuildPadding(n); len(got) != n {
			t.Errorf("BuildPadding(%d) 长度应为 %d，实际 %d", n, n, len(got))
		}
	}
	if got := BuildPadding(0); got != "" {
		t.Errorf("BuildPadding(0) 应为空串，实际 %q", got)
	}
}

// 值必须是标准 base64 字符集（形态要像 Chrome 的 blob）
func TestPaddingValueCharset(t *testing.T) {
	const allowed = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	for i := 0; i < 20; i++ {
		v := BuildPadding(40)
		for _, r := range v {
			if !strings.ContainsRune(allowed, r) {
				t.Fatalf("填充值含非 base64 字符 %q（值=%q）", r, v)
			}
		}
	}
}

// 两次取值应当不同（填充不该可预测）
func TestPaddingValueIsRandomized(t *testing.T) {
	a := BuildPadding(64)
	b := BuildPadding(64)
	if a == b {
		t.Error("两次填充值相同：随机源没生效，可预测的填充等于没填充")
	}
	if len(a) != len(b) {
		t.Error("两次填充长度不同（长度必须是确定的）")
	}
}

// 常量必须与服务端模板一致，否则两边伪装的头长度对不上
func TestPaddingConstants(t *testing.T) {
	if PaddingHeader != "X-Client-Data" {
		t.Errorf("头名应与服务端 traffic_obfuscator 模板一致，实际 %q", PaddingHeader)
	}
	if MinPadLen != 40 {
		t.Errorf("最小填充应与服务端 {{RAND:40}} 一致，实际 %d", MinPadLen)
	}
	if len(PadBuckets) == 0 {
		t.Fatal("分桶不能为空")
	}
	// 桶必须严格递增且都是 2 的幂
	for i, b := range PadBuckets {
		if b <= 0 || b&(b-1) != 0 {
			t.Errorf("桶 %d 不是 2 的幂: %d", i, b)
		}
		if i > 0 && b <= PadBuckets[i-1] {
			t.Errorf("桶必须严格递增，第 %d 个 %d 不大于前一个 %d", i, b, PadBuckets[i-1])
		}
	}
}
