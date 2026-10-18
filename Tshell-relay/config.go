package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Config 中继节点配置（JSON，纯标准库解析——中继是部署在一次性 VPS 上的小工具，
// 不应该引入任何第三方依赖）。
type Config struct {
	// Listen TCP 监听地址（转发 TCP/WebSocket/HTTP(S) 等持久或轮询传输）
	Listen string `json:"listen"`
	// Upstream 上游地址 host:port（可以是最终服务端，也可以是另一个中继 —— 多级中继）
	Upstream string `json:"upstream"`

	// UDPListen/UDPUpstream 可选中继 KCP 传输（UDP）
	UDPListen   string `json:"udp_listen,omitempty"`
	UDPUpstream string `json:"udp_upstream,omitempty"`

	// HealthListen 健康检查监听地址（/healthz），建议只绑回环
	HealthListen string `json:"health_listen,omitempty"`

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
	if c.Listen == "" || c.Upstream == "" {
		return c, fmt.Errorf("listen 与 upstream 必填")
	}
	if (c.UDPListen == "") != (c.UDPUpstream == "") {
		return c, fmt.Errorf("udp_listen 与 udp_upstream 必须同时配置")
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
