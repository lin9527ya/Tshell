package main

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ===== 测试用假 DNS 上游 =====

// fakeDNS 一个极简 DNS 服务器：把查询里的问题名回显成响应，用于验证中继的行为。
//
// 它**故意不是**一个完整 DNS 实现 —— 测试关心的是中继有没有正确转发、
// 有没有改写事务 ID、截断时会不会改走 TCP，而不是 DNS 语义本身。
type fakeDNS struct {
	udp     *net.UDPConn
	tcp     net.Listener
	addr    string
	seenIDs []uint16
	mu      sync.Mutex
	// truncate 置位时 UDP 响应带 TC 位（模拟超长响应），TCP 响应完整
	truncate bool
	// failUDP 置位时 UDP 查询一律不回（模拟上游不可用）
	failUDP bool
}

func newFakeDNS(t *testing.T) *fakeDNS {
	t.Helper()
	uc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("假 DNS UDP 监听失败: %v", err)
	}
	ln, err := net.Listen("tcp", uc.LocalAddr().String())
	if err != nil {
		_ = uc.Close()
		t.Fatalf("假 DNS TCP 监听失败: %v", err)
	}
	f := &fakeDNS{udp: uc, tcp: ln, addr: uc.LocalAddr().String()}

	go func() {
		buf := make([]byte, 4096)
		for {
			n, client, err := uc.ReadFromUDP(buf)
			if err != nil {
				return
			}
			query := append([]byte(nil), buf[:n]...)
			f.mu.Lock()
			f.seenIDs = append(f.seenIDs, binary.BigEndian.Uint16(query[:2]))
			fail, trunc := f.failUDP, f.truncate
			f.mu.Unlock()
			if fail {
				continue // 不回 = 超时
			}
			_, _ = uc.WriteToUDP(buildResponse(query, trunc), client)
		}
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var lb [2]byte
				if _, err := readFull(c, lb[:]); err != nil {
					return
				}
				q := make([]byte, binary.BigEndian.Uint16(lb[:]))
				if _, err := readFull(c, q); err != nil {
					return
				}
				resp := buildResponse(q, false)
				out := make([]byte, 2+len(resp))
				binary.BigEndian.PutUint16(out, uint16(len(resp)))
				copy(out[2:], resp)
				_, _ = c.Write(out)
			}(conn)
		}
	}()
	t.Cleanup(func() {
		_ = uc.Close()
		_ = ln.Close()
	})
	return f
}

func (f *fakeDNS) setTruncate(v bool) {
	f.mu.Lock()
	f.truncate = v
	f.mu.Unlock()
}

func (f *fakeDNS) setFailUDP(v bool) {
	f.mu.Lock()
	f.failUDP = v
	f.mu.Unlock()
}

func (f *fakeDNS) ids() []uint16 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]uint16(nil), f.seenIDs...)
}

// buildResponse 造一个响应：把查询头复制过来，QR=1，附一条问题段原样返回。
// 不做真实解析 —— 中继也不做，两边只要"报文结构合法"即可。
func buildResponse(query []byte, truncate bool) []byte {
	resp := append([]byte(nil), query...)
	resp[2] = 0x80 // QR=1, RD=0
	if truncate {
		resp[2] |= 0x02
	}
	resp[3] = 0x00
	// ANCOUNT 保持与 QDCOUNT 无关的 0：中继不解析记录段
	resp[6], resp[7] = 0, 0
	resp[8], resp[9] = 0, 0
	resp[10], resp[11] = 0, 0
	return resp
}

// buildQuery 造一个最小合法查询（1 个 A 问题）
func buildQuery(id uint16, name string) []byte {
	var b []byte
	head := make([]byte, 12)
	binary.BigEndian.PutUint16(head[0:], id)
	binary.BigEndian.PutUint16(head[2:], 0x0100) // RD=1
	binary.BigEndian.PutUint16(head[4:], 1)      // QDCOUNT=1
	b = append(b, head...)
	for _, label := range strings.Split(name, ".") {
		if label == "" {
			continue
		}
		b = append(b, byte(len(label)))
		b = append(b, label...)
	}
	b = append(b, 0)    // 名字结束
	b = append(b, 0, 1) // QTYPE=A
	b = append(b, 0, 1) // QCLASS=IN
	return b
}

func testDNSConfig(upstreams ...string) Config {
	return Config{
		DNSListen:              "127.0.0.1:0",
		DNSUpstreams:           upstreams,
		DialTimeout:            Duration(2 * time.Second),
		MaxConns:               64,
		MaxConnsPerIP:          64,
		MaxUDPFlows:            256,
		UDPIdleTimeout:         Duration(30 * time.Second),
		IdleTimeout:            Duration(time.Minute),
		UpstreamHealthInterval: Duration(200 * time.Millisecond),
	}
}

// serveDNSRelay 起一个中继并返回它的实际 UDP 地址
func serveDNSRelay(t *testing.T, cfg Config) (*dnsRelay, string) {
	t.Helper()
	r, err := newDNSRelay(cfg)
	if err != nil {
		t.Fatalf("创建 DNS 中继失败: %v", err)
	}
	uc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	r.udpLn = uc
	go func() {
		buf := make([]byte, 4096)
		for {
			n, client, err := uc.ReadFromUDP(buf)
			if err != nil {
				return
			}
			q := append([]byte(nil), buf[:n]...)
			go r.handleUDP(uc, client, q)
		}
	}()
	t.Cleanup(func() { _ = r.Close() })
	return r, uc.LocalAddr().String()
}

// ask 向中继发一次查询并取回响应
func ask(t *testing.T, relayAddr string, id uint16, name string) ([]byte, error) {
	t.Helper()
	raddr, err := net.ResolveUDPAddr("udp", relayAddr)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write(buildQuery(id, name)); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// ===== 用例 =====

// 基本转发：查询到达上游、响应回到客户端，且事务 ID 被改回客户端原本的 ID
func TestDNSRelayForwardsAndRestoresID(t *testing.T) {
	up := newFakeDNS(t)
	_, relayAddr := serveDNSRelay(t, testDNSConfig(up.addr))

	const clientID = 0x4d2
	resp, err := ask(t, relayAddr, clientID, "beacon.c2.example")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if binary.BigEndian.Uint16(resp[:2]) != clientID {
		t.Fatalf("响应事务 ID = 0x%04x，应为客户端原本的 0x%04x",
			binary.BigEndian.Uint16(resp[:2]), clientID)
	}
	if resp[2]&0x80 == 0 {
		t.Error("响应应带 QR=1")
	}
	// 上游看到的 ID 应该**不是**客户端那个（否则等于没改写）
	ids := up.ids()
	if len(ids) == 0 {
		t.Fatal("上游没收到查询")
	}
	if ids[0] == clientID {
		t.Log("注意：改写后的 ID 恰好等于客户端 ID（概率 1/65536，重跑即可）")
	}
}

// 核心动机：两个客户端用同一个事务 ID 时不能串台。
// 中继必须把它们改写成不同的 ID 分别送上游、再各自改回来。
func TestDNSRelayAvoidsIDCollision(t *testing.T) {
	up := newFakeDNS(t)
	_, relayAddr := serveDNSRelay(t, testDNSConfig(up.addr))

	const sameID = 0x1234
	var wg sync.WaitGroup
	results := make([][]byte, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			resp, err := ask(t, relayAddr, sameID, fmt.Sprintf("beacon%d.c2.example", idx))
			if err == nil {
				results[idx] = resp
			}
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		if r == nil {
			t.Fatalf("第 %d 个查询没有响应", i)
		}
		if got := binary.BigEndian.Uint16(r[:2]); got != sameID {
			t.Errorf("第 %d 个响应的事务 ID = 0x%04x，应改回 0x%04x", i, got, sameID)
		}
	}
	// 上游应看到 4 个查询，且 ID 不是全部相同（撞车被改写掉了）
	ids := up.ids()
	if len(ids) != 4 {
		t.Fatalf("上游应收到 4 个查询，实际 %d", len(ids))
	}
	uniq := map[uint16]bool{}
	for _, id := range ids {
		uniq[id] = true
	}
	if len(uniq) == 1 {
		t.Errorf("4 个查询在上游看到的是同一个 ID（0x%04x）—— 改写没生效，多客户端会串台", ids[0])
	}
}

// UDP 响应带 TC 位时，中继应自动改用 TCP 重问并清掉 TC
func TestDNSRelayFallsBackToTCPOnTruncation(t *testing.T) {
	up := newFakeDNS(t)
	up.setTruncate(true)
	_, relayAddr := serveDNSRelay(t, testDNSConfig(up.addr))

	resp, err := ask(t, relayAddr, 0x0abc, "long.c2.example")
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if resp[2]&0x02 != 0 {
		t.Error("中继已用 TCP 取回完整响应，应清掉 TC 位")
	}
	if got := binary.BigEndian.Uint16(resp[:2]); got != 0x0abc {
		t.Errorf("TCP 回退后的响应 ID = 0x%04x，应为 0x%04x", got, 0x0abc)
	}
}

// 多上游：第一个不可用时自动转移到第二个
func TestDNSRelayFailsOverToSecondUpstream(t *testing.T) {
	dead := newFakeDNS(t)
	dead.setFailUDP(true)
	live := newFakeDNS(t)

	cfg := testDNSConfig(dead.addr, live.addr)
	_, relayAddr := serveDNSRelay(t, cfg)

	// 多试几次：轮询会先打到 dead，失败后转移
	ok := false
	for i := 0; i < 4; i++ {
		resp, err := ask(t, relayAddr, uint16(0x1000+i), "c2.example")
		if err == nil && len(resp) >= 12 {
			ok = true
			break
		}
	}
	if !ok {
		t.Fatal("第一个上游不可用时，应转移到第二个上游")
	}
}

// 全部上游不可用时应返回错误（而不是一直挂着）
func TestDNSRelayFailsWhenAllUpstreamsDown(t *testing.T) {
	dead := newFakeDNS(t)
	dead.setFailUDP(true)
	cfg := testDNSConfig(dead.addr)
	cfg.DialTimeout = Duration(300 * time.Millisecond)
	_, relayAddr := serveDNSRelay(t, cfg)

	if _, err := ask(t, relayAddr, 1, "c2.example"); err == nil {
		t.Fatal("上游全部不可用时应超时报错")
	}
}

// ===== 上游池的单测（不涉及网络） =====

func TestUpstreamPoolRotatesAndSkipsUnhealthy(t *testing.T) {
	p := newUpstreamPool([]string{"a:1", "b:2", "c:3"}, 0)
	defer p.close()

	// 轮询：连续取 3 次应覆盖全部三个
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		seen[p.pick()] = true
	}
	if len(seen) != 3 {
		t.Fatalf("轮询应覆盖 3 个上游，实际 %v", seen)
	}

	// 把 b 标记为不健康：之后不该再被选中
	for i := 0; i < upstreamFailThreshold; i++ {
		p.markFail("b:2", fmt.Errorf("测试"))
	}
	for i := 0; i < 8; i++ {
		if got := p.pick(); got == "b:2" {
			t.Fatalf("已标记不健康的上游仍被选中（第 %d 次）", i)
		}
	}
	if p.healthy() != 2 {
		t.Fatalf("健康上游数应为 2，实际 %d", p.healthy())
	}

	// 恢复后应重新可用
	p.markOK("b:2")
	if p.healthy() != 3 {
		t.Fatalf("恢复后健康上游数应为 3，实际 %d", p.healthy())
	}
	back := false
	for i := 0; i < 6; i++ {
		if p.pick() == "b:2" {
			back = true
			break
		}
	}
	if !back {
		t.Error("恢复后的上游应重新参与轮询")
	}
}

// 连续失败阈值：不到阈值不移除，达到才移除（避免一次抖动就打掉一个上游）
func TestUpstreamPoolFailureThreshold(t *testing.T) {
	p := newUpstreamPool([]string{"a:1", "b:2"}, 0)
	defer p.close()

	for i := 1; i < upstreamFailThreshold; i++ {
		p.markFail("a:1", fmt.Errorf("第 %d 次", i))
	}
	if p.healthy() != 2 {
		t.Fatalf("未达阈值不应移除上游，健康数 = %d", p.healthy())
	}
	p.markFail("a:1", fmt.Errorf("第 %d 次", upstreamFailThreshold))
	if p.healthy() != 1 {
		t.Fatalf("达到阈值应移除上游，健康数 = %d", p.healthy())
	}
}

// 全部不健康时仍要返回一个候选：被动恢复依赖真实流量去试
func TestUpstreamPoolNeverReturnsEmpty(t *testing.T) {
	p := newUpstreamPool([]string{"a:1", "b:2"}, 0)
	defer p.close()
	for i := 0; i < upstreamFailThreshold; i++ {
		p.markFail("a:1", fmt.Errorf("x"))
		p.markFail("b:2", fmt.Errorf("x"))
	}
	if got := p.pick(); got == "" {
		t.Fatal("全部不健康时也应返回一个候选上游（否则永远无法被动恢复）")
	}
}

// ===== 配置解析 =====

func TestUpstreamListMergesAndDedups(t *testing.T) {
	// 列表优先并去重
	c := Config{Upstream: "a:1", Upstreams: []string{"b:2", "b:2", " c:3 ", ""}}
	got := c.UpstreamList()
	if len(got) != 2 || got[0] != "b:2" || got[1] != "c:3" {
		t.Fatalf("upstreams 应优先并去重去空白，实际 %v", got)
	}

	// 只配单值
	c2 := Config{Upstream: "a:1"}
	if got := c2.UpstreamList(); len(got) != 1 || got[0] != "a:1" {
		t.Fatalf("只配 upstream 时应得到单个上游，实际 %v", got)
	}

	// DNS 未单独配 → 回落 TCP 上游（常见部署同一个地址，让运维少写一遍）
	c3 := Config{Upstream: "a:1"}
	if got := c3.DNSUpstreamList(); len(got) != 1 || got[0] != "a:1" {
		t.Fatalf("DNS 上游应回落 TCP 上游，实际 %v", got)
	}

	// DNS 单独配 → 用 DNS 自己的
	c4 := Config{Upstream: "a:1", DNSUpstream: "d:53"}
	if got := c4.DNSUpstreamList(); len(got) != 1 || got[0] != "d:53" {
		t.Fatalf("DNS 上游应优先用自己的配置，实际 %v", got)
	}
}

// 校验：只配 DNS 中继（没有 listen/upstream）也应合法
func TestConfigValidationDNSOnly(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/dns-only.json"
	body := `{"dns_listen":"0.0.0.0:53","dns_upstream":"10.0.0.9:53"}`
	if err := writeFile(path, body); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("纯 DNS 中继配置应合法: %v", err)
	}
	if cfg.HealthInterval() != 30*time.Second {
		t.Errorf("未配置时健康探测间隔应为 30s，实际 %v", cfg.HealthInterval())
	}

	// 什么都不配 → 报错
	bad := dir + "/empty.json"
	if err := writeFile(bad, `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(bad); err == nil {
		t.Error("listen 与 dns_listen 都没有时应报错")
	}

	// 配了 dns_listen 但没有上游 → 报错（回落到 TCP 上游也没有）
	bad2 := dir + "/dns-noup.json"
	if err := writeFile(bad2, `{"dns_listen":"0.0.0.0:53"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(bad2); err == nil {
		t.Error("dns_listen 无上游时应报错")
	}
}

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}
