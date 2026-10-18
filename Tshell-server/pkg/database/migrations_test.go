package database

import (
	"errors"
	"path/filepath"
	"testing"
)

// 版本化迁移的验收点：新库按序应用、重复启动不重复应用、失败不记录（可重试）、
// 已上线但没有 schema_migrations 的老库能平滑接入。

func setupMigrationTest(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	old := DatabaseName
	DatabaseName = filepath.Join(tmpDir, "test.db")
	ConnectDateBase()
	DatabaseName = old
	t.Cleanup(func() {
		if Engine != nil {
			Engine.Close()
		}
	})
}

func countApplied(t *testing.T) int {
	t.Helper()
	rows, err := Engine.QueryString("SELECT version FROM schema_migrations")
	if err != nil {
		t.Fatalf("查询迁移记录失败: %v", err)
	}
	return len(rows)
}

// TestMigrationsApplyOnFreshDatabase 新库：内置迁移全部应用并记录
func TestMigrationsApplyOnFreshDatabase(t *testing.T) {
	setupMigrationTest(t)

	version, err := SchemaVersion()
	if err != nil {
		t.Fatalf("读取版本失败: %v", err)
	}
	want := 0
	for _, m := range migrations() {
		if m.Version > want {
			want = m.Version
		}
	}
	if version != want {
		t.Fatalf("schema 版本 = %d，期望 %d", version, want)
	}
	if n := countApplied(t); n != len(migrations()) {
		t.Fatalf("迁移记录 = %d 条，期望 %d 条", n, len(migrations()))
	}
}

// TestMigrationsAreIdempotentAcrossRestarts 重复启动不重复应用、不报错
func TestMigrationsAreIdempotentAcrossRestarts(t *testing.T) {
	setupMigrationTest(t)
	before := countApplied(t)
	if err := runMigrations(); err != nil {
		t.Fatalf("第二次运行失败: %v", err)
	}
	if err := runMigrations(); err != nil {
		t.Fatalf("第三次运行失败: %v", err)
	}
	if after := countApplied(t); after != before {
		t.Fatalf("重复运行把迁移又记了一遍: %d → %d", before, after)
	}
}

// TestNewMigrationAppliesOnce 增量迁移：只补未应用的版本
func TestNewMigrationAppliesOnce(t *testing.T) {
	setupMigrationTest(t)

	applied := 0
	list := append(migrations(), Migration{
		Version: 99,
		Name:    "测试增量",
		Apply: func() error {
			applied++
			return nil
		},
	})
	if err := runMigrationList(list); err != nil {
		t.Fatalf("增量迁移失败: %v", err)
	}
	if applied != 1 {
		t.Fatalf("增量迁移应执行 1 次，实际 %d", applied)
	}
	// 再跑一次不得重复执行
	if err := runMigrationList(list); err != nil {
		t.Fatalf("再次运行失败: %v", err)
	}
	if applied != 1 {
		t.Fatalf("增量迁移被重复执行: %d", applied)
	}
	if v, _ := SchemaVersion(); v != 99 {
		t.Fatalf("版本应为 99，实际 %d", v)
	}
}

// TestFailedMigrationIsNotRecorded 失败不记录：下次启动会重试（要求迁移幂等）
func TestFailedMigrationIsNotRecorded(t *testing.T) {
	setupMigrationTest(t)

	fail := append(migrations(), Migration{
		Version: 98,
		Name:    "会失败的迁移",
		Apply: func() error {
			return errors.New("模拟失败")
		},
	})
	err := runMigrationList(fail)
	if err == nil {
		t.Fatal("失败的迁移应返回错误（启动应中止）")
	}
	if v, _ := SchemaVersion(); v >= 98 {
		t.Fatalf("失败的迁移不应被记录，当前版本 %d", v)
	}
	// 修复后重试可成功
	ok := append(migrations(), Migration{Version: 98, Name: "修复版", Apply: func() error { return nil }})
	if err := runMigrationList(ok); err != nil {
		t.Fatalf("重试失败: %v", err)
	}
	if v, _ := SchemaVersion(); v != 98 {
		t.Fatalf("重试后版本应为 98，实际 %d", v)
	}
}

// TestMigrationWithoutImplementation 空实现必须报错而不是静默标记完成
func TestMigrationWithoutImplementation(t *testing.T) {
	setupMigrationTest(t)
	if err := runMigrationList(append(migrations(), Migration{Version: 97, Name: "空实现"})); err == nil {
		t.Fatal("没有 Apply 的迁移应报错")
	}
}
