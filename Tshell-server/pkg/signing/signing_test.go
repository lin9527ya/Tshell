//go:build !windows

package signing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeSigner 造一个假签名器：把 {{in}} 复制到 {{out}} 并追加一行标记。
// 参数约定（测试自定义）：argv[1]=in argv[2]=out。
func fakeSigner(t *testing.T, script string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "signer.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSignDisabled 未启用时原样返回（调用方无需分支）
func TestSignDisabled(t *testing.T) {
	Configure(Config{Enabled: false})
	in := []byte("PE-bytes")
	out, err := Sign(in)
	if err != nil || string(out) != string(in) {
		t.Fatalf("未启用时应原样返回: %v %q", err, out)
	}
}

// TestSignInvokesCommand 启用时按参数模板调用外部命令并读回 {{out}}
func TestSignInvokesCommand(t *testing.T) {
	bin := fakeSigner(t, `cp "$1" "$2"; echo SIGNED >> "$2"`)
	Configure(Config{
		Enabled: true, Command: bin,
		Args: []string{"{{in}}", "{{out}}"},
		PFX:  "/tmp/x.pfx", Password: "pw",
		Timeout: 5 * time.Second,
	})
	t.Cleanup(func() { Configure(Config{}) })

	out, err := Sign([]byte("PE-bytes"))
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}
	if !strings.Contains(string(out), "PE-bytes") || !strings.Contains(string(out), "SIGNED") {
		t.Fatalf("输出不是签名器写的内容: %q", out)
	}
}

// TestSignCommandFails 签名器失败必须报错（含 stderr 摘要），不能静默返回未签名数据
func TestSignCommandFails(t *testing.T) {
	bin := fakeSigner(t, `echo "cert not found" >&2; exit 3`)
	Configure(Config{Enabled: true, Command: bin, Args: []string{"{{in}}", "{{out}}"}, Timeout: 5 * time.Second})
	t.Cleanup(func() { Configure(Config{}) })

	if _, err := Sign([]byte("x")); err == nil || !strings.Contains(err.Error(), "cert not found") {
		t.Fatalf("应返回含 stderr 的错误: %v", err)
	}
}

// TestSignCommandNoOutput 命令成功但没写 {{out}} → 明确报错
func TestSignCommandNoOutput(t *testing.T) {
	bin := fakeSigner(t, `exit 0`)
	Configure(Config{Enabled: true, Command: bin, Args: []string{"{{in}}", "{{out}}"}, Timeout: 5 * time.Second})
	t.Cleanup(func() { Configure(Config{}) })

	if _, err := Sign([]byte("x")); err == nil || !strings.Contains(err.Error(), "签名输出不存在") {
		t.Fatalf("应报输出缺失: %v", err)
	}
}

// TestValidate 启用签名的配置校验：缺 pfx / pfx 不可读 / 命令不存在都要显式失败
func TestValidate(t *testing.T) {
	if err := Validate(Config{Enabled: false}); err != nil {
		t.Fatalf("未启用不应校验: %v", err)
	}
	dir := t.TempDir()
	pfx := filepath.Join(dir, "c.pfx")
	if err := os.WriteFile(pfx, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := fakeSigner(t, "exit 0")

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"缺 pfx", Config{Enabled: true, Command: bin}, "未配置 signing.pfx"},
		{"pfx 不存在", Config{Enabled: true, Command: bin, PFX: filepath.Join(dir, "no.pfx")}, "不可读"},
		{"命令不存在", Config{Enabled: true, Command: filepath.Join(dir, "nope"), PFX: pfx}, "找不到签名命令"},
	}
	for _, c := range cases {
		err := Validate(c.cfg)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: 期望包含 %q，实际 %v", c.name, c.want, err)
		}
	}
	if err := Validate(Config{Enabled: true, Command: bin, PFX: pfx}); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
}

// TestConfigureDefaults 空命令/零超时回落到默认值
func TestConfigureDefaults(t *testing.T) {
	Configure(Config{Enabled: true})
	defer Configure(Config{})
	cur := Current()
	if cur.Command != DefaultCommand {
		t.Fatalf("命令应回落到 %s，实际 %q", DefaultCommand, cur.Command)
	}
	if cur.Timeout <= 0 {
		t.Fatalf("超时应回落到正值: %v", cur.Timeout)
	}
}
