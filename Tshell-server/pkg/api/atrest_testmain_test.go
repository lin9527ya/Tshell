package api

import (
	"os"
	"path/filepath"
	"testing"

	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/security"
)

// ===== 测试环境的战果静态加密初始化 =====
//
// OPSEC P0-1 之后，`LootStore` 的写路径在"加密未配置"时**硬失败**
// （刻意不静默退化为明文）。这是生产环境要的行为，但测试必须先把加密配上，
// 否则所有涉及凭据入库的用例都会以"战果静态加密未配置"失败 ——
// 那种失败看起来像功能坏了，实际是环境没初始化。
//
// 用 TestMain 在整包测试启动时初始化一次：
//  - 密钥写临时文件再用公开的 InitAtRestCipherFromKeyFile 初始化，
//    这样测试走的是**与生产相同的代码路径**（不额外开后门）；
//  - 密钥固定，保证同一包内不同用例看到一致的加解密行为。

func TestMain(m *testing.M) {
	if err := initTestAtRestCipher(); err != nil {
		// 初始化失败就没法跑任何凭据用例，直接失败退出，不静默跳过
		panic("测试环境初始化战果静态加密失败: " + err.Error())
	}
	database.SetLootCrypto(security.Seal, security.Open, security.Fingerprint)
	os.Exit(m.Run())
}

func initTestAtRestCipher() error {
	dir, err := os.MkdirTemp("", "tshell-atrest-test")
	if err != nil {
		return err
	}
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	path := filepath.Join(dir, "atrest.key")
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return err
	}
	return security.InitAtRestCipherFromKeyFile(path)
}

// ensureLootCryptoForTest 供**不经过 TestMain 的场景**（如直接 go test -run 单个用例
// 且本地已有包级状态被其他用例重置）兜底：幂等地再配一次。
func ensureLootCryptoForTest(t *testing.T) {
	t.Helper()
	if database.LootCryptoReady() {
		return
	}
	if err := initTestAtRestCipher(); err != nil {
		t.Fatalf("初始化战果静态加密失败: %v", err)
	}
	database.SetLootCrypto(security.Seal, security.Open, security.Fingerprint)
}

// plainSecret 把库里读到的 Secret 解成明文。
//
// OPSEC P0-1 之后 Secret 列是密文（v1:...）。直接断言原始行会看到密文，
// 测试要验证的是"入库后还能还原成原口令"，所以统一在这里解一次。
// security.Open 对历史明文（无 v1: 前缀）原样返回，因此老夹具也能直接用。
func plainSecret(t *testing.T, stored string) string {
	t.Helper()
	plain, err := security.Open(stored)
	if err != nil {
		t.Fatalf("解密战果失败: %v", err)
	}
	return plain
}

// decryptLootForTest 就地解密一批战果行（测试里直接读原始行的场景）
func decryptLootForTest(t *testing.T, rows []database.Loot) {
	t.Helper()
	for i := range rows {
		rows[i].Secret = plainSecret(t, rows[i].Secret)
	}
}
