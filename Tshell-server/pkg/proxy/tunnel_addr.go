package proxy

import (
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"net"
	"strings"
)

// tunnelAdvertisedHost 返回客户端回连时应使用的服务端主机名/IP。
//
// 监听器记录里的 connect_address 经常是 0.0.0.0 / ::（服务端 bind 地址），
// 直接下发给客户端会导致客户端拨号 0.0.0.0 失败。优先级：
//  1. connect_address 中可拨号的 host
//  2. config.server.external_ip（运维显式配置的对外地址）
//  3. listen_address 中可拨号的 host
//  4. 回环地址（同机测试兜底）
func tunnelAdvertisedHost(listener database.Listener) string {
	// 优先非回环、非通配地址
	for _, addr := range []string{listener.ConnectAddress, listener.ListenAddress} {
		if host := dialableHost(addr, false); host != "" {
			return host
		}
	}
	if g := config.GetGlobalConfig(); g != nil {
		if host := strings.Trim(strings.TrimSpace(g.Server.ExternalIP), "[]"); host != "" {
			return host
		}
	}
	// 最后才回退到回环（同机测试）
	for _, addr := range []string{listener.ConnectAddress, listener.ListenAddress} {
		if host := dialableHost(addr, true); host != "" {
			return host
		}
	}
	return "127.0.0.1"
}

// dialableHost 从 host:port / [v6]:port / 裸 host 中提取 host，过滤通配地址。
func dialableHost(addr string, allowLoopback bool) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	// 兼容 protocol://host:port 形式
	if i := strings.Index(addr, "://"); i >= 0 {
		addr = addr[i+3:]
	}
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	switch strings.ToLower(host) {
	case "", "0.0.0.0", "::", "[::]", "localhost":
		return ""
	}
	if !allowLoopback {
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return ""
		}
	}
	return host
}
