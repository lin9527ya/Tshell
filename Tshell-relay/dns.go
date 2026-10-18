package main

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// ===== 上游池（多上游 + 故障转移）=====
//
// 为什么需要：单上游中继本身就是单点 —— 上游挂了整条链路就断了，而中继通常部署在
// 一次性 VPS 上、没人盯着。多上游至少做到"上游之一可恢复时流量自动回去"。
//
// 健康判定是**被动 + 主动**两条：
//   - 被动：拨号/交换失败累计到阈值即标记不健康（真实流量的反馈最快）；
//   - 主动：后台每 health_interval 探一次不健康的上游，成功即恢复。
//
// 只做"轮询 + 跳过不健康"（round-robin），不做加权/最少连接：中继是哑的，
// 它看不到会话归属，按连接粒度分散已经够用，复杂调度只会增加出错面。
type upstreamPool struct {
	targets []string

	mu      sync.Mutex
	fails   map[string]int
	unheal  map[string]time.Time
	next    int
	failMax int

	picked  atomic.Uint64
	skipped atomic.Uint64

	stopOnce sync.Once
	stop     chan struct{}
}

const (
	// upstreamFailThreshold 连续失败多少次后标记不健康。
	// 用"连续成功清零"而不是滑动窗口：中继上没有足够状态做精细统计，
	// 而连续失败已经足够说明问题。
	upstreamFailThreshold = 3
)

func newUpstreamPool(targets []string, healthInterval time.Duration) *upstreamPool {
	p := &upstreamPool{
		targets: append([]string(nil), targets...),
		fails:   make(map[string]int),
		unheal:  make(map[string]time.Time),
		failMax: upstreamFailThreshold,
		stop:    make(chan struct{}),
	}
	if healthInterval > 0 && len(targets) > 1 {
		go p.probeLoop(healthInterval)
	}
	return p
}

// Len 上游数量
func (p *upstreamPool) Len() int { return len(p.targets) }

// pick 取一个（优先健康的）上游。全部不健康时兜底返回第一个：
// 宁可用一个可能坏的上游试一次，也不要直接拒绝服务 —— 被动恢复依赖真实流量。
func (p *upstreamPool) pick() string {
	if len(p.targets) == 1 {
		return p.targets[0]
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(p.targets)
	for i := 0; i < n; i++ {
		t := p.targets[(p.next+i)%n]
		if _, bad := p.unheal[t]; bad {
			p.skipped.Add(1)
			continue
		}
		p.next = (p.next + i + 1) % n
		p.picked.Add(1)
		return t
	}
	// 全部标记不健康：轮转着试，让被动探测有机会把它们逐个救回来
	t := p.targets[p.next%n]
	p.next = (p.next + 1) % n
	p.picked.Add(1)
	return t
}

// markOK 该上游可用（清零连续失败计数，并解除不健康标记）
func (p *upstreamPool) markOK(target string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fails[target] != 0 {
		p.fails[target] = 0
	}
	delete(p.unheal, target)
}

// markFail 该上游失败一次；达到阈值则标记不健康并打日志
func (p *upstreamPool) markFail(target string, reason error) {
	p.mu.Lock()
	p.fails[target]++
	n := p.fails[target]
	marked := false
	if n >= p.failMax {
		if _, already := p.unheal[target]; !already {
			p.unheal[target] = time.Now()
			marked = true
		}
	}
	p.mu.Unlock()

	if marked {
		log.Printf("⚠️ 上游 %s 连续失败 %d 次，暂时移出轮转: %v", target, n, reason)
	}
}

// healthy 当前健康的上游数量（健康检查接口用）
func (p *upstreamPool) healthy() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.targets) - len(p.unheal)
}

func (p *upstreamPool) close() {
	p.stopOnce.Do(func() { close(p.stop) })
}

// probeLoop 周期性探测"不健康"的上游，成功即恢复。
// 只探不健康的：健康上游由真实流量证明，额外探测纯属浪费。
func (p *upstreamPool) probeLoop(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.mu.Lock()
			bad := make([]string, 0, len(p.unheal))
			for target := range p.unheal {
				bad = append(bad, target)
			}
			p.mu.Unlock()

			for _, target := range bad {
				conn, err := net.DialTimeout("tcp", target, 3*time.Second)
				if err != nil {
					continue
				}
				_ = conn.Close()
				p.markOK(target)
				log.Printf("✅ 上游 %s 已恢复，重新加入轮转", target)
			}
		}
	}
}

// ===== DNS 中继 =====
//
// 为什么中继要"看懂"DNS，而不像 UDP 那样纯搬字节：
//
//	KCP 的 UDP 转发是语法无关的 —— 每个包自带一切信息，直接从 A 搬到 B 即可。
//	DNS 不是：一次查询与它的响应之间靠**事务 ID** 关联，而中继要同时服务多个
//	beacon，事务 ID 会撞车（两个 beacon 恰好都用 0x1234 时，响应会被投递给错的
//	那一方，表现为"DNS 通道时好时坏"）。所以必须逐查询改写事务 ID 并在回程改回。
//
// 顺带解决两件 UDP 裸转发做不到的事：
//   - 多上游故障转移（单上游的中继是单点）；
//   - UDP 被截断（TC 位）时自动改走 TCP 重问一遍（DNS 标准要求）。
type dnsRelay struct {
	cfg    Config
	pool   *upstreamPool
	udpLn  *net.UDPConn
	tcpLn  net.Listener
	closed atomic.Bool

	// 统计
	queries  atomic.Uint64
	errors   atomic.Uint64
	tcpTries atomic.Uint64
}

func newDNSRelay(cfg Config) (*dnsRelay, error) {
	targets := cfg.DNSUpstreamList()
	if len(targets) == 0 {
		return nil, fmt.Errorf("dns_listen 已配置但缺少 dns_upstream/dns_upstreams")
	}
	return &dnsRelay{
		cfg:  cfg,
		pool: newUpstreamPool(targets, cfg.HealthInterval()),
	}, nil
}

// Serve 同时起 UDP 与 TCP：DNS 标准要求同一端口上两者都提供
// （UDP 响应超长时置 TC 位，客户端改用 TCP 重问 —— 只做 UDP 会在长 TXT 响应上失败）。
func (r *dnsRelay) Serve() error {
	if err := r.serveUDP(); err != nil {
		return err
	}
	return nil
}

func (r *dnsRelay) serveUDP() error {
	addr, err := net.ResolveUDPAddr("udp", r.cfg.DNSListen)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	r.udpLn = conn
	log.Printf("🔀 DNS 中继监听 %s/udp → %v", r.cfg.DNSListen, r.pool.targets)

	// TCP 同端口：端口被占用之类的问题不应让 UDP 也停掉
	if ln, err := net.Listen("tcp", r.cfg.DNSListen); err == nil {
		r.tcpLn = ln
		log.Printf("🔀 DNS 中继监听 %s/tcp（UDP 截断时走这里）", r.cfg.DNSListen)
		go r.serveTCP(ln)
	} else {
		log.Printf("⚠️ DNS 中继 TCP 监听失败（UDP 仍可用，长响应会失败）: %v", err)
	}

	buf := make([]byte, 4096)
	for {
		n, client, err := conn.ReadFromUDP(buf)
		if err != nil {
			if r.closed.Load() {
				return nil
			}
			log.Printf("⚠️ DNS 读取失败: %v", err)
			continue
		}
		query := make([]byte, n)
		copy(query, buf[:n])
		go r.handleUDP(conn, client, query)
	}
}

func (r *dnsRelay) handleUDP(conn *net.UDPConn, client *net.UDPAddr, query []byte) {
	r.queries.Add(1)
	resp, err := r.exchange(query, false)
	if err != nil {
		r.errors.Add(1)
		return
	}
	if _, err := conn.WriteToUDP(resp, client); err != nil {
		r.errors.Add(1)
	}
}

func (r *dnsRelay) serveTCP(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if r.closed.Load() {
				return
			}
			continue
		}
		go r.handleTCP(conn)
	}
}

func (r *dnsRelay) handleTCP(conn net.Conn) {
	defer conn.Close()
	r.queries.Add(1)
	r.tcpTries.Add(1)

	// TCP 上的 DNS 报文前置 2 字节长度
	var lenBuf [2]byte
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	if _, err := readFull(conn, lenBuf[:]); err != nil {
		r.errors.Add(1)
		return
	}
	n := int(binary.BigEndian.Uint16(lenBuf[:]))
	if n == 0 || n > 65535 {
		r.errors.Add(1)
		return
	}
	query := make([]byte, n)
	if _, err := readFull(conn, query); err != nil {
		r.errors.Add(1)
		return
	}

	resp, err := r.exchange(query, true)
	if err != nil {
		r.errors.Add(1)
		return
	}
	out := make([]byte, 2+len(resp))
	binary.BigEndian.PutUint16(out, uint16(len(resp)))
	copy(out[2:], resp)
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, _ = conn.Write(out)
}

// exchange 把一次查询送到某个上游并取回响应。
//
// 关键动作：**换一个新的事务 ID**。中继同时服务多个 beacon，如果原样透传，
// 两个客户端撞上同一个 ID 时响应会串到错的客户端。
// 回程时再改回客户端原本的 ID。
func (r *dnsRelay) exchange(query []byte, overTCP bool) ([]byte, error) {
	if len(query) < 12 {
		return nil, fmt.Errorf("DNS 报文过短: %d 字节", len(query))
	}
	clientID := binary.BigEndian.Uint16(query[:2])
	newID := randomID()

	rewritten := make([]byte, len(query))
	copy(rewritten, query)
	binary.BigEndian.PutUint16(rewritten[:2], newID)

	// 允许重试到另一个上游：多上游的意义就在这里
	attempts := r.pool.Len()
	if attempts > 2 {
		attempts = 2
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		target := r.pool.pick()
		resp, err := r.exchangeOnce(target, rewritten, overTCP)
		if err != nil {
			lastErr = err
			r.pool.markFail(target, err)
			continue
		}
		r.pool.markOK(target)

		// 回程改回客户端的事务 ID
		if len(resp) < 12 {
			lastErr = fmt.Errorf("上游 %s 返回的响应过短", target)
			continue
		}
		out := make([]byte, len(resp))
		copy(out, resp)
		binary.BigEndian.PutUint16(out[:2], clientID)

		// UDP 被截断：按标准改用 TCP 再问一次（只对 UDP 入向有意义）
		if !overTCP && truncated(out) {
			r.tcpTries.Add(1)
			if full, err := r.exchangeOnce(target, rewritten, true); err == nil && len(full) >= 12 {
				full = append([]byte(nil), full...)
				binary.BigEndian.PutUint16(full[:2], clientID)
				// 清掉 TC 位：已经拿到完整响应了
				full[2] &^= 0x02
				return full, nil
			}
			// 拿不到完整的就把截断响应给客户端（它会自己改走 TCP）
		}
		return out, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的上游")
	}
	return nil, lastErr
}

func (r *dnsRelay) exchangeOnce(target string, query []byte, overTCP bool) ([]byte, error) {
	timeout := r.cfg.DialTimeout.Or(10 * time.Second)
	if overTCP {
		conn, err := net.DialTimeout("tcp", target, timeout)
		if err != nil {
			return nil, err
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(timeout))

		out := make([]byte, 2+len(query))
		binary.BigEndian.PutUint16(out, uint16(len(query)))
		copy(out[2:], query)
		if _, err := conn.Write(out); err != nil {
			return nil, err
		}
		var lenBuf [2]byte
		if _, err := readFull(conn, lenBuf[:]); err != nil {
			return nil, err
		}
		n := int(binary.BigEndian.Uint16(lenBuf[:]))
		if n == 0 || n > 65535 {
			return nil, fmt.Errorf("上游返回长度非法: %d", n)
		}
		resp := make([]byte, n)
		if _, err := readFull(conn, resp); err != nil {
			return nil, err
		}
		return resp, nil
	}

	raddr, err := net.ResolveUDPAddr("udp", target)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(query); err != nil {
		return nil, err
	}
	buf := make([]byte, 65535)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (r *dnsRelay) Close() error {
	r.closed.Store(true)
	if r.udpLn != nil {
		_ = r.udpLn.Close()
	}
	if r.tcpLn != nil {
		_ = r.tcpLn.Close()
	}
	r.pool.close()
	return nil
}

// ===== DNS 报文小工具（只读头部 + 改写事务 ID/TC 位，不做完整解析）=====
//
// 刻意**不解析**问题段/记录：中继只需要知道"这是一个 DNS 报文"以及几个头部标志位。
// 解析得越少，越不会因为协议扩展（EDNS0、DNSSEC、新记录类型）而出错。

// truncated 响应是否置了 TC（截断）位
func truncated(resp []byte) bool {
	return len(resp) >= 4 && resp[2]&0x02 != 0
}

// randomID 随机事务 ID（避免可预测的 ID 让响应被伪造投递）
func randomID() uint16 {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		// 随机源不可用是系统级故障；退回时间戳至少不与"固定常量"等价
		return uint16(time.Now().UnixNano())
	}
	return binary.BigEndian.Uint16(b[:])
}

// readFull 读满整个缓冲区（io.ReadFull 的薄封装，避免本文件再引 io 只为一处调用）
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
