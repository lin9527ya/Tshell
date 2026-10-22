package delivery

import (
	"bytes"
	"testing"

	"BackendTemplate/pkg/database"
)

func TestAEADRoundTrip(t *testing.T) {
	key, err := NewAEADKey()
	if err != nil {
		t.Fatalf("NewAEADKey: %v", err)
	}
	if len(key) < 32 {
		t.Fatalf("密钥过短: %d", len(key))
	}
	plain := []byte("MZ-fake-payload-\x00\x01\x02")
	enc, err := AEADEncrypt(plain, key)
	if err != nil {
		t.Fatalf("AEADEncrypt: %v", err)
	}
	if bytes.Contains(enc, plain) {
		t.Fatal("密文不应包含明文")
	}
	dec, err := AEADDecrypt(enc, key)
	if err != nil {
		t.Fatalf("AEADDecrypt: %v", err)
	}
	if !bytes.Equal(dec, plain) {
		t.Fatalf("解密结果不一致: %q != %q", dec, plain)
	}

	// 同一明文两次加密必须因随机 nonce 而不同
	enc2, _ := AEADEncrypt(plain, key)
	if bytes.Equal(enc, enc2) {
		t.Fatal("随机 nonce 失效：两次密文相同")
	}

	// 篡改密文必须失败
	enc[len(enc)-1] ^= 0xFF
	if _, err := AEADDecrypt(enc, key); err == nil {
		t.Fatal("篡改密文应认证失败")
	}

	// 错误密钥必须失败
	other, _ := NewAEADKey()
	if _, err := AEADDecrypt(enc2, other); err == nil {
		t.Fatal("错误密钥应认证失败")
	}
}

func TestEnsureCryptoDefaultsLegacyCompatibility(t *testing.T) {
	// 旧记录：只有 XorKey，没有 CryptoMode → 自动归为 xor
	m := &database.WebDelivery{XorKey: 0x5A}
	EnsureCryptoDefaults(m)
	if m.CryptoMode != "xor" {
		t.Fatalf("旧 XorKey 记录应归为 xor，实际 %q", m.CryptoMode)
	}

	// xor 模式缺 key → 自动生成非零单字节
	m = &database.WebDelivery{CryptoMode: "xor"}
	EnsureCryptoDefaults(m)
	if m.XorKey == 0 {
		t.Fatal("xor 模式应自动生成非零 XorKey")
	}

	// aead 模式缺 key → 自动生成合法密钥
	m = &database.WebDelivery{CryptoMode: "aead"}
	EnsureCryptoDefaults(m)
	if _, err := DecodeAEADKey(m.DeliveryKey); err != nil {
		t.Fatalf("aead 模式应生成合法密钥: %v", err)
	}

	// 未知模式 → none
	m = &database.WebDelivery{CryptoMode: "rot13"}
	EnsureCryptoDefaults(m)
	if m.CryptoMode != "none" {
		t.Fatalf("未知模式应回退 none，实际 %q", m.CryptoMode)
	}
}
