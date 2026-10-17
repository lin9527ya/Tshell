package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCmdFileContent 修复前 cmdType 19 在客户端没有 dispatch 分支，
// 服务端下发后一律回 "not supported command: 19"（控制台帮助里却列着这个命令）。
func TestCmdFileContent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("hello-file-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := CmdFileContent([]byte(p))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(got) != "hello-file-content" {
		t.Fatalf("内容不符: %q", got)
	}

	// 服务端 rawArg 传的是命令后的原文，末尾可能带空白
	got, err = CmdFileContent([]byte("  " + p + "\n"))
	if err != nil {
		t.Fatalf("带空白路径读取失败: %v", err)
	}
	if string(got) != "hello-file-content" {
		t.Fatalf("带空白路径内容不符: %q", got)
	}
}

func TestCmdFileContentErrors(t *testing.T) {
	if _, err := CmdFileContent(nil); err == nil {
		t.Error("空路径应返回用法错误")
	} else if !strings.Contains(err.Error(), "用法") {
		t.Errorf("空路径错误信息应含用法提示: %v", err)
	}
	if _, err := CmdFileContent([]byte(filepath.Join(t.TempDir(), "nope.txt"))); err == nil {
		t.Error("不存在的文件应返回错误")
	}
}

func TestCmdFileContentTruncates(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.bin")
	big := bytes.Repeat([]byte("A"), maxFileContentBytes+4096)
	if err := os.WriteFile(p, big, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := CmdFileContent([]byte(p))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(got) >= len(big) {
		t.Fatalf("超大文件未截断: %d >= %d", len(got), len(big))
	}
	if !strings.Contains(string(got), "已截断") {
		t.Fatalf("缺少截断标记")
	}
	if !strings.Contains(string(got), "仅显示前") {
		t.Fatalf("缺少截断说明")
	}
}
