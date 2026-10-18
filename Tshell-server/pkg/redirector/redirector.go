// Package redirector 生成 nginx / apache 重定向器（redirector）配置。
//
// 重定向器是 C2 与目标之间的一层：只把**画像里声明的那几个 URI**转发到真正的
// 服务端，其余请求一律送给一个无害的诱饵站点。这样即使防守方顺着 beacon 的
// 回连地址摸过来，看到的也是一个正常站点，而 C2 服务端本身不直接暴露在公网。
//
// 配置由画像生成而不是手写：手写的重定向器最容易出的问题是与画像漂移
// （改了 uri 忘了改 nginx，beacon 直接失联，而现场只表现为"上线失败"）。
package redirector

import (
	"fmt"
	"regexp"
	"strings"

	"BackendTemplate/pkg/c2profile"
)

// 类型
const (
	TypeNginx  = "nginx"
	TypeApache = "apache"
)

// Types 支持的重定向器类型
var Types = []string{TypeNginx, TypeApache}

// Params 生成参数
type Params struct {
	Profile *c2profile.Profile
	// Backend 真正的 C2 服务端地址（host:port）。重定向器到服务端这一段
	// 建议走内网或隧道，不要暴露在公网。
	Backend string
	// ServerName 对外域名（TLS 证书名 / server_name）
	ServerName string
	// CertPath / KeyPath TLS 证书路径
	CertPath string
	KeyPath  string
	// DecoyURL 非画像请求的落点（诱饵站点），如 https://www.example.com
	DecoyURL string
	// Full 生成完整可用的配置文件（含 events/http 外壳），
	// 否则只生成可直接 include 的 server 片段
	Full bool
}

// Generate 按类型生成配置
func Generate(typ string, p Params) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	switch typ {
	case TypeNginx:
		if err := p.validateFullNginx(); err != nil {
			return "", err
		}
		return generateNginx(p), nil
	case TypeApache:
		// apache 的模块集按发行版差异极大，生成"完整配置"在别的发行版上装不起来，
		// 因此只产出 vhost 片段（见 generateApache 注释）
		if p.Full {
			return "", fmt.Errorf("apache 只生成 VirtualHost 片段（模块集按发行版而异，完整配置无法跨发行版通用）；请把它 include 进现有 apache 配置")
		}
		return generateApache(p), nil
	default:
		return "", fmt.Errorf("不支持的重定向器类型: %q（支持 %s）", typ, strings.Join(Types, "/"))
	}
}

// URIs 返回画像中需要转发的全部路径（http-get / http-post / websocket）
func URIs(p *c2profile.Profile) []string {
	out := make([]string, 0, 3)
	for _, u := range []string{p.HTTP.Get.URI, p.HTTP.Post.URI, p.WS.Path} {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if !strings.HasPrefix(u, "/") {
			u = "/" + u
		}
		out = append(out, u)
	}
	return out
}

// tokenRe 配置里出现的"外部输入"只允许这些字符。
// 配置注入不是理论问题：backend 写成 `127.0.0.1; } server { ...` 就能往
// 生成的配置里插任意指令，所以每个字段都要卡死字符集。
var tokenRe = regexp.MustCompile(`^[A-Za-z0-9._:\-\[\]]+$`)

func (p Params) validate() error {
	if p.Profile == nil {
		return fmt.Errorf("缺少画像（重定向器必须按画像生成，否则会与 URI 漂移）")
	}
	if strings.TrimSpace(p.Backend) == "" {
		return fmt.Errorf("缺少后端地址（-backend host:port）")
	}
	if !tokenRe.MatchString(p.Backend) {
		return fmt.Errorf("后端地址含非法字符: %q", p.Backend)
	}
	if strings.Contains(p.Backend, "://") {
		return fmt.Errorf("后端地址不要带协议前缀: %q", p.Backend)
	}
	if p.ServerName != "" && !tokenRe.MatchString(p.ServerName) {
		return fmt.Errorf("域名含非法字符: %q", p.ServerName)
	}
	if len(URIs(p.Profile)) == 0 {
		return fmt.Errorf("画像中没有可转发的路径（http-get/http-post/ws 全为空）")
	}
	if p.DecoyURL != "" && !strings.HasPrefix(p.DecoyURL, "http://") && !strings.HasPrefix(p.DecoyURL, "https://") {
		return fmt.Errorf("诱饵地址必须是 http(s) URL: %q", p.DecoyURL)
	}
	return nil
}

// validateFullNginx 独立 nginx 配置需要证书路径（自包含配置里没有别的地方声明证书）
func (p Params) validateFullNginx() error {
	if !p.Full {
		return nil
	}
	if p.CertPath == "" || p.KeyPath == "" {
		return fmt.Errorf("生成完整配置需要 -cert 与 -key（TLS 证书路径）")
	}
	if !strings.HasPrefix(p.CertPath, "/") || !strings.HasPrefix(p.KeyPath, "/") {
		return fmt.Errorf("证书路径必须是绝对路径")
	}
	return nil
}

// defaultServerName 未指定域名时的占位（提示运维替换）
const defaultServerName = "cdn.example.com"

// defaultDecoy 未指定诱饵时的落点
const defaultDecoy = "https://www.example.com"

func serverName(p Params) string {
	if p.ServerName != "" {
		return p.ServerName
	}
	return defaultServerName
}

func decoy(p Params) string {
	if p.DecoyURL != "" {
		return p.DecoyURL
	}
	return defaultDecoy
}
