package encrypt

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"
)

// ===== OPSEC：主密钥运行时派生（ECDH / X25519）=====
//
// 这组用例的重点不是"两边算出来一样"（两边一起写错也能通过），而是：
//  1. 用 RFC 7748 §6.1 的官方测试向量逐字节验证 X25519 这一层；
//  2. 低阶点被拒绝（否则共享密钥可被固定成已知值）；
//  3. ReplaceMasterKey 真的把旧派生物作废了（会话密钥、AEAD 缓存）。

// rfc7748 向量（RFC 7748 §6.1）
const (
	rfcAlicePrivHex = "77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a"
	rfcAlicePubHex  = "8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a"
	rfcBobPrivHex   = "5dab087e624a8a4b79e17f8b83800ee66f3bb1292618b6fd1c2f8b27ff88e0eb"
	rfcBobPubHex    = "de9edb7d7b7dc1b4d35b61c2ece435373f8343c85b78674dadfc7e146f882b4f"
	rfcSharedHex    = "4a5d9d5ba4ce2de1728e3bf480350f25e07e21c947d19e3376f09b3c1e161742"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex 解码失败: %v", err)
	}
	return b
}

// X25519 这一层必须与 RFC 向量逐字节一致。
//
// 这条是整个特性正确性的基石：HKDF 套在错误共享密钥上，两端的
// "派生一致"测试依然会通过（错得一模一样），只有外部向量能发现。
func TestRawX25519MatchesRFC7748(t *testing.T) {
	alicePriv, err := ecdh.X25519().NewPrivateKey(mustHex(t, rfcAlicePrivHex))
	if err != nil {
		t.Fatalf("加载 alice 私钥失败: %v", err)
	}
	// 先验证公钥推导与向量一致（确认测试自己没写反）
	if got := hex.EncodeToString(alicePriv.PublicKey().Bytes()); got != rfcAlicePubHex {
		t.Fatalf("alice 公钥与 RFC 不符: %s", got)
	}

	got, err := rawX25519Shared(alicePriv, mustHex(t, rfcBobPubHex))
	if err != nil {
		t.Fatalf("计算共享密钥失败: %v", err)
	}
	if hex.EncodeToString(got) != rfcSharedHex {
		t.Errorf("共享密钥与 RFC 7748 §6.1 不符\n期望 %s\n实际 %s",
			rfcSharedHex, hex.EncodeToString(got))
	}

	// 反方向必须得到同一个共享密钥
	bobPriv, err := ecdh.X25519().NewPrivateKey(mustHex(t, rfcBobPrivHex))
	if err != nil {
		t.Fatal(err)
	}
	rev, err := rawX25519Shared(bobPriv, mustHex(t, rfcAlicePubHex))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rev, got) {
		t.Error("双向共享密钥不一致")
	}
}

// 两端独立生成密钥对，派生出的主密钥必须相同
func TestDeriveMasterKeyBothSidesAgree(t *testing.T) {
	salt := []byte("0123456789abcdef") // 监听器盐（16B）

	a, err := NewKeyExchange()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewKeyExchange()
	if err != nil {
		t.Fatal(err)
	}

	ka, err := a.DeriveMasterKey(b.PublicKey(), salt)
	if err != nil {
		t.Fatalf("A 派生失败: %v", err)
	}
	kb, err := b.DeriveMasterKey(a.PublicKey(), salt)
	if err != nil {
		t.Fatalf("B 派生失败: %v", err)
	}
	if !bytes.Equal(ka, kb) {
		t.Fatal("两端派生出的主密钥不一致")
	}
	if len(ka) != ECDHSharedKeySize {
		t.Errorf("主密钥长度应为 %d，实际 %d", ECDHSharedKeySize, len(ka))
	}
	// 派生值不能等于共享密钥本身（必须经过 HKDF）
	if bytes.Equal(ka, mustHex(t, rfcSharedHex)) {
		t.Error("主密钥等于原始共享密钥，HKDF 没生效")
	}
}

// 换 salt 必须得到不同主密钥（防跨监听器重放）
func TestDeriveMasterKeyDependsOnSalt(t *testing.T) {
	a, _ := NewKeyExchange()
	b, _ := NewKeyExchange()
	k1, err := a.DeriveMasterKey(b.PublicKey(), []byte("salt-aaaaaaaaaa"))
	if err != nil {
		t.Fatal(err)
	}
	k2, err := a.DeriveMasterKey(b.PublicKey(), []byte("salt-bbbbbbbbbb"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(k1, k2) {
		t.Error("不同 salt 应得到不同主密钥")
	}
}

// 每次交换都应是全新的临时密钥（复用会让"一次泄露=全部会话"重新成立）
func TestKeyExchangeIsEphemeral(t *testing.T) {
	a, _ := NewKeyExchange()
	b, _ := NewKeyExchange()
	if bytes.Equal(a.PublicKey(), b.PublicKey()) {
		t.Error("两次生成的临时公钥相同，说明没有真正随机")
	}
}

// 对端公钥非法必须拒绝：长度错、空、base64 坏
func TestRejectBadPeerPublicKey(t *testing.T) {
	a, _ := NewKeyExchange()
	salt := []byte("0123456789abcdef")

	cases := map[string][]byte{
		"空":       {},
		"太短":      make([]byte, 31),
		"太长":      make([]byte, 33),
		"全零(低阶点)": make([]byte, 32),
	}
	for name, pub := range cases {
		if _, err := a.DeriveMasterKey(pub, salt); err == nil {
			t.Errorf("%s 公钥应被拒绝", name)
		} else if !errors.Is(err, ErrBadPeerPublicKey) {
			t.Errorf("%s 的错误应包裹 ErrBadPeerPublicKey，实际: %v", name, err)
		}
	}

	if _, err := ParsePeerPublicKey("not-base64!!"); err == nil {
		t.Error("坏 base64 应被拒绝")
	}
	if _, err := ParsePeerPublicKey(""); err == nil {
		t.Error("空值应被拒绝")
	}
	if got, err := ParsePeerPublicKey(a.HeaderValue()); err != nil || !bytes.Equal(got, a.PublicKey()) {
		t.Errorf("合法公钥应能往返，err=%v", err)
	}
}

// 低阶点必须被拒绝 —— 这是小子群攻击的防线，不能只靠长度校验
func TestRejectLowOrderPoints(t *testing.T) {
	// X25519 的已知低阶点（RFC 7748 §6.1 讨论的一类），全零是其中最典型的一个
	lowOrder := [][]byte{
		make([]byte, 32),                       // 全零
		append([]byte{1}, make([]byte, 31)...), // 1（阶为 1 的点）
	}
	a, _ := NewKeyExchange()
	for i, pub := range lowOrder {
		shared, err := rawX25519Shared(a.priv, pub)
		if err == nil {
			t.Errorf("第 %d 个低阶点应被拒绝，实际算出 shared=%x", i, shared)
		}
	}
}

// ReplaceMasterKey 必须作废旧派生物
func TestReplaceMasterKeyInvalidatesDerivedMaterial(t *testing.T) {
	InitClientKeyManager("test-client")
	ckm := GlobalClientKeyManager
	salt := []byte("0123456789abcdef")
	staticMaster := bytes.Repeat([]byte{0xAA}, 32)

	if err := ckm.SetSessionKey(salt, staticMaster); err != nil {
		t.Fatal(err)
	}
	if err := ckm.SetSessionKeyWithNonce([]byte("0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	oldSession, err := ckm.GetSessionKey()
	if err != nil {
		t.Fatal(err)
	}
	if MasterKeyReplaced() {
		t.Fatal("初始状态不应标记为已替换")
	}

	// 灌一条 AEAD 缓存，验证替换后会被清掉
	if _, err := getAEAD(staticMaster); err != nil {
		t.Fatal(err)
	}
	if aeadCacheLen() == 0 {
		t.Fatal("前置条件：AEAD 缓存应非空")
	}

	newMaster := bytes.Repeat([]byte{0x55}, 32)
	if err := ckm.ReplaceMasterKey(newMaster); err != nil {
		t.Fatalf("替换主密钥失败: %v", err)
	}

	if !MasterKeyReplaced() {
		t.Error("替换后应标记为已替换")
	}
	if got := aeadCacheLen(); got != 0 {
		t.Errorf("AEAD 缓存应被清空，实际剩 %d 条", got)
	}
	// 旧会话密钥必须作废（否则会继续用旧主密钥派生的密钥加密）
	if _, err := ckm.GetSessionKey(); err == nil {
		t.Error("替换后旧会话密钥应作废")
	}

	// 用新主密钥重建后，会话密钥必须与旧的不同
	if err := ckm.SetSessionKeyWithNonce([]byte("0123456789abcdef")); err != nil {
		t.Fatalf("重建会话密钥失败: %v", err)
	}
	newSession, err := ckm.GetSessionKey()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(oldSession, newSession) {
		t.Error("替换主密钥后重建的会话密钥不应与旧的相同")
	}
	if !hmac.Equal(globalMasterKey, newMaster) {
		t.Error("globalMasterKey 应已替换为派生值")
	}
}

// 长度不足的主密钥必须拒绝（不能用短密钥凑合）
func TestReplaceMasterKeyRejectsShortSecret(t *testing.T) {
	InitClientKeyManager("test-client")
	if err := GlobalClientKeyManager.ReplaceMasterKey(make([]byte, 16)); err == nil {
		t.Error("过短的主密钥应被拒绝")
	}
}

// base64 头部值的编码/解码对称性（跨语言互通的前提）
func TestHeaderValueIsStableB64(t *testing.T) {
	a, _ := NewKeyExchange()
	v := a.HeaderValue()
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		t.Fatalf("头部值应是标准 base64: %v", err)
	}
	if !bytes.Equal(raw, a.PublicKey()) {
		t.Error("头部值解码后应等于原始公钥")
	}
	// 长度固定：32 字节 → 44 字符（含一个 '='）
	if len(v) != 44 {
		t.Errorf("32 字节的 base64 应为 44 字符，实际 %d", len(v))
	}
}

// 派生算法自洽性：同一输入必须稳定（重启/多副本一致）
func TestDeriveMasterKeyIsDeterministic(t *testing.T) {
	a, _ := NewKeyExchange()
	b, _ := NewKeyExchange()
	salt := []byte("0123456789abcdef")
	k1, _ := a.DeriveMasterKey(b.PublicKey(), salt)
	k2, _ := a.DeriveMasterKey(b.PublicKey(), salt)
	if !bytes.Equal(k1, k2) {
		t.Error("同一输入的派生结果必须稳定")
	}
}

// 防止有人把 ECDHKeyInfo 改成与会话密钥 info 相同（会让两把密钥相关）
func TestECDHKeyInfoIsDistinct(t *testing.T) {
	if ECDHKeyInfo == "" {
		t.Fatal("ECDHKeyInfo 不能为空")
	}
	want := "tshell-ecdh-v1"
	if ECDHKeyInfo != want {
		t.Errorf("ECDHKeyInfo 变更会影响互通性：期望 %q，实际 %q", want, ECDHKeyInfo)
	}
	// HKDF 输出不应与直接 sha256(shared) 相同（确认确实走了 HKDF）
	shared := mustHex(t, rfcSharedHex)
	out, err := deriveMasterKeyFromShared(shared, nil)
	if err != nil {
		t.Fatal(err)
	}
	naive := sha256.Sum256(shared)
	if bytes.Equal(out, naive[:]) {
		t.Error("派生结果等于裸 sha256(shared)，没有经过 HKDF")
	}
}

// 黄金向量：与服务端 pkg/encrypt/ecdh_test.go 中的同名常量**必须完全相同**。
//
// 跨 module 无法互相 import，所以只能靠这个固定值把两侧钉在一起。
// 任一侧改了 info / HKDF 参数，这条就会失败 —— 而不是等到上线后
// 表现为"GCM 认证失败"（那种症状根本定位不到派生差异）。
func TestGoldenVectorMatchesServer(t *testing.T) {
	const goldenMasterKeyHex = "4fd4f22a231b3432f76bea4dfa55c51268fd5bbffc28029dba6f47a4f974d4b8"
	got, err := deriveMasterKeyFromShared(mustHex(t, rfcSharedHex), []byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != goldenMasterKeyHex {
		t.Errorf("客户端派生值与服务端黄金向量不一致\n期望 %s\n实际 %s",
			goldenMasterKeyHex, hex.EncodeToString(got))
	}
}

// ===== KX 之后建立会话密钥（回归守卫）=====
//
// 下面这条守的是一个真实踩到的坑：SetSessionKey(salt, staticMasterKey) 会用
// **参数里的**静态主密钥覆盖 globalMasterKey。若在 kxHandshake 之后仍调用它，
// 客户端会话密钥变成 HKDF(静态密钥,…)，而服务端是 HKDF(派生密钥,…)：
// 两者不同 → 上线后所有加解密失败，现象只是"连上又掉"。
//
// 这个 bug 是靠人工复查发现、而不是测试 —— 因为端到端跑 Connect 需要 DB/网络
// 全局状态。所以这里至少把"该用哪把密钥"这一层钉住。

func TestSetSessionKeyPreferringKXHonorsDerivedKey(t *testing.T) {
	InitClientKeyManager("test-client-preferring")
	ckm := GlobalClientKeyManager
	salt := []byte("0123456789abcdef")
	staticMK := bytes.Repeat([]byte{0xAA}, 32)

	// 先走静态路径：会话密钥应由静态密钥派生
	if err := ckm.SetSessionKeyPreferringKX(salt, staticMK); err != nil {
		t.Fatalf("静态路径失败: %v", err)
	}
	if MasterKeyReplaced() {
		t.Fatal("未替换时不应标记为已替换")
	}
	staticSession, err := ckm.GetSessionKey()
	if err != nil {
		t.Fatal(err)
	}

	// 替换主密钥（模拟 KX 完成）
	derivedMK := bytes.Repeat([]byte{0x55}, 32)
	if err := ckm.ReplaceMasterKey(derivedMK); err != nil {
		t.Fatal(err)
	}
	if !MasterKeyReplaced() {
		t.Fatal("替换后应标记为已替换")
	}

	// 再次建立会话密钥：必须用**派生**密钥，而不是再次传入的静态值
	if err := ckm.SetSessionKeyPreferringKX(salt, staticMK); err != nil {
		t.Fatalf("KX 路径失败: %v", err)
	}
	kxSession, err := ckm.GetSessionKey()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(staticSession, kxSession) {
		t.Fatal("KX 完成后会话密钥仍由静态密钥派生 —— " +
			"服务端用的是派生密钥，双方会完全对不上（上线即失败）")
	}

	// 且必须与"直接用派生密钥调 SetSessionKey"的结果一致
	InitClientKeyManager("test-client-preferring")
	ref := GlobalClientKeyManager
	if err := ref.ReplaceMasterKey(derivedMK); err != nil {
		t.Fatal(err)
	}
	if err := ref.SetSessionKey(salt, derivedMK); err != nil {
		t.Fatal(err)
	}
	refSession, err := ref.GetSessionKey()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(refSession, kxSession) {
		t.Error("SetSessionKeyPreferringKX 的结果应与直接用派生密钥派生一致")
	}
}

// 未替换时必须与原始 SetSessionKey 完全等价（老载荷路径行为不变）
func TestSetSessionKeyPreferringKXEqualsLegacyWhenNotReplaced(t *testing.T) {
	salt := []byte("0123456789abcdef")
	staticMK := bytes.Repeat([]byte{0x77}, 32)

	InitClientKeyManager("legacy-a")
	if err := GlobalClientKeyManager.SetSessionKey(salt, staticMK); err != nil {
		t.Fatal(err)
	}
	legacy, err := GlobalClientKeyManager.GetSessionKey()
	if err != nil {
		t.Fatal(err)
	}

	InitClientKeyManager("legacy-a")
	if err := GlobalClientKeyManager.SetSessionKeyPreferringKX(salt, staticMK); err != nil {
		t.Fatal(err)
	}
	viaHelper, err := GlobalClientKeyManager.GetSessionKey()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacy, viaHelper) {
		t.Error("未替换主密钥时，PreferringKX 必须与 SetSessionKey 结果完全相同")
	}
}
