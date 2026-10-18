package security

import "github.com/gin-gonic/gin"

// TrustedProxyCIDRs 可信代理网段：回环 + 私网。
//
// gin 默认信任**任意来源**的 X-Forwarded-For（TrustedProxies 为空时等价于 0.0.0.0/0），
// 于是直连部署下任何远程请求都能靠 `X-Forwarded-For: 1.2.3.4` 决定自己被记录成谁：
// 审计日志的 client_ip、在线操作员的 last_ip、错误响应里的来源 IP 全部可被伪造。
// 这与 pkg/security 自己的 getRealIP（只在**对端**是回环/私网时才采信转发头）不一致 ——
// 这里把 gin 的口径对齐到同一套纪律：只有来自可信代理的连接，其转发头才被采信。
var TrustedProxyCIDRs = []string{
	"127.0.0.0/8", "::1/128",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7",
}

// ApplyTrustedProxies 应用可信代理网段（main 启动时调用）。
// 失败不阻断启动：退回 gin 默认行为并在调用方记录告警。
func ApplyTrustedProxies(r *gin.Engine) error {
	return r.SetTrustedProxies(TrustedProxyCIDRs)
}
