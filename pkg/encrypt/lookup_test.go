package encrypt

import (
	"BackendTemplate/pkg/database"
	"os"
	"path/filepath"
	"testing"
	"BackendTemplate/pkg/utils/logx"
)

func TestListenerKeyLookupWithRealDB(t *testing.T) {
	dir, _ := os.MkdirTemp("", "keydb")
	defer os.RemoveAll(dir)
	dbPath := filepath.Join(dir, "test.db")

	old := database.DatabaseName
	database.DatabaseName = dbPath
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	// 模拟前端创建监听器(未指定密钥 → AddListener 自动生成 base64 随机密钥)
	_, err := database.Engine.Insert(&database.Listener{
		Type:              "tcp",
		ListenAddress:     "0.0.0.0:8089",
		ConnectAddress:    "1.2.3.4:8089",
		Status:            1,
		HeartbeatTimeout:  "30",
		HeartbeatInterval: "10",
		VKey:              "y26OvV/BP8ehZBByrB4Z1PvhFFnXGnlTWW6TSS/0v8Y=",
		EncryptionSalt:    "f5Mu+Qqp4RG/+gBKxaFkaA==",
	})
	if err != nil {
		t.Fatalf("insert listener: %v", err)
	}

	// FirstBlood 路径:GetListenerKeyConfigForForward("tcp")
	mk, sl := GetListenerKeyConfigForForward("tcp")
	logx.Infof("ForForward: mk=%q sl=%q\n", string(mk), string(sl))

	// 备用路径:GetListenerKeyConfig("tcp")
	mk2, sl2 := GetListenerKeyConfig("tcp")
	logx.Infof("GetConfig : mk=%q sl=%q\n", string(mk2), string(sl2))

	// 客户端 patch 的密钥
	clientMk := string([]byte("y26OvV/BP8ehZBByrB4Z1PvhFFnXGnlTWW6TSS/0v8Y=")[:32])
	clientSl := string([]byte("f5Mu+Qqp4RG/+gBKxaFkaA==")[:16])
	logx.Infof("Client    : mk=%q sl=%q\n", clientMk, clientSl)

	if string(mk) != clientMk || string(sl) != clientSl {
		t.Fatalf("SERVER DB LOOKUP MISMATCH with client patched keys")
	}
}

func TestGenerateListenerKeyConfigFormat(t *testing.T) {
	for i := 0; i < 5; i++ {
		vk, sl := GenerateListenerKeyConfig()
		// 实现返回 base64 编码：VKey=base64(32字节)=44字符，Salt=base64(16字节)=24字符
		if len(vk) != 44 || len(sl) != 24 {
			t.Fatalf("bad length: vk=%d sl=%d", len(vk), len(sl))
		}
		for _, c := range vk + sl {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=') {
				t.Fatalf("non-base64 char in key: %q", c)
			}
		}
	}
	vk, sl := GenerateListenerKeyConfig()
	t.Logf("sample: vk=%s sl=%s", vk, sl)
}

// P0-4 回归：IsDefaultListenerKey 必须识别客户端占位符未替换的公开密钥（forward 变体）
func TestIsDefaultListenerKeyPlaceholder(t *testing.T) {
	// 客户端占位符截断规则：主密钥取前 32 字节、盐取前 16 字节
	placeholderMK := make([]byte, 32)
	copy(placeholderMK, []byte("LISTENER_MASTER_KEY_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))
	placeholderSalt := make([]byte, 16)
	copy(placeholderSalt, []byte("LISTENER_SALT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"))

	if !IsDefaultListenerKey(placeholderMK, placeholderSalt) {
		t.Fatalf("placeholder keys must be rejected")
	}
	// 只传主密钥占位符也应命中
	if !IsDefaultListenerKey(placeholderMK, nil) {
		t.Fatalf("master placeholder alone must be rejected")
	}
	// 只传盐占位符也应命中
	if !IsDefaultListenerKey(nil, placeholderSalt) {
		t.Fatalf("salt placeholder alone must be rejected")
	}

	// 历史默认密钥仍须拒绝
	if !IsDefaultListenerKey([]byte("initial_master_key_12345678901"), []byte("initial_salt_123")) {
		t.Fatalf("legacy default keys must be rejected")
	}

	// 随机密钥必须放行
	randMK, randSalt := GenerateListenerKeyConfig()
	if IsDefaultListenerKey([]byte(randMK), []byte(randSalt)) {
		t.Fatalf("random keys must not be rejected")
	}
}
