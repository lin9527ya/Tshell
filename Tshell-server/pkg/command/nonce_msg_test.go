package command

import (
	"BackendTemplate/pkg/encrypt"
	"testing"
)

// TestBuildSessionNonceMsgWithKey 验证 nonce 消息帧构造：type=4|nonce，用初始密钥加密后客户端可解密
func TestBuildSessionNonceMsgWithKey(t *testing.T) {
	if encrypt.InitSecureKeyManager() != nil {
		t.Fatal("init failed")
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	nonce := []byte("abcdefghijklmnop") // 16B
	b64 := BuildSessionNonceMsgWithKey(key, nonce)
	if len(b64) == 0 {
		t.Fatal("BuildSessionNonceMsgWithKey 返回空")
	}
	encrypted, err := encrypt.DecodeBase64(b64)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	decrypted, err := encrypt.SecureDecrypt(encrypted, key)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if len(decrypted) != 20 {
		t.Fatalf("载荷长度应为 4+16=20, 实际 %d", len(decrypted))
	}
	if uint32(decrypted[0])<<24|uint32(decrypted[1])<<16|uint32(decrypted[2])<<8|uint32(decrypted[3]) != 4 {
		t.Fatalf("类型头应为 4, 实际 %v", decrypted[:4])
	}
	if string(decrypted[4:]) != string(nonce) {
		t.Fatalf("nonce 内容不一致")
	}
	t.Log("nonce 消息帧构造验证通过")
}
