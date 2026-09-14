package delivery

import (
	"bytes"
	"testing"
)

// TestXorEncodeRoundtrip 验证 XOR 加密-解密闭环 + 载荷不含明文特征
func TestXorEncodeRoundtrip(t *testing.T) {
	key, err := GenerateXorKey()
	if err != nil || key == 0 {
		t.Fatalf("GenerateXorKey: key=%d err=%v", key, err)
	}
	payload := []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00\xff\xff\x00\x00payload-bytes")
	enc, err := XorEncode(payload, key)
	if err != nil {
		t.Fatalf("XorEncode: %v", err)
	}
	// 加密后不应包含明文特征（除偶发单字节巧合）
	if bytes.Contains(enc, []byte("payload-bytes")) {
		t.Fatalf("加密载荷仍包含明文特征")
	}
	dec, err := XorEncode(enc, key)
	if err != nil {
		t.Fatalf("XorDecode: %v", err)
	}
	if !bytes.Equal(dec, payload) {
		t.Fatalf("往返不一致: got %x want %x", dec, payload)
	}
	// 密钥 0 应报错
	if _, err := XorEncode(payload, 0); err == nil {
		t.Fatal("key=0 应报错")
	}
}
