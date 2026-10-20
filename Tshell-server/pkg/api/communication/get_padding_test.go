package communication

import (
	"strings"
	"testing"
)

// ===== 响应侧 padding（阶段二，与客户端请求 padding 同一套规则）=====
//
// 改造前的实现是 16~31 字符**随机长度**小写串 —— 长度随机等于把"定长指纹"
// 换成"随机长度指纹"，对长度分析没有帮助。这里守住三条：
// 定长（同体积同长度）、落桶、字符集像真实字段值。

func TestResponsePadHitsBucket(t *testing.T) {
	for _, bodyLen := range []int{10, 100, 200, 500, 1000, 2000, 4000} {
		pad := ResponsePadFor(bodyLen)
		total := bodyLen + len(pad)
		hit := false
		for _, b := range ResponsePadBuckets {
			if total == b {
				hit = true
				break
			}
		}
		if !hit {
			t.Errorf("bodyLen=%d + pad=%d = %d 未落在任何桶 %v 上",
				bodyLen, len(pad), total, ResponsePadBuckets)
		}
	}
}

func TestResponsePadMinimum(t *testing.T) {
	for bodyLen := 0; bodyLen <= 9000; bodyLen += 137 {
		if got := len(ResponsePadFor(bodyLen)); got < MinResponsePad {
			t.Fatalf("bodyLen=%d 的填充长度 %d 小于下限 %d", bodyLen, got, MinResponsePad)
		}
	}
}

// 同一体积必须得到同一填充长度（长度确定的，值才随机）
func TestResponsePadLengthIsDeterministic(t *testing.T) {
	for _, bodyLen := range []int{0, 50, 300, 1500, 5000, 20000} {
		want := len(ResponsePadFor(bodyLen))
		for i := 0; i < 20; i++ {
			if got := len(ResponsePadFor(bodyLen)); got != want {
				t.Fatalf("bodyLen=%d 的填充长度不稳定: %d vs %d", bodyLen, want, got)
			}
		}
	}
}

// 超过最大桶时退化为下限，不无限放大
func TestResponsePadCapsHugeBody(t *testing.T) {
	if got := len(ResponsePadFor(100000)); got > 4096 {
		t.Errorf("超大响应体的填充不应超过 4096，实际 %d", got)
	}
}

// 值必须是标准 base64 字符集（小写字母串不像真实字段值）
func TestResponsePadCharset(t *testing.T) {
	v := ResponsePadFor(100)
	for _, r := range v {
		if !strings.ContainsRune(base64Alphabet, r) {
			t.Fatalf("填充值含非 base64 字符 %q（值=%q）", r, v)
		}
	}
}

// 桶必须严格递增且都是 2 的幂
func TestResponsePadBucketsSane(t *testing.T) {
	for i, b := range ResponsePadBuckets {
		if b <= 0 || b&(b-1) != 0 {
			t.Errorf("桶 %d 不是 2 的幂: %d", i, b)
		}
		if i > 0 && b <= ResponsePadBuckets[i-1] {
			t.Errorf("桶必须严格递增: 第 %d 个 %d", i, b)
		}
	}
}
