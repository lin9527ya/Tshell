package main

import (
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// udpRelay UDP 中继：为每个客户端地址维护一条到上游的 UDP 流。
//
// KCP 传输走 UDP，TCP 中继覆盖不到；这里做一张简单的流表。
// 与 TCP 中继同样的原则：只搬字节、不解析 KCP 协议、不持有密钥。
type udpRelay struct {
	cfg   Config
	conn  *net.UDPConn
	flows sync.Map // clientAddr string -> *udpFlow

	count atomic.Int64 // 当前流数
	// 统计
	datagramsIn  atomic.Uint64
	datagramsOut atomic.Uint64
	rejected     atomic.Uint64

	done chan struct{}
}

type udpFlow struct {
	upstream *net.UDPConn
	client   *net.UDPAddr
	lastSeen atomic.Int64 // unix nano
}

func newUDPRelay(cfg Config) *udpRelay {
	return &udpRelay{cfg: cfg, done: make(chan struct{})}
}

// Serve 启动 UDP 监听并阻塞
func (r *udpRelay) Serve() error {
	addr, err := net.ResolveUDPAddr("udp", r.cfg.UDPListen)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	return r.ServePacket(conn)
}

// ServePacket 在已有 UDP socket 上服务（测试用 :0 拿随机端口时使用）
func (r *udpRelay) ServePacket(conn *net.UDPConn) error {
	r.conn = conn
	log.Printf("🔀 UDP 中继监听 %s → %s", r.cfg.UDPListen, r.cfg.UDPUpstream)

	go r.reapLoop()

	buf := make([]byte, 65535)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-r.done:
				return nil
			default:
			}
			log.Printf("⚠️ UDP 读取失败: %v", err)
			continue
		}
		r.datagramsIn.Add(1)

		f, err := r.flowFor(clientAddr)
		if err != nil {
			r.rejected.Add(1)
			continue
		}
		f.lastSeen.Store(time.Now().UnixNano())
		if _, err := f.upstream.Write(buf[:n]); err != nil {
			r.closeFlow(clientAddr)
		}
	}
}

// Close 停止中继并关闭所有流
func (r *udpRelay) Close() error {
	select {
	case <-r.done:
	default:
		close(r.done)
	}
	if r.conn != nil {
		_ = r.conn.Close()
	}
	r.flows.Range(func(k, _ interface{}) bool {
		if addr, ok := k.(string); ok {
			r.closeFlowByKey(addr)
		}
		return true
	})
	return nil
}

func (r *udpRelay) flowFor(client *net.UDPAddr) (*udpFlow, error) {
	key := client.String()
	if v, ok := r.flows.Load(key); ok {
		return v.(*udpFlow), nil
	}
	max := r.cfg.MaxUDPFlows
	if r.count.Load() >= int64(max) {
		return nil, errTooManyFlows
	}
	upstreamAddr, err := net.ResolveUDPAddr("udp", r.cfg.UDPUpstream)
	if err != nil {
		return nil, err
	}
	up, err := net.DialUDP("udp", nil, upstreamAddr)
	if err != nil {
		return nil, err
	}
	f := &udpFlow{upstream: up, client: client}
	f.lastSeen.Store(time.Now().UnixNano())
	// 并发创建同一 key 时以先存入者为准，多余的关闭
	if actual, loaded := r.flows.LoadOrStore(key, f); loaded {
		_ = up.Close()
		return actual.(*udpFlow), nil
	}
	r.count.Add(1)
	go r.returnLoop(key, f)
	return f, nil
}

// returnLoop 上游 → 客户端方向
func (r *udpRelay) returnLoop(key string, f *udpFlow) {
	buf := make([]byte, 65535)
	for {
		_ = f.upstream.SetReadDeadline(time.Now().Add(r.cfg.UDPIdleTimeout.Or(60 * time.Second)))
		n, err := f.upstream.Read(buf)
		if err != nil {
			select {
			case <-r.done:
			default:
				r.closeFlowByKey(key)
			}
			return
		}
		r.datagramsOut.Add(1)
		if _, err := r.conn.WriteToUDP(buf[:n], f.client); err != nil {
			r.closeFlowByKey(key)
			return
		}
	}
}

// reapLoop 回收空闲流（上游不再回包时 returnLoop 也会退出，这里兜底）
func (r *udpRelay) reapLoop() {
	ticker := time.NewTicker(r.cfg.UDPIdleTimeout.Or(60*time.Second) / 2)
	defer ticker.Stop()
	cutoff := r.cfg.UDPIdleTimeout.Or(60 * time.Second)
	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			now := time.Now()
			r.flows.Range(func(k, v interface{}) bool {
				f := v.(*udpFlow)
				if now.Sub(time.Unix(0, f.lastSeen.Load())) > cutoff {
					r.closeFlowByKey(k.(string))
				}
				return true
			})
		}
	}
}

func (r *udpRelay) closeFlow(client *net.UDPAddr) { r.closeFlowByKey(client.String()) }

func (r *udpRelay) closeFlowByKey(key string) {
	v, ok := r.flows.LoadAndDelete(key)
	if !ok {
		return
	}
	f := v.(*udpFlow)
	_ = f.upstream.Close()
	r.count.Add(-1)
}

var errTooManyFlows = &flowLimitError{}

type flowLimitError struct{}

func (*flowLimitError) Error() string { return "UDP 流数量达到上限" }
