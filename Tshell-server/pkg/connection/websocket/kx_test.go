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
