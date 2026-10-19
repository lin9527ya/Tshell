package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/hkdf"
)

// ===== 战果（凭据/口令/哈希）的静态加密（OPSEC P0-1）=====
//
// 威胁模型：数据库文件、备份文件或一份 SQL dump 泄露。`Loot.Secret` 里是**明文口令、
// NTLM 哈希、票据密钥** —— 拿到库就等于拿到全部战果，而且不需要再回到目标机器。
//
// 这里的不对称最刺眼：MFA 的 TOTP 密钥早就是 AES-GCM 加密的（mfa.go），
// 而**价值更高的战果**一直是明文。本文件补齐这一块。
//
// 密钥来源（按优先级）：
//  1. `security.at_rest_key_file` 指向的文件（32 字节原始密钥或 hex/base64 文本，0600）；
//  2. 由 JWT 私钥 HKDF 派生（与 MFA 同一手法，但用**不同的 info**，
//     保证两个用途的密钥互不相关）。
//
// 与 MFA 一致的代价说明：**JWT 私钥泄露 ⇒ 战果也可解**。这不是新增弱点
// （私钥本来就等于"能伪造任意管理员令牌"），但必须写明，别让人以为战果有独立于私钥的保护。
//
// 另加一条派生密钥用于**去重指纹**：见 Fingerprint 的说明。

const (
	atRestKeyInfo   = "tshell-atrest-v1"
	atRestFPInfo    = "tshell-atrest-fp-v1"
	atRestKeyLen    = 32 // AES-256
	atRestCipherTag = "v1:"
)

var (
	atRestMu  sync.RWMutex
	atRestAE  cipher.AEAD
	atRestFPK []byte
)

// ErrAtRestNotInitialized 静态加密未初始化。
//
// 刻意让它成为**硬错误**而不是"退化为明文"：静默降级正是本项目反复踩过的失败模式
// （sleep mask 保护副本、kirbi 被误判需密钥）。宁可在写入时失败并在日志里喊出来，
// 也不要让战果悄悄变回明文。
var ErrAtRestNotInitialized = errors.New("战果静态加密未初始化（缺少密钥）")

// InitAtRestCipherFromKeyFile 用独立密钥文件初始化（推荐：密钥与库分离存放）。
//
// 文件内容两种形态都接受：32 字节原始密钥，或 hex/base64 编码的文本（便于运维生成）。
func InitAtRestCipherFromKeyFile(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取静态加密密钥文件失败: %w", err)
	}
	key, err := decodeAtRestKey(raw)
	if err != nil {
		return err
	}
	return initAtRestCipher(key)
}

// InitAtRestCipherFromPrivateKey 由 JWT 私钥派生（未配置独立密钥文件时的默认路径）。
func InitAtRestCipherFromPrivateKey(privateKey *rsa.PrivateKey) error {
	if privateKey == nil {
		return fmt.Errorf("战果静态加密初始化失败：私钥为空")
	}
	raw, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("序列化私钥失败: %w", err)
	}
	key := make([]byte, atRestKeyLen)
	if _, err := io.ReadFull(hkdf.New(sha256.New, raw, nil, []byte(atRestKeyInfo)), key); err != nil {
		return fmt.Errorf("派生战果加密密钥失败: %w", err)
	}
	fpKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, raw, nil, []byte(atRestFPInfo)), fpKey); err != nil {
		return fmt.Errorf("派生去重指纹密钥失败: %w", err)
	}
	return initAtRestCipherWithFP(key, fpKey)
}

func initAtRestCipher(key []byte) error {
	// 独立密钥文件场景下，指纹密钥用不同 info 从同一密钥派生，
	// 保证"加密密钥"与"指纹密钥"不是同一把
	fpKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, key, nil, []byte(atRestFPInfo)), fpKey); err != nil {
		return fmt.Errorf("派生去重指纹密钥失败: %w", err)
	}
	return initAtRestCipherWithFP(key, fpKey)
}

func initAtRestCipherWithFP(key, fpKey []byte) error {
	if len(key) != atRestKeyLen {
		return fmt.Errorf("静态加密密钥长度应为 %d 字节，实际 %d", atRestKeyLen, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("创建 AES 失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("创建 GCM 失败: %w", err)
	}
	atRestMu.Lock()
	atRestAE = aead
	atRestFPK = fpKey
	atRestMu.Unlock()
	return nil
}

func decodeAtRestKey(raw []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(raw))
	// 32 字节原始二进制（不做任何解码）
	if len(raw) == atRestKeyLen {
		return raw, nil
	}
	if b, err := hex.DecodeString(trimmed); err == nil && len(b) == atRestKeyLen {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(b) == atRestKeyLen {
		return b, nil
	}
	return nil, fmt.Errorf("静态加密密钥文件格式不对：需要 %d 字节原始密钥，或等长的 hex/base64 文本", atRestKeyLen)
}

// Seal 加密一条战果明文，返回带 "v1:" 前缀的编码串（非 secret 内容不加密）。
func Seal(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	atRestMu.RLock()
	aead, fpKey := atRestAE, atRestFPK
	atRestMu.RUnlock()
	if aead == nil || fpKey == nil {
		return "", ErrAtRestNotInitialized
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成 nonce 失败: %w", err)
	}
	ct := aead.Seal(nil, nonce, []byte(plaintext), nil)
	return atRestCipherTag + base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// Open 解密 Seal 的产物。
//
// 对**没有前缀**的值按"历史明文"原样返回：迁移前写入的行必须仍可读，
// 否则升级后老库直接不可用。迁移会把它们逐个加密回写（见 pkg/database）。
func Open(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, atRestCipherTag) {
		return stored, nil // 历史明文
	}
	atRestMu.RLock()
	aead := atRestAE
	atRestMu.RUnlock()
	if aead == nil {
		return "", ErrAtRestNotInitialized
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, atRestCipherTag))
	if err != nil {
		return "", fmt.Errorf("战果密文 base64 解码失败: %w", err)
	}
	ns := aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("战果密文长度不足")
	}
	plain, err := aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		// 密钥换过或数据被篡改。明确报错，不要返回空串让上层以为"这条是空的"
		return "", fmt.Errorf("战果密文解密失败（密钥不匹配或数据被篡改）: %w", err)
	}
	return string(plain), nil
}

// IsSealed 判断某个值是否已是密文形态（迁移用）
func IsSealed(stored string) bool {
	return strings.HasPrefix(stored, atRestCipherTag)
}

// Fingerprint 计算去重指纹。
//
// 为什么需要它：`LootStore.AddAutoIngest/AddIfNotEmpty` 原先用 `secret = ?` 做去重，
// 而 AES-GCM 是**随机化**的 —— 同一口令每次加密结果不同，直接加密会让去重彻底失效
// （同一台机器重复收集会刷屏）。所以改存一个确定性的 keyed 指纹。
//
// 用 HMAC 而不是裸 SHA256：裸哈希会让持有数据库的人对常见口令做字典反查，
// 等于把加密白做了。用派生密钥做 HMAC 后，没有密钥就无法离线比对。
//
// 代价（要写明）：指纹仍是确定性的 ⇒ 能看出"两条记录的口令相同"。
// 这是去重功能本身的必然代价；若将来不需要去重，应直接去掉指纹列。
func Fingerprint(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	atRestMu.RLock()
	k := atRestFPK
	atRestMu.RUnlock()
	if len(k) == 0 {
		return "", ErrAtRestNotInitialized
	}
	m := hmac.New(sha256.New, k)
	m.Write([]byte(plaintext))
	return hex.EncodeToString(m.Sum(nil)), nil
}

// AtRestReady 静态加密是否已可用（供启动自检与测试断言）
func AtRestReady() bool {
	atRestMu.RLock()
	defer atRestMu.RUnlock()
	return atRestAE != nil
}
