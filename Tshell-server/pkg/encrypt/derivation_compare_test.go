package encrypt

import (
	"BackendTemplate/pkg/utils/logx"
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"golang.org/x/crypto/hkdf"
	"io"
	"testing"
	"time"
)

// 模拟客户端派生逻辑（implant/core/encrypt/encrypt.go SetSessionKey）
func clientDerive(masterKey, salt []byte, uid string) []byte {
	hk := hkdf.New(sha256.New, masterKey, salt, []byte("8d2c91f4a6"+uid))
	key := make([]byte, 32)
	io.ReadFull(hk, key)
	return key
}

func TestDerivationClientServerConsistency(t *testing.T) {
	vkey := "y26OvV/BP8ehZBByrB4Z1PvhFFnXGnlTWW6TSS/0v8Y="
	salt := "f5Mu+Qqp4RG/+gBKxaFkaA=="
	uid := "deadbeefdeadbeefdeadbeefdeadbeef"

	// 客户端:GetListenerKeyConfig 占位符被 patch 为 padRight(first32(vkey),48)/padRight(first16(salt),48),
	// 取 masterKey[:32], salt[:16]
	mkPatched := make([]byte, 48)
	copy(mkPatched, []byte(vkey))
	slPatched := make([]byte, 48)
	copy(slPatched, []byte(salt))

	clientKey := clientDerive(mkPatched[:32], slPatched[:16], uid)

	// 服务端:GetListenerKeyConfigForForward 从 DB 取 VKey/EncryptionSalt,同样 copy 前 32/16 字节
	serverMasterKey := make([]byte, 32)
	copy(serverMasterKey, []byte(vkey))
	serverSalt := make([]byte, 16)
	copy(serverSalt, []byte(salt))

	if InitSecureKeyManager() != nil {
		t.Fatal("init key manager failed")
	}
	sk, err := GlobalKeyManager.DeriveSessionKeyForListenerWithKeys(uid, serverSalt, serverMasterKey, serverSalt)
	if err != nil {
		t.Fatalf("server derive failed: %v", err)
	}

	logx.Infof("client key: %s\n", hex.EncodeToString(clientKey))
	logx.Infof("server key: %s\n", hex.EncodeToString(sk))
	if string(clientKey) != string(sk) {
		t.Fatalf("KEYS MISMATCH")
	}

	// 服务端默认路径(DB miss)
	dk := clientDerive([]byte("initial_master_key_12345678901"), []byte("initial_salt_123"), uid)
	keyIDInput := uid + "_session_" + string(serverSalt)
	h := md5.Sum([]byte(keyIDInput))
	logx.Infof("client keyID: %s\n", hex.EncodeToString(h[:4]))
	logx.Infof("default key : %s\n", hex.EncodeToString(dk))
}

// TestNonceDerivationConsistency 验证会话密钥唯一化（nonce）派生一致性：
// 服务端 RotateSessionKeyWithNonce 的新密钥 必须等于
// 客户端 SetSessionKeyWithNonce 的派生结果（HKDF(masterKey, salt, "8d2c91f4a6"+uid+nonce)）。
func TestNonceDerivationConsistency(t *testing.T) {
	vkey := "emd9IjJJxpyAkqhftfA5pJuEygHsn0lc"
	saltStr := "KJgPl1emASxQafzq"
	uid := "deadbeefdeadbeefdeadbeefdeadbeef"
	nonce := []byte("0123456789abcdef") // 16B

	masterKey := make([]byte, 32)
	copy(masterKey, []byte(vkey))
	encSalt := make([]byte, 16)
	copy(encSalt, []byte(saltStr))

	if InitSecureKeyManager() != nil {
		t.Fatal("init failed")
	}
	// 1. 初始派生（无 nonce，客户端 Connect 时）
	if _, err := GlobalKeyManager.DeriveSessionKeyForListenerWithKeys(uid, encSalt, masterKey, encSalt); err != nil {
		t.Fatalf("initial derive: %v", err)
	}
	// 2. 服务端唯一化
	initialKey, err := GlobalKeyManager.RotateSessionKeyWithNonce(uid, encSalt, masterKey, encSalt, nonce)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	currentKey, err := GlobalKeyManager.GetSessionKey(uid)
	if err != nil {
		t.Fatalf("get current: %v", err)
	}

	// 3. 客户端公式（SetSessionKeyWithNonce 的 HKDF 实现）
	hk := hkdf.New(sha256.New, masterKey, encSalt, []byte("8d2c91f4a6"+uid+string(nonce)))
	clientKey := make([]byte, 32)
	if _, err := io.ReadFull(hk, clientKey); err != nil {
		t.Fatalf("client derive: %v", err)
	}

	if string(currentKey) != string(clientKey) {
		t.Fatalf("NONCE KEY MISMATCH: server=%x client=%x", currentKey, clientKey)
	}
	if string(initialKey) == string(clientKey) {
		t.Fatalf("nonce key should differ from initial key")
	}
	t.Logf("nonce 派生一致性验证通过: current=%x", currentKey[:8])
}

// TestKeyRotationChain 验证 P3-1 密钥轮换完整链路：
// 1. prepareKeyUpdate 用会话记录的监听器 VKey/盐派生 NextKey（非全局默认密钥）
// 2. NextKey 必须等于客户端 SetSessionKeyWithNonce 的派生结果
// 3. ConfirmKeyRotation 后 CurrentKey=NextKey，旧密钥降为 PreviousKey
// 4. KeyRotationSender 回调被调用（消息 type=5|nonce）
func TestKeyRotationChain(t *testing.T) {
	vkey := "emd9IjJJxpyAkqhftfA5pJuEygHsn0lc"
	saltStr := "KJgPl1emASxQafzq"
	uid := "deadbeefdeadbeefdeadbeefdeadbeef"

	masterKey := make([]byte, 32)
	copy(masterKey, []byte(vkey))
	encSalt := make([]byte, 16)
	copy(encSalt, []byte(saltStr))

	if InitSecureKeyManager() != nil {
		t.Fatal("init failed")
	}

	// 记录下发回调
	var sentUID string
	var sentPayload []byte
	KeyRotationSender = func(clientID string, payload []byte) {
		sentUID = clientID
		sentPayload = append([]byte{}, payload...)
	}
	defer func() { KeyRotationSender = nil }()

	// 1. 初始派生（记录派生输入）
	if _, err := GlobalKeyManager.DeriveSessionKeyForListenerWithKeys(uid, encSalt, masterKey, encSalt); err != nil {
		t.Fatalf("initial derive: %v", err)
	}
	initialKey, _ := GlobalKeyManager.GetSessionKey(uid)

	// 2. 触发轮换准备（模拟过期检查）
	value, _ := GlobalKeyManager.sessionKeys.Load(uid)
	keySet := value.(*SessionKeySet)
	GlobalKeyManager.prepareKeyUpdate(uid, keySet)

	if keySet.NextKey == nil {
		t.Fatalf("NextKey 未生成")
	}
	if string(keySet.NextKey.Key) == string(initialKey) {
		t.Fatalf("轮换密钥不应与初始密钥相同")
	}
	// 新密钥必须不等于全局默认密钥派生结果（原 bug：误用 skm.masterKey）
	defaultDerived := clientDerive([]byte("initial_master_key_12345678901"), []byte("initial_salt_123"), uid)
	if string(keySet.NextKey.Key) == string(defaultDerived) {
		t.Fatalf("轮换密钥误用全局默认密钥派生（原 bug 回归）")
	}

	// 3. 回调已下发 type=5|nonce
	if sentUID != uid || len(sentPayload) != 20 {
		t.Fatalf("轮换消息未下发: uid=%q payloadLen=%d", sentUID, len(sentPayload))
	}
	msgType := binary.BigEndian.Uint32(sentPayload[:4])
	if msgType != KeyRotationMsgType {
		t.Fatalf("消息类型错误: %d", msgType)
	}
	nonce := sentPayload[4:20]

	// 4. 客户端派生公式（SetSessionKeyWithNonce）：必须与服务端 NextKey 一致
	hk := hkdf.New(sha256.New, masterKey, encSalt, []byte("8d2c91f4a6"+uid+string(nonce)))
	clientKey := make([]byte, 32)
	if _, err := io.ReadFull(hk, clientKey); err != nil {
		t.Fatalf("client derive: %v", err)
	}
	if string(clientKey) != string(keySet.NextKey.Key) {
		t.Fatalf("轮换密钥不一致: server=%x client=%x", keySet.NextKey.Key, clientKey)
	}

	// 5. 确认轮换：CurrentKey=NextKey, 旧密钥降为 PreviousKey
	if err := GlobalKeyManager.ConfirmKeyRotation(uid); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	current, _ := GlobalKeyManager.GetSessionKey(uid)
	if string(current) != string(clientKey) {
		t.Fatalf("轮换后 CurrentKey 错误")
	}
	if keySet.PreviousKey == nil || string(keySet.PreviousKey.Key) != string(initialKey) {
		t.Fatalf("旧密钥未降为 PreviousKey")
	}
	if keySet.NextKey != nil {
		t.Fatalf("轮换后 NextKey 未清空")
	}

	// 6. 未确认超时清理：悬挂 NextKey 应可被重试
	keySet.NextKey = &SessionKey{Key: []byte("x"), CreatedAt: time.Now().Add(-3 * time.Hour)}
	GlobalKeyManager.rotateExpiredKeys()
	if keySet.NextKey != nil {
		t.Fatalf("超时未确认的 NextKey 未清理")
	}
}
