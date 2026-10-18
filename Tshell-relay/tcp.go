package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"
	"time"
)

// tcpRelay TCP 中继：监听 → 逐连接拨上游 → 双向 pipe。
//
// 为什么中继是"哑"的（只搬字节，不解析协议、不持有密钥）：
//   - beacon ↔ 服务端的会话加密是**端到端**的，中继即使被拿下也拿不到会话密钥，
//     更解不开指令内容；
//   - 不解析协议意味着传输类型（TCP/KCP/WS/HTTP(S)）对中继透明，不需要跟着
//     协议演进维护；
//   - 中继上没有任何密钥/配置，被取证时价值最低。
//
// 代价：服务端看到的是中继的 IP（beacon 的真实来源 IP 在 MetaInfo 里，仍可见）。
type tcpRelay struct {
	cfg     Config
	limiter *connLimiter
	tlsConf *tls.Config
	// pool 上游池：多上游轮询 + 失败转移（单上游时就是一个固定目标）
	pool *upstreamPool

	// 统计（健康检查用）
	accepted atomic.Uint64
	rejected atomic.Uint64
	active   atomic.Int64
}

func newTCPRelay(cfg Config, limiter *connLimiter) (*tcpRelay, error) {
	r := &tcpRelay{
		cfg:     cfg,
		limiter: limiter,
		pool:    newUpstreamPool(cfg.UpstreamList(), cfg.HealthInterval()),
	}
	if cfg.TLSCertFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, err
		}
		r.tlsConf = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	}
	return r, nil
}

// Serve 启动监听并阻塞
func (r *tcpRelay) Serve() error {
	ln, err := net.Listen("tcp", r.cfg.Listen)
	if err != nil {
		return err
	}
	return r.ServeListener(ln)
}

// ServeListener 在已有监听器上服务（测试用 :0 拿随机端口时使用）
func (r *tcpRelay) ServeListener(ln net.Listener) error {
	if r.tlsConf != nil {
		ln = tls.NewListener(ln, r.tlsConf)
		log.Printf("🔀 TCP 中继监听 %s (TLS) → %v", r.cfg.Listen, r.pool.targets)
	} else {
		log.Printf("🔀 TCP 中继监听 %s → %v", r.cfg.Listen, r.pool.targets)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			// 临时错误（EMFILE 等）不应让中继退出
			log.Printf("⚠️ accept 失败: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go r.handle(conn)
	}
}

func (r *tcpRelay) handle(client net.Conn) {
	ip, ok := r.limiter.acquire(client.RemoteAddr())
	if !ok {
		r.rejected.Add(1)
		_ = client.Close()
		return
	}
	defer r.limiter.release(ip)
	r.accepted.Add(1)
	r.active.Add(1)
	defer r.active.Add(-1)

	// TLS 握手放到连接 goroutine 里做：慢握手不能阻塞 accept 循环
	if tc, ok := client.(*tls.Conn); ok {
		client.SetDeadline(time.Now().Add(10 * time.Second))
		if err := tc.Handshake(); err != nil {
			_ = client.Close()
			return
		}
		client.SetDeadline(time.Time{})
	}

	// 多上游：拨号失败就换下一个再试。attempts 取"上游数量"与 2 的较大者、
	// 上限 3 —— 每次都要重新握手，试太多次只会让客户端多等。
	upstream, err := r.dialUpstream()
	if err != nil {
		_ = client.Close()
		return
	}

	// 任一方向结束即关闭双方，避免半开连接占着名额
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, idleReader{src, r.cfg.IdleTimeout.Or(time.Hour)})
		done <- struct{}{}
	}
	go pipe(upstream, client)
	go pipe(client, upstream)
	<-done
	_ = client.Close()
	_ = upstream.Close()
}

// dialUpstream 按上游池顺序尝试拨号，成功即记录健康、失败累计并换下一个
func (r *tcpRelay) dialUpstream() (net.Conn, error) {
	timeout := r.cfg.DialTimeout.Or(10 * time.Second)
	attempts := r.pool.Len()
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 3 {
		attempts = 3
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		target := r.pool.pick()
		conn, err := net.DialTimeout("tcp", target, timeout)
		if err == nil {
			r.pool.markOK(target)
			return conn, nil
		}
		lastErr = err
		r.pool.markFail(target, err)
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的上游")
	}
	return nil, lastErr
}

// idleReader 每次读之前重置读超时：空闲超过 idle 的持久连接会被关闭。
// 用读超时而不是定时扫描连接表：beacon 有心跳，正常连接不会触发；半开/闲置
// 连接会自然退出，不需要额外的状态机。
type idleReader struct {
	conn net.Conn
	idle time.Duration
}

func (r idleReader) Read(p []byte) (int, error) {
	_ = r.conn.SetReadDeadline(time.Now().Add(r.idle))
	return r.conn.Read(p)
}
