package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 中继的验收点：字节双向无丢失、可级联、有连接/空闲上限、TLS 终止可用、
// UDP 流表正确。中继本身不解析协议，所以用 echo 服务器做对端即可覆盖。

// startEcho 启动一个回显 TCP 服务
func startEcho(t *testing.T) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(c)
		}
	}()
	return ln.Addr().String(), func() { close(done); _ = ln.Close() }
}

// startTCPRelay 启动一个指向 upstream 的中继，返回其地址
func startTCPRelay(t *testing.T, cfg Config, upstream string) (addr string, r *tcpRelay) {
	t.Helper()
	cfg.Upstream = upstream
	limiter := newConnLimiter(cfg.MaxConns, cfg.MaxConnsPerIP)
	relay, err := newTCPRelay(cfg, limiter)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = relay.ServeListener(ln) }()
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String(), relay
}

func TestTCPRelayEcho(t *testing.T) {
	echoAddr, stop := startEcho(t)
	defer stop()
	relayAddr, _ := startTCPRelay(t, Config{MaxConns: 16, MaxConnsPerIP: 8}, echoAddr)

	conn, err := net.Dial("tcp", relayAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	payload := bytes.Repeat([]byte("relay-test-"), 10000) // 120KB，跨多次读
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("回显读取失败: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("回显数据不一致（%d vs %d 字节）", len(got), len(payload))
	}
}

// TestTCPRelayChain 多级中继：client → relay1 → relay2 → echo
func TestTCPRelayChain(t *testing.T) {
	echoAddr, stop := startEcho(t)
	defer stop()
	relay2Addr, _ := startTCPRelay(t, Config{MaxConns: 16, MaxConnsPerIP: 8}, echoAddr)
	relay1Addr, _ := startTCPRelay(t, Config{MaxConns: 16, MaxConnsPerIP: 8}, relay2Addr)

	conn, err := net.Dial("tcp", relay1Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	msg := []byte("two-hops")
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("二级中继回显失败: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("二级中继数据不一致: %q", got)
	}
}

func TestTCPRelayConnLimit(t *testing.T) {
	echoAddr, stop := startEcho(t)
	defer stop()
	relayAddr, relay := startTCPRelay(t, Config{MaxConns: 1, MaxConnsPerIP: 1}, echoAddr)

	first, err := net.Dial("tcp", relayAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	// 确保第一条连接已被占用
	first.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := first.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	if _, err := io.ReadFull(first, buf); err != nil {
		t.Fatalf("第一条连接不可用: %v", err)
	}

	second, err := net.Dial("tcp", relayAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	second.SetReadDeadline(time.Now().Add(2 * time.Second))
	// 超限连接会被直接关闭：写入后读取应得到 EOF/RST
	_, _ = second.Write([]byte("y"))
	if _, err := second.Read(buf); err == nil {
		t.Fatal("超过连接上限的连接不应被转发")
	}
	if relay.rejected.Load() == 0 {
		t.Fatal("拒绝计数应增加")
	}
}

func TestTCPRelayIdleTimeout(t *testing.T) {
	echoAddr, stop := startEcho(t)
	defer stop()
	relayAddr, _ := startTCPRelay(t, Config{
		MaxConns: 16, MaxConnsPerIP: 8,
		IdleTimeout: Duration(200 * time.Millisecond),
	}, echoAddr)

	conn, err := net.Dial("tcp", relayAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// 空闲超过 idle 后连接应被关闭
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1)
	start := time.Now()
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("空闲连接应被关闭")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("空闲关闭太慢: %v", elapsed)
	}
}

// TestTCPRelayTLSTermination 中继终止 TLS 后转发到明文上游
func TestTCPRelayTLSTermination(t *testing.T) {
	echoAddr, stop := startEcho(t)
	defer stop()

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeSelfSigned(t, certPath, keyPath)

	cfg := Config{MaxConns: 16, MaxConnsPerIP: 8, TLSCertFile: certPath, TLSKeyFile: keyPath, Upstream: echoAddr}
	limiter := newConnLimiter(cfg.MaxConns, cfg.MaxConnsPerIP)
	relay, err := newTCPRelay(cfg, limiter)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = relay.ServeListener(ln) }()
	defer ln.Close()

	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("TLS 连接失败: %v", err)
	}
	defer conn.Close()
	msg := []byte("tls-terminated")
	if _, err := conn.Write(msg); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("TLS 中继回显失败: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("TLS 中继数据不一致: %q", got)
	}
}

func TestUDPRelayEcho(t *testing.T) {
	// UDP 回显服务
	upAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	up, err := net.ListenUDP("udp", upAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()
	go func() {
		buf := make([]byte, 65535)
		for {
			n, addr, err := up.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = up.WriteToUDP(buf[:n], addr)
		}
	}()

	cfg := Config{UDPUpstream: up.LocalAddr().String(), MaxUDPFlows: 8, UDPIdleTimeout: Duration(time.Second)}
	relay := newUDPRelay(cfg)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go func() { _ = relay.ServePacket(conn) }()
	defer relay.Close()

	client, err := net.DialUDP("udp", nil, conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	msg := []byte("kcp-like-datagram")
	if _, err := client.Write(msg); err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 128)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("UDP 中继回显失败: %v", err)
	}
	if !bytes.Equal(buf[:n], msg) {
		t.Fatalf("UDP 数据不一致: %q", buf[:n])
	}
	if relay.count.Load() != 1 {
		t.Fatalf("流数 = %d，期望 1", relay.count.Load())
	}
}

func TestUDPRelayFlowLimit(t *testing.T) {
	up, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer up.Close()

	cfg := Config{UDPUpstream: up.LocalAddr().String(), MaxUDPFlows: 1, UDPIdleTimeout: Duration(time.Minute)}
	relay := newUDPRelay(cfg)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go func() { _ = relay.ServePacket(conn) }()
	defer relay.Close()

	c1, _ := net.DialUDP("udp", nil, conn.LocalAddr().(*net.UDPAddr))
	defer c1.Close()
	c2, _ := net.DialUDP("udp", nil, conn.LocalAddr().(*net.UDPAddr))
	defer c2.Close()

	_, _ = c1.Write([]byte("one"))
	time.Sleep(100 * time.Millisecond)
	_, _ = c2.Write([]byte("two"))
	time.Sleep(200 * time.Millisecond)

	if relay.count.Load() != 1 {
		t.Fatalf("流数应受上限约束为 1，实际 %d", relay.count.Load())
	}
	if relay.rejected.Load() == 0 {
		t.Fatal("超限流应计入 rejected")
	}
}

func TestHealthEndpoint(t *testing.T) {
	echoAddr, stop := startEcho(t)
	defer stop()
	_, relay := startTCPRelay(t, Config{MaxConns: 4, MaxConnsPerIP: 2}, echoAddr)

	h := &healthServer{tcp: relay, started: time.Now(), version: "test"}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := h.serve(ln)
	defer srv.Close()

	resp, err := httpGet("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"status":"ok"`, `"tcp_active":0`, `"version":"test"`} {
		if !bytes.Contains(resp, []byte(want)) {
			t.Fatalf("健康检查响应缺少 %s: %s", want, resp)
		}
	}
}

// writeSelfSigned 生成自签证书（TLS 终止测试用）
func writeSelfSigned(t *testing.T, certPath, keyPath string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "relay-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"relay-test"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certOut, _ := os.Create(certPath)
	_ = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der})
	certOut.Close()
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyOut, _ := os.Create(keyPath)
	_ = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	keyOut.Close()
}

func httpGet(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
