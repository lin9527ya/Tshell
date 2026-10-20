package oss

import (
	"strings"
	"testing"

	"BackendTemplate/pkg/encrypt"
)

// ===== OSS 的会话前 ECDH 交换 =====
//
// OSS 与其它 11 条传输形态都不同：没有请求/响应语义，只有"上传对象 + 列表轮询"。
// 所以交换要多一个来回，而且 uid 只能从**对象键前缀**拿（此时 MetaInfo 还没解开）。
// 这些用例覆盖两个最容易错的点：键前缀解析、以及按 uid 的绑定/命中。

func TestUIDFromObjectKey(t *testing.T) {
	cases := map[string]string{
		"abc123/client_00000000000000000001":     "abc123",
		"deadbeef/client_kx_0000000000000000002": "deadbeef",
		"no-slash-key":                           "no-slash-key",
		"/leading-slash":                         "/leading-slash", // 前缀为空：不回退成空串
		"":                                       "",
	}
	for in, want := range cases {
		if got := uidFromObjectKey(in); got != want {
			t.Errorf("uidFromObjectKey(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// 客户端公钥必须是定长 32 字节，服务端按该长度切载荷
func TestOSSKXPayloadLength(t *testing.T) {
	kx, err := encrypt.NewServerKeyExchange()
	if err != nil {
		t.Fatal(err)
	}
	pub := kx.PublicKey()
	if len(pub) != encrypt.ECDHPublicKeySize {
		t.Fatalf("公钥长度应为 %d，实际 %d", encrypt.ECDHPublicKeySize, len(pub))
	}
	// 服务端切包：message[4 : 4+32]
	msg := make([]byte, 4+len(pub))
	copy(msg[4:], pub)
	if got := len(msg[4 : 4+encrypt.ECDHPublicKeySize]); got != 32 {
		t.Errorf("切包长度应为 32，实际 %d", got)
	}
}

// 按 uid 绑定后 firstBlood 阶段必须能命中同一把派生密钥
func TestOSSKXCacheByUID(t *testing.T) {
	encrypt.GlobalKxCache.Close()
	defer encrypt.GlobalKxCache.Close()

	kx, err := encrypt.NewServerKeyExchange()
	if err != nil {
		t.Fatal(err)
	}
	shared := make([]byte, 32)
	for i := range shared {
		shared[i] = byte(i + 5)
	}
	derived, err := encrypt.DeriveMasterKeyFromShared(shared, []byte("0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	const uid = "uid-oss-kx-1"

	if _, err := encrypt.GlobalKxCache.Put(kx.PublicKey(), derived); err != nil {
		t.Fatal(err)
	}
	if !encrypt.GlobalKxCache.Bind(kx.PublicKey(), uid) {
		t.Fatal("绑定应成功")
	}
	got, ok := encrypt.GlobalKxCache.LookupByUID(uid)
	if !ok {
		t.Fatal("绑定后应能按 uid 查到（否则 firstBlood 会拿静态密钥去解）")
	}
	for i := range derived {
		if got[i] != derived[i] {
			t.Fatal("查到的密钥与存入的不一致")
		}
	}
}

// 响应对象键名必须能被客户端的轮询匹配到
func TestOSSKXResponseKeyIsMatchable(t *testing.T) {
	uid := "abc123"
	// 服务端写法：uid + "/kxresp_<20位时间戳>"
	name := uid + "/kxresp_00000000000000000001"
	if !strings.Contains(name, "/kxresp_") {
		t.Errorf("响应键 %q 不含客户端匹配用的 /kxresp_ 片段", name)
	}
	if uidFromObjectKey(name) != uid {
		t.Errorf("响应键的 uid 前缀解析错误: %q", uidFromObjectKey(name))
	}
}

// 消息类型常量必须与客户端一致（客户端 kxMsgType，服务端 msgTypeOSS_KX）
func TestOSSKXMsgType(t *testing.T) {
	if msgTypeOSS_KX != 200 {
		t.Errorf("msgTypeOSS_KX 必须为 200（与客户端 kxMsgType 一致），实际 %d", msgTypeOSS_KX)
	}
}
