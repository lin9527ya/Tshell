package proxy

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// GenerateRandomPSK 生成随机隧道 PSK（32 字节，64 位十六进制），
// 替代原先由 uid+端口推导的可预测密钥。PSK 经 C2 加密通道下发客户端。
func GenerateRandomPSK() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("psk_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// DeriveMagicPacket 从 PSK 派生 64 字节魔法包（会话级随机），
// 替代源码内固定魔法包常量：即使源码泄露，未持有该会话 PSK 也无法完成握手。
func DeriveMagicPacket(psk string) [64]byte {
	var out [64]byte
	h := sha256.New()
	h.Write([]byte("http-tunnel-magic-v1"))
	h.Write([]byte(psk))
	sum := h.Sum(nil)
	copy(out[:], sum)
	copy(out[32:], sum) // 用两段哈希填充 64 字节
	return out
}
