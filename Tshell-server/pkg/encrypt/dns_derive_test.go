package encrypt

import (
	"BackendTemplate/pkg/database"
	"crypto/sha256"
	"golang.org/x/crypto/hkdf"
	"io"
	"path/filepath"
	"testing"
)

// TestDnsDeriveConsistency 验证 DNS 场景（监听器配置来自 DB）下
// 服务端派生与客户端 SetSessionKey 公式（HKDF(masterKey, salt, "8d2c91f4a6"+uid)）一致
func TestDnsDeriveConsistency(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "t.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	database.Engine.Insert(&database.Listener{
		Type:           "dns",
		ListenAddress:  "0.0.0.0:8053:dns.tshell",
		Status:         1,
		VKey:           "emd9IjJJxpyAkqhftfA5pJuEygHsn0lc",
		EncryptionSalt: "KJgPl1emASxQafzq",
	})
	InvalidateListenerKeyCache()

	uid := "679fb23d41a0bd8e845e4cc72e998396"
	masterKey, salt := GetListenerKeyConfig("dns")
	if len(masterKey) != 32 || len(salt) != 16 {
		t.Fatalf("配置长度错误: masterKey=%d salt=%d", len(masterKey), len(salt))
	}

	InitSecureKeyManager()
	serverKey, err := GlobalKeyManager.DeriveSessionKeyForListener(uid, salt, "dns")
	if err != nil {
		t.Fatalf("derive: %v", err)
	}

	hk := hkdf.New(sha256.New, masterKey, salt, []byte("8d2c91f4a6"+uid))
	clientKey := make([]byte, 32)
	if _, err := io.ReadFull(hk, clientKey); err != nil {
		t.Fatal(err)
	}
	if string(serverKey) != string(clientKey) {
		t.Fatalf("MISMATCH server=%x client=%x", serverKey, clientKey)
	}
	t.Log("DNS 监听器派生一致性验证通过")
}
