package main

import (
	"net"
	"sync"
)

// connLimiter 全局 + 单 IP 并发连接限制。
// 中继是暴露在公网的组件：没有上限时，一个扫描器或者被攻陷的客户端就能把
// 连接表和文件描述符吃光，进而拖垮上游服务端。
type connLimiter struct {
	mu     sync.Mutex
	max    int
	maxPer int
	total  int
	perIP  map[string]int
}

func newConnLimiter(max, maxPer int) *connLimiter {
	return &connLimiter{max: max, maxPer: maxPer, perIP: make(map[string]int)}
}

// acquire 尝试占用一个连接名额；失败返回 false（调用方应立即关闭连接）
func (l *connLimiter) acquire(addr net.Addr) (string, bool) {
	ip := ipOf(addr)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= l.max || l.perIP[ip] >= l.maxPer {
		return ip, false
	}
	l.total++
	l.perIP[ip]++
	return ip, true
}

func (l *connLimiter) release(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total--
	if l.perIP[ip] > 0 {
		l.perIP[ip]--
	}
	if l.perIP[ip] == 0 {
		delete(l.perIP, ip)
	}
}

// counts 当前 (总数, 独立 IP 数)
func (l *connLimiter) counts() (int, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total, len(l.perIP)
}

func ipOf(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
