package encrypt

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// ===== 主密钥运行时派生（ECDH / X25519）—— 服务端侧 =====
//
// 与客户端 implant/core/encrypt/ecdh.go **必须逐字节一致**。两个模块无法互相
// import（不同的 Go module），所以一致性靠**固定黄金向量**在两侧各钉一次：
//
//	priv = RFC 7748 §6.1 Alice 私钥
//	pub  = RFC 7748 §6.1 Bob 公钥
//	salt = "0123456789abcdef"
//	→ 主密钥 = 4fd4f22a231b3432f76bea4dfa55c51268fd5bbffc28029dba6f47a4f974d4b8
//
// 任何一侧改了 info 串、HKDF 参数或 X25519 处理方式，这个向量就会对不上，
// 而不是等到实机上线时表现为"GCM 认证失败"。
//
// 实现要点与客户端一致：拒绝长度错误与**低阶点**（共享密钥全零）。

const (
	// ECDHKeyInfo HKDF 上下文（与客户端同名同值，不得改成与会话密钥 info 相同）
	ECDHKeyInfo = "tshell-ecdh-v1"

	// ECDHPublicKeySize X25519 公钥长度
	ECDHPublicKeySize = 32

	// ECDHSharedKeySize 派生主密钥长度
	ECDHSharedKeySize = 32

	// KxHeaderName 会话前明文交换用的头部名（与客户端一致）
	KxHeaderName = "X-Kx"
)

// ErrBadPeerPublicKey 对端公钥不可用
var ErrBadPeerPublicKey = errors.New("对端公钥不可用")

// ServerKeyExchange 服务端为单个 beacon 保有的一次性临时密钥对。
type ServerKeyExchange struct {
	priv *ecdh.PrivateKey
	pub  []byte
}

// NewServerKeyExchange 生成临时 X25519 密钥对
func NewServerKeyExchange() (*ServerKeyExchange, error) {
	curve := ecdh.X25519()
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("生成临时密钥对失败: %w", err)
	}
	pub := priv.PublicKey().Bytes()
	if len(pub) != ECDHPublicKeySize {
		return nil, fmt.Errorf("公钥长度异常: %d", len(pub))
	}
	return &ServerKeyExchange{priv: priv, pub: append([]byte{}, pub...)}, nil
}

// PublicKey 32 字节原始公钥
func (k *ServerKeyExchange) PublicKey() []byte {
	if k == nil {
		return nil
	}
	return append([]byte{}, k.pub...)
}

// HeaderValue 放进响应头的 base64 形式
func (k *ServerKeyExchange) HeaderValue() string {
	if k == nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(k.pub)
}

// DeriveMasterKey 用客户端公钥与监听器盐派生主密钥
func (k *ServerKeyExchange) DeriveMasterKey(peerPub, salt []byte) ([]byte, error) {
	if k == nil || k.priv == nil {
		return nil, errors.New("临时密钥对未初始化")
	}
	shared, err := RawX25519Shared(k.priv, peerPub)
	if err != nil {
		return nil, err
	}
	defer wipe(shared)
	return DeriveMasterKeyFromShared(shared, salt)
}

// RawX25519Shared 计算原始共享密钥并拒绝全零（低阶点攻击）。
//
// 导出是为了让测试能用 RFC 7748 向量逐字节校验这一层。
func RawX25519Shared(priv *ecdh.PrivateKey, peerPub []byte) ([]byte, error) {
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
	var acc byte
	for _, b := range shared {
		acc |= b
	}
	if acc == 0 {
		wipe(shared)
		return nil, fmt.Errorf("%w: 共享密钥为全零（低阶点攻击）", ErrBadPeerPublicKey)
	}
	return shared, nil
}

// DeriveMasterKeyFromShared HKDF-SHA256(shared, salt, ECDHKeyInfo) → 32 字节
func DeriveMasterKeyFromShared(shared, salt []byte) ([]byte, error) {
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

// ParsePeerPublicKey 解析客户端头部里的 base64 公钥
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

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
