package communication

import (
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"bytes"
	"path/filepath"
	"testing"
)

type keyPair struct{ mk, salt []byte }

func setupTestDB(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	encrypt.InvalidateListenerKeyCache()
	t.Cleanup(func() {
		encrypt.InvalidateListenerKeyCache()
		if database.Engine != nil {
			database.Engine.Close()
		}
	})
}

// keysOf 调 listenerKeys 并校验长度（密钥必须是 32/16 字节，否则 HKDF 输入与客户端不一致）
func keysOf(t *testing.T, vkey, salt, listenerType string) keyPair {
	t.Helper()
	mk, s := listenerKeys([]byte(vkey), []byte(salt), listenerType)
	return pair(t, mk, s)
}

func pair(t *testing.T, mk, salt []byte) keyPair {
	t.Helper()
	if len(mk) != 32 || len(salt) != 16 {
		t.Fatalf("密钥长度异常: mk=%d salt=%d（应为 32/16）", len(mk), len(salt))
	}
	return keyPair{mk, salt}
}

func same(a, b keyPair) bool {
	return bytes.Equal(a.mk, b.mk) && bytes.Equal(a.salt, b.salt)
}

// TestListenerKeysExplicitMatchesPatchedKeys 关键不变量：服务端按监听器记录解析出的密钥，
// 必须与客户端生成时被 patch 进二进制的密钥一致（patch 走 GetListenerKeyConfigByAddress
// 按 connect_address 取同一条记录）。不一致则 MetaInfo 解密失败、客户端无法上线。
func TestListenerKeysExplicitMatchesPatchedKeys(t *testing.T) {
	setupTestDB(t)

	const (
		vkey = "Zm9vYmFyLXYta2V5LWZvci1odHRwLWxpc3RlbmVyLTEyMzQ1Ng==" // 44 字符 base64
		salt = "c2FsdC1mb3ItaHR0cC0x"                                 // 16 字符
		addr = "c2.example.com:8081"
	)
	database.Engine.Insert(&database.Listener{
		Type: "http", ListenAddress: "0.0.0.0:8081", ConnectAddress: addr,
		Status: 1, VKey: vkey, EncryptionSalt: salt,
	})
	encrypt.InvalidateListenerKeyCache()

	got := keysOf(t, vkey, salt, "http")
	pk, ps := encrypt.GetListenerKeyConfigByAddress(addr)
	patched := pair(t, pk, ps)

	if !same(got, patched) {
		t.Fatalf("按监听器密钥解析与客户端 patch 密钥不一致\n got=%x/%x\n patch=%x/%x",
			got.mk, got.salt, patched.mk, patched.salt)
	}
}

// TestListenerKeysSingleListenerNoChange 单监听器场景（绝大多数部署）：显式路径与
// 原按类型查库逐字节等价，改动是纯增量，不改变既有行为。
func TestListenerKeysSingleListenerNoChange(t *testing.T) {
	setupTestDB(t)

	const vkey, salt = "c2luZ2xlLWxpc3RlbmVyLXZrZXktMDEyMzQ1Njc4OQ==", "c2luZ2xlLXNhbHQtMDE="
	database.Engine.Insert(&database.Listener{
		Type: "http", ListenAddress: "0.0.0.0:8081", ConnectAddress: "c2.example.com:8081",
		Status: 1, VKey: vkey, EncryptionSalt: salt,
	})
	encrypt.InvalidateListenerKeyCache()

	explicit := keysOf(t, vkey, salt, "http")
	lm, ls := encrypt.GetListenerKeyConfig("http")
	legacy := pair(t, lm, ls)

	if !same(explicit, legacy) {
		t.Fatalf("单监听器场景行为发生变化\n explicit=%x/%x\n legacy=%x/%x",
			explicit.mk, explicit.salt, legacy.mk, legacy.salt)
	}
}

// TestListenerKeysPerListenerWithSameType 同类型两个 http 监听器各持独立密钥时，
// 显式路径各自解析到自己的密钥（修复前按类型查库对两者返回同一份，其中一个必然错）。
func TestListenerKeysPerListenerWithSameType(t *testing.T) {
	setupTestDB(t)

	type lis struct{ vkey, salt string }
	a := lis{"QUFBQS1saXN0ZW5lci1hLWtleS0wMTIzNDU2Nzg5YWJjZGVm", "c2FsdC1hLTAxMjM0NTY="}
	b := lis{"QkJCQi1saXN0ZW5lci1iLWtleS0wMTIzNDU2Nzg5YWJjZGVm", "c2FsdC1iLTAxMjM0NTY="}

	database.Engine.Insert(&database.Listener{
		Type: "http", ListenAddress: "0.0.0.0:8081", ConnectAddress: "a.example.com:8081",
		Status: 1, VKey: a.vkey, EncryptionSalt: a.salt,
	})
	database.Engine.Insert(&database.Listener{
		Type: "http", ListenAddress: "0.0.0.0:8082", ConnectAddress: "b.example.com:8082",
		Status: 1, VKey: b.vkey, EncryptionSalt: b.salt,
	})
	encrypt.InvalidateListenerKeyCache()

	gotA := keysOf(t, a.vkey, a.salt, "http")
	gotB := keysOf(t, b.vkey, b.salt, "http")

	am, as := encrypt.NormalizeListenerKeyConfig([]byte(a.vkey), []byte(a.salt))
	wantA := pair(t, am, as)
	bm, bs := encrypt.NormalizeListenerKeyConfig([]byte(b.vkey), []byte(b.salt))
	wantB := pair(t, bm, bs)

	if !same(gotA, wantA) {
		t.Errorf("监听器 A 未解析到自己的密钥")
	}
	if !same(gotB, wantB) {
		t.Errorf("监听器 B 未解析到自己的密钥")
	}
	// 两个监听器密钥确实不同，否则本用例证明不了任何事
	if bytes.Equal(wantA.mk, wantB.mk) {
		t.Fatal("用例构造有误：两个监听器密钥相同")
	}
	// 修复前的路径：按类型只能取到其中一行，对两个监听器返回同一份密钥
	lm, ls := encrypt.GetListenerKeyConfig("http")
	legacy := pair(t, lm, ls)
	if !same(legacy, wantA) && !same(legacy, wantB) {
		t.Fatalf("按类型查库返回了非预期密钥: %x", legacy.mk)
	}
}
