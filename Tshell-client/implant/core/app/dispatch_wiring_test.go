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

// TestCleanupDispatched 守卫 CLEANUP(134) 的接线。
// 清理现场是"任务做完了要走人"的最后一步，接线断了的表现是操作员以为清干净了、
// 实际持久化项还在机器上——比命令直接报错危险得多，所以单独钉住。
func TestCleanupDispatched(t *testing.T) {
	src, err := os.ReadFile("dispatch.go")
	if err != nil {
		t.Fatalf("读取 dispatch.go 失败: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "case services.CLEANUP:") {
		t.Error("dispatch.go 缺少 `case services.CLEANUP:` —— 清理命令会被回 not supported")
	}
	if !strings.Contains(body, "services.CmdCleanup(cmdBuf)") {
		t.Error("dispatch.go 的 CLEANUP 分支没有调用 services.CmdCleanup")
	}
	// 范围串原样透传：cleanup persist / cleanup files 这类参数必须能到 CmdCleanup
	svc, err := os.ReadFile("../services/cleanup.go")
	if err != nil {
		t.Fatalf("读取 services/cleanup.go 失败: %v", err)
	}
	if !strings.Contains(string(svc), "commands.Cleanup(") {
		t.Error("services.CmdCleanup 没有把范围串交给 commands.Cleanup")
	}
}

// TestCleanupPurgedOnStartup 守卫"更新残留自清理"的启动钩子。
// 自更新残留（<exe>.old）留在盘上就是一条显眼的取证线索，清理必须挂在启动路径上，
// 只在命令里做意味着"没发过 cleanup 的机器永远不清理"。
func TestCleanupPurgedOnStartup(t *testing.T) {
	src, err := os.ReadFile("init.go")
	if err != nil {
		t.Fatalf("读取 init.go 失败: %v", err)
	}
	body := string(src)
	if !strings.Contains(body, "commands.PurgeUpdateResidue()") {
		t.Error("init.go 没有调用 commands.PurgeUpdateResidue() —— <exe>.old 不会被自动清理")
	}
	if !strings.Contains(body, "go commands.PurgeUpdateResidue()") {
		t.Error("PurgeUpdateResidue 未异步执行 —— 会被文件锁/前台启动路径阻塞")
	}
}
