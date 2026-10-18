package cli

import (
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/redirector"
	"fmt"
	"os"
	"strings"
)

// 重定向器配置生成 CLI：`./tshell -gen-redirector nginx -backend 10.0.0.5:8089 ...`
//
// 配置由**当前画像**生成，而不是让运维手写：手写的重定向器最容易与画像漂移
// （改了 uri 忘了改 nginx，beacon 直接失联，现场只表现为"上线失败"）。
func RunGenRedirector(typ string, opts RedirectorOptions) int {
	profileName := opts.Profile
	if profileName == "" {
		profileName = "default"
	}
	profile := c2profile.Get(profileName)
	if profile == nil {
		fmt.Printf("❌ 找不到画像 %q（可用: %s）\n", profileName, strings.Join(c2profile.List(), ", "))
		return 1
	}

	out, err := redirector.Generate(typ, redirector.Params{
		Profile:    profile,
		Backend:    opts.Backend,
		ServerName: opts.Domain,
		CertPath:   opts.Cert,
		KeyPath:    opts.Key,
		DecoyURL:   opts.Decoy,
		Full:       opts.Full,
	})
	if err != nil {
		fmt.Printf("❌ 生成失败: %v\n", err)
		return 1
	}

	if opts.Out == "" {
		fmt.Print(out)
		return 0
	}
	if err := os.WriteFile(opts.Out, []byte(out), 0o644); err != nil {
		fmt.Printf("❌ 写入 %s 失败: %v\n", opts.Out, err)
		return 1
	}
	fmt.Printf("✅ 已生成 %s（画像: %s，后端: %s）\n", opts.Out, profileName, opts.Backend)
	fmt.Printf("   部署前请按 docs/REDIRECTOR.md 校验：nginx -t -c %s\n", opts.Out)
	return 0
}

// RedirectorOptions 重定向器生成参数
type RedirectorOptions struct {
	Backend string
	Domain  string
	Cert    string
	Key     string
	Decoy   string
	Profile string
	Out     string
	Full    bool
}
