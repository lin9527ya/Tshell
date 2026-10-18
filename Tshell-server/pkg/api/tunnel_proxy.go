package api

import (
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/proxy"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
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
// 两张表都要查：新表 TunnelProxy 与旧版 Socks5 各管一套隧道，不交叉校验的话同一端口可以在
// 两张表里各建一条，直到 bind 时才撞车；同时 Agent 端口也参与占用判定 —— 端口 5000 的隧道
// 与端口 6000 的隧道会抢 6000（原实现只校验 server_port 唯一）。
// excludeTunnelID / excludeSocks5ID：更新时排除自身记录（传 0 表示不排除）。
func checkTunnelPortAvailable(port int, excludeTunnelID, excludeSocks5ID int64) error {
	if port < 1 || port > 65535-1000 {
		return fmt.Errorf("端口需在 1-%d 之间（Agent 端口 = 端口+1000 也须合法）", 65535-1000)
	}
	// 需要避开的既有 server_port：本端口 / 本端口的 Agent 端口 / 以本端口为 Agent 端口的记录
	conflict := []int{port, port + 1000}
	if port > 1000 {
		conflict = append(conflict, port-1000)
	}
	// 用 xorm 的 In(column, args...)：Where("col IN (?)", slice) 在本版本不会展开切片，
	// 会把整个切片当成单个参数交给驱动而报 "unsupported type"（错误若被忽略就会静默放行）。
	// 这里同时显式返回查询错误：校验失败即视为占用（fail-closed）。
	intArgs := make([]interface{}, len(conflict))
	strArgs := make([]interface{}, len(conflict))
	for i, p := range conflict {
		intArgs[i] = p
		strArgs[i] = strconv.Itoa(p)
	}

	var tun database.TunnelProxy
	q := database.DB().In("server_port", intArgs...)
	if excludeTunnelID > 0 {
		q = q.And("id != ?", excludeTunnelID)
	}
	has, err := q.Get(&tun)
	if err != nil {
		return fmt.Errorf("检查端口占用失败: %w", err)
	}
	if has {
		return fmt.Errorf("端口 %d 与已有隧道冲突（Agent 端口 = 端口+1000）：隧道 %d 占用 %d", port, tun.ID, tun.ServerPort)
	}

	var s5 database.Socks5
	q2 := database.DB().In("socks5port", strArgs...)
	if excludeSocks5ID > 0 {
		q2 = q2.And("id != ?", excludeSocks5ID)
	}
	has, err = q2.Get(&s5)
	if err != nil {
		return fmt.Errorf("检查端口占用失败: %w", err)
	}
	if has {
		return fmt.Errorf("端口 %d 与已有 SOCKS5 代理冲突（Agent 端口 = 端口+1000）：记录 %d 占用 %s", port, s5.ID, s5.Socks5port)
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

	// 检查客户端是否存在且在线
	var client database.Clients
	exists, err := database.DB().Where("uid = ? AND online = '1'", req.ClientUID).Get(&client)
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

	// 检查端口是否已被占用
	var existingTunnel database.TunnelProxy
	exists, err = database.DB().Where("server_port = ?", req.ServerPort).Get(&existingTunnel)
	if err != nil {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_CREATE", "FAILURE", "检查端口占用失败: "+err.Error(), nil)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "检查端口占用失败"})
		return
	}
	if exists {
		if audit.GlobalAuditLogger != nil {
			audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_CREATE", "FAILURE", "端口已被占用", nil)
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "端口已被占用"})
		return
	}

	// Agent 端口（端口+1000）与旧版 Socks5 表也参与占用判定
	if err := checkTunnelPortAvailable(req.ServerPort, 0, 0); err != nil {
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

	_, err = database.DB().Insert(&tunnel)
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
		var clients []database.Clients
		if err := database.DB().In("uid", clientUIDs).Find(&clients); err == nil {
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

	var tunnel database.TunnelProxy
	tunnel.ID = id
	exists, err := database.DB().Get(&tunnel)
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

	if req.ServerPort != tunnel.ServerPort {
		var existingTunnel database.TunnelProxy
		exists, err = database.DB().Where("server_port = ? AND id != ?", req.ServerPort, id).Get(&existingTunnel)
		if err != nil {
			if audit.GlobalAuditLogger != nil {
				audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "FAILURE", "检查端口占用失败: "+err.Error(), nil)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "检查端口占用失败"})
			return
		}
		if exists {
			if audit.GlobalAuditLogger != nil {
				audit.GlobalAuditLogger.LogWithContext(c, audit.LogLevelError, audit.CategoryAPI, "TUNNEL_PROXY_UPDATE", "FAILURE", "端口已被占用", nil)
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "端口已被占用"})
			return
		}

		if err := checkTunnelPortAvailable(req.ServerPort, id, 0); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// 构建更新数据映射，确保字段正确映射
	updateData := map[string]interface{}{
		"client_uid":     req.ClientUID,
		"protocol":        req.Protocol,
		"server_port":     req.ServerPort,
		"target_address":  req.TargetAddress,
		"note":            req.Note,
		"username":        req.Username,
		"password":        req.Password,
		"updated_at":      time.Now(),
	}

	// 使用ID作为唯一标识符进行更新，避免使用其他可能变更的字段
	// 使用Where条件避免xorm的ID方法表引用问题
	affected, err := database.DB().Table(&database.TunnelProxy{}).Where("id = ?", id).Update(updateData)
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

	var tunnel database.TunnelProxy
	tunnel.ID = id
	exists, err := database.DB().Get(&tunnel)
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
		database.DB().ID(id).Update(&tunnel)
	} else {
		logx.Infof("💫 [隧道删除] 隧道已停止，直接删除\n")
	}

	_, err = database.DB().ID(id).Delete(&database.TunnelProxy{})
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

	var tunnel database.TunnelProxy
	tunnel.ID = id
	exists, err := database.DB().Get(&tunnel)
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

	var client database.Clients
	exists, err = database.DB().Where("uid = ? AND online = '1'", tunnel.ClientUID).Get(&client)
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
	affected, err := database.DB().ID(id).Cols("status", "updated_at").Update(&tunnel)
	if err != nil {
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

	var tunnel database.TunnelProxy
	tunnel.ID = id
	exists, err := database.DB().Get(&tunnel)
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
	affected, err := database.DB().ID(id).Cols("status", "updated_at").Update(&tunnel)
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
