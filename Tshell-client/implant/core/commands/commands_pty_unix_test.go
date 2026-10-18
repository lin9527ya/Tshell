//go:build linux || darwin

package commands

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// 交互式 PTY 在 macOS 上此前是"暂不支持"的桩；抽出为 Linux/macOS 共用实现后，
// 这里覆盖平台无关的机制：会话生命周期、特殊键转换、窗口大小调整。
// 真正的端到端交互（xterm 页面输入输出）依赖命令通道，见 scripts/e2e.sh。

func TestInteractiveShellLifecycle(t *testing.T) {
	out, err := StartInteractiveShell()
	if err != nil {
		t.Fatalf("启动交互式终端失败: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("启动应返回提示")
	}
	// 重复启动是幂等的（返回"已在运行"）
	if out2, err := StartInteractiveShell(); err != nil || !strings.Contains(string(out2), "already") {
		t.Fatalf("重复启动应幂等: %q %v", out2, err)
	}

	// 发送一行命令：不应报错（输出由通信层回送，单测里没有 transport 会被丢弃）
	if _, err := SendInteractiveInput([]byte("echo pty-lifecycle-ok\n")); err != nil {
		t.Fatalf("发送输入失败: %v", err)
	}
	// 调整窗口大小（协议：\x01 前缀 + JSON）
	sizeMsg, _ := json.Marshal(map[string]interface{}{"type": "resize", "cols": 120, "rows": 40})
	if _, err := SendInteractiveInput(append([]byte{0x01}, sizeMsg...)); err != nil {
		t.Fatalf("调整窗口大小失败: %v", err)
	}
	if ptyTerm := globalXTermPTY; ptyTerm != nil {
		ptyTerm.mutex.RLock()
		if ptyTerm.size.Cols != 120 || ptyTerm.size.Rows != 40 {
			t.Errorf("窗口大小未更新: %+v", ptyTerm.size)
		}
		ptyTerm.mutex.RUnlock()
	}

	// 停止
	if _, err := StopInteractiveShell(); err != nil {
		t.Fatalf("停止失败: %v", err)
	}
	// 停止后不应有活跃会话；重复停止是安全的
	if globalXTermPTY != nil {
		t.Fatal("停止后全局会话应被清理")
	}
	if _, err := StopInteractiveShell(); err != nil {
		t.Fatalf("重复停止应安全: %v", err)
	}
	// 停止后发送输入应报错（而不是静默丢弃）
	if _, err := SendInteractiveInput([]byte("x")); err == nil {
		t.Fatal("无会话时发送输入应报错")
	}
}

func TestProcessSpecialKeys(t *testing.T) {
	ptyTerm := &XTermPTY{size: TerminalSize{Rows: 24, Cols: 80}}
	cases := []struct {
		in   []byte
		want string
	}{
		{[]byte{0x1b}, "\x1b"}, // ESC 原样
		{[]byte{0x0d}, "\r"},   // 回车透传（转 CRLF 会让 bash 收到两个换行）
		{[]byte{0x7f}, "\x08"}, // DEL → BS（历史行为，保持与 Linux 一致）
		{[]byte{' '}, " "},
		{[]byte{'a'}, "a"},
		{[]byte{0x1b, 0x5b, 0x41}, "\x1b[A"}, // 上
		{[]byte{0x1b, 0x5b, 0x42}, "\x1b[B"}, // 下
		{[]byte{0x1b, 0x5b, 0x43}, "\x1b[C"}, // 右
		{[]byte{0x1b, 0x5b, 0x44}, "\x1b[D"}, // 左
	}
	for _, c := range cases {
		if got := string(ptyTerm.processSpecialKeys(c.in)); got != c.want {
			t.Errorf("processSpecialKeys(% x) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestPTYShellPathPerPlatform 默认 shell 必须存在（Linux bash / macOS zsh）
func TestPTYShellPathPerPlatform(t *testing.T) {
	path := interactiveShellPath()
	if !strings.HasPrefix(path, "/bin/") {
		t.Fatalf("shell 路径异常: %q", path)
	}
	if _, err := StartInteractiveShell(); err != nil {
		t.Fatalf("用 %s 启动交互式终端失败: %v", path, err)
	}
	defer StopInteractiveShell()
	time.Sleep(100 * time.Millisecond) // 让 shell 完成启动
	if ptyTerm := globalXTermPTY; ptyTerm == nil {
		t.Fatal("会话未建立")
	}
}
