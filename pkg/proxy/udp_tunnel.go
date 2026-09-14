package proxy

import (
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/proxy/mux"
	"BackendTemplate/pkg/sendcommand"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// udpTunnelInstance UDP转发隧道实例（经客户端穿透，复用反向mux架构与 httpMagicPacket）。
// 协议：每条UDP数据报独占一个mux流。服务端开流 -> 发目标(2B长度+addr) -> 发数据报(4B长度+data)；
// 客户端拨号UDP目标、发送、读响应(5s超时) -> 回写响应(4B长度+data) -> 关流。适合DNS等请求-响应场景。
type udpTunnelInstance struct {
	tunnelID      int64
	clientUID     string
	serverPort    int
	agentPort     int
	targetAddress string
	psk           string
	session       atomic.Pointer[mux.Mux]
	agentListener *net.TCPListener
	udpListener   *net.UDPConn
}

var (
	udpTunnelMu sync.Mutex
	udpTunnels  = make(map[int64]*udpTunnelInstance) // 按 tunnel.ID 索引
)

// maxConcurrentUDPDatagrams UDP 数据报并发上限：
// 原实现每个数据报 spawn 一个 goroutine + 一条 mux 流，洪泛时无界；
// 改为信号量限流，满则阻塞读循环 → 内核 UDP 缓冲溢出丢包（对 UDP 是正确背压）。
const maxConcurrentUDPDatagrams = 256

var udpDatagramSem = make(chan struct{}, maxConcurrentUDPDatagrams)

// startUDPTunnel 启动UDP转发隧道：服务端监听 ServerPort(UDP)，每个数据报经客户端穿透到 TargetAddress。
// 通过 http_tunnel_connect 命令(第3段 udp 标识)让客户端回连，客户端用 handleUDPTunnelStream 处理每条流。
func (tm *TunnelManager) startUDPTunnel(tunnel database.TunnelProxy) error {
	log.Printf("[TUNNEL-UDP] 🚀 启动UDP转发隧道: ID=%d, 端口=%d, 目标=%s", tunnel.ID, tunnel.ServerPort, tunnel.TargetAddress)

	if tunnel.TargetAddress == "" {
		return fmt.Errorf("UDP转发缺少目标地址")
	}
	var client database.Clients
	exists, err := database.Engine.Where("uid = ? AND online = '1'", tunnel.ClientUID).Get(&client)
	if err != nil || !exists {
		return fmt.Errorf("客户端不在线或不存在")
	}
	var listener database.Listener
	exists, err = database.Engine.Where("type = ? AND status = ?", client.Listener, 1).Get(&listener)
	if err != nil || !exists {
		return fmt.Errorf("监听器不存在或未运行")
	}

	agentPort := tunnel.ServerPort + 1000
	serverIP := listener.ConnectAddress
	if strings.Contains(serverIP, ":") {
		serverIP = strings.Split(serverIP, ":")[0]
	}
	psk := GenerateRandomPSK()

	agentListener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: agentPort})
	if err != nil {
		return fmt.Errorf("启动Agent监听器失败: %v", err)
	}
	udpAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", tunnel.ServerPort))
	if err != nil {
		agentListener.Close()
		return fmt.Errorf("解析UDP地址失败: %v", err)
	}
	udpListener, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		agentListener.Close()
		return fmt.Errorf("启动UDP转发监听器失败: %v", err)
	}

	inst := &udpTunnelInstance{
		tunnelID:      tunnel.ID,
		clientUID:     tunnel.ClientUID,
		serverPort:    tunnel.ServerPort,
		agentPort:     agentPort,
		targetAddress: tunnel.TargetAddress,
		psk:           psk,
		agentListener: agentListener,
		udpListener:   udpListener,
	}

	udpTunnelMu.Lock()
	udpTunnels[tunnel.ID] = inst
	udpTunnelMu.Unlock()

	go udpTunnelAgentServer(inst)

	// 通知客户端回连（复用 http_tunnel_connect，第3段 udp 标识UDP模式）
	connectCmd := fmt.Sprintf("%s:%d|%s|udp", serverIP, agentPort, psk)
	sendcommand.SendCommand(tunnel.ClientUID, "http_tunnel_connect "+connectCmd)
	log.Printf("[TUNNEL-UDP] 📤 已通知客户端回连: %s:%d -> 目标 %s", serverIP, agentPort, tunnel.TargetAddress)

	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategorySystem, "UDP_TUNNEL_START", "SUCCESS",
			fmt.Sprintf("UDP转发隧道 %d 启动，端口:%d -> %s", tunnel.ID, tunnel.ServerPort, tunnel.TargetAddress),
			map[string]interface{}{"tunnel_id": tunnel.ID, "port": tunnel.ServerPort, "target": tunnel.TargetAddress, "client_uid": tunnel.ClientUID})
	}
	return nil
}

// udpTunnelAgentServer 接受客户端回连，魔法包认证后建立mux会话，随后启动UDP转发循环。
func udpTunnelAgentServer(inst *udpTunnelInstance) {
	for {
		conn, err := inst.agentListener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		buffer := make([]byte, 64)
		if _, err := io.ReadFull(conn, buffer); err != nil {
			log.Printf("[TUNNEL-UDP] ❌ 读取魔法包失败: %v", err)
			conn.Close()
			continue
		}
		magic := DeriveMagicPacket(inst.psk)
		if !bytes.Equal(buffer, magic[:]) {
			log.Printf("[TUNNEL-UDP] ❌ 魔法包验证失败")
			conn.Close()
			continue
		}
		conn.SetReadDeadline(time.Time{})
		session := mux.Client(conn, inst.psk)
		inst.session.Store(session)
		log.Printf("[TUNNEL-UDP] ✅ Agent已连接，mux会话已建立，启动UDP转发循环")
		go udpTunnelForwardLoop(inst)
		// agent 断开时关闭UDP监听器，使 udpTunnelForwardLoop 退出
		go func() {
			<-session.Done()
			inst.udpListener.Close()
		}()
		return // 1对1
	}
}

// udpTunnelForwardLoop 接收外部UDP数据报，每个经mux流转发到客户端的目标。
func udpTunnelForwardLoop(inst *udpTunnelInstance) {
	buf := make([]byte, 65535)
	for {
		n, srcAddr, err := inst.udpListener.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		// 拷贝数据（ReadFromUDP 复用 buf）
		data := make([]byte, n)
		copy(data, buf[:n])
		session := inst.session.Load()
		// 信号量限流：满则阻塞读循环（背压到内核丢包），避免 goroutine/流无界增长
		udpDatagramSem <- struct{}{}
		go func() {
			defer func() { <-udpDatagramSem }()
			handleUDPForwardDatagram(inst.udpListener, srcAddr, data, session, inst.targetAddress)
		}()
	}
}

// handleUDPForwardDatagram 处理单个UDP数据报：开流 -> 发目标+数据报 -> 读响应 -> 回srcAddr。
func handleUDPForwardDatagram(udpListener *net.UDPConn, srcAddr *net.UDPAddr, data []byte, session *mux.Mux, targetAddress string) {
	if session == nil {
		return
	}
	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[TUNNEL-UDP] ❌ 创建流失败: %v", err)
		return
	}
	defer stream.Close()
	stream.SetReadDeadline(time.Now().Add(6 * time.Second))
	defer stream.SetReadDeadline(time.Time{})

	// 发目标 addr (2B len + addr)
	var tlen [2]byte
	binary.BigEndian.PutUint16(tlen[:], uint16(len(targetAddress)))
	if _, err := stream.Write(tlen[:]); err != nil {
		return
	}
	if _, err := stream.Write([]byte(targetAddress)); err != nil {
		return
	}
	// 发数据报 (4B len + data)
	var dlen [4]byte
	binary.BigEndian.PutUint32(dlen[:], uint32(len(data)))
	if _, err := stream.Write(dlen[:]); err != nil {
		return
	}
	if _, err := stream.Write(data); err != nil {
		return
	}
	// 读响应 (4B len + data)
	var rlen [4]byte
	if _, err := io.ReadFull(stream, rlen[:]); err != nil {
		return
	}
	respLen := binary.BigEndian.Uint32(rlen[:])
	if respLen == 0 {
		return // 客户端UDP失败，发回0长度
	}
	resp := make([]byte, respLen)
	if _, err := io.ReadFull(stream, resp); err != nil {
		return
	}
	udpListener.WriteToUDP(resp, srcAddr)
}

// stopUDPTunnel 停止UDP转发隧道。
func (tm *TunnelManager) stopUDPTunnel(tunnelID int64) error {
	udpTunnelMu.Lock()
	inst, exists := udpTunnels[tunnelID]
	if exists {
		delete(udpTunnels, tunnelID)
	}
	udpTunnelMu.Unlock()
	if !exists {
		return fmt.Errorf("UDP隧道不存在或未运行")
	}
	if session := inst.session.Load(); session != nil {
		session.Close()
	}
	if inst.agentListener != nil {
		inst.agentListener.Close()
	}
	if inst.udpListener != nil {
		inst.udpListener.Close()
	}
	log.Printf("[TUNNEL-UDP] ✅ UDP转发隧道已停止: ID=%d", tunnelID)
	return nil
}
