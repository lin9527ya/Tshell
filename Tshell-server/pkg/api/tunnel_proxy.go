package api

import (
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/proxy"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"BackendTemplate/pkg/utils/logx"
	"github.com/gin-gonic/gin"
)

// TunnelProxyRequest 隧道代理请求结构
// tunnelPasswordMask 隧道密码的固定掩码：不回显任何字符，也不泄露长度。
// 含 maskedMarker（****），后端据此识别"这是掩码，不是真实值"。
const tunnelPasswordMask = "******"

// maskTunnelPassword 密码脱敏；空密码保持空（前端据此显示 "-"）
func maskTunnelPassword(p string) string {
	if p == "" {
		return ""
	}
	return tunnelPasswordMask
}

// checkTunnelPortAvailable 校验业务端口及其 Agent 端口（= 端口 + 1000）在服务端范围内可用。
// Agent 端口也参与占用判定 —— 端口 5000 的隧道与端口 6000 的隧道会抢 6000
// （原实现只校验 server_port 唯一）。
// excludeTunnelID：更新时排除自身记录（传 0 表示不排除）。
func checkTunnelPortAvailable(port int, excludeTunnelID int64) error {
	if port < 1 || port > 65535-1000 {
		return fmt.Errorf("端口需在 1-%d 之间（Agent 端口 = 端口+1000 也须合法）", 65535-1000)
	}
	// 需要避开的既有 server_port：本端口 / 本端口的 Agent 端口 / 以本端口为 Agent 端口的记录
	conflict := []int{port, port + 1000}
	if port > 1000 {
		conflict = append(conflict, port-1000)
	}

	tun, has, err := database.Tunnels().PortConflict(conflict, excludeTunnelID)
	if err != nil {
		return fmt.Errorf("检查端口占用失败: %w", err)
	}
	if has {
		return fmt.Errorf("端口 %d 与已有隧道冲突（Agent 端口 = 端口+1000）：隧道 %d 占用 %d", port, tun.ID, tun.ServerPort)
	}
	return nil
}

// requireTunnelAuth socks5/http 隧道的端口是直接对外提供服务的：不设用户名密码时，
// SOCKS5 侧会降级为 NoAuth、HTTP 侧此前完全没有认证 —— 等同于在服务器上开了一个
// 公开代理。故这两类协议强制要求认证；tcp/udp 是按目标地址的定向转发，不涉及。
func requireTunnelAuth(protocol, username, password string) error {
	switch strings.ToLower(protocol) {
	case "socks5", "http":
		if username == "" || password == "" {
			return fmt.Errorf("%s 隧道必须设置用户名和密码（无认证时等同于公开代理）", protocol)
		}
	}
	return nil
}

// validateTunnelTarget tcp/udp 必须填写可解析的 host:port 目标；
// 创建/更新时就拦住，避免"创建成功、启动时才报缺目标"。
func validateTunnelTarget(protocol, target string) error {
	switch strings.ToLower(protocol) {
	case "tcp", "udp":
		target = strings.TrimSpace(target)
		if target == "" {
			return fmt.Errorf("%s 隧道必须填写目标地址（host:port）", protocol)
		}
		host, portStr, err := net.SplitHostPort(target)
		if err != nil || strings.TrimSpace(host) == "" {
			return fmt.Errorf("目标地址格式错误，应为 host:port（IPv6 使用 [::1]:80）")
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("目标端口非法: %s", portStr)
		}
	}
	return nil
}

type TunnelProxyRequest struct {
	ClientUID     string `json:"client_uid" binding:"required"`
	Protocol      string `json:"protocol" binding:"required,oneof=socks5 http tcp udp"`
	ServerPort    int    `json:"server_port" binding:"required,min=1,max=65535"`
	TargetAddress string `json:"target_address"` // TCP/UDP代理的目标地址(IP:端口)，对SOCKS5/HTTP隧道可选
	Note          string `json:"note"`
	Username      string `json:"username"`
	Password      string `json:"password"`
}

// TunnelProxyResponse 隧道代理响应结构
type TunnelProxyResponse struct {
	ID            int64       `json:"id"`
	ClientUID     string      `json:"client_uid"`
	Protocol      string      `json:"protocol"`
	ServerPort    int         `json:"server_port"`
	TargetAddress string      `json:"target_address"` // TCP/UDP代理的目标地址
	Note          string      `json:"note"`
	Username      string      `json:"username"`
	Password      string      `json:"password"`
	Status        int         `json:"status"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	ClientInfo    *ClientInfo `json:"client_info,omitempty"`
}

// TunnelProxyListResponse 隧道代理列表响应
type TunnelProxyListResponse struct {
	Data       []TunnelProxyResponse `json:"data"`
	Total      int64                 `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	TotalPages int                   `json:"total_pages"`
}

// CreateTunnelProxy 创建隧道代理
func CreateTunnelProxy(c *gin.Context) {
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_CREATE", "START", "开始创建隧道代理", nil)
	}

	var req TunnelProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_CREATE", "FAILURE", "参数错误: "+err.Error(), nil)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	// 列表回给前端的是掩码：粘贴过来的掩码会被当成真实密码，直接拒绝
	if isMaskedValue(req.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "检测到脱敏掩码，请填写真实密码"})
		return
	}
	if err := requireTunnelAuth(req.Protocol, req.Username, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateTunnelTarget(req.Protocol, req.TargetAddress); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查客户端是否存在且在线
	_, exists, err := database.Clients_().OnlineByUID(req.ClientUID)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_CREATE", "FAILURE", "查询客户端失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询客户端失败"})
		return
	}
	if !exists {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_CREATE", "FAILURE", "客户端不存在或不在线", nil)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "客户端不存在或不在线"})
		return
	}

	// 端口占用（含 Agent 端口 = 端口+1000）
	if err := checkTunnelPortAvailable(req.ServerPort, 0); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 创建隧道代理记录
	now := time.Now()
	tunnel := database.TunnelProxy{
		ClientUID:     req.ClientUID,
		Protocol:      req.Protocol,
		ServerPort:    req.ServerPort,
		TargetAddress: req.TargetAddress,
		Note:          req.Note,
		Username:      req.Username,
		Password:      req.Password,
		Status:        0, // 默认停止状态
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	_, err = database.Tunnels().Insert(&tunnel)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_CREATE", "FAILURE", "创建隧道代理失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建隧道代理失败"})
		return
	}

	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_CREATE", "SUCCESS", "隧道代理创建成功", nil)
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "隧道代理创建成功",
		"data": TunnelProxyResponse{
			ID:            tunnel.ID,
			ClientUID:     tunnel.ClientUID,
			Protocol:      tunnel.Protocol,
			ServerPort:    tunnel.ServerPort,
			TargetAddress: tunnel.TargetAddress,
			Note:          tunnel.Note,
			Username:      tunnel.Username,
			Password:      maskTunnelPassword(tunnel.Password),
			Status:        tunnel.Status,
			CreatedAt:     tunnel.CreatedAt,
			UpdatedAt:     tunnel.UpdatedAt,
		},
	})
}

// GetTunnelProxyList 获取隧道代理列表
func GetTunnelProxyList(c *gin.Context) {
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_LIST", "START", "开始查询隧道代理列表", nil)
	}

	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 获取搜索参数
	clientUID := c.Query("client_uid")
	protocol := c.Query("protocol")
	status := c.Query("status")

	// 构建查询
	session := database.DB().NewSession()
	defer session.Close()

	if clientUID != "" {
		session = session.Where("client_uid LIKE ?", "%"+clientUID+"%")
	}
	if protocol != "" {
		session = session.Where("protocol = ?", protocol)
	}
	if status != "" {
		session = session.Where("status = ?", status)
	}

	// 获取总数
	total, err := session.Count(&database.TunnelProxy{})
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_LIST", "FAILURE", "查询总数失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	// 获取数据
	var tunnels []database.TunnelProxy
	err = session.Desc("created_at").Limit(pageSize, (page-1)*pageSize).Find(&tunnels)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_LIST", "FAILURE", "查询数据失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}

	// 转换响应格式并获取客户端信息（【安全修复】密码脱敏：仅返回是否已设置）
	// 批量获取客户端信息（避免 N+1 查询：每行一次 Where(uid)）
	clientUIDs := make([]string, 0, len(tunnels))
	seenUIDs := make(map[string]bool)
	for _, tunnel := range tunnels {
		if tunnel.ClientUID != "" && !seenUIDs[tunnel.ClientUID] {
			seenUIDs[tunnel.ClientUID] = true
			clientUIDs = append(clientUIDs, tunnel.ClientUID)
		}
	}
	clientMap := make(map[string]database.Clients, len(clientUIDs))
	if len(clientUIDs) > 0 {
		if clients, err := database.Clients_().GetByUIDs(clientUIDs); err == nil {
			for _, cl := range clients {
				clientMap[cl.Uid] = cl
			}
		}
	}

	var responseList []TunnelProxyResponse
	for _, tunnel := range tunnels {
		hasPass := maskTunnelPassword(tunnel.Password)
		response := TunnelProxyResponse{
			ID:            tunnel.ID,
			ClientUID:     tunnel.ClientUID,
			Protocol:      tunnel.Protocol,
			ServerPort:    tunnel.ServerPort,
			TargetAddress: tunnel.TargetAddress,
			Note:          tunnel.Note,
			Username:      tunnel.Username,
			Password:      hasPass,
			Status:        tunnel.Status,
			CreatedAt:     tunnel.CreatedAt,
			UpdatedAt:     tunnel.UpdatedAt,
		}

		// 从批量结果 map 中取客户端信息（一次查询替代每行一次）
		if client, ok := clientMap[tunnel.ClientUID]; ok {
			response.ClientInfo = &ClientInfo{
				Computer:   client.Computer,
				ExternalIP: client.ExternalIP,
				Username:   client.Username,
				Online:     client.Online,
				Listener:   client.Listener,
			}
		}

		responseList = append(responseList, response)
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_LIST", "SUCCESS", "查询隧道代理列表成功", nil)
	}
	c.JSON(http.StatusOK, TunnelProxyListResponse{
		Data:       responseList,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// UpdateTunnelProxy 更新隧道代理
func UpdateTunnelProxy(c *gin.Context) {
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "START", "开始更新隧道代理", nil)
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "FAILURE", "无效的ID", nil)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	var req TunnelProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "FAILURE", "参数错误: "+err.Error(), nil)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	tunnel, exists, err := database.Tunnels().GetByID(id)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "FAILURE", "查询隧道代理失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询隧道代理失败"})
		return
	}
	if !exists {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "FAILURE", "隧道代理不存在", nil)
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "隧道代理不存在"})
		return
	}

	// 列表回给前端的是固定掩码，编辑框会原样回传：视为"不修改"，否则改个备注就会把
	// 密码写成掩码，socks5/http 认证随即失效。必须放在凭据校验之前——这样"记录本无密码
	// 却回传掩码"会归一成空密码并被校验拦下。
	if isMaskedValue(req.Password) {
		req.Password = tunnel.Password
	}

	if err := requireTunnelAuth(req.Protocol, req.Username, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateTunnelTarget(req.Protocol, req.TargetAddress); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 运行中的隧道已在内存里绑定监听器/Agent 会话，直接改库会造成"界面配置 A、
	// 实际运行 B"，甚至停止时按新协议去找旧监听器。仅允许运行中改备注。
	if tunnel.Status == 1 {
		protocolChanged := !strings.EqualFold(req.Protocol, tunnel.Protocol)
		targetChanged := (strings.EqualFold(req.Protocol, "tcp") || strings.EqualFold(req.Protocol, "udp")) &&
			strings.TrimSpace(req.TargetAddress) != strings.TrimSpace(tunnel.TargetAddress)
		if req.ClientUID != tunnel.ClientUID || protocolChanged || req.ServerPort != tunnel.ServerPort ||
			targetChanged || req.Username != tunnel.Username || req.Password != tunnel.Password {
			c.JSON(http.StatusBadRequest, gin.H{"error": "隧道正在运行，请先停止后再修改连接配置（备注可直接编辑）"})
			return
		}
	}

	if req.ServerPort != tunnel.ServerPort {
		if err := checkTunnelPortAvailable(req.ServerPort, id); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// 构建更新数据映射，确保字段正确映射
	updateData := map[string]interface{}{
		"client_uid":     req.ClientUID,
		"protocol":       req.Protocol,
		"server_port":    req.ServerPort,
		"target_address": req.TargetAddress,
		"note":           req.Note,
		"username":       req.Username,
		"password":       req.Password,
		"updated_at":     time.Now(),
	}

	// 使用ID作为唯一标识符进行更新，避免使用其他可能变更的字段
	// 使用Where条件避免xorm的ID方法表引用问题
	affected, err := database.Tunnels().UpdateMap(id, updateData)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "FAILURE", "更新隧道代理失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新隧道代理失败: " + err.Error()})
		return
	}

	// 检查是否实际更新了记录
	if affected == 0 {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "FAILURE", "未找到要更新的隧道记录", nil)
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到要更新的隧道记录"})
		return
	}

	logx.Infof("[DEBUG] 隧道更新成功，影响行数: %d\n", affected)

	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "SUCCESS", "隧道代理更新成功", nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "隧道代理更新成功"})
}

// DeleteTunnelProxy 删除隧道代理
func DeleteTunnelProxy(c *gin.Context) {
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_DELETE", "START", "开始删除隧道代理", nil)
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_DELETE", "FAILURE", "无效的ID", nil)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	tunnel, exists, err := database.Tunnels().GetByID(id)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_DELETE", "FAILURE", "查询隧道代理失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询隧道代理失败"})
		return
	}
	if !exists {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_DELETE", "FAILURE", "隧道代理不存在", nil)
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "隧道代理不存在"})
		return
	}

	// 如果隧道正在运行，先停止它
	if tunnel.Status == 1 {
		if proxy.GlobalTunnelManager != nil {
			err := proxy.GlobalTunnelManager.StopTunnel(id)
			if err != nil {
				// 记录停止失败，但不阻止删除操作
				logx.Warnf("⚠️ [隧道删除] 停止隧道失败，但继续删除: %v\n", err)
				if audit.GlobalAuditLogger != nil {
					audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelWarn, audit.CategoryAPI, "TUNNEL_PROXY_DELETE", "WARNING", "停止隧道失败但继续删除: "+err.Error(), nil)
				}
			} else {
				logx.Infof("💫 [隧道删除] 隧道停止成功\n")
			}
		} else {
			logx.Warnf("⚠️ [隧道删除] 隧道管理器未初始化，跳过停止操作\n")
		}
		// 更新状态为停止
		tunnel.Status = 0
		_, _ = database.Tunnels().UpdateAll(id, &tunnel)
	} else {
		logx.Infof("💫 [隧道删除] 隧道已停止，直接删除\n")
	}

	_, err = database.Tunnels().DeleteByID(id)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_DELETE", "FAILURE", "删除隧道代理失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除隧道代理失败"})
		return
	}

	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_DELETE", "SUCCESS", "隧道代理删除成功", nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "隧道代理删除成功"})
}

// StartTunnelProxy 启动隧道代理
func StartTunnelProxy(c *gin.Context) {
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_START", "START", "开始启动隧道代理", nil)
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_START", "FAILURE", "无效的ID", nil)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	tunnel, exists, err := database.Tunnels().GetByID(id)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_START", "FAILURE", "查询隧道代理失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询隧道代理失败"})
		return
	}
	if !exists {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_START", "FAILURE", "隧道代理不存在", nil)
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "隧道代理不存在"})
		return
	}

	_, exists, err = database.Clients_().OnlineByUID(tunnel.ClientUID)
	if err != nil || !exists {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_START", "FAILURE", "客户端离线", nil)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "客户端离线"})
		return
	}

	// 历史记录可能没有凭据：启动时同样拦住（否则等于把无认证代理重新开起来）
	if err := requireTunnelAuth(tunnel.Protocol, tunnel.Username, tunnel.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if proxy.GlobalTunnelManager == nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_START", "FAILURE", "隧道管理器未初始化", nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "隧道管理器未初始化"})
		return
	}

	err = proxy.GlobalTunnelManager.StartTunnel(id)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_START", "FAILURE", "启动隧道失败: "+err.Error(), nil)
		}

		errorMsg := err.Error()
		if strings.Contains(errorMsg, "尚未建立隧道连接") || strings.Contains(errorMsg, "连接不稳定") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "客户端连接异常",
				"details":    errorMsg,
				"suggestion": "请检查客户端连接状态，确保客户端正常在线并已建立稳定连接",
			})
		} else if strings.Contains(errorMsg, "端口") && strings.Contains(errorMsg, "占用") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":      "端口被占用",
				"details":    errorMsg,
				"suggestion": "请选择其他可用端口或停止占用该端口的服务",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":      "启动隧道失败",
				"details":    errorMsg,
				"suggestion": "请检查服务器日志获取详细错误信息",
			})
		}
		return
	}

	tunnel.Status = 1
	tunnel.UpdatedAt = time.Now()
	affected, err := database.Tunnels().UpdateColumns(id, []string{"status", "updated_at"}, &tunnel)
	if err != nil {
		// 状态写不进库就把刚启动的监听器回滚，避免"实际在跑、界面显示停止"
		_ = proxy.GlobalTunnelManager.StopTunnel(id)
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_START", "FAILURE", "更新状态失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "启动失败"})
		return
	}
	logx.Infof("[DEBUG] 数据库更新影响行数: %d\n", affected)

	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_START", "SUCCESS", "隧道代理启动成功", nil)
	}

	c.JSON(http.StatusOK, gin.H{"message": "隧道代理启动成功"})
}

// StopTunnelProxy 停止隧道代理
func StopTunnelProxy(c *gin.Context) {
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_STOP", "START", "开始停止隧道代理", nil)
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_STOP", "FAILURE", "无效的ID", nil)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	tunnel, exists, err := database.Tunnels().GetByID(id)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_STOP", "FAILURE", "查询隧道代理失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询隧道代理失败"})
		return
	}
	if !exists {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_STOP", "FAILURE", "隧道代理不存在", nil)
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "隧道代理不存在"})
		return
	}

	if proxy.GlobalTunnelManager == nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_STOP", "FAILURE", "隧道管理器未初始化", nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "隧道管理器未初始化"})
		return
	}

	err = proxy.GlobalTunnelManager.StopTunnel(id)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_STOP", "FAILURE", "停止隧道失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "停止隧道失败: " + err.Error()})
		return
	}

	tunnel.Status = 0
	tunnel.UpdatedAt = time.Now()
	affected, err := database.Tunnels().UpdateColumns(id, []string{"status", "updated_at"}, &tunnel)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_STOP", "FAILURE", "更新状态失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "停止失败"})
		return
	}
	logx.Infof("[DEBUG] 数据库更新影响行数: %d\n", affected)

	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelInfo, audit.CategoryAPI, "TUNNEL_PROXY_STOP", "SUCCESS", "隧道代理停止成功", nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "隧道代理停止成功"})
}

// RestoreTunnelsOnStartup 服务重启后恢复 status=1 的隧道代理。
//
// 旧版 SOCKS5 实现有这套恢复逻辑；统一到 TunnelProxy 后必须一并搬过来，
// 否则重启后所有运行中的隧道都变成"数据库里是运行中、实际没有监听"。
//
// 服务重启会断开全部客户端会话，而 StartTunnel 要求客户端在线，因此先给客户端
// 一个重连窗口（共 60 秒，每 2 秒探测一次，全部在线即提前结束），再逐个拉起；
// 超时仍离线或启动失败的记录置为停止，避免界面继续显示"运行中"。
func RestoreTunnelsOnStartup() {
	if database.DB() == nil || proxy.GlobalTunnelManager == nil {
		return
	}
	tunnels, err := database.Tunnels().ListByStatus(1)
	if err != nil {
		logx.Warnf("[TUNNEL] 读取待恢复隧道失败: %v\n", err)
		return
	}
	if len(tunnels) == 0 {
		return
	}

	go func() {
		const (
			reconnectWindow = 60 * time.Second
			pollInterval    = 2 * time.Second
		)
		pending := make(map[string]bool, len(tunnels))
		for _, t := range tunnels {
			pending[t.ClientUID] = true
		}
		deadline := time.Now().Add(reconnectWindow)
		for time.Now().Before(deadline) {
			for uid := range pending {
				if _, online, _ := database.Clients_().OnlineByUID(uid); online {
					delete(pending, uid)
				}
			}
			if len(pending) == 0 {
				break
			}
			time.Sleep(pollInterval)
		}

		restored, stopped := 0, 0
		for _, t := range tunnels {
			_, online, _ := database.Clients_().OnlineByUID(t.ClientUID)
			if !online {
				_, _ = database.Tunnels().SetStatus(t.ID, 0)
				stopped++
				continue
			}
			if err := proxy.GlobalTunnelManager.StartTunnel(t.ID); err != nil {
				_, _ = database.Tunnels().SetStatus(t.ID, 0)
				stopped++
				logx.Warnf("[TUNNEL] 恢复隧道 %d (%s:%d) 失败，已置停: %v\n", t.ID, t.Protocol, t.ServerPort, err)
				continue
			}
			restored++
			logx.Infof("[TUNNEL] 已恢复隧道 %d (%s:%d) → 客户端 %s\n", t.ID, t.Protocol, t.ServerPort, t.ClientUID)
		}
		if restored > 0 || stopped > 0 {
			logx.Infof("[TUNNEL] 启动恢复完成：成功 %d，失败/离线置停 %d\n", restored, stopped)
		}
	}()
}
