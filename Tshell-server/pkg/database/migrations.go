package database

import (
	"fmt"
	"time"

	"BackendTemplate/pkg/utils/logx"
)

// 版本化 schema 迁移。
//
// 原来只有两个"检查列是否存在 → ALTER"的函数，靠 xorm Sync2 兜底建表：
// 没有版本记录，就不知道一个库"到哪一步了"，也无法安全地加后续变更
// （每次都要写一遍"列在不在"的检查，改错了只能靠人肉判断）。
//
// 这里引入 schema_migrations 表：已应用的版本落库，启动时按序补齐缺口。
// 约定：
//   - 迁移按 Version 升序执行，**必须幂等**（用"列存在则跳过"这类检查，
//     而不是假设一定没跑过）—— SQLite 的 DDL 虽然在事务里，但迁移函数可能
//     跨多条语句，幂等是失败重试的安全底线；
//   - 每个迁移成功后立刻记录版本；失败则中止整个启动（schema 不对就别带病运行）。

// Migration 一条 schema 变更
type Migration struct {
	Version int
	Name    string
	Apply   func() error
}

// migrations 迁移清单（新增变更只追加、不改已发布的版本号）
func migrations() []Migration {
	return []Migration{
		{1, "clients 表补齐 listener/os/心跳等字段", migrateClientsTable},
		{2, "listener 表补齐字段", migrateListenerTable},
		{3, "users 表补齐 MFA/TOTP 字段", migrateUsersMFATable},
		{4, "listener 表补齐 profile 字段", migrateListenerProfileColumn},
	}
}

// SchemaVersion 返回当前已应用的最高版本（未初始化时返回 0）
func SchemaVersion() (int, error) {
	if Engine == nil {
		return 0, fmt.Errorf("数据库引擎未初始化")
	}
	exists, err := Engine.IsTableExist("schema_migrations")
	if err != nil || !exists {
		return 0, err
	}
	rows, err := Engine.QueryString("SELECT COALESCE(MAX(version), 0) AS v FROM schema_migrations")
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	var v int
	fmt.Sscanf(rows[0]["v"], "%d", &v)
	return v, nil
}

// runMigrations 应用所有未执行的迁移（使用内置清单）
func runMigrations() error {
	return runMigrationList(migrations())
}

// runMigrationList 应用给定的迁移清单（抽出来便于测试失败与增量场景）
func runMigrationList(list []Migration) error {
	if Engine == nil {
		return fmt.Errorf("数据库引擎未初始化")
	}
	// DATETIME 是 SQLite 的类型名，PostgreSQL 用 TIMESTAMP
	tsType := "DATETIME"
	if IsPostgres() {
		tsType = "TIMESTAMP"
	}
	if _, err := Engine.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at %s NOT NULL
	)`, tsType)); err != nil {
		return fmt.Errorf("创建 schema_migrations 失败: %v", err)
	}

	applied := map[int]bool{}
	rows, err := Engine.QueryString("SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("读取已应用版本失败: %v", err)
	}
	for _, r := range rows {
		var v int
		fmt.Sscanf(r["version"], "%d", &v)
		applied[v] = true
	}

	for _, m := range list {
		if applied[m.Version] {
			continue
		}
		if m.Apply == nil {
			return fmt.Errorf("迁移 %d(%s) 没有实现", m.Version, m.Name)
		}
		if err := m.Apply(); err != nil {
			// 不记录版本：下次启动会重试该迁移（要求迁移本身幂等）
			return fmt.Errorf("迁移 %d(%s) 失败: %v", m.Version, m.Name, err)
		}
		if _, err := ExecSQL(
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			m.Version, m.Name, time.Now()); err != nil {
			return fmt.Errorf("记录迁移 %d 失败: %v", m.Version, err)
		}
		logx.Infof("🗃️ [迁移] 已应用 v%d: %s\n", m.Version, m.Name)
	}
	return nil
}
