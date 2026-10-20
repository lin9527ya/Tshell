//go:build darwin

package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cleanupPersistItems macOS 持久化项：LaunchAgents / LaunchDaemons 里启动本程序的 plist。
//
// 按 plist 内容里是否出现自身真实路径反查（persist 写入的是 EvalSymlinks 后的路径），
// 命中后先 launchctl unload 再删文件——只删文件不卸载，本次登录周期内 launchd
// 仍持有该任务，表现为"删了但进程还在起"。
func cleanupPersistItems(exe string) []cleanupItem {
	var items []cleanupItem
	items = append(items, cleanupLaunchdDir(launchAgentDir(), "LaunchAgents", exe)...)
	items = append(items, cleanupLaunchdDir("/Library/LaunchDaemons", "LaunchDaemons", exe)...)
	return items
}

// launchAgentDir 当前用户的 LaunchAgents 目录。
func launchAgentDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Library", "LaunchAgents")
}

// cleanupLaunchdDir 删除 dir 下引用了 exe 的 plist（并先卸载）。
func cleanupLaunchdDir(dir, label, exe string) []cleanupItem {
	if strings.TrimSpace(dir) == "" {
		return []cleanupItem{cSkip("无法定位 %s 目录", label)}
	}
	if strings.TrimSpace(exe) == "" {
		return []cleanupItem{cSkip("未知自身路径，跳过 %s", label)}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []cleanupItem{cSkip("%s 不可读: %v", label, err)}
	}
	items := make([]cleanupItem, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".plist") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			continue
		}
		if !plistReferencesPath(string(data), exe) {
			continue
		}
		// 未载入时 unload 返回非零，属正常；只关心文件是否删掉
		_ = exec.Command("launchctl", "unload", p).Run()
		if derr := os.Remove(p); derr != nil {
			items = append(items, cWarn("%s\\%s 删除失败: %v", label, e.Name(), derr))
			continue
		}
		items = append(items, cOK("%s\\%s 已卸载并删除", label, e.Name()))
	}
	if len(items) == 0 {
		return []cleanupItem{cSkip("%s 无指向本程序的 plist", label)}
	}
	return items
}

// cleanupFileItems macOS 文件残留：<exe>.old。
func cleanupFileItems(exe string) []cleanupItem {
	return cleanupOldBinary(exe)
}

// cleanupPlatformItems macOS 无平台专属清理项。
func cleanupPlatformItems(exe string, sc cleanupScopes) []cleanupItem {
	return nil
}
