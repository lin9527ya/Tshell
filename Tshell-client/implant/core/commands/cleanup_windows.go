//go:build windows

package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const cleanupRunKeyPath = `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`

// cleanupPersistItems Windows 持久化项：注册表 Run / 计划任务 / 启动文件夹。
//
// 匹配原则是"指向本程序路径"而不是"名字等于某个固定值"：persist 的对象名由
// 生成时 patch 的伪装修饰前缀决定，运行期无法反推，按路径反查才不会漏。
func cleanupPersistItems(exe string) []cleanupItem {
	var items []cleanupItem
	items = append(items, cleanupRegistryRun(exe)...)
	items = append(items, cleanupScheduledTasks(exe)...)
	items = append(items, cleanupStartupFolder(exe)...)
	return items
}

// cleanupRegistryRun 删除 HKCU Run 下数据指向本程序的项。
func cleanupRegistryRun(exe string) []cleanupItem {
	if strings.TrimSpace(exe) == "" {
		return []cleanupItem{cSkip("未知自身路径，跳过注册表 Run")}
	}
	out, err := exec.Command("reg", "query", cleanupRunKeyPath).CombinedOutput()
	if err != nil {
		// 键不存在（全新系统未写过 Run）与权限不足都会走到这里
		return []cleanupItem{cSkip("注册表 Run 不可读: %v", strings.TrimSpace(string(out)))}
	}
	names := regRunValueNames(string(out), exe)
	if len(names) == 0 {
		return []cleanupItem{cSkip("注册表 Run 无指向本程序的项")}
	}
	items := make([]cleanupItem, 0, len(names))
	for _, n := range names {
		if derr := exec.Command("reg", "delete", cleanupRunKeyPath, "/v", n, "/f").Run(); derr != nil {
			items = append(items, cWarn("注册表 Run\\%s 删除失败: %v", n, derr))
			continue
		}
		items = append(items, cOK("注册表 Run\\%s 已删除", n))
	}
	return items
}

// cleanupScheduledTasks 删除 Task To Run 指向本程序的计划任务。
// /fo list /v 是全量导出（机器上任务多时要几秒），但非 -v 的导出不含命令行，
// 无法按路径匹配——单次清理命令的几秒延迟可以接受。
func cleanupScheduledTasks(exe string) []cleanupItem {
	if strings.TrimSpace(exe) == "" {
		return []cleanupItem{cSkip("未知自身路径，跳过计划任务")}
	}
	out, err := exec.Command("schtasks", "/query", "/fo", "list", "/v").CombinedOutput()
	if err != nil && len(out) == 0 {
		return []cleanupItem{cSkip("计划任务不可枚举: %v", err)}
	}
	names := taskNamesForExe(string(out), exe)
	if len(names) == 0 {
		return []cleanupItem{cSkip("计划任务无指向本程序的项")}
	}
	items := make([]cleanupItem, 0, len(names))
	for _, n := range names {
		if derr := exec.Command("schtasks", "/delete", "/tn", n, "/f").Run(); derr != nil {
			items = append(items, cWarn("计划任务 %s 删除失败: %v", n, derr))
			continue
		}
		items = append(items, cOK("计划任务 %s 已删除", n))
	}
	return items
}

// cleanupStartupFolder 删除启动文件夹里本程序的副本。
func cleanupStartupFolder(exe string) []cleanupItem {
	dir := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []cleanupItem{cSkip("启动文件夹不可读: %v", err)}
	}
	var exeSize int64
	if st, serr := os.Stat(exe); serr == nil {
		exeSize = st.Size()
	}
	items := make([]cleanupItem, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		if !startupFileMatches(e.Name(), info.Size(), exe, exeSize) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if rerr := os.Remove(p); rerr != nil {
			items = append(items, cWarn("启动项 %s 删除失败: %v", e.Name(), rerr))
			continue
		}
		items = append(items, cOK("启动项 %s 已删除", e.Name()))
	}
	if len(items) == 0 {
		return []cleanupItem{cSkip("启动文件夹无本程序副本")}
	}
	return items
}

// cleanupFileItems 文件残留：<exe>.old 与 %TEMP% 下的更新临时文件。
func cleanupFileItems(exe string) []cleanupItem {
	items := cleanupOldBinary(exe)
	residues := findUpdateResidues(os.TempDir())
	if len(residues) == 0 {
		return append(items, cSkip("临时目录无更新临时文件"))
	}
	removed := 0
	for _, p := range residues {
		if err := os.Remove(p); err == nil || os.IsNotExist(err) {
			removed++
		}
	}
	if removed < len(residues) {
		return append(items, cWarn("临时目录更新残留 %d/%d 个已删除（其余被占用）", removed, len(residues)))
	}
	return append(items, cOK("临时目录更新残留 %d 个已删除", removed))
}

// cleanupPlatformItems Windows 附加项：UAC 劫持键（getsystem uac 副产物）与 WMI 订阅。
func cleanupPlatformItems(exe string, sc cleanupScopes) []cleanupItem {
	var items []cleanupItem
	if sc.uac {
		out, err := uacCleanupReport()
		if err != nil {
			items = append(items, cWarn("UAC 劫持残留清理失败: %v", err))
		} else {
			items = append(items, cOK("%s", trimReplyMarker(out)))
		}
	}
	if sc.wmi {
		out, err := wmiPersistRemove()
		if err != nil {
			items = append(items, cWarn("WMI 订阅清理失败: %v", err))
		} else {
			items = append(items, cOK("%s", trimReplyMarker(out)))
		}
	}
	return items
}
