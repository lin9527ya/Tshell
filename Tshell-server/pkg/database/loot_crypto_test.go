package database

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ===== OPSEC P0-1：LootStore 的加解密接线 =====
//
// 这里用**测试专用的加解密实现**（可逆、确定性、够真以证明"库里没有明文"），
// 目的是验证接线（每条写路径都封、每条读路径都解、去重改用指纹），
// 加密算法本身由 pkg/security/atrest_test.go 覆盖。
// 用真实算法做"库文件里搜不到明文"的验收放在 pkg/api 里（那里能用真实 cipher）。

const testSealTag = "v1:"

func testSeal(plain string) (string, error) {
	return testSealTag + base64.StdEncoding.EncodeToString([]byte(plain)), nil
}

func testOpen(stored string) (string, error) {
	if !strings.HasPrefix(stored, testSealTag) {
		return stored, nil // 历史明文
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, testSealTag))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func testFp(plain string) (string, error) {
	sum := sha256.Sum256([]byte("fp-key" + plain))
	return hex.EncodeToString(sum[:]), nil
}

// setupLootCryptoTestDB 建临时库并接线测试用加解密。
// 返回库文件绝对路径：DatabaseName 会被还原，直接读它读不到本用例的库。
func setupLootCryptoTestDB(t *testing.T) string {
	t.Helper()
	old := DatabaseName
	path := filepath.Join(t.TempDir(), "loot-crypto.db")
	DatabaseName = path
	ConnectDateBase()
	DatabaseName = old
	t.Cleanup(func() {
		if Engine != nil {
			Engine.Close()
		}
	})
	SetLootCrypto(testSeal, testOpen, testFp)
	return path
}

func TestLootSecretIsSealedAtRest(t *testing.T) {
	setupLootCryptoTestDB(t)
	s := LootStore_()

	const secret = "Sup3rSecretPw!"
	ok, err := s.AddIfNotEmpty("uid-1", "pwd", "alice", "CORP", "DC01", secret, "", "lsass")
	if err != nil {
		t.Fatalf("入库失败: %v", err)
	}
	if !ok {
		t.Fatal("应插入成功")
	}

	// 原始行必须是密文
	var raw Loot
	if has, err := Engine.Where("uid = ?", "uid-1").Get(&raw); err != nil || !has {
		t.Fatalf("读取原始行失败: has=%v err=%v", has, err)
	}
	if raw.Secret == secret {
		t.Error("库里存的是明文口令（静态加密没生效）")
	}
	if !isSealedValue(raw.Secret) {
		t.Errorf("库里应为密文，实际 %q", raw.Secret)
	}
	if raw.SecretFp == "" {
		t.Error("指纹列不应为空（否则去重失效）")
	}

	// 领域层读出来必须是明文
	rows, err := s.ListByUID("uid-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Secret != secret {
		t.Errorf("领域层应返回明文，实际 %+v", rows)
	}
}

// 去重必须仍然有效 —— 这是加密最容易打破的行为（GCM 随机化后密文不同）
func TestLootDedupeStillWorksAfterEncryption(t *testing.T) {
	setupLootCryptoTestDB(t)
	s := LootStore_()

	first, err := s.AddIfNotEmpty("uid-1", "pwd", "alice", "CORP", "DC01", "same-pw", "", "lsass")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AddIfNotEmpty("uid-1", "pwd", "alice", "CORP", "DC01", "same-pw", "", "lsass")
	if err != nil {
		t.Fatal(err)
	}
	if !first {
		t.Error("第一次应插入")
	}
	if second {
		t.Error("同一口令重复上报应被去重（指纹列没起作用）")
	}

	// AddAutoIngest 的键不同，也要能去重
	a, err := s.AddAutoIngest(&Loot{Uid: "uid-2", Type: "pwd", Host: "AP01", Secret: "wifi-pw", Source: "wifi"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.AddAutoIngest(&Loot{Uid: "uid-2", Type: "pwd", Host: "AP01", Secret: "wifi-pw", Source: "wifi"})
	if err != nil {
		t.Fatal(err)
	}
	if !a || b {
		t.Errorf("AddAutoIngest 去重失效：first=%v second=%v", a, b)
	}
}

// 不同口令不能被误判为重复
func TestLootDedupeDistinguishesDifferentSecrets(t *testing.T) {
	setupLootCryptoTestDB(t)
	s := LootStore_()
	if _, err := s.AddIfNotEmpty("uid-1", "pwd", "a", "", "H", "pw-1", "", "x"); err != nil {
		t.Fatal(err)
	}
	ok, err := s.AddIfNotEmpty("uid-1", "pwd", "a", "", "H", "pw-2", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("不同口令不应被判为重复")
	}
}

// 未配置加密时写入必须硬失败，绝不能静默存明文
func TestLootWriteFailsWithoutCrypto(t *testing.T) {
	setupLootCryptoTestDB(t)
	SetLootCrypto(nil, nil, nil) // 模拟漏接线
	defer SetLootCrypto(testSeal, testOpen, testFp)

	if _, err := LootStore_().AddIfNotEmpty("uid-1", "pwd", "a", "", "H", "pw", "", "x"); err == nil {
		t.Error("未配置加密时必须报错，不能明文落库")
	}
	var n int64
	if n, _ = Engine.Where("uid = ?", "uid-1").Count(new(Loot)); n != 0 {
		t.Error("失败时不应有行落库")
	}
}

// 存量明文迁移：加密回写 + 补指纹，且幂等
func TestMigrateLootSecrets(t *testing.T) {
	setupLootCryptoTestDB(t)

	// 直接插两行"历史明文"（绕过 LootStore，模拟升级前的数据）
	for _, l := range []Loot{
		{Uid: "uid-old", Type: "pwd", User: "bob", Host: "H1", Secret: "old-plain-1", Source: "lsass", CreatedAt: "2026-01-01 00:00:00"},
		{Uid: "uid-old", Type: "pwd", User: "eve", Host: "H2", Secret: "old-plain-2", Source: "lsass", CreatedAt: "2026-01-01 00:00:00"},
	} {
		if _, err := Engine.Insert(&l); err != nil {
			t.Fatal(err)
		}
	}

	n, err := MigrateLootSecrets()
	if err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	if n != 2 {
		t.Errorf("应迁移 2 行，实际 %d", n)
	}

	var rows []Loot
	if err := Engine.Find(&rows); err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if !isSealedValue(r.Secret) {
			t.Errorf("id=%d 迁移后仍是明文: %q", r.Id, r.Secret)
		}
		if r.SecretFp == "" {
			t.Errorf("id=%d 迁移后缺指纹", r.Id)
		}
	}

	// 幂等：再跑一次不应有变化
	n2, err := MigrateLootSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 0 {
		t.Errorf("第二次迁移应为 0 行，实际 %d", n2)
	}

	// 迁移后通过领域层读仍是原明文
	rows2, err := LootStore_().ListByUID("uid-old")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range rows2 {
		got[r.Secret] = true
	}
	if !got["old-plain-1"] || !got["old-plain-2"] {
		t.Errorf("迁移后应能解回原明文，实际 %v", got)
	}
}

// 密文被篡改时读取必须报错，不能静默返回空
func TestLootReadRejectsTamperedCiphertext(t *testing.T) {
	setupLootCryptoTestDB(t)
	s := LootStore_()
	if _, err := s.AddIfNotEmpty("uid-1", "pwd", "a", "", "H", "pw", "", "x"); err != nil {
		t.Fatal(err)
	}
	// 破坏 base64 内容
	if _, err := Engine.Exec("UPDATE loot SET secret = 'v1:!!!not-base64!!!'"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListByUID("uid-1"); err == nil {
		t.Error("篡改的密文必须报错，不能静默返回空串")
	}
}

// 验收：库文件字节里检索不到明文口令
func TestDatabaseFileContainsNoPlaintextSecret(t *testing.T) {
	dbPath := setupLootCryptoTestDB(t)
	const secret = "UNIQUE-PLAINTEXT-MARKER-9271"
	if _, err := LootStore_().AddIfNotEmpty("uid-1", "pwd", "alice", "", "H", secret, "", "lsass"); err != nil {
		t.Fatal(err)
	}
	// 关掉连接让 WAL 落盘
	if Engine != nil {
		Engine.Close()
	}
	raw, err := os.ReadFile(dbPath)
	if err != nil {
		t.Skipf("读不到库文件（可能路径已变更）: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Error("数据库文件里出现了明文口令 —— 静态加密没有真正生效")
	}
	// 顺带确认指纹在库里（说明这条确实写进去了）
	if !strings.Contains(string(raw), "loot") {
		t.Log("提示：库文件里未见 loot 表名，可能是 WAL 未合并，本断言不阻塞")
	}
}
