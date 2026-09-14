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
	"strconv"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

// FeishuMessage 飞书消息结构
type FeishuMessage struct {
	MsgType   string              `json:"msg_type"`
	Content   FeishuContent       `json:"content"`
	Timestamp string              `json:"timestamp,omitempty"`
	Sign      string              `json:"sign,omitempty"`
}

// FeishuContent 飞书消息内容
type FeishuContent struct {
	Text string `json:"text"`
}

// FeishuResponse 飞书响应
type FeishuResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// SendFeishuNotification 发送飞书通知
func SendFeishuNotification(client interface{}, feishuConfig config.FeishuConfig) error {
	if !feishuConfig.Enable || feishuConfig.Webhook == "" {
		return fmt.Errorf("飞书通知未启用或Webhook为空")
	}

	// 构建消息内容
	content := buildClientNotificationContent(client, feishuConfig.Keyword)
	
	// 构建飞书消息
	message := FeishuMessage{
		MsgType: "text",
		Content: FeishuContent{
			Text: content,
		},
	}

	// 如果有签名密钥，添加签名验证
	if feishuConfig.Secret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		sign := generateFeishuSign(timestamp, feishuConfig.Secret)
		message.Timestamp = timestamp
		message.Sign = sign
	}

	// 序列化消息
	msgBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化飞书消息失败: %v", err)
	}

	// 发送HTTP请求
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(feishuConfig.Webhook, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送飞书请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("飞书API返回错误状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var feishuResp FeishuResponse
	if err := json.NewDecoder(resp.Body).Decode(&feishuResp); err != nil {
		return fmt.Errorf("解析飞书响应失败: %v", err)
	}

	if feishuResp.Code != 0 {
		return fmt.Errorf("飞书通知发送失败: %s (错误码: %d)", feishuResp.Msg, feishuResp.Code)
	}

	logx.Infof("[INFO] 飞书通知发送成功\n")
	return nil
}

// generateFeishuSign 生成飞书签名
func generateFeishuSign(timestamp, secret string) string {
	stringToSign := timestamp + "\n" + secret
	hash := hmac.New(sha256.New, []byte(stringToSign))
	hash.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

// FeishuRichTextMessage 飞书富文本消息结构（可选，支持更丰富的格式）
type FeishuRichTextMessage struct {
	MsgType   string                 `json:"msg_type"`
	Content   FeishuRichTextContent  `json:"content"`
	Timestamp string                 `json:"timestamp,omitempty"`
	Sign      string                 `json:"sign,omitempty"`
}

// FeishuRichTextContent 飞书富文本内容
type FeishuRichTextContent struct {
	Post FeishuPost `json:"post"`
}

// FeishuPost 飞书帖子
type FeishuPost struct {
	ZhCn FeishuPostContent `json:"zh_cn"`
}

// FeishuPostContent 飞书帖子内容
type FeishuPostContent struct {
	Title   string              `json:"title"`
	Content [][]FeishuTextBlock `json:"content"`
}

// FeishuTextBlock 飞书文本块
type FeishuTextBlock struct {
	Tag  string `json:"tag"`
	Text string `json:"text,omitempty"`
	Href string `json:"href,omitempty"`
}

// SendFeishuRichTextNotification 发送飞书富文本通知（可选功能）
func SendFeishuRichTextNotification(client interface{}, feishuConfig config.FeishuConfig) error {
	if !feishuConfig.Enable || feishuConfig.Webhook == "" {
		return fmt.Errorf("飞书通知未启用或Webhook为空")
	}

	// 构建富文本消息
	message := FeishuRichTextMessage{
		MsgType: "post",
		Content: FeishuRichTextContent{
			Post: FeishuPost{
				ZhCn: FeishuPostContent{
					Title: fmt.Sprintf("【%s】客户端上线通知", feishuConfig.Keyword),
					Content: buildFeishuRichTextContent(client),
				},
			},
		},
	}

	// 如果有签名密钥，添加签名验证
	if feishuConfig.Secret != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		sign := generateFeishuSign(timestamp, feishuConfig.Secret)
		message.Timestamp = timestamp
		message.Sign = sign
	}

	// 序列化消息
	msgBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化飞书富文本消息失败: %v", err)
	}

	// 发送HTTP请求
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(feishuConfig.Webhook, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送飞书富文本请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("飞书API返回错误状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var feishuResp FeishuResponse
	if err := json.NewDecoder(resp.Body).Decode(&feishuResp); err != nil {
		return fmt.Errorf("解析飞书响应失败: %v", err)
	}

	if feishuResp.Code != 0 {
		return fmt.Errorf("飞书富文本通知发送失败: %s (错误码: %d)", feishuResp.Msg, feishuResp.Code)
	}

	logx.Infof("[INFO] 飞书富文本通知发送成功\n")
	return nil
}

// buildFeishuRichTextContent 构建飞书富文本内容
func buildFeishuRichTextContent(client interface{}) [][]FeishuTextBlock {
	content := [][]FeishuTextBlock{}
	
	// 根据客户端类型构建内容
	switch c := client.(type) {
	case map[string]interface{}:
		content = append(content, []FeishuTextBlock{
			{Tag: "text", Text: "外网IP: "},
			{Tag: "text", Text: fmt.Sprintf("%v", getValueFromMap(c, "ExternalIP", "external_ip"))},
		})
		content = append(content, []FeishuTextBlock{
			{Tag: "text", Text: "地址: "},
			{Tag: "text", Text: fmt.Sprintf("%v", getValueFromMap(c, "Address", "address"))},
		})
		content = append(content, []FeishuTextBlock{
			{Tag: "text", Text: "进程: "},
			{Tag: "text", Text: fmt.Sprintf("%v", getValueFromMap(c, "Process", "process"))},
		})
		content = append(content, []FeishuTextBlock{
			{Tag: "text", Text: "架构: "},
			{Tag: "text", Text: fmt.Sprintf("%v", getValueFromMap(c, "Arch", "arch"))},
		})
		content = append(content, []FeishuTextBlock{
			{Tag: "text", Text: "内网IP: "},
			{Tag: "text", Text: fmt.Sprintf("%v", getValueFromMap(c, "InternalIP", "internal_ip"))},
		})
		content = append(content, []FeishuTextBlock{
			{Tag: "text", Text: "用户: "},
			{Tag: "text", Text: fmt.Sprintf("%v", getValueFromMap(c, "Username", "username"))},
		})
	default:
		content = append(content, []FeishuTextBlock{
			{Tag: "text", Text: fmt.Sprintf("客户端信息: %v", client)},
		})
	}
	
	// 添加时间戳
	content = append(content, []FeishuTextBlock{
		{Tag: "text", Text: "时间: "},
		{Tag: "text", Text: time.Now().Format("2006-01-02 15:04:05")},
	})
	
	return content
}