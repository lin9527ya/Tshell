package security

import (
	"BackendTemplate/pkg/audit"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// 获取安全统计信息
func GetSecurityStatsHandler(c *gin.Context) {
	stats := GlobalRateLimiter.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data":   stats,
	})
}

// 获取IP白名单
func GetWhitelistHandler(c *gin.Context) {
	GlobalRateLimiter.ipWhitelist.mu.RLock()
	whitelist := make([]string, 0, len(GlobalRateLimiter.ipWhitelist.whitelist))
	for ip := range GlobalRateLimiter.ipWhitelist.whitelist {
		whitelist = append(whitelist, ip)
	}
	GlobalRateLimiter.ipWhitelist.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"whitelist": whitelist,
			"count":     len(whitelist),
		},
	})
}

// 添加IP到白名单
func AddToWhitelistHandler(c *gin.Context) {
	var request struct {
		IP      string `json:"ip" binding:"required"`
		Network string `json:"network"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	if request.Network != "" {
		// 添加网络段
		GlobalRateLimiter.ipWhitelist.AddNetwork(request.Network)
		auditSecurityEvent("WHITELIST_ADD", "添加网段白名单 "+request.Network, map[string]interface{}{"network": request.Network})
		c.JSON(http.StatusOK, gin.H{
			"status":  http.StatusOK,
			"message": "Network added to whitelist successfully",
			"network": request.Network,
		})
	} else {
		// 添加单个IP
		GlobalRateLimiter.ipWhitelist.AddIP(request.IP)
		auditSecurityEvent("WHITELIST_ADD", "添加IP白名单 "+request.IP, map[string]interface{}{"ip": request.IP})
		c.JSON(http.StatusOK, gin.H{
			"status":  http.StatusOK,
			"message": "IP added to whitelist successfully",
			"ip":      request.IP,
		})
	}
}

// 从白名单移除IP
func RemoveFromWhitelistHandler(c *gin.Context) {
	var request struct {
		IP string `json:"ip" binding:"required"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	GlobalRateLimiter.ipWhitelist.RemoveIP(request.IP)

	c.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "IP removed from whitelist successfully",
		"ip":      request.IP,
	})
}

// 解除IP封禁
func UnblockIPHandler(c *gin.Context) {
	ip := c.Param("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"message": "IP parameter is required",
		})
		return
	}

	GlobalRateLimiter.UnblockIP(ip)

	c.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "IP unblocked successfully",
		"ip":      ip,
	})
}

// 获取被封禁的IP列表
func GetBlockedIPsHandler(c *gin.Context) {
	blockedIPs := GlobalRateLimiter.GetBlockedIPs()

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"blocked_ips": blockedIPs,
			"count":       len(blockedIPs),
		},
	})
}

// 更新速率限制配置
func UpdateRateLimitConfigHandler(c *gin.Context) {
	var config struct {
		LoginRateLimit int `json:"login_rate_limit"`
		APIRateLimit   int `json:"api_rate_limit"`
		BlockDuration  int `json:"block_duration"` // 分钟
	}

	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  http.StatusBadRequest,
			"message": "Invalid request format",
			"error":   err.Error(),
		})
		return
	}

	GlobalRateLimiter.UpdateConfig(config.LoginRateLimit, config.APIRateLimit, time.Duration(config.BlockDuration)*time.Minute)

	// 读取当前配置用于回显（经 cfgMu）
	lr, ar, bd := GlobalRateLimiter.GetConfigSnapshot()

	c.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "Rate limit configuration updated successfully",
		"config": gin.H{
			"login_rate_limit": lr,
			"api_rate_limit":   ar,
			"block_duration":   bd.String(),
		},
	})
}

// 获取实时访问统计
func GetRealTimeStatsHandler(c *gin.Context) {
	recentIPs := GlobalRateLimiter.GetRecentActiveIPs(time.Now().Add(-time.Hour))

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"recent_active_ips": recentIPs,
			"total_active":      len(recentIPs),
			"timeframe":         "last 1 hour",
			"timestamp":         time.Now().Format("2006-01-02 15:04:05"),
		},
	})
}
// auditSecurityEvent 安全操作审计（D18）
func auditSecurityEvent(action, message string, details map[string]interface{}) {
	if audit.GlobalAuditLogger != nil {
		audit.GlobalAuditLogger.Log(audit.LogLevelInfo, audit.CategorySecurity, action, "SUCCESS", message, details)
	}
}