package main

// 交互终端快轮询（P5）的接线棘轮。
//
// 轮询传输下"一个字符 = 一条命令 = 一轮 GET"，若空闲轮询仍按信标节奏（sleep），
// 终端就是哑终端。这里钉住三件事：用了会话活跃判定、缩短后的间隔是 150ms、
// 并且会话结束后会回落（else 分支复位 fastPollActive）。

import (
	"os"
	"strings"
	"testing"
)

func TestRunloopFastPollsWhileTerminalActive(t *testing.T) {
	src, err := os.ReadFile("runloop.go")
	if err != nil {
		t.Fatalf("读取 runloop.go 失败: %v", err)
	}
	s := strings.ReplaceAll(string(src), "\r\n", "\n")

	if !strings.Contains(s, "commands.InteractiveShellActive()") {
		t.Error("runloop.go 没有判断 commands.InteractiveShellActive() —— 终端活跃时不会快轮询")
	}
	if !strings.Contains(s, "const fastPollInterval = 150 * time.Millisecond") {
		t.Error("fastPollInterval 未定义或数值被改动（预期 150ms）")
	}
	// 必须在轮询分支里、在 SleepMask 之前生效
	if !strings.Contains(s, "interval = fastPollInterval") {
		t.Error("runloop.go 没有把轮询间隔降到 fastPollInterval")
	}
	if !strings.Contains(s, "fastPollActive = false") {
		t.Error("runloop.go 缺少会话结束后的复位 —— 终端关掉后仍会一直快轮询")
	}
}

// 客户端与潜在的平台差异：两个平台都必须实现 InteractiveShellActive，
// 否则交叉编译会在不认识该符号的目标上失败（windows 用 ConPTY、unix 用 PTY）。
func TestInteractiveShellActiveImplementedPerPlatform(t *testing.T) {
	files := []string{
		"../../core/commands/conpty_windows.go",
		"../../core/commands/commands_pty_unix.go",
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", f, err)
		}
		if !strings.Contains(string(b), "func InteractiveShellActive() bool {") {
			t.Errorf("%s 缺少 InteractiveShellActive() 实现", f)
		}
	}
}
