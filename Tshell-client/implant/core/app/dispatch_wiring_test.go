package app

import (
	"os"
	"strings"
	"testing"
)

// TestFileContentDispatched 守卫 cmdType 19 的接线。
// 修复前 types.go 定义了 FileContent = 19 但 dispatch.go 没有对应 case，
// 服务端下发（控制台 `filecontent <path>`）后客户端一律回 "not supported command: 19"，
// 而控制台帮助里却列着这个命令。单测只覆盖 CmdFileContent 本体，接线本身需要在这里钉住。
func TestFileContentDispatched(t *testing.T) {
	src, err := os.ReadFile("dispatch.go")
	if err != nil {
		t.Fatalf("读取 dispatch.go 失败: %v", err)
	}
	if !strings.Contains(string(src), "case services.FileContent:") {
		t.Error("dispatch.go 缺少 `case services.FileContent:` —— cmdType 19 会被回 not supported")
	}
	if !strings.Contains(string(src), "commands.CmdFileContent(cmdBuf)") {
		t.Error("dispatch.go 的 FileContent 分支没有调用 commands.CmdFileContent")
	}
}

// TestGetSystemDispatched 守卫提权命令的接线。
// 与 cmdType 19 同类问题：types.go 定义了常量、服务端前缀表能下发，
// 但 dispatch.go 少一个 case 就变成"下发后一律回 not supported"。
// 提权还额外要求**跳过节流**（见下面的豁免名单断言）：提权动作对时序敏感，
// 被 WaitTime 拖住会让"执行了但半天没反应"。
func TestGetSystemDispatched(t *testing.T) {
	src, err := os.ReadFile("dispatch.go")
	if err != nil {
		t.Fatalf("读取 dispatch.go 失败: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "case services.GETSYSTEM:") {
		t.Error("dispatch.go 缺少 `case services.GETSYSTEM:` —— 提权命令会被回 not supported")
	}
	if !strings.Contains(body, "commands.CmdGetSystem(cmdBuf)") {
		t.Error("dispatch.go 的 GETSYSTEM 分支没有调用 commands.CmdGetSystem")
	}
	// 节流豁免：豁免名单里必须出现 GETSYSTEM
	if !strings.Contains(body, "cmdType != services.GETSYSTEM") {
		t.Error("dispatch.go 的命令节流豁免名单缺少 GETSYSTEM —— 提权会被 WaitTime 拖住")
	}
}
