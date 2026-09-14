package security

import (
	"math/rand/v2"
)

// 本文件提供非密码学用途的快速随机数生成。
//
// 历史上 traffic_obfuscator.go 的 randomInt / generateRandomString 误用了 crypto/rand
// （rand.Int(rand.Reader, …) 且逐字符调用），在每次混淆请求的热路径上产生大量 syscall，
// 相比 math/rand 约慢 100×。此处改用 math/rand/v2（Go 1.22+ 顶层函数并发安全且自动播种）。
//
// 重要：randomInt / generateRandomString 仅用于混淆模式选择、jitter、伪装 token、
// 协议指纹随机化等“看起来随机即可”的场景。真正的密钥 / nonce / 会话令牌仍必须使用
// crypto/rand（见 pkg/encrypt、jwt_manager.go、hidden_access.go 的 generateRandomKey）。

// randomInt 返回 [min, max) 区间内的非密码学随机整数。
func randomInt(min, max int) int {
	if max <= min {
		return min
	}
	return min + rand.IntN(max-min)
}

// generateRandomString 生成长度为 length 的非密码学随机字符串（大小写字母+数字）。
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}
