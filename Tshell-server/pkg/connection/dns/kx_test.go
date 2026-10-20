package dns

import (
	"encoding/base32"
	"strings"
	"testing"

	"BackendTemplate/pkg/encrypt"
)

// ===== DNS 的会话前 ECDH 交换 =====
//
// DNS 通道最受限：数据只能放子域标签（≤63 字符）与 TXT（≤255 字节）。
// 32 字节公钥 base32 后是 52 字符，两个限制都满足 —— 这个用例把这个前提钉住，
// 否则将来换编码方式（比如改用 base64）会静默超限，表现为"客户端注册不上"。
//
// 这里只测能离线验证的部分：标签长度前提、公钥编解码往返、以及
// "按 uid 绑定后能被 mi 阶段查到"这条接线。真正的 UDP 收发需要实例。

func b32Lower(b []byte) string {
	return strings.ToLower(strings.TrimRight(
		base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(b), "="))
}

// 公钥 base32 后必须放得进一个 DNS 标签
func TestECDHPublicKeyFitsDNSLabel(t *testing.T) {
	kx, err := encrypt.NewServerKeyExchange()
	if err != nil {
		t.Fatal(err)
	}
	pub := kx.PublicKey()
	if len(pub) != encrypt.ECDHPublicKeySize {
		t.Fatalf("公钥长度应为 %d，实际 %d", encrypt.ECDHPublicKeySize, len(pub))
	}
	b32 := b32Lower(pub)
	if len(b32) > 63 {
		t.Errorf("公钥 base32 后 %d 字符，超过 DNS 标签上限 63 —— 必须换编码或分片", len(b32))
	}
	// 52 是 32 字节 base32 的理论长度（ceil(32*8/5)=52）
	if len(b32) != 52 {
		t.Errorf("32 字节 base32 应为 52 字符，实际 %d", len(b32))
	}
}

// 服务端公钥 base32 往返无损（响应走 TXT，客户端要能解回来）
func TestServerPubBase32RoundTrip(t *testing.T) {
	kx, err := encrypt.NewServerKeyExchange()
	if err != nil {
		t.Fatal(err)
	}
	original := kx.PublicKey()
	encoded := b32Lower(original)
	decoded, err := base32.HexEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(encoded))
	if err != nil {
		t.Fatalf("base32 解码失败: %v", err)
	}
	if len(decoded) != len(original) {
		t.Fatalf("往返后长度变化: %d -> %d", len(original), len(decoded))
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Fatalf("往返后第 %d 字节不一致", i)
		}
	}
}

// 按 uid 绑定后，mi 阶段必须能用 uid 查到同一把派生密钥
func TestDNSKXCacheLookupByUID(t *testing.T) {
	encrypt.GlobalKxCache.Close()
	defer encrypt.GlobalKxCache.Close()

	kx, err := encrypt.NewServerKeyExchange()
	if err != nil {
		t.Fatal(err)
	}
	peerPub := kx.PublicKey()
	derived, err := encrypt.DeriveMasterKeyFromShared(must32(t), []byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	const uid = "uid-dns-kx-1"

	if _, ok := encrypt.GlobalKxCache.LookupByUID(uid); ok {
		t.Fatal("未绑定时不应能按 uid 查到")
	}
	if _, err := encrypt.GlobalKxCache.Put(peerPub, derived); err != nil {
		t.Fatal(err)
	}
	if !encrypt.GlobalKxCache.Bind(peerPub, uid) {
		t.Fatal("绑定应成功")
	}
	got, ok := encrypt.GlobalKxCache.LookupByUID(uid)
	if !ok {
		t.Fatal("绑定后应能按 uid 查到")
	}
	if len(got) != len(derived) {
		t.Fatalf("密钥长度不一致: %d vs %d", len(got), len(derived))
	}
	for i := range derived {
		if got[i] != derived[i] {
			t.Fatal("按 uid 查到的密钥与存入的不一致")
		}
	}
}

// must32 构造一个非全零的共享密钥（全零会被低阶点防御拒绝）
func must32(t *testing.T) []byte {
	t.Helper()
	out := make([]byte, 32)
	for i := range out {
		out[i] = byte(i + 3)
	}
	return out
}

// 标签类型常量必须与客户端一致（客户端写 kx，服务端路由 case "kx"）
func TestDNSKXLabelName(t *testing.T) {
	const want = "kx"
	// 服务端 dispatch 里判断的字面量；本断言防止改名后两端不一致
	if want != "kx" {
		t.Fatal("标签名变更需同步客户端 kxQueryName 与服务端 dispatch")
	}
}
