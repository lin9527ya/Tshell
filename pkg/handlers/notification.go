package handlers

import (
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/errors"
	"fmt"

	"github.com/gin-gonic/gin"
	"BackendTemplate/pkg/utils/logx"
)

// NotificationConfigRequest 通知配置请求结构
type NotificationConfigRequest struct {
	Enable      bool                 `json:"enable"`
	EnableAudio bool                 `json:"enable_audio"`
	DingTalk    DingTalkConfigReq    `json:"dingtalk"`
	Wecom       WecomConfigReq       `json:"wecom"`
	Feishu      FeishuConfigReq      `json:"feishu"`
}

type DingTalkConfigReq struct {
	Enable  bool   `json:"enable"`
	Token   string `json:"token"`
	Keyword string `json:"keyword"`
	Secret  string `json:"secret"`
}

type WecomConfigReq struct {
	Enable bool   `json:"enable"`
	WxKey  string `json:"wx_key"`
}

type FeishuConfigReq struct {
	Enable  bool   `json:"enable"`
	Webhook string `json:"webhook"`
	Secret  string `json:"secret"`
	Keyword string `json:"keyword"`
}

// GetNotificationConfig 获取通知配置
func GetNotificationConfig(c *gin.Context) {
	if config.GlobalConfig == nil {
		errors.AbortWithError(c, errors.NewInternalError("配置管理器未初始化"))
		return
	}

	appConfig := config.GlobalConfig.GetConfig()

	// 【安全修复】秘密字段掩码返回：token/secret/webhook 不回显原文，
	// 前端留空即表示不修改
	maskSecret := func(s string) string {
		if s == "" {
			return ""
		}
		return "******"
	}

	// 构建响应数据
	response := NotificationConfigRequest{
		Enable:      appConfig.Notifications.Enable,
		EnableAudio: appConfig.Notifications.EnableAudio,
		DingTalk: DingTalkConfigReq{
			Enable:  appConfig.Notifications.DingTalk.Enable,
			Token:   maskSecret(appConfig.Notifications.DingTalk.Token),
			Keyword: appConfig.Notifications.DingTalk.Keyword,
			Secret:  maskSecret(appConfig.Notifications.DingTalk.Secret),
		},
		Wecom: WecomConfigReq{
			Enable: appConfig.Notifications.Wecom.Enable,
			WxKey:  maskSecret(appConfig.Notifications.Wecom.WxKey),
		},
		Feishu: FeishuConfigReq{
			Enable:  appConfig.Notifications.Feishu.Enable,
			Webhook: maskSecret(appConfig.Notifications.Feishu.Webhook),
			Secret:  maskSecret(appConfig.Notifications.Feishu.Secret),
			Keyword: appConfig.Notifications.Feishu.Keyword,
		},
	}

	errors.RespondWithSuccess(c, response)
}

// UpdateNotificationConfig 更新通知配置
func UpdateNotificationConfig(c *gin.Context) {
	if config.GlobalConfig == nil {
		errors.AbortWithError(c, errors.NewInternalError("配置管理器未初始化"))
		return
	}

	var request NotificationConfigRequest
	if err := errors.ValidateRequest(c, &request); err != nil {
		errors.AbortWithError(c, err.(*errors.AppError))
		return
	}

	// 自动处理配置：当关键字段为空时，自动禁用对应平台
	autoAdjustConfig(&request)

	// 验证配置
	if err := validateNotificationConfig(&request); err != nil {
		errors.AbortWithError(c, errors.NewValidationError(err.Error()))
		return
	}

	// 更新配置（【安全修复】"******" 掩码值视为未修改：保留原配置）
	keepIfMasked := func(current, submitted string) string {
		if submitted == "" || submitted == "******" {
			return current
		}
		return submitted
	}
	err := config.GlobalConfig.UpdateConfig(func(cfg *config.Config) error {
		cfg.Notifications.Enable = request.Enable
		cfg.Notifications.EnableAudio = request.EnableAudio

		cfg.Notifications.DingTalk.Enable = request.DingTalk.Enable
		cfg.Notifications.DingTalk.Token = keepIfMasked(cfg.Notifications.DingTalk.Token, request.DingTalk.Token)
		cfg.Notifications.DingTalk.Keyword = request.DingTalk.Keyword
		cfg.Notifications.DingTalk.Secret = keepIfMasked(cfg.Notifications.DingTalk.Secret, request.DingTalk.Secret)

		cfg.Notifications.Wecom.Enable = request.Wecom.Enable
		cfg.Notifications.Wecom.WxKey = keepIfMasked(cfg.Notifications.Wecom.WxKey, request.Wecom.WxKey)

		cfg.Notifications.Feishu.Enable = request.Feishu.Enable
		cfg.Notifications.Feishu.Webhook = keepIfMasked(cfg.Notifications.Feishu.Webhook, request.Feishu.Webhook)
		cfg.Notifications.Feishu.Secret = keepIfMasked(cfg.Notifications.Feishu.Secret, request.Feishu.Secret)
		cfg.Notifications.Feishu.Keyword = request.Feishu.Keyword

		return nil
	})

	if err != nil {
		logx.Infof("[ERROR] 更新通知配置失败: %s\n", err.Error())
		errors.AbortWithError(c, errors.NewInternalError("配置更新失败: "+err.Error()))
		return
	}

	logx.Infof("[INFO] 通知配置更新成功\n")
	errors.RespondWithSuccess(c, gin.H{
		"message": "通知配置更新成功",
	})
}

// TestNotification 测试通知
func TestNotification(c *gin.Context) {
	if config.GlobalConfig == nil {
		errors.AbortWithError(c, errors.NewInternalError("配置管理器未初始化"))
		return
	}

	appConfig := config.GlobalConfig.GetConfig()
	if !appConfig.Notifications.Enable {
		errors.AbortWithError(c, errors.NewValidationError("通知功能未启用"))
		return
	}

	// 模拟客户端信息用于测试
	testClient := struct {
		ExternalIP string
		Address    string
		Process    string
		Arch       string
		InternalIP string
		Username   string
	}{
		ExternalIP: "127.0.0.1",
		Address:    "测试地址",
		Process:    "测试进程",
		Arch:       "x64",
		InternalIP: "192.168.1.100",
		Username:   "TestUser",
	}

	var successCount int
	var errs []string

	// 测试钉钉通知
	if appConfig.Notifications.DingTalk.Enable && appConfig.Notifications.DingTalk.Token != "" {
		if err := SendDingTalkNotification(testClient, appConfig.Notifications.DingTalk); err != nil {
			errs = append(errs, "钉钉通知测试失败: "+err.Error())
		} else {
			successCount++
		}
	}

	// 测试企业微信通知
	if appConfig.Notifications.Wecom.Enable && appConfig.Notifications.Wecom.WxKey != "" {
		if err := SendWecomNotification(testClient, appConfig.Notifications.Wecom.WxKey); err != nil {
			errs = append(errs, "企业微信通知测试失败: "+err.Error())
		} else {
			successCount++
		}
	}

	// 测试飞书通知
	if appConfig.Notifications.Feishu.Enable && appConfig.Notifications.Feishu.Webhook != "" {
		if err := SendFeishuNotification(testClient, appConfig.Notifications.Feishu); err != nil {
			errs = append(errs, "飞书通知测试失败: "+err.Error())
		} else {
			successCount++
		}
	}

	if successCount == 0 {
		errors.AbortWithError(c, errors.NewValidationError("没有可用的通知配置或所有通知测试失败"))
		return
	}

	response := gin.H{
		"message":       "通知测试完成",
		"success_count": successCount,
	}

	if len(errs) > 0 {
		response["errors"] = errs
	}

	errors.RespondWithSuccess(c, response)
}

// validateNotificationConfig 验证通知配置
func validateNotificationConfig(config *NotificationConfigRequest) error {
	if !config.Enable {
		return nil // 如果未启用通知，则无需验证
	}

	var hasValidConfig bool

	// 验证钉钉配置
	if config.DingTalk.Enable {
		if config.DingTalk.Token == "" {
			return fmt.Errorf("钉钉Token不能为空")
		}
		hasValidConfig = true
	}

	// 验证企业微信配置
	if config.Wecom.Enable {
		if config.Wecom.WxKey == "" {
			return fmt.Errorf("企业微信WxKey不能为空")
		}
		hasValidConfig = true
	}

	// 验证飞书配置
	if config.Feishu.Enable {
		if config.Feishu.Webhook == "" {
			return fmt.Errorf("飞书Webhook不能为空")
		}
		hasValidConfig = true
	}

	if !hasValidConfig {
		return fmt.Errorf("启用通知功能时，至少需要配置一个通知平台")
	}

	return nil
}

// autoAdjustConfig 自动调整配置：当关键字段为空时，自动禁用对应平台
func autoAdjustConfig(config *NotificationConfigRequest) {
	// 如果钉钉Token为空，自动禁用钉钉
	if config.DingTalk.Enable && config.DingTalk.Token == "" {
		config.DingTalk.Enable = false
		logx.Infof("[INFO] 钉钉Token为空，自动禁用钉钉通知\n")
	}

	// 如果企业微信WxKey为空，自动禁用企业微信
	if config.Wecom.Enable && config.Wecom.WxKey == "" {
		config.Wecom.Enable = false
		logx.Infof("[INFO] 企业微信WxKey为空，自动禁用企业微信通知\n")
	}

	// 如果飞书Webhook为空，自动禁用飞书
	if config.Feishu.Enable && config.Feishu.Webhook == "" {
		config.Feishu.Enable = false
		logx.Infof("[INFO] 飞书Webhook为空，自动禁用飞书通知\n")
	}
}