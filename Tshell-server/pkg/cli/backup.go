package cli

import (
	"BackendTemplate/pkg/backup"
	"fmt"
)

// Package cli 服务端的命令行入口（备份/恢复、重定向器生成等）。
//
// 单独成包而不是放在 main 包：服务端的权威构建入口 build.sh 用**文件模式**
// (`go build ./main.go`)——这是 Windows+TDM-GCC 下验证过的唯一可用形态。
// 文件模式只编译被点名的那个文件，main 包里再多一个 .go 就会编译失败，
// 因此 main 包必须保持单文件，CLI 实现放在这里由 main.go 调用。
//
// 备份/恢复 CLI 入口（`./tshell -backup` / `-restore` / `-list-backups`）。
//
// 为什么恢复不做成 HTTP 接口：SQLite 的数据库就是那个文件，运行中的进程持有它
// 以及 -wal/-shm，热替换会把两边都弄坏。运维上正确的形态就是"停服务 → 恢复 → 起服务"，
// 这里把它做成一条命令，避免运维手工 cp 出错（漏删 -wal 是最常见的坑）。
// 步骤见 docs/BACKUP_RESTORE.md。

func RunListBackups(m *backup.Manager) int {
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

func RunBackupNow(m *backup.Manager) int {
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

func RunRestore(m *backup.Manager, arg, dbPath string, force bool) int {
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
