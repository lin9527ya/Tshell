package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestInsecurePermPaths 启动自检只报告"组/其他用户可访问"的敏感文件，0600 不报、缺失跳过。
func TestInsecurePermPaths(t *testing.T) {
	dir := t.TempDir()
	loose := filepath.Join(dir, "database.db")
	tight := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(loose, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tight, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope.pem")

	got := insecurePermPaths(tight, loose, missing, "")
	if len(got) != 1 || got[0] != loose {
		t.Fatalf("应只报告权限过宽的文件，实际: %v", got)
	}
}
