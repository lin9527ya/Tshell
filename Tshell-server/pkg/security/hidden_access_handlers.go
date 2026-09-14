package security

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

// HiddenAccessConfigPersister 由 config 包在初始化时注入，用于将隐藏访问配置持久化到配置文件
var HiddenAccessConfigPersister func(enabled bool, accessPaths, paramNames []string, secretKey string, sessionTTL int) error

// GetHiddenAccessStatusHandler 获取隐藏访问状态
func GetHiddenAccessStatusHandler(c *gin.Context) {
	if GlobalHiddenAccessManager == nil {
		c.JSON(http.StatusOK, gin.H{
			"status": http.StatusOK,
			"data": gin.H{
				"enabled":       false,
				"session_count": 0,
				"config":        make(map[string]interface{}),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data":   GlobalHiddenAccessManager.GetConfig(),
	})
}

// RegenerateHiddenAccessKeyHandler 重新生成隐藏访问密钥
func RegenerateHiddenAccessKeyHandler(c *gin.Context) {
	if GlobalHiddenAccessManager == nil || !GlobalHiddenAccessManager.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"message": "隐藏访问功能未启用",
		})
		return
	}

	newKey := GlobalHiddenAccessManager.RegenerateKey()

	// 同步持久化到配置文件
	persistHiddenAccessConfig()

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"new_secret_key": newKey,
			"access_urls":    generateAccessURLs(newKey),
			"message":        "访问密钥已重新生成，所有现有会话已失效",
		},
	})
}

// ClearHiddenAccessSessionsHandler 清空所有隐藏访问会话
func ClearHiddenAccessSessionsHandler(c *gin.Context) {
	if GlobalHiddenAccessManager == nil || !GlobalHiddenAccessManager.IsEnabled() {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"message": "隐藏访问功能未启用",
		})
		return
	}

	GlobalHiddenAccessManager.ClearSessions()

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"message": "所有隐藏访问会话已清空",
		},
	})
}

// generateAccessURLs 生成访问 URL 列表
func generateAccessURLs(secretKey string) []string {
	if GlobalHiddenAccessManager == nil {
		return []string{}
	}

	var urls []string
	for _, path := range GlobalHiddenAccessManager.accessPaths {
		for _, param := range GlobalHiddenAccessManager.paramNames {
			urls = append(urls, fmt.Sprintf("%s?%s=%s", path, param, secretKey))
		}
	}
	return urls
}

// persistHiddenAccessConfig 将当前隐藏访问配置持久化到配置文件
func persistHiddenAccessConfig() {
	if GlobalHiddenAccessManager == nil || HiddenAccessConfigPersister == nil {
		return
	}
	ham := GlobalHiddenAccessManager
	ham.mu.RLock()
	enabled := ham.enabled
	paths := make([]string, len(ham.accessPaths))
	copy(paths, ham.accessPaths)
	params := make([]string, len(ham.paramNames))
	copy(params, ham.paramNames)
	key := ham.secretKey
	ttl := int(ham.sessionTTL.Hours())
	ham.mu.RUnlock()

	if err := HiddenAccessConfigPersister(enabled, paths, params, key, ttl); err != nil {
		logx.Warnf("⚠️ 隐藏访问配置持久化失败: %v\n", err)
	}
}

// UpdateHiddenAccessConfigHandler 更新隐藏访问配置（启用开关/路径/参数/密钥/会话有效期）
func UpdateHiddenAccessConfigHandler(c *gin.Context) {
	if GlobalHiddenAccessManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  http.StatusInternalServerError,
			"message": "隐藏访问管理器未初始化",
		})
		return
	}

	var req struct {
		Enabled     *bool    `json:"enabled"`
		AccessPaths []string `json:"access_paths"`
		ParamNames  []string `json:"param_names"`
		SecretKey   string   `json:"secret_key"`
		SessionTTL  int      `json:"session_ttl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"message": "参数格式错误: " + err.Error(),
		})
		return
	}

	ham := GlobalHiddenAccessManager

	// 读取当前值
	ham.mu.RLock()
	enabled := ham.enabled
	paths := make([]string, len(ham.accessPaths))
	copy(paths, ham.accessPaths)
	params := make([]string, len(ham.paramNames))
	copy(params, ham.paramNames)
	key := ham.secretKey
	ttl := int(ham.sessionTTL.Hours())
	ham.mu.RUnlock()

	// 应用请求中的变更
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.AccessPaths != nil {
		paths = req.AccessPaths
	}
	if req.ParamNames != nil {
		params = req.ParamNames
	}
	if req.SecretKey != "" {
		key = req.SecretKey
	}
	if req.SessionTTL > 0 {
		ttl = req.SessionTTL
	}

	// 启用时校验配置合法性
	if enabled {
		if len(paths) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "启用时至少需要一个访问路径"})
			return
		}
		if len(params) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "启用时至少需要一个参数名"})
			return
		}
		for _, p := range paths {
			if p == "" || !strings.HasPrefix(p, "/") {
				c.JSON(http.StatusBadRequest, gin.H{"status": http.StatusBadRequest, "message": "访问路径必须以 / 开头且不能为空"})
				return
			}
		}
		if key == "" {
			key = generateRandomKey()
		}
	}

	// 先持久化，失败则不改动内存
	if HiddenAccessConfigPersister != nil {
		if err := HiddenAccessConfigPersister(enabled, paths, params, key, ttl); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  http.StatusInternalServerError,
				"message": "保存配置失败: " + err.Error(),
			})
			return
		}
	}

	// 应用变更到内存
	keyChanged := key != ham.secretKey
	ham.mu.Lock()
	ham.enabled = enabled
	ham.accessPaths = paths
	ham.paramNames = params
	ham.secretKey = key
	ham.sessionTTL = time.Duration(ttl) * time.Hour
	if keyChanged || !enabled {
		ham.sessionMap = make(map[string]time.Time)
	}
	ham.mu.Unlock()

	logx.Infof("🔒 隐藏访问配置已更新: enabled=%v, paths=%v, params=%v, ttl=%dh\n", enabled, paths, params, ttl)

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"message": "配置已更新",
			"config":  ham.GetConfig(),
		},
	})
}
