package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config 中继节点配置（JSON，纯标准库解析——中继是部署在一次性 VPS 上的小工具，
// 不应该引入任何第三方依赖）。
type Config struct {
	// Listen TCP 监听地址（转发 TCP/WebSocket/HTTP(S) 等持久或轮询传输）
	Listen string `json:"listen"`
	// Upstream 上游地址 host:port（可以是最终服务端，也可以是另一个中继 —— 多级中继）
	Upstream string `json:"upstream"`

	// Upstreams 多上游列表：非空时**取代** Upstream，按轮询分发并在失败时转移。
	// 单上游的中继本身就是单点，而中继多半部署在没人盯着的 VPS 上 —— 上游挂一个
	// 就整条链路断掉。配两个及以上上游才有意义。
	Upstreams []string `json:"upstreams,omitempty"`

	// UDPListen/UDPUpstream 可选中继 KCP 传输（UDP）
	UDPListen   string `json:"udp_listen,omitempty"`
	UDPUpstream string `json:"udp_upstream,omitempty"`

	// DNSListen/DNSUpstream(s) 可选中继 DNS 传输（C2 的 DNS 通道）。
	//
	// 与 UDP/KCP 的裸转发不同，DNS 中继是**懂协议**的：它逐查询改写事务 ID
	// （多 beacon 共用中继时 ID 会撞车，响应会串到错的客户端），并在响应被截断
	// （TC 位）时自动改用 TCP 重问。因此 DNS 中继多花了这点复杂度。
	DNSListen    string   `json:"dns_listen,omitempty"`
	DNSUpstream  string   `json:"dns_upstream,omitempty"`
	DNSUpstreams []string `json:"dns_upstreams,omitempty"`

	// HealthListen 健康检查监听地址（/healthz），建议只绑回环
	HealthListen string `json:"health_listen,omitempty"`

	// UpstreamHealthInterval 上游健康探测间隔（0 = 30s）。
	// 只探"已被标记不健康"的上游：健康上游由真实流量证明。
	UpstreamHealthInterval Duration `json:"upstream_health_interval,omitempty"`

	// TLS 可选：在中继上终止 TLS（转发到服务端时是明文）。
	// 命令内容本身由 C2 的应用层加密保护，TLS 终止不会暴露指令内容。
	TLSCertFile string `json:"tls_cert,omitempty"`
	TLSKeyFile  string `json:"tls_key,omitempty"`

	// MaxConns 全局并发连接上限（0 = 4096）
	MaxConns int `json:"max_conns,omitempty"`
	// MaxConnsPerIP 单 IP 并发连接上限（0 = 256）
	MaxConnsPerIP int `json:"max_conns_per_ip,omitempty"`
	// DialTimeout 连接上游超时（0 = 10s）
	DialTimeout Duration `json:"dial_timeout,omitempty"`
	// IdleTimeout 连接空闲多久后关闭（0 = 1h）
	IdleTimeout Duration `json:"idle_timeout,omitempty"`
	// MaxUDPFlows UDP 并发流上限（0 = 2048）
	MaxUDPFlows int `json:"max_udp_flows,omitempty"`
	// UDPIdleTimeout UDP 流空闲回收（0 = 60s）
	UDPIdleTimeout Duration `json:"udp_idle_timeout,omitempty"`
}

// HealthInterval 上游健康探测间隔（带默认值）
func (c Config) HealthInterval() time.Duration {
	return c.UpstreamHealthInterval.Or(30 * time.Second)
}

// UpstreamList TCP 上游列表：upstreams 非空时取代 upstream（去重、去空）
func (c Config) UpstreamList() []string {
	return mergeUpstreams(c.Upstreams, c.Upstream)
}

// DNSUpstreamList DNS 上游列表：dns_upstreams 优先，其次 dns_upstream。
// 都为空时回落到 TCP/UDP 的上游 —— 常见部署是"同一个服务端地址"，让运维少写一遍。
func (c Config) DNSUpstreamList() []string {
	if l := mergeUpstreams(c.DNSUpstreams, c.DNSUpstream); len(l) > 0 {
		return l
	}
	return c.UpstreamList()
}

// mergeUpstreams 合并"列表为主、单值为辅"的写法：列表非空时忽略单值，
// 同时做去重与去空白（运维容易把同一个地址写两遍）。
func mergeUpstreams(list []string, single string) []string {
	src := list
	if len(src) == 0 {
		if single == "" {
			return nil
		}
		src = []string{single}
	}
	seen := make(map[string]bool, len(src))
	out := make([]string, 0, len(src))
	for _, s := range src {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Duration JSON 里用 "10s"/"1h" 这类字符串（与项目其他配置一致）
type Duration time.Duration

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s == "" {
		*d = 0
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("时长格式错误 %q: %v", s, err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) Or(def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return time.Duration(d)
}

// LoadConfig 读取并校验配置
func LoadConfig(path string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("解析配置失败: %v", err)
	}
	// 至少要有一个监听面：TCP 或 DNS（纯 DNS 中继是合法部署）
	if c.Listen == "" && c.DNSListen == "" {
		return c, fmt.Errorf("listen 与 dns_listen 至少要配置一个")
	}
	// 配了 TCP 监听就必须有上游；**纯 DNS 中继不需要** ——
	// 它的上游由 dns_upstream/dns_upstreams 提供，与 TCP 上游无关
	if c.Listen != "" && len(c.UpstreamList()) == 0 {
		return c, fmt.Errorf("配置了 listen，但 upstream 或 upstreams 为空")
	}
	if (c.UDPListen == "") != (c.UDPUpstream == "") {
		return c, fmt.Errorf("udp_listen 与 udp_upstream 必须同时配置")
	}
	if c.DNSListen != "" && len(c.DNSUpstreamList()) == 0 {
		return c, fmt.Errorf("dns_listen 已配置，但 dns_upstream/dns_upstreams 为空" +
			"（也没有可回落的上游）")
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return c, fmt.Errorf("tls_cert 与 tls_key 必须同时配置")
	}
	if c.MaxConns <= 0 {
		c.MaxConns = 4096
	}
	if c.MaxConnsPerIP <= 0 {
		c.MaxConnsPerIP = 256
	}
	if c.MaxUDPFlows <= 0 {
		c.MaxUDPFlows = 2048
	}
	return c, nil
}
