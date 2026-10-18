package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/hkdf"
)

// MFA/TOTP 密钥的静态保护。
//
// 威胁模型：数据库文件（或备份、或一份 SQL dump）泄露。若 TOTP 密钥明文落库，
// 拿到库的人可以自己算出动态码 —— 第二因子当场失效。所以密钥在库里必须是密文。
//
// 为什么用 JWT 私钥派生而不是另建一份密钥：
//   - 私钥已在 ./jwt_private.pem（0600），已有备份/轮换/运维约定；
//   - 新造一份密钥意味着又多一个"丢了就全员无法登录"的东西要备份；
//   - HKDF 派生是确定性的，重启/多副本都能算出同一把密钥。
//
// 反过来说清楚代价：**JWT 私钥一旦泄露，TOTP 密钥也随之可解**。
// 这不是额外弱点（私钥本来就等于"能伪造任意管理员令牌"），但要写明白，
// 免得有人以为 MFA 密钥有独立于私钥的保护。
//
// 另外：私钥轮换（rotateKeys 只换内存里的当前/上一把，不改写 pem 文件）不影响本方案；
// 若将来真的重写 pem，旧密文会解不开 —— 那时表现为"登录时报 MFA 密钥无法解密"，
// 走管理员 reset 即可，**绝不静默降级为单因子**。
const (
	mfaKeyInfo     = "tshell-mfa-secret-v1"
	mfaKeyLen      = 32 // AES-256
	mfaCipherLabel = "v1:"
)

// ErrMFANotEnabled 该用户没有开启 MFA。
// 与"密文解不开"是两件事：前者是正常状态（单因子登录），后者必须报错并让人去 reset，
// 混为一谈会导致"解不开就当没开启"，即静默关闭 MFA。
var ErrMFANotEnabled = errors.New("该用户未开启 MFA")

var (
	mfaCipherMu sync.RWMutex
	mfaAEAD     cipher.AEAD
)

// InitMFASecretCipher 由 JWT 私钥派生 MFA 密钥加密器。必须在 InitJWTManager 之后调用。
func InitMFASecretCipher(privateKey *rsa.PrivateKey) error {
	if privateKey == nil {
		return fmt.Errorf("MFA 密钥加密器初始化失败：私钥为空")
	}
	// 用 PKCS#8 编码作为 HKDF 的输入密钥材料（比直接喂 DER 更明确，且带算法标识）
	raw, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("序列化私钥失败: %w", err)
	}
	reader := hkdf.New(sha256.New, raw, nil, []byte(mfaKeyInfo))
	key := make([]byte, mfaKeyLen)
	if _, err := io.ReadFull(reader, key); err != nil {
		return fmt.Errorf("派生 MFA 密钥失败: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("创建 AES 失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("创建 GCM 失败: %w", err)
	}

	mfaCipherMu.Lock()
	mfaAEAD = aead
	mfaCipherMu.Unlock()
	return nil
}

// MFACipherReady 加密器是否可用（未初始化时不允许写 MFA 密钥）
func MFACipherReady() bool {
	mfaCipherMu.RLock()
	defer mfaCipherMu.RUnlock()
	return mfaAEAD != nil
}

// SealMFASecret 加密 TOTP 密钥，返回可直接入库的字符串。
//
// 未初始化时**返回错误而不是明文**：静默降级成明文入库，就等于把 MFA 的保护
// 悄悄削掉一半，而且没人会发现。
func SealMFASecret(secret []byte) (string, error) {
	mfaCipherMu.RLock()
	aead := mfaAEAD
	mfaCipherMu.RUnlock()
	if aead == nil {
		return "", fmt.Errorf("MFA 密钥加密器未初始化，拒绝写入（不允许明文落库）")
	}
	if len(secret) == 0 {
		return "", fmt.Errorf("TOTP 密钥为空")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("生成随机数失败: %w", err)
	}
	// 密文 = 前缀 ‖ nonce ‖ ciphertext+tag：前缀用于将来换算法时区分格式
	sealed := aead.Seal(nil, nonce, secret, []byte(mfaKeyInfo))
	out := append(append([]byte{}, nonce...), sealed...)
	return mfaCipherLabel + base64.StdEncoding.EncodeToString(out), nil
}

// OpenMFASecret 解密 TOTP 密钥。
//
// 失败原因要能区分（"没开启" vs "解不开"）：调用方据此决定是"正常单因子登录"
// 还是"报错并让人走管理员 reset"。混在一起会导致"解不开就当没开启"这种
// 静默关闭 MFA 的行为。
func OpenMFASecret(enc string) ([]byte, error) {
	if enc == "" {
		return nil, ErrMFANotEnabled
	}
	if !hasPrefix(enc, mfaCipherLabel) {
		return nil, fmt.Errorf("MFA 密文格式无法识别（缺少 %q 前缀）", mfaCipherLabel)
	}
	raw, err := base64.StdEncoding.DecodeString(enc[len(mfaCipherLabel):])
	if err != nil {
		return nil, fmt.Errorf("MFA 密文不是合法 base64: %w", err)
	}

	mfaCipherMu.RLock()
	aead := mfaAEAD
	mfaCipherMu.RUnlock()
	if aead == nil {
		return nil, fmt.Errorf("MFA 密钥加密器未初始化，无法解密")
	}
	if len(raw) < aead.NonceSize() {
		return nil, fmt.Errorf("MFA 密文过短（%d 字节）", len(raw))
	}
	nonce, ct := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ct, []byte(mfaKeyInfo))
	if err != nil {
		return nil, fmt.Errorf("MFA 密钥解密失败（JWT 私钥是否变更过？）: %w", err)
	}
	return plain, nil
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
