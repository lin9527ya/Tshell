package encrypt

import (
	"crypto/sha256"
	"golang.org/x/crypto/hkdf"
	"implant/core/config"
	"io"
	"testing"
)

// TestClientSetSessionKeyWithNonce 验证客户端唯一化派生公式与服务端一致：
// HKDF(masterKey, salt, "TshellSessionKey"+clientID+nonce)
func TestClientSetSessionKeyWithNonce(t *testing.T) {
	vkey := "emd9IjJJxpyAkqhftfA5pJuEygHsn0lc"
	saltStr := "KJgPl1emASxQafzq"
	cid := "deadbeefdeadbeefdeadbeefdeadbeef"
	nonce := []byte("0123456789abcdef")

	InitClientKeyManager(cid)
	masterKey := make([]byte, 32)
	copy(masterKey, []byte(vkey))
	salt := make([]byte, 16)
	copy(salt, []byte(saltStr))

	if err := GlobalClientKeyManager.SetSessionKey(salt, masterKey); err != nil {
		t.Fatalf("initial: %v", err)
	}
	if err := GlobalClientKeyManager.SetSessionKeyWithNonce(nonce); err != nil {
		t.Fatalf("nonce: %v", err)
	}

	got, err := GlobalClientKeyManager.GetSessionKey()
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// 服务端公式（与 encrypt.go SetSessionKeyWithNonce 一致：可配置盐上下文）
	hk := hkdf.New(sha256.New, masterKey, salt, []byte(config.SessionKeyInfo()+cid+string(nonce)))
	want := make([]byte, 32)
	if _, err := io.ReadFull(hk, want); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("KEY MISMATCH: got=%x want=%x", got, want)
	}
	t.Logf("客户端 nonce 派生一致性验证通过")
}
