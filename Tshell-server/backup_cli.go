package main

import (
	"BackendTemplate/pkg/backup"
	"BackendTemplate/pkg/database"
	"fmt"
)

// 备份/恢复 CLI 入口（`./tshell -backup` / `-restore` / `-list-backups`）。
//
// 为什么恢复不做成 HTTP 接口：SQLite 的数据库就是那个文件，运行中的进程持有它
// 以及 -wal/-shm，热替换会把两边都弄坏。运维上正确的形态就是"停服务 → 恢复 → 起服务"，
// 这里把它做成一条命令，避免运维手工 cp 出错（漏删 -wal 是最常见的坑）。
// 步骤见 docs/BACKUP_RESTORE.md。

func runListBackups(m *backup.Manager) int {
	list, err := m.List()
	if err != nil {
		fmt.Printf("读取备份目录失败: %v\n", err)
		return 1
	}
	if len(list) == 0 {
		fmt.Printf("备份目录 %s 中没有备份\n", m.Options().Dir)
		return 0
	}
	fmt.Printf("备份目录: %s（保留 %d 份）\n", m.Options().Dir, m.Options().Keep)
	for _, info := range list {
		state := "未校验"
		if info.Verified {
			state = "已校验"
		}
		fmt.Printf("  %s  %8.1f MB  %s  %s  %s\n",
			info.Name, float64(info.Size)/1024/1024,
			info.CreatedAt.Format("2006-01-02 15:04:05"), state, info.Note)
	}
	return 0
}

func runBackupNow(m *backup.Manager) int {
	info, err := m.Create("cli")
	if err != nil {
		fmt.Printf("❌ 备份失败: %v\n", err)
		return 1
	}
	fmt.Printf("✅ 备份完成: %s (%.1f MB)\n", info.Path, float64(info.Size)/1024/1024)
	if removed, err := m.Cleanup(); err != nil {
		fmt.Printf("⚠️ 清理旧备份失败: %v\n", err)
	} else if len(removed) > 0 {
		fmt.Printf("🗑️ 按保留策略删除 %d 份旧备份\n", len(removed))
	}
	return 0
}

func runRestore(m *backup.Manager, arg, dbPath string, force bool) int {
	backupPath, err := backup.ResolveBackup(m, arg)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return 1
	}
	fmt.Printf("从 %s 恢复到 %s ...\n", backupPath, dbPath)
	safety, err := backup.Restore(backupPath, dbPath, force)
	if err != nil {
		fmt.Printf("❌ 恢复失败: %v\n", err)
		if safety != "" {
			fmt.Printf("   现有数据未被改动，恢复前快照: %s\n", safety)
		}
		return 1
	}
	fmt.Printf("✅ 恢复完成\n")
	if safety != "" {
		fmt.Printf("   恢复前快照: %s（确认无误后可删除）\n", safety)
	}
	fmt.Printf("   现在可以启动服务端: ./tshell -config <配置文件>\n")
	return 0
}

// 让 database 包被引用（恢复路径使用 database.DatabaseName 作为默认库路径）
var _ = database.DatabaseName
