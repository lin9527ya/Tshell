package config

import (
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/security"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// GetConfig 获取当前配置
func GetConfigHandler(c *gin.Context) {
	if GlobalConfig == nil {
		errors.AbortWithError(c, errors.NewInternalError("配置管理器未初始化"))
		return
	}

	config := GlobalConfig.GetConfig()

	// 敏感信息脱敏
	sanitizedConfig := sanitizeConfig(config)

	errors.RespondWithSuccess(c, sanitizedConfig)
}

// UpdateConfig 更新配置
func UpdateConfigHandler(c *gin.Context) {
	if GlobalConfig == nil {
		errors.AbortWithError(c, errors.NewInternalError("配置管理器未初始化"))
		return
	}

	var updateRequest map[string]interface{}
	if err := errors.ValidateRequest(c, &updateRequest); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	// 更新配置
	err := GlobalConfig.UpdateConfig(func(config *Config) error {
		return applyConfigUpdates(config, updateRequest)
	})

	if err != nil {
		errors.AbortWithError(c, errors.NewBadRequestError("配置更新失败: "+err.Error()))
		return
	}

	errors.RespondWithSuccess(c, gin.H{
		"message": "配置更新成功",
	})
}

// ReloadConfig 重新加载配置
func ReloadConfigHandler(c *gin.Context) {
	if GlobalConfig == nil {
		errors.AbortWithError(c, errors.NewInternalError("配置管理器未初始化"))
		return
	}

	err := GlobalConfig.loadConfig()
	if err != nil {
		errors.AbortWithError(c, errors.NewInternalError("重新加载配置失败: "+err.Error()))
		return
	}

	// 热重载 HTTP masquerade profile
	ApplyHTTPConfig(GetGlobalConfig().HTTP)

	errors.RespondWithSuccess(c, gin.H{
		"message": "配置重新加载成功",
	})
}

// GetConfigSchema 获取配置结构
func GetConfigSchemaHandler(c *gin.Context) {
	schema := map[string]interface{}{
		"server": map[string]interface{}{
			"host":          "string - 服务器监听地址",
			"port":          "int - 服务器监听端口 (1-65535)",
			"mode":          "string - 运行模式 (debug/release)",
			"read_timeout":  "duration - 读取超时时间",
			"write_timeout": "duration - 写入超时时间",
			"idle_timeout":  "duration - 空闲超时时间",
		},
		"database": map[string]interface{}{
			"type":           "string - 数据库类型",
			"path":           "string - 数据库文件路径",
			"max_open_conns": "int - 最大连接数",
			"max_idle_conns": "int - 最大空闲连接数",
			"max_lifetime":   "duration - 连接最大生存时间",
		},
		"jwt": map[string]interface{}{
			"secret_key_file":   "string - JWT密钥文件路径",
			"access_token_ttl":  "duration - 访问令牌有效期",
			"refresh_token_ttl": "duration - 刷新令牌有效期",
			"max_refresh_count": "int - 最大刷新次数",
			"algorithm":         "string - 签名算法",
		},
		"logging": map[string]interface{}{
			"level":           "string - 日志级别",
			"format":          "string - 日志格式",
			"output":          "string - 日志输出",
			"audit_log_dir":   "string - 审计日志目录",
			"max_file_size":   "int64 - 最大文件大小(字节)",
			"max_retain_days": "int - 最大保留天数",
			"enable_console":  "bool - 是否启用控制台输出",
		},
		"security": map[string]interface{}{
			"enable_rate_limit":   "bool - 是否启用速率限制",
			"login_rate_limit":    "int - 登录速率限制",
			"api_rate_limit":      "int - API速率限制",
			"block_duration":      "duration - 封禁持续时间",
			"sliding_window":      "duration - 滑动窗口时间",
			"max_login_attempts":  "int - 最大登录尝试次数",
			"enable_ip_whitelist": "bool - 是否启用IP白名单",
			"whitelist_ips":       "[]string - 白名单IP列表",
		},
		"connection_pool": map[string]interface{}{
			"max_connections":     "int - 最大连接数",
			"max_idle_time":       "duration - 最大空闲时间",
			"health_check_period": "duration - 健康检查周期",
			"conn_timeout":        "duration - 连接超时时间",
			"read_timeout":        "duration - 读取超时时间",
			"write_timeout":       "duration - 写入超时时间",
		},
		"worker_pool": map[string]interface{}{
			"worker_count":   "int - 工作协程数 (0表示自动)",
			"max_queue_size": "int - 最大队列大小 (0表示自动)",
			"enable_metrics": "bool - 是否启用性能指标",
		},
	}

	errors.RespondWithSuccess(c, schema)
}

// ValidateConfig 验证配置
func ValidateConfigHandler(c *gin.Context) {
	var config Config
	if err := errors.ValidateRequest(c, &config); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	// 创建临时配置管理器进行验证
	tempManager := &ConfigManager{config: &config}
	if err := tempManager.validateConfig(); err != nil {
		errors.AbortWithError(c, errors.NewValidationError("配置验证失败: "+err.Error()))
		return
	}

	errors.RespondWithSuccess(c, gin.H{
		"message": "配置验证通过",
		"valid":   true,
	})
}

// ExportConfig 导出配置
func ExportConfigHandler(c *gin.Context) {
	if GlobalConfig == nil {
		errors.AbortWithError(c, errors.NewInternalError("配置管理器未初始化"))
		return
	}

	format := c.DefaultQuery("format", "json")
	config := GlobalConfig.GetConfig()

	// 敏感信息脱敏
	sanitizedConfig := sanitizeConfig(config)

	switch format {
	case "yaml":
		c.Header("Content-Type", "application/x-yaml")
		c.Header("Content-Disposition", "attachment; filename=config.yaml")
		c.YAML(http.StatusOK, sanitizedConfig)
	default:
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", "attachment; filename=config.json")
		c.JSON(http.StatusOK, sanitizedConfig)
	}
}

// GetConfigEnvironmentVariables 获取环境变量配置说明
func GetConfigEnvironmentVariablesHandler(c *gin.Context) {
	envVars := map[string]string{
		"TSHELL_SERVER_HOST":                     "服务器监听地址",
		"TSHELL_SERVER_PORT":                     "服务器监听端口",
		"TSHELL_SERVER_MODE":                     "运行模式",
		"TSHELL_DATABASE_TYPE":                   "数据库类型",
		"TSHELL_DATABASE_PATH":                   "数据库路径",
		"TSHELL_JWT_SECRET_KEY_FILE":             "JWT密钥文件路径",
		"TSHELL_JWT_ACCESS_TOKEN_TTL":            "访问令牌有效期",
		"TSHELL_JWT_REFRESH_TOKEN_TTL":           "刷新令牌有效期",
		"TSHELL_LOGGING_LEVEL":                   "日志级别",
		"TSHELL_LOGGING_AUDIT_LOG_DIR":           "审计日志目录",
		"TSHELL_SECURITY_ENABLE_RATE_LIMIT":      "是否启用速率限制",
		"TSHELL_SECURITY_LOGIN_RATE_LIMIT":       "登录速率限制",
		"TSHELL_SECURITY_API_RATE_LIMIT":         "API速率限制",
		"TSHELL_SECURITY_ENABLE_IP_WHITELIST":    "是否启用IP白名单",
		"TSHELL_SECURITY_WHITELIST_IPS":          "白名单IP列表(逗号分隔)",
		"TSHELL_CONNECTION_POOL_MAX_CONNECTIONS": "连接池最大连接数",
		"TSHELL_WORKER_POOL_WORKER_COUNT":        "工作池协程数",
	}

	errors.RespondWithSuccess(c, envVars)
}

// sanitizeConfig 敏感信息脱敏
func sanitizeConfig(config *Config) *Config {
	// 创建配置副本
	sanitized := *config

	// JWT 密钥文件路径
	if sanitized.JWT.SecretKeyFile != "" {
		sanitized.JWT.SecretKeyFile = "[REDACTED]"
	}

	// 认证敏感信息（默认口令 / 隐藏访问密钥）
	if sanitized.Auth.DefaultPassword != "" {
		sanitized.Auth.DefaultPassword = "[REDACTED]"
	}
	if sanitized.Auth.HiddenAccessKey != "" {
		sanitized.Auth.HiddenAccessKey = "[REDACTED]"
	}
	if sanitized.Auth.HiddenAccessConfig.SecretKey != "" {
		sanitized.Auth.HiddenAccessConfig.SecretKey = "[REDACTED]"
	}

	// 通知平台凭证
	if sanitized.Notifications.DingTalk.Token != "" {
		sanitized.Notifications.DingTalk.Token = "[REDACTED]"
	}
	if sanitized.Notifications.DingTalk.Secret != "" {
		sanitized.Notifications.DingTalk.Secret = "[REDACTED]"
	}
	if sanitized.Notifications.Wecom.WxKey != "" {
		sanitized.Notifications.Wecom.WxKey = "[REDACTED]"
	}
	if sanitized.Notifications.Feishu.Webhook != "" {
		sanitized.Notifications.Feishu.Webhook = "[REDACTED]"
	}
	if sanitized.Notifications.Feishu.Secret != "" {
		sanitized.Notifications.Feishu.Secret = "[REDACTED]"
	}

	// 指标抓取 token 与代码签名密码
	if sanitized.Metrics.Token != "" {
		sanitized.Metrics.Token = "[REDACTED]"
	}
	if sanitized.Signing.Password != "" {
		sanitized.Signing.Password = "[REDACTED]"
	}

	// HTTP配置中可能包含敏感信息也要脱敏
	if len(sanitized.HTTP.GetMetadataPrepend) > 50 {
		sanitized.HTTP.GetMetadataPrepend = sanitized.HTTP.GetMetadataPrepend[:50] + "...[REDACTED]"
	}
	if len(sanitized.HTTP.PostIDPrepend) > 50 {
		sanitized.HTTP.PostIDPrepend = sanitized.HTTP.PostIDPrepend[:50] + "...[REDACTED]"
	}
	if len(sanitized.HTTP.PostIDAppend) > 50 {
		sanitized.HTTP.PostIDAppend = sanitized.HTTP.PostIDAppend[:50] + "...[REDACTED]"
	}

	return &sanitized
}

// applyConfigUpdates 应用配置更新
func applyConfigUpdates(config *Config, updates map[string]interface{}) error {
	// 这里可以实现更复杂的配置更新逻辑
	// 为了简化，这里只处理一些常见的配置项

	if server, ok := updates["server"].(map[string]interface{}); ok {
		if port, ok := server["port"].(float64); ok {
			config.Server.Port = int(port)
		}
		if host, ok := server["host"].(string); ok {
			config.Server.Host = host
		}
		if mode, ok := server["mode"].(string); ok {
			config.Server.Mode = mode
		}
	}

	if sec, ok := updates["security"].(map[string]interface{}); ok {
		if enableRateLimit, ok := sec["enable_rate_limit"].(bool); ok {
			config.Security.EnableRateLimit = enableRateLimit
		}
		if loginRateLimit, ok := sec["login_rate_limit"].(float64); ok {
			config.Security.LoginRateLimit = int(loginRateLimit)
		}
		if apiRateLimit, ok := sec["api_rate_limit"].(float64); ok {
			config.Security.APIRateLimit = int(apiRateLimit)
		}
		if d, ok := durationValue(sec["block_duration"]); ok {
			config.Security.BlockDuration = d
		}
		if d, ok := durationValue(sec["sliding_window"]); ok {
			config.Security.SlidingWindow = d
		}
		// 配置写入后同步运行时，否则页面显示已保存、实际限流仍是旧值。
		security.GlobalRateLimiter.UpdateConfigFull(
			config.Security.LoginRateLimit,
			config.Security.APIRateLimit,
			config.Security.BlockDuration,
			config.Security.SlidingWindow,
		)
		security.GlobalRateLimiter.SetEnabled(config.Security.EnableRateLimit)
	}

	// 白名单/强制模式独立块：避免局部变量 security 遮蔽包名
	if secMap, ok := updates["security"].(map[string]interface{}); ok {
		if enableWhitelist, ok := secMap["enable_ip_whitelist"].(bool); ok {
			config.Security.EnableIPWhitelist = enableWhitelist
			security.WhitelistEnforced = enableWhitelist
		}
		if whitelistIPs, ok := secMap["whitelist_ips"].([]interface{}); ok {
			entries := make([]string, 0, len(whitelistIPs))
			for _, item := range whitelistIPs {
				if s, ok := item.(string); ok {
					entries = append(entries, s)
				}
			}
			config.Security.WhitelistIPs = entries
			security.GlobalRateLimiter.RebuildWhitelist(entries)
		}
	}

	if logging, ok := updates["logging"].(map[string]interface{}); ok {
		if level, ok := logging["level"].(string); ok {
			config.Logging.Level = level
		}
		if enableConsole, ok := logging["enable_console"].(bool); ok {
			config.Logging.EnableConsole = enableConsole
		}
	}

	// 载荷 OPSEC 默认值（生成期开关的团队级默认）。
	// 只接受 "1"/"0"/合法枚举/空串，非法值直接拒绝——写进配置的非法值会在
	// 每次生成载荷时被 resolveOpsec 二次校验并 400，但在这里挡掉能给出更早的提示。
	if opsec, ok := updates["opsec"].(map[string]interface{}); ok {
		if err := applyOpsecUpdates(&config.Opsec, opsec); err != nil {
			return err
		}
	}

	return nil
}

// durationValue 解析 JSON 中的 duration：既接受 time.Duration 的数字（纳秒），
// 也接受 "15m" 这类字符串，便于手工编辑配置文件。
func durationValue(v interface{}) (time.Duration, bool) {
	switch x := v.(type) {
	case float64:
		return time.Duration(x), true
	case string:
		d, err := time.ParseDuration(strings.TrimSpace(x))
		if err == nil {
			return d, true
		}
	}
	return 0, false
}

// applyOpsecUpdates 逐字段更新 opsec 段（字段名与 OpsecConfig 的 json tag 一致）。
func applyOpsecUpdates(o *OpsecConfig, m map[string]interface{}) error {
	onOff := map[string]*string{
		"etw_patch":       &o.ETWPatch,
		"amsi_bypass":     &o.AmsiBypass,
		"auto_delete":     &o.AutoDelete,
		"quiet_mode":      &o.QuietMode,
		"sleep_mask_hide": &o.SleepMaskHide,
		"stack_spoof":     &o.StackSpoof,
		"ppid_spoof":      &o.PpidSpoof,
		"block_dlls":      &o.BlockDLLs,
		"unhook_on_start": &o.UnhookOnStart,
	}
	for field, dst := range onOff {
		v, ok := m[field]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("opsec.%s 必须是字符串 \"1\"/\"0\"", field)
		}
		s = strings.ToLower(strings.TrimSpace(s))
		switch s {
		case "", "1", "0":
			*dst = s
		default:
			return fmt.Errorf("opsec.%s 取值非法: %q（仅接受 1/0，空表示未配置）", field, s)
		}
	}
	enums := map[string]struct {
		dst    *string
		values []string
	}{
		"syscall_mode": {&o.SyscallMode, []string{"none", "direct", "indirect"}},
		"inject_tech":  {&o.InjectTech, []string{"auto", "ntc", "apc", "crt"}},
	}
	for field, spec := range enums {
		v, ok := m[field]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("opsec.%s 必须是字符串", field)
		}
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			*spec.dst = ""
			continue
		}
		valid := false
		for _, cand := range spec.values {
			if s == cand {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("opsec.%s 取值非法: %q（仅接受 %s）", field, s, strings.Join(spec.values, "/"))
		}
		*spec.dst = s
	}
	return nil
}
