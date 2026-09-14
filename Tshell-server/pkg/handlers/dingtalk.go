package handlers

import (
	"BackendTemplate/pkg/config"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

// DingTalkMessage 钉钉消息结构
type DingTalkMessage struct {
	MsgType string              `json:"msgtype"`
	Text    DingTalkTextContent `json:"text"`
	At      *DingTalkAt         `json:"at,omitempty"`
}

// DingTalkTextContent 钉钉文本内容
type DingTalkTextContent struct {
	Content string `json:"content"`
}

// DingTalkAt 钉钉@功能
type DingTalkAt struct {
	AtMobiles []string `json:"atMobiles,omitempty"`
	IsAtAll   bool     `json:"isAtAll,omitempty"`
}

// DingTalkResponse 钉钉响应
type DingTalkResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// SendDingTalkNotification 发送钉钉通知
func SendDingTalkNotification(client interface{}, dingTalkConfig config.DingTalkConfig) error {
	if !dingTalkConfig.Enable || dingTalkConfig.Token == "" {
		return fmt.Errorf("钉钉通知未启用或Token为空")
	}

	// 构建消息内容
	content := buildClientNotificationContent(client, dingTalkConfig.Keyword)
	
	// 构建钉钉消息
	message := DingTalkMessage{
		MsgType: "text",
		Text: DingTalkTextContent{
			Content: content,
		},
	}

	// 构建webhook URL
	webhookURL := fmt.Sprintf("https://oapi.dingtalk.com/robot/send?access_token=%s", dingTalkConfig.Token)
	
	// 如果有签名密钥，添加签名
	if dingTalkConfig.Secret != "" {
		timestamp := time.Now().UnixMilli()
		sign := generateDingTalkSign(timestamp, dingTalkConfig.Secret)
		webhookURL += fmt.Sprintf("&timestamp=%d&sign=%s", timestamp, url.QueryEscape(sign))
	}

	// 序列化消息
	msgBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化钉钉消息失败: %v", err)
	}

	// 发送HTTP请求
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(webhookURL, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送钉钉请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 解析响应
	var dingResp DingTalkResponse
	if err := json.NewDecoder(resp.Body).Decode(&dingResp); err != nil {
		return fmt.Errorf("解析钉钉响应失败: %v", err)
	}

	if dingResp.ErrCode != 0 {
		return fmt.Errorf("钉钉通知发送失败: %s (错误码: %d)", dingResp.ErrMsg, dingResp.ErrCode)
	}

	logx.Infof("[INFO] 钉钉通知发送成功\n")
	return nil
}

// generateDingTalkSign 生成钉钉签名
func generateDingTalkSign(timestamp int64, secret string) string {
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	hash := hmac.New(sha256.New, []byte(secret))
	hash.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

// buildClientNotificationContent 构建客户端通知内容
func buildClientNotificationContent(client interface{}, keyword string) string {
	// 使用类型断言获取客户端信息
	var content string
	
	// 尝试从不同的客户端类型获取信息
	switch c := client.(type) {
	case map[string]interface{}:
		content = fmt.Sprintf(
			"【%s】客户端上线通知\n"+
				"外网IP: %v\n"+
				"地址: %v\n"+
				"进程: %v\n"+
				"架构: %v\n"+
				"内网IP: %v\n"+
				"用户: %v\n"+
				"时间: %s",
			keyword,
			getValueFromMap(c, "ExternalIP", "external_ip"),
			getValueFromMap(c, "Address", "address"),
			getValueFromMap(c, "Process", "process"),
			getValueFromMap(c, "Arch", "arch"),
			getValueFromMap(c, "InternalIP", "internal_ip"),
			getValueFromMap(c, "Username", "username"),
			time.Now().Format("2006-01-02 15:04:05"),
		)
	default:
		// 使用反射或其他方式处理
		content = fmt.Sprintf(
			"【%s】客户端上线通知\n"+
				"时间: %s\n"+
				"详情: %v",
			keyword,
			time.Now().Format("2006-01-02 15:04:05"),
			client,
		)
	}
	
	return content
}

// getValueFromMap 从map中获取值，支持多种键名
func getValueFromMap(m map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, exists := m[key]; exists {
			return value
		}
	}
	return "未知"
}