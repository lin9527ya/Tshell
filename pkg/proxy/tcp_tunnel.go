package proxy

import (
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/proxy/mux"
	"BackendTemplate/pkg/sendcommand"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// tcpTunnelInstance TCP转发隧道实例（服务端）。
// 复用 HTTP 隧道(http_tunnel.go)的反向mux架构与 httpMagicPacket 认证；客户端侧完全复用
// handleHTTPTunnelStream（读 target\n -> 拨号 -> 回 OK -> 双向转发），故本文件只实现服务端。
//
// 限制：TCP转发与HTTP隧道共用客户端的隧道Agent(同一个 http_tunnel_connect 命令 + 全局 agent)，
// 同一客户端上两者建议不要同时运行；SOCKS5 与 TCP转发 可共存（各自独立 agent）。
type tcpTunnelInstance struct {
	tunnelID        int64
	clientUID       string
	serverPort      int
	agentPort       int
	targetAddress   string
	psk             string
	session         *mux.Mux
	agentListener   *net.TCPListener
	forwardListener *net.TCPListener
}

var (
	tcpTunnelMu sync.Mutex
	tcpTunnels  = make(map[int64]*tcpTunnelInstance) // 按 tunnel.ID 索引
)

// startTCPTunnel 启动TCP转发隧道：服务端监听 ServerPort，每个入站连接经客户端穿透到 TargetAddress。
// 通过 http_tunnel_connect 命令让客户端回连，客户端用 handleHTTPTunnelStream 处理每条流。
func (tm *TunnelManager) startTCPTunnel(tunnel database.TunnelProxy) error {
	log.Printf("[TUNNEL-TCP] 🚀 启动TCP转发隧道: ID=%d, 端口=%d, 目标=%s", tunnel.ID, tunnel.ServerPort, tunnel.TargetAddress)

	if tunnel.TargetAddress == "" {
		return fmt.Errorf("TCP转发缺少目标地址")
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
	forwardListener, err := net.ListenTCP("tcp", &net.TCPAddr{Port: tunnel.ServerPort})
	if err != nil {
		agentListener.Close()
		return fmt.Errorf("启动TCP转发监听器失败: %v", err)
	}

	inst := &tcpTunnelInstance{
		tunnelID:        tunnel.ID,
		clientUID:       tunnel.ClientUID,
		serverPort:      tunnel.ServerPort,
		agentPort:       agentPort,
		targetAddress:   tunnel.TargetAddress,
		psk:             psk,
		agentListener:   agentListener,
		forwardListener: forwardListener,
	}

	tcpTunnelMu.Lock()
	tcpTunnels[tunnel.ID] = inst
	tcpTunnelMu.Unlock()

	go tcpTunnelAgentServer(inst)

	// 通知客户端回连（复用 http_tunnel_connect；客户端连接后用 handleHTTPTunnelStream 处理流）
	connectCmd := fmt.Sprintf("%s:%d|%s", serverIP, agentPort, psk)
	sendcommand.SendCommand(tunnel.ClientUID, "http_tunnel_connect "+connectCmd)
	log.Printf("[TUNNEL-TCP] 📤 已通知客户端回连: %s:%d -> 目标 %s", serverIP, agentPort, tunnel.TargetAddress)

	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategorySystem, "TCP_TUNNEL_START", "SUCCESS",
			fmt.Sprintf("TCP转发隧道 %d 启动，端口:%d -> %s", tunnel.ID, tunnel.ServerPort, tunnel.TargetAddress),
			map[string]interface{}{"tunnel_id": tunnel.ID, "port": tunnel.ServerPort, "target": tunnel.TargetAddress, "client_uid": tunnel.ClientUID})
	}
	return nil
}

// tcpTunnelAgentServer 接受客户端回连，魔法包认证后建立mux会话，随后启动转发监听器。
func tcpTunnelAgentServer(inst *tcpTunnelInstance) {
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
			log.Printf("[TUNNEL-TCP] ❌ 读取魔法包失败: %v", err)
			conn.Close()
			continue
		}
		magic := DeriveMagicPacket(inst.psk)
		if !bytes.Equal(buffer, magic[:]) {
			log.Printf("[TUNNEL-TCP] ❌ 魔法包验证失败")
			conn.Close()
			continue
		}
		conn.SetReadDeadline(time.Time{})
		session := mux.Client(conn, inst.psk)
		inst.session = session
		log.Printf("[TUNNEL-TCP] ✅ Agent已连接，mux会话已建立，启动转发监听器")
		go tcpTunnelForwardServer(inst)
		// agent 断开时关闭转发监听器，使 tcpTunnelForwardServer 退出，避免泄漏
		go func() {
			<-session.Done()
			inst.forwardListener.Close()
		}()
		return // 1对1，处理完一个 agent 即退出
	}
}

// tcpTunnelForwardServer 接受外部TCP连接，每个连接经mux流转发到客户端的目标。
func tcpTunnelForwardServer(inst *tcpTunnelInstance) {
	for {
		conn, err := inst.forwardListener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go handleTCPForwardClient(conn, inst)
	}
}

// handleTCPForwardClient 处理单个入站连接：开流 -> 发目标 -> 读OK -> 双向转发。
func handleTCPForwardClient(clientConn net.Conn, inst *tcpTunnelInstance) {
	defer clientConn.Close()
	if inst.session == nil {
		return
	}
	stream, err := inst.session.OpenStream()
	if err != nil {
		log.Printf("[TUNNEL-TCP] ❌ 创建流失败: %v", err)
		return
	}
	defer stream.Close()

	// 发送目标地址（与 handleHTTPTunnelStream 协议一致：target\n）
	if _, err := stream.Write([]byte(inst.targetAddress + "\n")); err != nil {
		log.Printf("[TUNNEL-TCP] ❌ 发送目标地址失败: %v", err)
		return
	}
	// 读取客户端确认（OK=成功 / ER=失败），用 io.ReadFull 避免短读误判
	buf := make([]byte, 2)
	if _, err := io.ReadFull(stream, buf); err != nil {
		log.Printf("[TUNNEL-TCP] ❌ 读取确认失败: %v", err)
		return
	}
	if string(buf) != "OK" {
		log.Printf("[TUNNEL-TCP] ❌ 客户端连接目标失败: %s", string(buf))
		return
	}

	connID := fmt.Sprintf("tcp_%d_%d", inst.tunnelID, time.Now().UnixNano())
	// 复用 HTTP 隧道的双向转发（已含半关闭修复）
	startHTTPTunnelBidirectionalForward(clientConn, stream, connID)
}

// stopTCPTunnel 停止TCP转发隧道。
func (tm *TunnelManager) stopTCPTunnel(tunnelID int64) error {
	tcpTunnelMu.Lock()
	inst, exists := tcpTunnels[tunnelID]
	if exists {
		delete(tcpTunnels, tunnelID)
	}
	tcpTunnelMu.Unlock()
	if !exists {
		return fmt.Errorf("TCP隧道不存在或未运行")
	}
	if inst.session != nil {
		inst.session.Close()
	}
	if inst.agentListener != nil {
		inst.agentListener.Close()
	}
	if inst.forwardListener != nil {
		inst.forwardListener.Close()
	}
	log.Printf("[TUNNEL-TCP] ✅ TCP转发隧道已停止: ID=%d", tunnelID)
	return nil
}
