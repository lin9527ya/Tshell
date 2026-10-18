package notification

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/handlers"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

// NotificationManager 通知管理器
type NotificationManager struct {
	mu                sync.RWMutex
	enabled           bool
	lastNotifications map[string]time.Time // 防止重复通知
}

var (
	GlobalNotificationManager *NotificationManager
	once                      sync.Once
)

// InitNotificationManager 初始化全局通知管理器
func InitNotificationManager() {
	once.Do(func() {
		GlobalNotificationManager = &NotificationManager{
			lastNotifications: make(map[string]time.Time),
		}
		
		// 从配置中获取初始状态
		if config.GlobalConfig != nil {
			appConfig := config.GlobalConfig.GetConfig()
			GlobalNotificationManager.enabled = appConfig.Notifications.Enable
		}
		
		// D17: 订阅事件总线，将告警类事件接入通知（离线/安全拦截/登录失败）
		go GlobalNotificationManager.watchEvents()
		
		logx.Infof("[INFO] ✅ 通知管理器初始化成功\n")
	})
}

// watchEvents 订阅全局事件总线，将告警事件转发为通知
func (nm *NotificationManager) watchEvents() {
	if command.GlobalEventBus == nil {
		return
	}
	ch := command.GlobalEventBus.Subscribe()
	defer command.GlobalEventBus.Unsubscribe(ch)
	for ev := range ch {
		switch ev.Type {
		case command.EventClientOffline:
			nm.NotifyAlert("warning", "客户端离线", fmt.Sprintf("客户端 %s 已离线", ev.UID))
		case command.EventSecurityBlock:
			nm.NotifyAlert("danger", "安全拦截", fmt.Sprintf("IP 访问被拦截: %v", ev.Detail))
		case command.EventLoginFailed:
			nm.NotifyAlert("danger", "登录失败", fmt.Sprintf("登录失败告警: %v", ev.Detail))
		}
	}
}

// GetNotificationManager 获取全局通知管理器实例
func GetNotificationManager() *NotificationManager {
	if GlobalNotificationManager == nil {
		InitNotificationManager()
	}
	return GlobalNotificationManager
}

// OnClientOnline 客户端上线时触发通知
func (nm *NotificationManager) OnClientOnline(clientUID string) {
	nm.mu.RLock()
	if !nm.enabled {
		nm.mu.RUnlock()
		return
	}
	nm.mu.RUnlock()

	// 防止短时间内重复通知
	nm.mu.Lock()
	lastTime, exists := nm.lastNotifications[clientUID]
	if exists && time.Since(lastTime) < 30*time.Second {
		nm.mu.Unlock()
		logx.Infof("[INFO] ⏱️ 客户端 %s 在30秒内已发送过通知，跳过重复通知\n", clientUID)
		return
	}
	nm.lastNotifications[clientUID] = time.Now()
	nm.mu.Unlock()

	// 异步发送通知，避免阻塞客户端连接
	go nm.sendClientOnlineNotification(clientUID)
}

// sendClientOnlineNotification 发送客户端上线通知
func (nm *NotificationManager) sendClientOnlineNotification(clientUID string) {
	// 获取客户端信息
	var client database.Clients
	exists, err := database.DB().Where("uid = ?", clientUID).Get(&client)
	if err != nil || !exists {
		logx.Infof("[ERROR] 获取客户端信息失败: %s, 错误: %v\n", clientUID, err)
		return
	}

	// 构建客户端信息映射
	clientInfo := map[string]interface{}{
		"ExternalIP": client.ExternalIP,
		"Address":    client.Address,
		"Process":    client.Process,
		"Arch":       client.Arch,
		"InternalIP": client.InternalIP,
		"Username":   client.Username,
		"Computer":   client.Computer,
		"FirstStart": client.FirstStart,
		"Listener":   client.Listener,
	}

	// 获取当前通知配置
	if config.GlobalConfig == nil {
		logx.Infof("[ERROR] 配置管理器未初始化，无法发送通知\n")
		return
	}

	appConfig := config.GlobalConfig.GetConfig()
	notificationConfig := appConfig.Notifications

	if !notificationConfig.Enable {
		return
	}

	var successCount int
	var errors []string

	// 发送钉钉通知
	if notificationConfig.DingTalk.Enable && notificationConfig.DingTalk.Token != "" {
		if err := handlers.SendDingTalkNotification(clientInfo, notificationConfig.DingTalk); err != nil {
			errors = append(errors, fmt.Sprintf("钉钉通知失败: %v", err))
			logx.Infof("[ERROR] 客户端 %s 钉钉通知发送失败: %v\n", clientUID, err)
		} else {
			successCount++
			logx.Infof("[INFO] 客户端 %s 钉钉通知发送成功\n", clientUID)
		}
	}

	// 发送企业微信通知
	if notificationConfig.Wecom.Enable && notificationConfig.Wecom.WxKey != "" {
		if err := handlers.SendWecomNotification(clientInfo, notificationConfig.Wecom.WxKey); err != nil {
			errors = append(errors, fmt.Sprintf("企业微信通知失败: %v", err))
			logx.Infof("[ERROR] 客户端 %s 企业微信通知发送失败: %v\n", clientUID, err)
		} else {
			successCount++
			logx.Infof("[INFO] 客户端 %s 企业微信通知发送成功\n", clientUID)
		}
	}

	// 发送飞书通知
	if notificationConfig.Feishu.Enable && notificationConfig.Feishu.Webhook != "" {
		if err := handlers.SendFeishuNotification(clientInfo, notificationConfig.Feishu); err != nil {
			errors = append(errors, fmt.Sprintf("飞书通知失败: %v", err))
			logx.Infof("[ERROR] 客户端 %s 飞书通知发送失败: %v\n", clientUID, err)
		} else {
			successCount++
			logx.Infof("[INFO] 客户端 %s 飞书通知发送成功\n", clientUID)
		}
	}

	// 记录通知结果
	if successCount > 0 {
		logx.Infof("[INFO] 🎉 客户端 %s 上线通知发送完成，成功 %d 个平台\n", clientUID, successCount)
	}

	if len(errors) > 0 {
		for _, errMsg := range errors {
			logx.Infof("[ERROR] 客户端 %s 通知错误: %s\n", clientUID, errMsg)
		}
	}

	// 语音/提示音(P1-7): 服务端为无头进程播音频无意义,提示音由前端播放
	// (Tshell-web events store 收 client_online 事件时经 Web Audio 播放,
	// 开关在 preferences.audioNotify)。此处仅保留日志说明,不做系统级播放。
	if notificationConfig.EnableAudio && successCount > 0 {
		logx.Infof("[INFO] 🔊 客户端 %s 语音通知已触发(前端播放)\n", clientUID)
	}
}

// notifySendSem 通知发送并发上限（防大量客户端同时上线时 HTTP 风暴/goroutine 无界堆积）
var notifySendSem = make(chan struct{}, 5)

// NotifyAlert 通用告警通知（D17）：异步发送到所有已启用的渠道（有界并发 + 每渠道一次重试）
func (nm *NotificationManager) NotifyAlert(level, title, content string) {
	nm.mu.RLock()
	enabled := nm.enabled
	nm.mu.RUnlock()
	if !enabled {
		return
	}
	if config.GlobalConfig == nil {
		return
	}
	nc := config.GlobalConfig.GetConfig().Notifications
	if !nc.Enable {
		return
	}
	msg := fmt.Sprintf("[%s] %s\n%s\n时间: %s", level, title, content, time.Now().Format("2006-01-02 15:04:05"))

	// 异步发送：不阻塞事件消费路径；并发已满时丢弃本次（避免 goroutine 堆积）
	select {
	case notifySendSem <- struct{}{}:
	default:
		logx.Infof("[INFO] 告警发送并发已满，丢弃: %s\n", title)
		return
	}
	go func() {
		defer func() { <-notifySendSem }()

		var sent []string
		// 每渠道失败重试一次
		if nc.DingTalk.Enable && nc.DingTalk.Token != "" {
			if err := sendDingTalkText(msg, nc.DingTalk.Token, nc.DingTalk.Secret); err != nil {
				sendDingTalkText(msg, nc.DingTalk.Token, nc.DingTalk.Secret)
			} else {
				sent = append(sent, "钉钉")
			}
		}
		if nc.Wecom.Enable && nc.Wecom.WxKey != "" {
			if err := sendWecomText(msg, nc.Wecom.WxKey); err != nil {
				sendWecomText(msg, nc.Wecom.WxKey)
			} else {
				sent = append(sent, "企微")
			}
		}
		if nc.Feishu.Enable && nc.Feishu.Webhook != "" {
			if err := sendFeishuText(msg, nc.Feishu.Webhook, nc.Feishu.Secret); err != nil {
				sendFeishuText(msg, nc.Feishu.Webhook, nc.Feishu.Secret)
			} else {
				sent = append(sent, "飞书")
			}
		}
		if len(sent) > 0 {
			logx.Infof("[INFO] 🔔 告警已发送(%s): %s\n", strings.Join(sent, "/"), title)
		} else {
			logx.Infof("[INFO] 告警未发送（无可用渠道或发送失败）: %s\n", title)
		}
	}()
}

// sendDingTalkText 发送钉钉文本消息
func sendDingTalkText(content, token, secret string) error {
	webhookURL := fmt.Sprintf("https://oapi.dingtalk.com/robot/send?access_token=%s", token)
	if secret != "" {
		ts := time.Now().UnixMilli()
		sign := notifyDingTalkSign(ts, secret)
		webhookURL += fmt.Sprintf("&timestamp=%d&sign=%s", ts, url.QueryEscape(sign))
	}
	body := map[string]interface{}{
		"msgtype": "text",
		"text":    map[string]string{"content": content},
	}
	return postJSON(webhookURL, body)
}

// notifyDingTalkSign 钉钉签名（与 handlers 包算法一致）
func notifyDingTalkSign(timestamp int64, secret string) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	hash := hmac.New(sha256.New, []byte(secret))
	hash.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

// sendWecomText 发送企业微信文本消息
func sendWecomText(content, wxKey string) error {
	webhookURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%s", wxKey)
	body := map[string]interface{}{
		"msgtype": "text",
		"text":    map[string]string{"content": content},
	}
	return postJSON(webhookURL, body)
}

// sendFeishuText 发送飞书文本消息
func sendFeishuText(content, webhook, secret string) error {
	body := map[string]interface{}{
		"msg_type": "text",
		"content":  map[string]string{"text": content},
	}
	if secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		sign := notifyFeishuSign(ts, secret)
		body["timestamp"] = ts
		body["sign"] = sign
	}
	return postJSON(webhook, body)
}

// notifyFeishuSign 飞书签名（与 handlers 包算法一致）
func notifyFeishuSign(timestamp, secret string) string {
	stringToSign := timestamp + "\n" + secret
	hash := hmac.New(sha256.New, []byte(stringToSign))
	hash.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

// postJSON 通用 webhook POST
func postJSON(url string, body interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Post(url, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// OnClientOffline 客户端离线时的处理（可选）
func (nm *NotificationManager) OnClientOffline(clientUID string) {
	nm.mu.Lock()
	// 清理通知记录
	delete(nm.lastNotifications, clientUID)
	nm.mu.Unlock()

	logx.Infof("[INFO] 👋 客户端 %s 离线，已清理通知记录\n", clientUID)
}

// UpdateConfig 更新通知配置
func (nm *NotificationManager) UpdateConfig(enabled bool) {
	nm.mu.Lock()
	nm.enabled = enabled
	nm.mu.Unlock()

	if enabled {
		logx.Infof("[INFO] ✅ 通知功能已启用\n")
	} else {
		logx.Infof("[INFO] ❌ 通知功能已禁用\n")
	}
}

// GetNotificationStatus 获取通知状态
func (nm *NotificationManager) GetNotificationStatus() map[string]interface{} {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	return map[string]interface{}{
		"enabled":              nm.enabled,
		"pending_notifications": len(nm.lastNotifications),
		"last_notifications":   nm.lastNotifications,
	}
}

// CleanupOldNotifications 清理旧的通知记录
func (nm *NotificationManager) CleanupOldNotifications() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	cutoff := time.Now().Add(-1 * time.Hour) // 保留1小时内的记录
	var toDelete []string

	for uid, lastTime := range nm.lastNotifications {
		if lastTime.Before(cutoff) {
			toDelete = append(toDelete, uid)
		}
	}

	for _, uid := range toDelete {
		delete(nm.lastNotifications, uid)
	}

	if len(toDelete) > 0 {
		logx.Infof("[INFO] 🧹 清理了 %d 个旧的通知记录\n", len(toDelete))
	}
}

// StartCleanupRoutine 启动定期清理例程
func (nm *NotificationManager) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(30 * time.Minute) // 每30分钟清理一次
		defer ticker.Stop()

		for range ticker.C {
			nm.CleanupOldNotifications()
		}
	}()
}