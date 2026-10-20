package websocket

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"BackendTemplate/pkg/encrypt"
)

// ===== WebSocket 的会话前 ECDH 交换 =====
//
// WS 是 HTTP 升级，所以公钥可以搭在升级请求/响应头上（与 http/https 同一套思路）。
// 但时序上与 http 家族有一处关键差别：WS 客户端的 MetaInfo 是**升级后的第一帧**，
// 不是升级请求头 —— 所以"升级请求带公钥 / 升级响应回公钥"这一个来回就够了，
// 不会出现"MetaInfo 已经发出去但服务端还没派生密钥"的窗口。
//
// 这些用例覆盖的是密钥解析这一段（resolveWSKX），也就是最容易算错的地方。

func TestResolveWSKXLegacyPath(t *testing.T) {
	r := httptest.NewRequest("GET", "/ws", nil) // 无 X-Kx 头
	mk, hdr, err := resolveWSKX(r, []byte("0123456789abcdef"))
	if err != nil {
		t.Fatalf("无头时不应报错: %v", err)
	}
	if mk != nil || hdr != "" {
		t.Error("无 X-Kx 头时应返回空值，让调用方走静态密钥")
	}
}

func TestResolveWSKXRoundTrip(t *testing.T) {
	salt := []byte("0123456789abcdef")
	cli, err := encrypt.NewServerKeyExchange() // 对称实现充当客户端
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/ws", nil)
	r.Header.Set(encrypt.KxHeaderName, cli.HeaderValue())

	mk, hdr, err := resolveWSKX(r, salt)
	if err != nil {
		t.Fatalf("交换失败: %v", err)
	}
	if len(mk) != encrypt.ECDHSharedKeySize {
		t.Fatalf("主密钥长度异常: %d", len(mk))
	}
	if hdr == "" {
		t.Fatal("必须回服务端公钥，否则客户端无法派生")
	}

	// 客户端视角复算，必须与服务端一致
	serverPub, err := encrypt.ParsePeerPublicKey(hdr)
	if err != nil {
		t.Fatalf("服务端公钥不可解析: %v", err)
	}
	cliMK, err := cli.DeriveMasterKey(serverPub, salt)
	if err != nil {
		t.Fatal(err)
	}
	for i := range cliMK {
		if cliMK[i] != mk[i] {
			t.Fatalf("**两侧主密钥不一致**（第 %d 字节）—— MetaInfo 会解不开", i)
		}
	}
}

// 坏公钥必须报错（调用方据此 400 拒绝），不能降级
func TestResolveWSKXRejectsBadKey(t *testing.T) {
	salt := []byte("0123456789abcdef")
	cases := map[string]string{
		"坏 base64": "@@@",
		"长度不足":     base64.StdEncoding.EncodeToString(make([]byte, 16)),
		"低阶点(全零)":  base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}
	for name, v := range cases {
		r := httptest.NewRequest("GET", "/ws", nil)
		r.Header.Set(encrypt.KxHeaderName, v)
		mk, hdr, err := resolveWSKX(r, salt)
		if err == nil {
			t.Errorf("%s 应报错", name)
		}
		if mk != nil || hdr != "" {
			t.Errorf("%s 失败时不应返回密钥或响应头", name)
		}
	}
}

// 不同监听器盐应得到不同主密钥
func TestResolveWSKXDependsOnSalt(t *testing.T) {
	a, _ := encrypt.NewServerKeyExchange()
	b, _ := encrypt.NewServerKeyExchange()

	r1 := httptest.NewRequest("GET", "/ws", nil)
	r1.Header.Set(encrypt.KxHeaderName, a.HeaderValue())
	mk1, _, err := resolveWSKX(r1, []byte("salt-aaaaaaaaaa"))
	if err != nil {
		t.Fatal(err)
	}

	r2 := httptest.NewRequest("GET", "/ws", nil)
	r2.Header.Set(encrypt.KxHeaderName, b.HeaderValue())
	mk2, _, err := resolveWSKX(r2, []byte("salt-bbbbbbbbbb"))
	if err != nil {
		t.Fatal(err)
	}
	if string(mk1) == string(mk2) {
		t.Error("不同盐应得到不同主密钥")
	}
}

// ===== 密钥优先级（含 forward 派生密钥）=====
//
// 这几条守的是"派生密钥必须压过静态密钥"。tcp 那边曾经因为同类逻辑写错
// （覆盖主密钥时盐没跟着覆盖）导致两端算出不同密钥，现场只表现为"解密失败"。

func TestResolveWSMetaKeyPrecedence(t *testing.T) {
	staticMK := make([]byte, 32)
	for i := range staticMK {
		staticMK[i] = 0xAA
	}
	derived := make([]byte, encrypt.ECDHSharedKeySize)
	for i := range derived {
		derived[i] = 0x33
	}

	// 1) 无派生密钥：显式 vkey 生效
	got := resolveWSMetaKey(staticMK, []byte("0123456789abcdef"), nil)
	if string(got) != string(staticMK) {
		t.Error("无 KX 时应使用显式 vkey")
	}

	// 2) 有派生密钥：必须压过 vkey
	got2 := resolveWSMetaKey(staticMK, []byte("0123456789abcdef"), derived)
	if string(got2) != string(derived) {
		t.Error("KX 完成后必须使用派生主密钥（否则上线后所有收发失败）")
	}

	// 3) 派生密钥过短：忽略并回落
	got3 := resolveWSMetaKey(staticMK, []byte("0123456789abcdef"), make([]byte, 16))
	if string(got3) != string(staticMK) {
		t.Error("过短的派生密钥应被忽略")
	}

	// 4) 什么都没有：回落到监听器配置（不 panic 即可）
	if len(resolveWSMetaKey(nil, nil, nil)) == 0 {
		t.Error("应回落到监听器配置")
	}
}

// forward 入口必须把派生密钥透传到 handleWSConn（通过返回值体现）
func TestResolveWSMetaKeyForwardsDerivedKey(t *testing.T) {
	derived := make([]byte, encrypt.ECDHSharedKeySize)
	for i := range derived {
		derived[i] = 0x77
	}
	// ws_forward 场景：vkey/salt 都有，但派生密钥仍须胜出
	got := resolveWSMetaKey(bytes32(0x11), bytes32(0x22), derived)
	for i := range got {
		if got[i] != 0x77 {
			t.Fatal("ws_forward 的派生密钥未生效")
		}
	}
}

func bytes32(b byte) []byte {
	out := make([]byte, 32)
	for i := range out {
		out[i] = b
	}
	return out
}
