package encrypt

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"
)

// ===== 主密钥运行时派生（服务端侧）=====
//
// 客户端在 implant/core/encrypt/ecdh.go。两个模块无法互相 import，
// 所以互通性靠**固定黄金向量**在两侧各钉一次 —— 这是唯一能跨 module
// 保证"两端算法真的一样"的办法。

// 黄金向量：两侧测试都断言同一个值
//
//	priv  = RFC 7748 §6.1 Alice 私钥
//	peerPub = RFC 7748 §6.1 Bob 公钥
//	salt  = "0123456789abcdef"
//
// 任一侧改了 info 串 / HKDF 参数 / X25519 处理，这个向量立刻对不上，
// 而不是等到上线时表现为"GCM 认证失败"（那种症状几乎无法定位到派生差异）。
const (
	goldenMasterKeyHex = "4fd4f22a231b3432f76bea4dfa55c51268fd5bbffc28029dba6f47a4f974d4b8"
	goldenSalt         = "0123456789abcdef"

	rfcAlicePrivHex = "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a"
	rfcAlicePubHex  = "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a"
	rfcBobPrivHex   = "5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb"
	rfcBobPubHex    = "de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f"
	rfcSharedHex    = "4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742"
)

func mustHexSrv(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex 解码失败: %v", err)
	}
	return b
}

// X25519 层与 RFC 7748 逐字节一致
func TestServerRawX25519MatchesRFC7748(t *testing.T) {
	priv, err := ecdh.X25519().NewPrivateKey(mustHexSrv(t, rfcBobPrivHex))
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(priv.PublicKey().Bytes()); got != rfcBobPubHex {
		t.Fatalf("bob 公钥与 RFC 不符: %s", got)
	}
	shared, err := RawX25519Shared(priv, mustHexSrv(t, rfcAlicePubHex))
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(shared); got != rfcSharedHex {
		t.Errorf("共享密钥与 RFC 7748 §6.1 不符\n期望 %s\n实际 %s", rfcSharedHex, got)
	}
}

// **互通性核心断言**：固定输入必须得到与客户端完全相同的派生值
func TestGoldenVectorMatchesClient(t *testing.T) {
	got, err := DeriveMasterKeyFromShared(mustHexSrv(t, rfcSharedHex), []byte(goldenSalt))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != goldenMasterKeyHex {
		t.Errorf("派生主密钥与黄金向量不符（客户端会算不出同一把密钥）\n期望 %s\n实际 %s",
			goldenMasterKeyHex, hex.EncodeToString(got))
	}
}

// 服务端生成密钥对、用客户端公钥派生，结果必须与客户端视角一致
func TestServerKeyExchangeDerivesSameAsPeer(t *testing.T) {
	srv, err := NewServerKeyExchange()
	if err != nil {
		t.Fatal(err)
	}
	cli, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	salt := []byte("0123456789abcdef")

	fromSrv, err := srv.DeriveMasterKey(cli.PublicKey().Bytes(), salt)
	if err != nil {
		t.Fatalf("服务端派生失败: %v", err)
	}
	// 客户端视角：用同一 salt 从另一端派生
	fromCli, err := RawX25519Shared(cli, srv.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	fromCliDerived, err := DeriveMasterKeyFromShared(fromCli, salt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fromSrv, fromCliDerived) {
		t.Fatal("服务端与对端派生出的主密钥不一致")
	}
}

// 低阶点与长度错误必须拒绝
func TestServerRejectsBadPeerPublicKey(t *testing.T) {
	srv, _ := NewServerKeyExchange()
	salt := []byte("0123456789abcdef")

	for name, pub := range map[string][]byte{
		"空":       {},
		"太短":      make([]byte, 31),
		"太长":      make([]byte, 33),
		"全零(低阶点)": make([]byte, 32),
		"1(低阶点)":  append([]byte{1}, make([]byte, 31)...),
	} {
		if _, err := srv.DeriveMasterKey(pub, salt); err == nil {
			t.Errorf("%s 应被拒绝", name)
		} else if !errors.Is(err, ErrBadPeerPublicKey) {
			t.Errorf("%s 的错误应包裹 ErrBadPeerPublicKey，实际: %v", name, err)
		}
	}

	if _, err := ParsePeerPublicKey("@@@"); err == nil {
		t.Error("坏 base64 应被拒绝")
	}
	if _, err := ParsePeerPublicKey(""); err == nil {
		t.Error("空值应被拒绝")
	}
}

// 头部值格式与客户端一致（44 字符 base64）
func TestServerHeaderValueFormat(t *testing.T) {
	srv, _ := NewServerKeyExchange()
	v := srv.HeaderValue()
	if len(v) != 44 {
		t.Errorf("32 字节 base64 应为 44 字符，实际 %d", len(v))
	}
	raw, err := ParsePeerPublicKey(v)
	if err != nil {
		t.Fatalf("自己产出的头部值应能被解析: %v", err)
	}
	if !bytes.Equal(raw, srv.PublicKey()) {
		t.Error("往返不一致")
	}
}

// 常量与客户端同值（改了会静默失去互通）
func TestServerECDHConstants(t *testing.T) {
	if ECDHKeyInfo != "tshell-ecdh-v1" {
		t.Errorf("ECDHKeyInfo 必须与客户端一致，实际 %q", ECDHKeyInfo)
	}
	if KxHeaderName != "X-Kx" {
		t.Errorf("KxHeaderName 必须与客户端一致，实际 %q", KxHeaderName)
	}
	if ECDHPublicKeySize != 32 || ECDHSharedKeySize != 32 {
		t.Error("密钥长度常量应为 32")
	}
}
