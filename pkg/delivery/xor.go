package delivery

import (
	"crypto/rand"
	"errors"
)

// ===== Stage 载荷 XOR 加密（隐身衣：传输层载荷非明文） =====
// 单字节 XOR 混淆（密钥 1 字节）：轻量、无状态、Stager 内联解密成本极低。
// 目的不是密码学强度（对抗流量检测/随手抓包分析），传输层仍有 AES-GCM
// 会话密钥体系兜底真实加密。

// XorKey 默认 XOR 密钥（可由 GenerateXorKey 生成随机后持久化）
const XorKey byte = 0x5A

// GenerateXorKey 生成随机单字节 XOR 密钥（非零）
func GenerateXorKey() (byte, error) {
	b := make([]byte, 1)
	if _, err := rand.Read(b); err != nil {
		return 0, err
	}
	if b[0] == 0 {
		b[0] = 0x5A
	}
	return b[0], nil
}

// XorEncode 单字节 XOR 加密/解密（对称）
func XorEncode(data []byte, key byte) ([]byte, error) {
	if key == 0 {
		return nil, errors.New("XOR key 不能为 0")
	}
	out := make([]byte, len(data))
	for i, b := range data {
		out[i] = b ^ key
	}
	return out, nil
}
