package database

import (
	"encoding/json"
	"strings"
	"testing"
)

// 迁移必须幂等：重跑不报错，且列一定存在。
// 这一条对 MFA 尤其重要 —— 迁移没跑成会让 SetMFASecret 之类的调用直接报
// "no such column"，而那是登录路径上的代码，等于全员登录失败。
func TestMigrateUsersMFATableIdempotent(t *testing.T) {
	setupMigrationTest(t)
	// 删掉这三列，模拟"迁移前的老库"（列名与迁移里的目标一致）。
	// 若本机 SQLite 不支持 DROP COLUMN，就退化为"只验证幂等 + 列存在"。
	dropped := true
	for _, c := range []string{"mfa_secret_enc", "mfa_enabled", "mfa_recovery_hashes"} {
		if _, err := Engine.Exec("ALTER TABLE users DROP COLUMN " + c); err != nil {
			dropped = false
			break
		}
	}
	before := tableColumns("users")
	if dropped {
		for _, c := range []string{"mfa_secret_enc", "mfa_enabled", "mfa_recovery_hashes"} {
			if before[c] {
				t.Fatalf("前置条件不成立：DROP COLUMN 后不该还有 %s", c)
			}
		}
	} else {
		t.Log("本机 SQLite 不支持 DROP COLUMN，退化为幂等性验证（列已存在时迁移不应报错）")
	}

	// 连跑两次：第二次应全部命中"已存在，跳过"
	for i := 1; i <= 2; i++ {
		if err := migrateUsersMFATable(); err != nil {
			t.Fatalf("第 %d 次执行迁移失败: %v", i, err)
		}
	}

	cols := tableColumns("users")
	for _, want := range []string{"mfa_secret_enc", "mfa_enabled", "mfa_recovery_hashes"} {
		if !cols[want] {
			t.Errorf("迁移后 users 表应存在列 %s", want)
		}
	}
}

// 迁移清单里不允许出现重复版本号（重复会让"记录已应用"与"实际执行"对不上）
func TestMigrationVersionsUnique(t *testing.T) {
	seen := map[int]string{}
	for _, m := range migrations() {
		if prev, dup := seen[m.Version]; dup {
			t.Errorf("迁移版本 %d 重复: %q 与 %q", m.Version, prev, m.Name)
		}
		seen[m.Version] = m.Name
		if m.Apply == nil {
			t.Errorf("迁移 %d(%s) 没有实现", m.Version, m.Name)
		}
		if strings.TrimSpace(m.Name) == "" {
			t.Errorf("迁移 %d 缺少名字（回执/日志里要能看出来改了什么）", m.Version)
		}
	}
	if _, ok := seen[3]; !ok {
		t.Error("应包含 v3（users 表 MFA 字段）")
	}
}

// MFA 三个字段绝不能出现在任何 API 响应里：结构体上必须是 json:"-"
func TestUsersMFAFieldsAreNotSerialized(t *testing.T) {
	u := Users{Username: "admin", MFASecretEnc: "v1:secret", MFAEnabled: true, MFARecoveryHashes: "[]"}
	data, err := jsonMarshalForTest(u)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, leak := range []string{"mfa_secret_enc", "v1:secret", "mfa_recovery_hashes", "mfa_enabled"} {
		if strings.Contains(s, leak) {
			t.Errorf("Users 序列化后泄露了 %q：%s", leak, s)
		}
	}
}

func jsonMarshalForTest(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
