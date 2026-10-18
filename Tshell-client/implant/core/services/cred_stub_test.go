//go:build !windows

package services

import "testing"

// 非 Windows 平台必须**显式报错**：静默返回空路径会让服务端把空制品当成"已获取"，
// 操作员在解析时才发现是废文件 —— 凭据获取的失败必须是显式的。
func TestDumpCredentialUnsupportedOnNonWindows(t *testing.T) {
	path, err := dumpCredential("lsass", "auto", "x.dmp")
	if err == nil {
		t.Fatalf("非 Windows 平台应报错，实际返回 %q", path)
	}
	if path != "" {
		t.Fatalf("失败时不应返回路径: %q", path)
	}
}
