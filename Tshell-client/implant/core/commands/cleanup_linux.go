//go:build linux

package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cleanupPersistItems Linux 持久化项：crontab 的 @reboot 行与 ~/.bashrc 的启动行。
//
// 两者的匹配都用"整行包含自身路径"：persist 写入的就是绝对路径，且不会被改写，
// 行级包含匹配既能删干净又不会误删同文件里其它正常内容。
func cleanupPersistItems(exe string) []cleanupItem {
	var items []cleanupItem
	items = append(items, cleanupCrontab(exe)...)
	items = append(items, cleanupShellRC(exe)...)
	return items
}

// cleanupCrontab 删除 crontab 中启动本程序的行。
// crontab -l 在"无任何条目"时返回非零退出码，这里按"没有条目"处理而不是失败。
func cleanupCrontab(exe string) []cleanupItem {
	if strings.TrimSpace(exe) == "" {
		return []cleanupItem{cSkip("未知自身路径，跳过 crontab")}
	}
	cur, err := exec.Command("crontab", "-l").Output()
	if err != nil {
		if len(strings.TrimSpace(string(cur))) == 0 {
			return []cleanupItem{cSkip("crontab 无条目")}
		}
		return []cleanupItem{cWarn("crontab 读取失败: %v", err)}
	}
	kept, removed := filterLinesReferencing(string(cur), exe)
	if len(removed) == 0 {
		return []cleanupItem{cSkip("crontab 无指向本程序的条目")}
	}
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(kept)
	if out, werr := cmd.CombinedOutput(); werr != nil {
		return []cleanupItem{cWarn("crontab 写回失败: %v: %s", werr, strings.TrimSpace(string(out)))}
	}
	return []cleanupItem{cOK("crontab 已删除 %d 条（%s）", len(removed), removed[0])}
}

// cleanupShellRC 删除 ~/.bashrc 中启动本程序的行。
func cleanupShellRC(exe string) []cleanupItem {
	if strings.TrimSpace(exe) == "" {
		return []cleanupItem{cSkip("未知自身路径，跳过 shell rc")}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return []cleanupItem{cSkip("无法定位用户目录，跳过 shell rc: %v", err)}
	}
	rcPath := filepath.Join(home, ".bashrc")
	data, rerr := os.ReadFile(rcPath)
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return []cleanupItem{cSkip("%s 不存在", rcPath)}
		}
		return []cleanupItem{cWarn("读取 %s 失败: %v", rcPath, rerr)}
	}
	kept, removed := filterLinesReferencing(string(data), exe)
	if len(removed) == 0 {
		return []cleanupItem{cSkip("%s 无指向本程序的行", rcPath)}
	}
	mode := os.FileMode(0o644)
	if st, serr := os.Stat(rcPath); serr == nil {
		mode = st.Mode().Perm()
	}
	if werr := os.WriteFile(rcPath, []byte(kept), mode); werr != nil {
		return []cleanupItem{cWarn("写回 %s 失败: %v", rcPath, werr)}
	}
	return []cleanupItem{cOK("%s 已删除 %d 行", rcPath, len(removed))}
}

// cleanupFileItems Linux 文件残留：<exe>.old。
func cleanupFileItems(exe string) []cleanupItem {
	return cleanupOldBinary(exe)
}

// cleanupPlatformItems Linux 无平台专属清理项。
func cleanupPlatformItems(exe string, sc cleanupScopes) []cleanupItem {
	return nil
}
