package commands

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 端口扫描（内网发现用，阶段三 · AD 攻击链的前置能力）。
//
// 设计取舍：
//   - TCP connect 扫描（不是 SYN 扫描）：跨平台、不需要 raw socket/root，
//     代价是会在目标上留下完整连接记录。对红队内的"先摸清网段"够用。
//   - 并发 + 每端口超时 + 总量上限：没有上限的扫描会把自己变成 DoS 工具，
//     也会让一条命令卡住整个命令队列（轮询型客户端尤其明显）。
//   - 输出按主机聚合成一行，便于在控制台里 grep，而不是刷几百行。

const (
	scanMaxHosts       = 256
	scanMaxPorts       = 4096
	scanConcurrency    = 128
	scanDefaultTimeout = 800 * time.Millisecond
	scanMinTimeout     = 50 * time.Millisecond
	scanMaxTimeout     = 10 * time.Second
)

// topPorts 常用端口（`scan <target> top`）：内网排查最常见的服务
var topPorts = []int{
	21, 22, 23, 25, 53, 80, 110, 135, 139, 143, 443, 445, 993, 995,
	1433, 1521, 3306, 3389, 5432, 5900, 5985, 6379, 8080, 8443, 9200, 27017,
}

// CmdPortScan 端口扫描入口。cmdBuf = "<targets> <ports> [timeoutMs]"
//
//	targets: 单个 IP / CIDR（10.0.0.0/24）/ 主机名 / 逗号分隔的混合
//	ports:   22,80,443 / 1-1024 / top / 逗号与范围混合
func CmdPortScan(cmdBuf []byte) ([]byte, error) {
	hosts, ports, timeout, err := parseScanSpec(string(cmdBuf))
	if err != nil {
		return nil, err
	}
	start := time.Now()
	results := scanHosts(hosts, ports, timeout)
	return formatScanResults(hosts, ports, results, time.Since(start)), nil
}

// parseScanSpec 解析并校验扫描参数（纯逻辑，单测覆盖）
func parseScanSpec(spec string) (hosts []string, ports []int, timeout time.Duration, err error) {
	fields := strings.Fields(strings.TrimSpace(spec))
	if len(fields) < 2 {
		return nil, nil, 0, errors.New("用法: scan <目标(CIDR/IP/主机名,可逗号分隔)> <端口(22,80,1-1024,top)> [超时毫秒]")
	}
	hosts, err = expandHosts(fields[0])
	if err != nil {
		return nil, nil, 0, err
	}
	ports, err = expandPorts(fields[1])
	if err != nil {
		return nil, nil, 0, err
	}
	timeout = scanDefaultTimeout
	if len(fields) >= 3 {
		ms, convErr := strconv.Atoi(fields[2])
		if convErr != nil {
			return nil, nil, 0, fmt.Errorf("超时必须是毫秒整数: %q", fields[2])
		}
		timeout = time.Duration(ms) * time.Millisecond
		if timeout < scanMinTimeout {
			timeout = scanMinTimeout
		}
		if timeout > scanMaxTimeout {
			timeout = scanMaxTimeout
		}
	}
	return hosts, ports, timeout, nil
}

// expandHosts 展开目标：CIDR / IP / 主机名 / 逗号分隔，总量受 scanMaxHosts 限制
func expandHosts(raw string) ([]string, error) {
	seen := make(map[string]bool)
	out := make([]string, 0, 16)
	add := func(ip string) error {
		if ip == "" || seen[ip] {
			return nil
		}
		if len(out) >= scanMaxHosts {
			return fmt.Errorf("目标主机过多（上限 %d 台）", scanMaxHosts)
		}
		seen[ip] = true
		out = append(out, ip)
		return nil
	}

	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "/") {
			ips, err := expandCIDR(part)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if err := add(ip); err != nil {
					return nil, err
				}
			}
			continue
		}
		if net.ParseIP(part) != nil {
			if err := add(part); err != nil {
				return nil, err
			}
			continue
		}
		// 主机名：解析后逐条加入
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		addrs, err := net.DefaultResolver.LookupHost(ctx, part)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("解析主机名 %q 失败: %v", part, err)
		}
		for _, ip := range addrs {
			if err := add(ip); err != nil {
				return nil, err
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("没有可扫描的目标")
	}
	return out, nil
}

// expandCIDR 展开 CIDR；超过上限直接拒绝（不静默截断——操作员应显式缩小范围）
func expandCIDR(cidr string) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("CIDR 格式错误 %q: %v", cidr, err)
	}
	ones, bits := ipnet.Mask.Size()
	hostCount := 1 << (bits - ones)
	if hostCount > scanMaxHosts+2 { // +2 是网络号/广播地址
		return nil, fmt.Errorf("网段 %s 含 %d 个地址，超过上限 %d（请缩小范围）", cidr, hostCount, scanMaxHosts)
	}
	out := make([]string, 0, hostCount)
	for ip := ipnet.IP.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		// 跳过网络号与广播地址（/31、/32 例外：只有 1-2 个地址时都扫）
		if hostCount > 2 && (isNetworkAddr(ip, ipnet) || isBroadcastAddr(ip, ipnet)) {
			continue
		}
		out = append(out, ip.String())
	}
	return out, nil
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func isNetworkAddr(ip net.IP, n *net.IPNet) bool {
	first := n.IP.Mask(n.Mask)
	return ip.Equal(first)
}

func isBroadcastAddr(ip net.IP, n *net.IPNet) bool {
	last := make(net.IP, len(n.IP))
	copy(last, n.IP.Mask(n.Mask))
	for i := range last {
		last[i] |= ^n.Mask[i]
	}
	return ip.Equal(last)
}

// expandPorts 展开端口：22,80 / 1-1024 / top / 混合，总量受 scanMaxPorts 限制
func expandPorts(raw string) ([]int, error) {
	if strings.EqualFold(raw, "top") {
		out := make([]int, len(topPorts))
		copy(out, topPorts)
		return out, nil
	}
	seen := make(map[int]bool)
	out := make([]int, 0, 32)
	addPort := func(p int) error {
		if p < 1 || p > 65535 {
			return fmt.Errorf("端口越界: %d", p)
		}
		if seen[p] {
			return nil
		}
		if len(out) >= scanMaxPorts {
			return fmt.Errorf("端口过多（上限 %d 个）", scanMaxPorts)
		}
		seen[p] = true
		out = append(out, p)
		return nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(bounds[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err1 != nil || err2 != nil || lo > hi {
				return nil, fmt.Errorf("端口范围非法: %q", part)
			}
			for p := lo; p <= hi; p++ {
				if err := addPort(p); err != nil {
					return nil, err
				}
			}
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("端口非法: %q", part)
		}
		if err := addPort(p); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, errors.New("没有可扫描的端口")
	}
	sort.Ints(out)
	return out, nil
}

// scanHosts 并发扫描，返回 host -> 开放端口
func scanHosts(hosts []string, ports []int, timeout time.Duration) map[string][]int {
	type job struct {
		host string
		port int
	}
	jobs := make(chan job)
	results := make(map[string][]int)
	var mu sync.Mutex
	var wg sync.WaitGroup

	workers := scanConcurrency
	if workers > len(hosts)*len(ports) {
		workers = len(hosts) * len(ports)
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				addr := net.JoinHostPort(j.host, strconv.Itoa(j.port))
				conn, err := net.DialTimeout("tcp", addr, timeout)
				if err != nil {
					continue
				}
				conn.Close()
				mu.Lock()
				results[j.host] = append(results[j.host], j.port)
				mu.Unlock()
			}
		}()
	}
	for _, h := range hosts {
		for _, p := range ports {
			jobs <- job{host: h, port: p}
		}
	}
	close(jobs)
	wg.Wait()
	for h := range results {
		sort.Ints(results[h])
	}
	return results
}

// formatScanResults 按主机聚合输出（便于控制台 grep，而不是刷几百行）
func formatScanResults(hosts []string, ports []int, results map[string][]int, elapsed time.Duration) []byte {
	var b strings.Builder
	totalOpen := 0
	for _, h := range hosts {
		open := results[h]
		if len(open) == 0 {
			continue
		}
		totalOpen += len(open)
		parts := make([]string, 0, len(open))
		for _, p := range open {
			parts = append(parts, strconv.Itoa(p))
		}
		fmt.Fprintf(&b, "%s  open: %s\n", h, strings.Join(parts, ","))
	}
	fmt.Fprintf(&b, "[scan] %d 台主机 × %d 个端口，开放 %d 个，用时 %.1fs\n",
		len(hosts), len(ports), totalOpen, elapsed.Seconds())
	if totalOpen == 0 {
		b.WriteString("[scan] 没有发现开放端口（目标可能过滤了入站或不在线）\n")
	}
	return []byte(b.String())
}
