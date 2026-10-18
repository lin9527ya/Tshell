package api

import (
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// KeyStatusResponse 密钥状态响应
type KeyStatusResponse struct {
	Status  int         `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// KeyRotationRequest 密钥轮换请求
type KeyRotationRequest struct {
	ClientID string `json:"client_id" binding:"required"`
	Force    bool   `json:"force,omitempty"`
}

// GetKeyStatus 获取指定客户端的密钥状态
func GetKeyStatus(c *gin.Context) {
	clientID := c.Param("client_id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, KeyStatusResponse{
			Status:  400,
			Message: "客户端ID不能为空",
		})
		return
	}

	if encrypt.GlobalKeyManager == nil {
		c.JSON(http.StatusServiceUnavailable, KeyStatusResponse{
			Status:  503,
			Message: "密钥管理器未初始化",
		})
		return
	}

	status := encrypt.GlobalKeyManager.GetKeyStatus(clientID)
	c.JSON(http.StatusOK, KeyStatusResponse{
		Status:  200,
		Message: "获取密钥状态成功",
		Data:    status,
	})
}

// GetAllKeyStatus 获取所有客户端的密钥状态
func GetAllKeyStatus(c *gin.Context) {
	if encrypt.GlobalKeyManager == nil {
		c.JSON(http.StatusServiceUnavailable, KeyStatusResponse{
			Status:  503,
			Message: "密钥管理器未初始化",
		})
		return
	}

	allStatus := encrypt.GlobalKeyManager.GetAllKeyStatus()

	c.JSON(http.StatusOK, KeyStatusResponse{
		Status:  200,
		Message: "获取所有密钥状态成功",
		Data:    allStatus,
	})
}

// ForceKeyRotation 强制密钥轮换
func ForceKeyRotation(c *gin.Context) {
	var req KeyRotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, KeyStatusResponse{
			Status:  400,
			Message: "请求参数错误: " + err.Error(),
		})
		return
	}

	if encrypt.GlobalKeyManager == nil {
		c.JSON(http.StatusServiceUnavailable, KeyStatusResponse{
			Status:  503,
			Message: "密钥管理器未初始化",
		})
		return
	}

	err := encrypt.GlobalKeyManager.ForceKeyRotation(req.ClientID)
	if err != nil {
		c.JSON(http.StatusBadRequest, KeyStatusResponse{
			Status:  400,
			Message: "密钥轮换失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, KeyStatusResponse{
		Status:  200,
		Message: "密钥轮换成功",
		Data: map[string]interface{}{
			"client_id": req.ClientID,
			"timestamp": "密钥轮换已启动",
		},
	})
}

// GetKeyMetrics 获取密钥系统统计信息
func GetKeyMetrics(c *gin.Context) {
	if encrypt.GlobalKeyManager == nil {
		c.JSON(http.StatusServiceUnavailable, KeyStatusResponse{
			Status:  503,
			Message: "密钥管理器未初始化",
		})
		return
	}

	metrics := encrypt.GlobalKeyManager.GetKeyMetrics()

	c.JSON(http.StatusOK, KeyStatusResponse{
		Status:  200,
		Message: "获取密钥系统统计信息成功",
		Data:    metrics,
	})
}

// ConfigureKeyRotation 配置密钥轮换参数
func ConfigureKeyRotation(c *gin.Context) {
	type ConfigRequest struct {
		OverlapDurationHours int  `json:"overlap_duration_hours"`
		RotationEnabled      bool `json:"rotation_enabled"`
	}

	var req ConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, KeyStatusResponse{
			Status:  400,
			Message: "请求参数错误: " + err.Error(),
		})
		return
	}

	if encrypt.GlobalKeyManager == nil {
		c.JSON(http.StatusServiceUnavailable, KeyStatusResponse{
			Status:  503,
			Message: "密钥管理器未初始化",
		})
		return
	}

	// 更新配置
	if req.OverlapDurationHours > 0 {
		encrypt.GlobalKeyManager.SetOverlapDuration(time.Duration(req.OverlapDurationHours) * time.Hour)
	}
	encrypt.GlobalKeyManager.SetRotationEnabled(req.RotationEnabled)

	c.JSON(http.StatusOK, KeyStatusResponse{
		Status:  200,
		Message: "密钥轮换配置更新成功",
		Data: map[string]interface{}{
			"overlap_duration_hours": req.OverlapDurationHours,
			"rotation_enabled":       req.RotationEnabled,
		},
	})
}

// ========================= 监听器密钥配置管理 =========================

// GetListenerKeyConfig 获取监听器密钥配置
func GetListenerKeyConfig(c *gin.Context) {
	listenerType := c.Query("type")
	listenAddress := c.Query("address")

	// 根据监听器类型或地址获取密钥配置。地址可能来自前端回传的脱敏值，查库前先还原；
	// 响应里回显的仍是调用方传入的值（不反向泄露真实地址）。
	var masterKey, encryptionSalt []byte
	if listenAddress != "" {
		masterKey, encryptionSalt = encrypt.GetListenerKeyConfigByAddress(resolveListenerAddress(listenAddress, "connect"))
	} else {
		masterKey, encryptionSalt = encrypt.GetListenerKeyConfig(listenerType)
	}

	// 返回配置信息（仅返回非敏感信息）
	c.JSON(http.StatusOK, KeyStatusResponse{
		Status:  200,
		Message: "获取监听器密钥配置成功",
		Data: map[string]interface{}{
			"listener_type":     listenerType,
			"listen_address":    listenAddress,
			"master_key_length": len(masterKey),
			"salt_length":       len(encryptionSalt),
			"has_custom_config": len(masterKey) > 0 && len(encryptionSalt) > 0,
		},
	})
}

// GetAllListenerKeyConfigs 获取所有监听器的密钥配置信息
func GetAllListenerKeyConfigs(c *gin.Context) {
	configs, err := encrypt.GetAllListenerKeyConfigs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, KeyStatusResponse{
			Status:  500,
			Message: "获取监听器密钥配置失败: " + err.Error(),
		})
		return
	}

	// 地址脱敏：oss 的 listen_address 承载 endpoint:AK:SK:bucket，
	// 不脱敏的话本接口会把云凭证送回浏览器（列表/详情接口已脱敏，这里原先漏了）
	for _, cfg := range configs {
		addr, ok := cfg["listen_address"].(string)
		if !ok {
			continue
		}
		typ, _ := cfg["type"].(string)
		cfg["listen_address"] = maskListenerAddress(typ, addr)
	}

	c.JSON(http.StatusOK, KeyStatusResponse{
		Status:  200,
		Message: "获取所有监听器密钥配置成功",
		Data:    configs,
	})
}

// UpdateListenerKeyConfig 更新监听器密钥配置
func UpdateListenerKeyConfig(c *gin.Context) {
	type UpdateRequest struct {
		ListenAddress  string `json:"listen_address" binding:"required"`
		VKey           string `json:"vkey"`
		EncryptionSalt string `json:"encryption_salt"`
	}

	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, KeyStatusResponse{
			Status:  400,
			Message: "请求参数错误: " + err.Error(),
		})
		return
	}

	// 地址可能来自前端回传的脱敏值，查库前先还原
	req.ListenAddress = resolveListenerAddress(req.ListenAddress, "listen")

	// 【修复】支持只更新单字段：VKey 与 Salt 可单独更新（原实现要求同时非空）
	// 先查出旧值，未提交的字段保留原值
	var oldListener database.Listener
	if _, oldHas, _ := database.Listeners().GetByAddress(req.ListenAddress); oldHas {
		if req.VKey == "" {
			req.VKey = oldListener.VKey
		}
		if req.EncryptionSalt == "" {
			req.EncryptionSalt = oldListener.EncryptionSalt
		}
	}

	// 验证密钥配置（合并后校验）
	if err := encrypt.ValidateListenerKeyConfig(req.VKey, req.EncryptionSalt); err != nil {
		c.JSON(http.StatusBadRequest, KeyStatusResponse{
			Status:  400,
			Message: "密钥配置验证失败: " + err.Error(),
		})
		return
	}

	// 更新数据库中的监听器配置
	if database.DB() == nil {
		c.JSON(http.StatusServiceUnavailable, KeyStatusResponse{
			Status:  503,
			Message: "数据库未初始化",
		})
		return
	}

	// 查找监听器
	_, has, err := database.Listeners().GetByAddress(req.ListenAddress)
	if err != nil {
		c.JSON(http.StatusInternalServerError, KeyStatusResponse{
			Status:  500,
			Message: "查询监听器失败: " + err.Error(),
		})
		return
	}

	if !has {
		c.JSON(http.StatusNotFound, KeyStatusResponse{
			Status:  404,
			Message: "监听器不存在",
		})
		return
	}

	// 更新密钥配置（显式 Cols：防止 xorm 跳过零值字段导致清空密钥静默失效）
	_, err = database.Listeners().UpdateColumnsByAddress(req.ListenAddress,
		[]string{"v_key", "encryption_salt"},
		&database.Listener{
			VKey:           req.VKey,
			EncryptionSalt: req.EncryptionSalt,
		})

	if err != nil {
		c.JSON(http.StatusInternalServerError, KeyStatusResponse{
			Status:  500,
			Message: "更新监听器密钥配置失败: " + err.Error(),
		})
		return
	}
	auditLogEvent("KEY_CONFIG_UPDATE", "SUCCESS", "更新监听器密钥 "+req.ListenAddress, nil)

	c.JSON(http.StatusOK, KeyStatusResponse{
		Status:  200,
		Message: "监听器密钥配置更新成功",
		Data: map[string]interface{}{
			"listen_address": req.ListenAddress,
			"vkey_length":    len(req.VKey),
			"salt_length":    len(req.EncryptionSalt),
			"updated_at":     time.Now(),
		},
	})
}

// ResetListenerKeyConfig 重置监听器密钥配置为默认值
func ResetListenerKeyConfig(c *gin.Context) {
	listenAddress := c.Param("address")
	if listenAddress == "" {
		c.JSON(http.StatusBadRequest, KeyStatusResponse{
			Status:  400,
			Message: "监听地址不能为空",
		})
		return
	}

	// 地址可能来自前端回传的脱敏值，查库前先还原
	listenAddress = resolveListenerAddress(listenAddress, "listen")

	// 更新数据库中的监听器配置为默认值
	if database.DB() == nil {
		c.JSON(http.StatusServiceUnavailable, KeyStatusResponse{
			Status:  503,
			Message: "数据库未初始化",
		})
		return
	}

	// 重置为默认值
	// 【安全修复】重置为随机生成的新密钥（不再回退到源码公开常量，
	// 否则重置后该监听器所有流量可被任何知道默认值的人解密）
	newVKey, newSalt := encrypt.GenerateListenerKeyConfig()
	// （显式 Cols：防止 xorm 跳过零值字段导致密钥未写入）
	_, err := database.Listeners().UpdateColumnsByAddress(listenAddress,
		[]string{"v_key", "encryption_salt"},
		&database.Listener{
			VKey:           newVKey,
			EncryptionSalt: newSalt,
		})

	if err != nil {
		c.JSON(http.StatusInternalServerError, KeyStatusResponse{
			Status:  500,
			Message: "重置监听器密钥配置失败: " + err.Error(),
		})
		return
	}
	encrypt.InvalidateListenerKeyCache()

	auditLogEvent("KEY_CONFIG_RESET", "SUCCESS", "重置监听器密钥 "+listenAddress, map[string]interface{}{"address": listenAddress})
	c.JSON(http.StatusOK, KeyStatusResponse{
		Status:  200,
		Message: "监听器密钥配置重置成功（已生成随机新密钥）",
		Data: map[string]interface{}{
			"listen_address": listenAddress,
			"reset_to":       "random_values",
			"updated_at":     time.Now(),
		},
	})
}
