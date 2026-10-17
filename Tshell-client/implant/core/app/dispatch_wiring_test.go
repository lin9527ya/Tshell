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
