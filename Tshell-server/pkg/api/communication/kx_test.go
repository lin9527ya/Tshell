package communication

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"BackendTemplate/pkg/encrypt"
)

// ===== 会话前 ECDH 交换（HTTP 家族）=====
//
// 这段是全链路的正确性枢纽：`metaMK` 一旦算错，MetaInfo 解不开、
// 会话密钥也对不上，表现为"beacon 上线失败"但看不出原因。
// 所以这里用真实的客户端 crypto 实现（同一 module 内可见）走完整往返，
// 而不是只断言"函数被调用了"。

// kxClientSide 模拟客户端。
//
// 用 ServerKeyExchange 充当客户端是**刻意的**：X25519 + HKDF 的数学
// 对两端完全对称，客户端那侧的 `KeyExchange` 只在 implant module 里、
// 本 module 看不到。服务端实现已在别处用 RFC 7748 向量逐字节验证过，
// 这里复用它来跑完整往返，验证的是**接线**（派生 → 缓存 → 响应头 → 客户端复算）。
func kxClientSide(t *testing.T) (*encrypt.ServerKeyExchange, string) {
	t.Helper()
	kx, err := encrypt.NewServerKeyExchange()
	if err != nil {
		t.Fatalf("生成临时密钥失败: %v", err)
	}
	return kx, kx.HeaderValue()
}

func kxRequest(pubB64 string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	if pubB64 != "" {
		r.Header.Set(encrypt.KxHeaderName, pubB64)
	}
	return r
}

// 无 X-Kx 头：必须走老路径（used=false），且不写响应头
func TestResolveRequestKeyLegacyPath(t *testing.T) {
	w := httptest.NewRecorder()
	r := kxRequest("")
	salt := []byte("0123456789abcdef")

	mk, used, err := ResolveRequestKey(w, r, salt)
	if err != nil {
		t.Fatalf("无头时不应报错: %v", err)
	}
	if used || mk != nil {
		t.Error("无 X-Kx 头时应返回 used=false，让调用方走静态密钥")
	}
	if got := w.Header().Get(encrypt.KxHeaderName); got != "" {
		t.Error("老路径不应写响应头")
	}
}

// 完整往返：服务端派生的密钥必须与客户端派生的**完全相同**
func TestResolveRequestKeyRoundTrip(t *testing.T) {
	salt := []byte("0123456789abcdef")
	cli, cliPub := kxClientSide(t)

	w := httptest.NewRecorder()
	serverMK, used, err := ResolveRequestKey(w, kxRequest(cliPub), salt)
	if err != nil {
		t.Fatalf("交换失败: %v", err)
	}
	if !used {
		t.Fatal("带 X-Kx 头时应返回 used=true")
	}
	if len(serverMK) != encrypt.ECDHSharedKeySize {
		t.Fatalf("主密钥长度异常: %d", len(serverMK))
	}

	// 客户端从响应头取服务端公钥，自行派生
	serverPubB64 := w.Header().Get(encrypt.KxHeaderName)
	if serverPubB64 == "" {
		t.Fatal("响应头必须带回服务端公钥，否则客户端无法派生")
	}
	serverPub, err := encrypt.ParsePeerPublicKey(serverPubB64)
	if err != nil {
		t.Fatalf("服务端公钥不可解析: %v", err)
	}
	cliMK, err := cli.DeriveMasterKey(serverPub, salt)
	if err != nil {
		t.Fatalf("客户端派生失败: %v", err)
	}

	if len(cliMK) != len(serverMK) {
		t.Fatal("两侧主密钥长度不同")
	}
	for i := range cliMK {
		if cliMK[i] != serverMK[i] {
			t.Fatalf("**两侧派生出的主密钥不一致**（第 %d 字节）——"+
				"这会导致 MetaInfo 解不开、beacon 上线失败", i)
		}
	}
}

// 同一客户端公钥重复请求：命中缓存，密钥与公钥都应保持稳定
func TestResolveRequestKeyCacheHitIsStable(t *testing.T) {
	salt := []byte("0123456789abcdef")
	_, cliPub := kxClientSide(t)

	w1 := httptest.NewRecorder()
	mk1, _, err := ResolveRequestKey(w1, kxRequest(cliPub), salt)
	if err != nil {
		t.Fatal(err)
	}
	pub1 := w1.Header().Get(encrypt.KxHeaderName)

	w2 := httptest.NewRecorder()
	mk2, _, err := ResolveRequestKey(w2, kxRequest(cliPub), salt)
	if err != nil {
		t.Fatal(err)
	}
	pub2 := w2.Header().Get(encrypt.KxHeaderName)

	if string(mk1) != string(mk2) {
		t.Error("同一公钥重复请求必须命中缓存并得到同一把主密钥")
	}
	// 缓存命中时也应重发服务端公钥：客户端可能在两次请求之间丢了状态，
	// 或者服务端条目过期后重建过密钥对（此时客户端需要重新派生以自愈）
	if pub2 == "" {
		t.Error("缓存命中时仍应返回服务端公钥，否则客户端无法自愈重派生")
	}
	if pub1 != pub2 {
		t.Error("同一会话的服务端公钥应保持稳定")
	}
}

// 不同客户端必须得到不同主密钥（每会话唯一）
func TestResolveRequestKeyPerClientUnique(t *testing.T) {
	salt := []byte("0123456789abcdef")
	_, pubA := kxClientSide(t)
	_, pubB := kxClientSide(t)

	wA := httptest.NewRecorder()
	mkA, _, err := ResolveRequestKey(wA, kxRequest(pubA), salt)
	if err != nil {
		t.Fatal(err)
	}
	wB := httptest.NewRecorder()
	mkB, _, err := ResolveRequestKey(wB, kxRequest(pubB), salt)
	if err != nil {
		t.Fatal(err)
	}
	if string(mkA) == string(mkB) {
		t.Fatal("两个客户端不应得到同一把主密钥")
	}
}

// 坏公钥必须被拒绝 —— 关键：**不能**静默退回静态密钥，
// 否则攻击者发个坏公钥就能把连接降级。
func TestResolveRequestKeyRejectsBadKeyWithoutFallback(t *testing.T) {
	salt := []byte("0123456789abcdef")

	cases := map[string]string{
		"坏 base64": "@@@@",
		"长度不足":     base64.StdEncoding.EncodeToString(make([]byte, 16)),
		"低阶点(全零)":  base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}
	for name, val := range cases {
		w := httptest.NewRecorder()
		mk, used, err := ResolveRequestKey(w, kxRequest(val), salt)
		if err == nil {
			t.Errorf("%s 应报错", name)
		}
		if !used {
			t.Errorf("%s 应返回 used=true（声明了 ECDH 就不能悄悄降级）", name)
		}
		if mk != nil {
			t.Errorf("%s 失败时不应返回主密钥", name)
		}
	}
}

// 换 salt 必须得到不同主密钥（防跨监听器重放）
func TestResolveRequestKeyDependsOnListenerSalt(t *testing.T) {
	_, cliPub := kxClientSide(t)
	w1 := httptest.NewRecorder()
	mk1, _, err := ResolveRequestKey(w1, kxRequest(cliPub), []byte("salt-aaaaaaaaaa"))
	if err != nil {
		t.Fatal(err)
	}
	// 用另一个公钥避免命中同一缓存条目
	_, cliPub2 := kxClientSide(t)
	w2 := httptest.NewRecorder()
	mk2, _, err := ResolveRequestKey(w2, kxRequest(cliPub2), []byte("salt-bbbbbbbbbb"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mk1) == string(mk2) {
		t.Error("不同监听器盐应得到不同主密钥")
	}
}

// uid 绑定：解出 uid 后可按 uid 反查
func TestBindExchangeUID(t *testing.T) {
	salt := []byte("0123456789abcdef")
	_, cliPub := kxClientSide(t)
	r := kxRequest(cliPub)
	w := httptest.NewRecorder()
	if _, _, err := ResolveRequestKey(w, r, salt); err != nil {
		t.Fatal(err)
	}

	if _, ok := KxCache.LookupByUID("uid-kx-1"); ok {
		t.Fatal("未绑定时不应能按 uid 查到")
	}
	BindExchangeUID(r, "uid-kx-1")
	if _, ok := KxCache.LookupByUID("uid-kx-1"); !ok {
		t.Error("绑定后应能按 uid 查到")
	}
	// 空 uid 与无头请求都不应 panic
	BindExchangeUID(r, "")
	BindExchangeUID(kxRequest(""), "uid-x")
}

// 握手缓存不应无限增长（上限由 KxCache 保证，这里只确认接线用的是同实例）
func TestKxCacheIsSharedInstance(t *testing.T) {
	if KxCache == nil {
		t.Fatal("KxCache 必须初始化，否则 ResolveRequestKey 会 panic")
	}
	before := KxCache.Len()
	_, cliPub := kxClientSide(t)
	w := httptest.NewRecorder()
	if _, _, err := ResolveRequestKey(w, kxRequest(cliPub), []byte("0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	if KxCache.Len() <= before && before == 0 {
		t.Error("交换成功后缓存条目应增加")
	}
}
