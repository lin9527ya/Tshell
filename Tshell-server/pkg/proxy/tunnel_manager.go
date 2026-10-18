package proxy

import (
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/sendcommand"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// TunnelManager 隧道代理管理器
type TunnelManager struct {
	mu        sync.RWMutex
	tunnels   map[int64]*TunnelInstance // key: tunnel ID
	listeners map[int]*net.Listener     // key: server port（含 socks5 的 nil 占位）
}

// TunnelInstance 隧道实例
type TunnelInstance struct {
	ID         int64
	ClientUID  string
	Protocol   string
	ServerPort int
	Username   string
	Password   string
	Status     int
	Listener   net.Listener // TCP监听器
	UDPConn    *net.UDPConn // UDP连接
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// GlobalTunnelManager 全局隧道管理器
var GlobalTunnelManager *TunnelManager

// InitTunnelManager 初始化隧道管理器
func InitTunnelManager() {
	GlobalTunnelManager = &TunnelManager{
		tunnels:   make(map[int64]*TunnelInstance),
		listeners: make(map[int]*net.Listener),
	}
}

// ResetTunnelStatusOnStartup 服务重启时把库中"运行中"的隧道置为已停止，并返回这些记录。
// 隧道不随服务重启自动恢复（需要客户端在线并重新回连 Agent 端口），但若不清状态，
// 界面会一直显示"运行中"而实际没有任何监听，点"停止"还会报"不存在或未运行"。
// 返回被置停的记录供启动日志列出——与监听器同一口径：不自动恢复，但要让人看见。
func ResetTunnelStatusOnStartup() []database.TunnelProxy {
	if database.DB() == nil {
		return nil
	}
	var running []database.TunnelProxy
	if err := database.DB().Where("status = ?", 1).Find(&running); err != nil {
		log.Printf("[TUNNEL] 读取重启前隧道状态失败: %v", err)
		return nil
	}
	if len(running) == 0 {
		return nil
	}
	// Status=0 是零值：xorm 的 Update(&struct) 会跳过零值列，必须用 Cols 显式指定
	if _, err := database.DB().Cols("status").Update(&database.TunnelProxy{Status: 0}); err != nil {
		log.Printf("[TUNNEL] 置停隧道状态失败: %v", err)
	}
	return running
}

// StartTunnel 启动隧道代理
func (tm *TunnelManager) StartTunnel(tunnelID int64) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 从数据库获取隧道配置
	var tunnel database.TunnelProxy
	tunnel.ID = tunnelID
	exists, err := database.DB().Get(&tunnel)
	if err != nil {
		return fmt.Errorf("查询隧道配置失败: %v", err)
	}
	if !exists {
		return fmt.Errorf("隧道不存在")
	}

	// 检查是否已经在运行
	if _, exists := tm.tunnels[tunnelID]; exists {
		return fmt.Errorf("隧道已在运行")
	}

	// 检查客户端连接
	var client database.Clients
	exists, err = database.DB().Where("uid = ? AND online = '1'", tunnel.ClientUID).Get(&client)
	if err != nil || !exists {
		return fmt.Errorf("客户端不在线")
	}

	// 检查端口是否被占用
	if _, exists := tm.listeners[tunnel.ServerPort]; exists {
		return fmt.Errorf("端口 %d 已被占用", tunnel.ServerPort)
	}

	// 根据协议类型启动不同的代理服务
	switch strings.ToLower(tunnel.Protocol) {
	case "socks5":
		// 启动SOCKS5代理
		return tm.startSocks5Tunnel(tunnel)
	case "http":
		// 启动HTTP隧道代理
		return StartHTTPTunnel(tunnel)
	case "tcp":
		// 启动TCP转发
		return tm.startTCPTunnel(tunnel)
	case "udp":
		// 启动UDP转发
		return tm.startUDPTunnel(tunnel)
	default:
		// 未知协议：显式报错，避免"端口监听但不转发"的假状态
		return fmt.Errorf("不支持的隧道协议: %s", tunnel.Protocol)
	}
}

// StopTunnel 停止隧道代理
func (tm *TunnelManager) StopTunnel(tunnelID int64) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 从数据库获取隧道配置
	var tunnel database.TunnelProxy
	tunnel.ID = tunnelID
	exists, err := database.DB().Get(&tunnel)
	if err != nil {
		return fmt.Errorf("查询隧道配置失败: %v", err)
	}
	if !exists {
		return fmt.Errorf("隧道不存在")
	}

	// 根据协议类型停止不同的代理服务
	switch strings.ToLower(tunnel.Protocol) {
	case "socks5":
		// 停止SOCKS5隧道代理
		return tm.stopSocks5Tunnel(tunnelID)
	case "http":
		// 停止HTTP隧道代理
		return StopHTTPTunnel(tunnel)
	case "tcp":
		// 停止TCP转发
		return tm.stopTCPTunnel(tunnelID)
	case "udp":
		// 停止UDP转发
		return tm.stopUDPTunnel(tunnelID)
	default:
		// 未知协议(与 StartTunnel 对称报错;原走 generic 停止逻辑,已随死代码移除)
		return fmt.Errorf("不支持的隧道协议: %s", tunnel.Protocol)
	}
}

func (tm *TunnelManager) startSocks5Tunnel(tunnel database.TunnelProxy) error {
	log.Printf("[TUNNEL-SOCKS5] 🚀 启动SOCKS5隧道，端口: %d", tunnel.ServerPort)

	// 获取客户端信息
	var client database.Clients
	exists, err := database.DB().Where("uid = ? AND online = '1'", tunnel.ClientUID).Get(&client)
	if err != nil || !exists {
		return fmt.Errorf("客户端不在线或不存在")
	}

	// 获取监听器信息以确定客户端连接地址
	var listener database.Listener
	exists, err = database.DB().Where("type = ? AND status = ?", client.Listener, 1).Get(&listener)
	if err != nil || !exists {
		return fmt.Errorf("监听器不存在或未运行")
	}

	// 【关键修复】构建正确的Agent端口 = SOCKS5端口 + 1000
	agentPort := tunnel.ServerPort + 1000

	// 从监听器连接地址中提取IP地址
	serverIP := listener.ConnectAddress
	if strings.Contains(serverIP, ":") {
		serverIP = strings.Split(serverIP, ":")[0]
	}

	// 构建完整的Agent地址和SOCKS5地址
	agentAddress := fmt.Sprintf("%s:%d", serverIP, agentPort)
	socksAddress := fmt.Sprintf(":%d", tunnel.ServerPort)

	log.Printf("[TUNNEL-SOCKS5] 📋 配置信息:")
	log.Printf("[TUNNEL-SOCKS5]    - Agent地址: %s (端口+1000)", agentAddress)
	log.Printf("[TUNNEL-SOCKS5]    - SOCKS5地址: %s", socksAddress)
	log.Printf("[TUNNEL-SOCKS5]    - 用户名: %s", tunnel.Username)
	log.Printf("[TUNNEL-SOCKS5]    - 密码长度: %d", len(tunnel.Password))

	// 检查端口是否被占用
	if _, exists := tm.listeners[tunnel.ServerPort]; exists {
		return fmt.Errorf("端口 %d 已被占用", tunnel.ServerPort)
	}

	// 随机 PSK（服务端与客户端必须一致），替代硬编码 "psk" 防止魔法包握手被预测
	psk := GenerateRandomPSK()

	// 同步绑定 Agent 端口：失败要能返回给启动接口（原实现放在 goroutine 里，
	// 端口被占用时接口仍报"启动成功"、DB 置 1，而实际没有监听）
	if _, err := ReverseSocksServer(agentAddress, socksAddress, psk, tunnel.Username, tunnel.Password); err != nil {
		return err
	}

	// 创建隧道实例
	instance := &TunnelInstance{
		ID:         tunnel.ID,
		ClientUID:  tunnel.ClientUID,
		Protocol:   tunnel.Protocol,
		ServerPort: tunnel.ServerPort,
		Username:   tunnel.Username,
		Password:   tunnel.Password,
		Status:     1,
		Listener:   nil, // SOCKS5使用内置监听器
		CreatedAt:  tunnel.CreatedAt,
		UpdatedAt:  time.Now(),
	}

	// 保存到管理器
	tm.tunnels[tunnel.ID] = instance
	tm.listeners[tunnel.ServerPort] = nil // 占位符，防止端口冲突

	// 通知客户端启动反向SOCKS5连接（复用同一 PSK，保证握手一致）
	go tm.notifyClientStartSocks5(tunnel, agentAddress, psk)

	// 记录审计日志
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategorySystem, "TUNNEL_START", "SUCCESS",
			fmt.Sprintf("SOCKS5隧道 %d 启动成功，端口: %d", tunnel.ID, tunnel.ServerPort),
			map[string]interface{}{
				"tunnel_id":  tunnel.ID,
				"protocol":   tunnel.Protocol,
				"port":       tunnel.ServerPort,
				"client_uid": tunnel.ClientUID,
			})
	}

	log.Printf("[TUNNEL-SOCKS5] ✅ SOCKS5隧道启动成功")
	return nil
}

// notifyClientStartSocks5 通知客户端启动反向SOCKS5连接
func (tm *TunnelManager) notifyClientStartSocks5(tunnel database.TunnelProxy, agentAddress, psk string) {
	log.Printf("[TUNNEL-SOCKS5] 📤 通知客户端启动SOCKS5连接...")

	// 构建命令参数 - 格式: serverAddress|psk|useTLS（psk 与 ReverseSocksServer 保持一致）
	cmdStr := fmt.Sprintf("%s|%s|false", agentAddress, psk)

	log.Printf("[TUNNEL-SOCKS5] 🔧 发送SOCKS5启动命令: %s", cmdStr)

	// 使用现有的命令发送机制
	sendcommand.SendCommand(tunnel.ClientUID, "socks5 "+cmdStr)

	log.Printf("[TUNNEL-SOCKS5] ✅ 客户端通知已发送")
}

// stopSocks5Tunnel 停止SOCKS5隧道
func (tm *TunnelManager) stopSocks5Tunnel(tunnelID int64) error {
	log.Printf("[TUNNEL-SOCKS5] 🚫 停止SOCKS5隧道: %d", tunnelID)

	// 检查隧道是否在内存中存在
	instance, exists := tm.tunnels[tunnelID]
	if !exists {
		// 【关键修复】隧道不在内存中，但仍然尝试停止可能的服务器
		log.Printf("[TUNNEL-SOCKS5] ⚠️ 隧道不在内存管理器中，但尝试清理可能的服务器...")

		// 尝试根据tunnelID从数据库获取端口信息来清理
		var tunnel database.TunnelProxy
		tunnel.ID = tunnelID
		exists, err := database.DB().Get(&tunnel)
		if err == nil && exists {
			socksAddress := fmt.Sprintf(":%d", tunnel.ServerPort)
			agentAddress := fmt.Sprintf(":%d", tunnel.ServerPort+1000)

			// 尝试停止SOCKS5服务器
			StopSocks5Server(socksAddress)
			StopSocks5Server(agentAddress)

			log.Printf("[TUNNEL-SOCKS5] 🧹 已清理端口 %d 和 %d 的服务器", tunnel.ServerPort, tunnel.ServerPort+1000)
		}

		log.Printf("[TUNNEL-SOCKS5] ✅ SOCKS5隧道清理完成（无内存实例）")
		return nil
	}

	// 停止SOCKS5服务器
	socksAddress := fmt.Sprintf(":%d", instance.ServerPort)
	agentAddress := fmt.Sprintf(":%d", instance.ServerPort+1000)
	StopSocks5Server(socksAddress)
	StopSocks5Server(agentAddress)

	// 从管理器中移除
	delete(tm.tunnels, tunnelID)
	delete(tm.listeners, instance.ServerPort)

	// 记录审计日志
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategorySystem, "TUNNEL_STOP", "SUCCESS",
			fmt.Sprintf("SOCKS5隧道 %d 停止成功，端口: %d", tunnelID, instance.ServerPort),
			map[string]interface{}{
				"tunnel_id":  tunnelID,
				"protocol":   instance.Protocol,
				"port":       instance.ServerPort,
				"client_uid": instance.ClientUID,
			})
	}

	log.Printf("[TUNNEL-SOCKS5] ✅ SOCKS5隧道停止成功")
	return nil
}

// startTCPTunnel 启动TCP隧道
// startTCPTunnel / stopTCPTunnel 实现已移至 tcp_tunnel.go（基于反向mux的真TCP转发，经客户端穿透）

// stopTCPTunnel 实现已移至 tcp_tunnel.go

// startUDPTunnel / stopUDPTunnel 实现已移至 udp_tunnel.go（基于反向mux的真UDP转发，经客户端穿透）
