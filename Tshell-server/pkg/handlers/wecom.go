package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

// WecomMessage 企业微信消息结构
type WecomMessage struct {
	MsgType string            `json:"msgtype"`
	Text    WecomTextContent  `json:"text"`
	At      *WecomAt          `json:"at,omitempty"`
}

// WecomTextContent 企业微信文本内容
type WecomTextContent struct {
	Content             string   `json:"content"`
	MentionedList       []string `json:"mentioned_list,omitempty"`
	MentionedMobileList []string `json:"mentioned_mobile_list,omitempty"`
}

// WecomAt 企业微信@功能
type WecomAt struct {
	IsAtAll bool `json:"isAtAll,omitempty"`
}

// WecomResponse 企业微信响应
type WecomResponse struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// WecomMarkdownMessage 企业微信Markdown消息结构
type WecomMarkdownMessage struct {
	MsgType  string                `json:"msgtype"`
	Markdown WecomMarkdownContent  `json:"markdown"`
}

// WecomMarkdownContent 企业微信Markdown内容
type WecomMarkdownContent struct {
	Content string `json:"content"`
}

// SendWecomNotification 发送企业微信通知（兼容现有接口）
func SendWecomNotification(client interface{}, wxKey string) error {
	if wxKey == "" {
		return fmt.Errorf("企业微信WxKey不能为空")
	}

	// 构建消息内容
	content := buildClientNotificationContent(client, "上线")
	
	// 构建企业微信消息
	message := WecomMessage{
		MsgType: "text",
		Text: WecomTextContent{
			Content: content,
		},
	}

	// 构建webhook URL
	webhookURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%s", wxKey)

	// 序列化消息
	msgBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化企业微信消息失败: %v", err)
	}

	// 发送HTTP请求
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(webhookURL, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送企业微信请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("企业微信API返回错误状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var wecomResp WecomResponse
	if err := json.NewDecoder(resp.Body).Decode(&wecomResp); err != nil {
		return fmt.Errorf("解析企业微信响应失败: %v", err)
	}

	if wecomResp.ErrCode != 0 {
		return fmt.Errorf("企业微信通知发送失败: %s (错误码: %d)", wecomResp.ErrMsg, wecomResp.ErrCode)
	}

	logx.Infof("[INFO] 企业微信通知发送成功\n")
	return nil
}

// SendWecomMarkdownNotification 发送企业微信Markdown格式通知
func SendWecomMarkdownNotification(client interface{}, wxKey string, keyword string) error {
	if wxKey == "" {
		return fmt.Errorf("企业微信WxKey不能为空")
	}

	// 构建Markdown格式的消息内容
	content := buildWecomMarkdownContent(client, keyword)
	
	// 构建企业微信Markdown消息
	message := WecomMarkdownMessage{
		MsgType: "markdown",
		Markdown: WecomMarkdownContent{
			Content: content,
		},
	}

	// 构建webhook URL
	webhookURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%s", wxKey)

	// 序列化消息
	msgBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化企业微信Markdown消息失败: %v", err)
	}

	// 发送HTTP请求
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(webhookURL, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送企业微信Markdown请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("企业微信API返回错误状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var wecomResp WecomResponse
	if err := json.NewDecoder(resp.Body).Decode(&wecomResp); err != nil {
		return fmt.Errorf("解析企业微信响应失败: %v", err)
	}

	if wecomResp.ErrCode != 0 {
		return fmt.Errorf("企业微信Markdown通知发送失败: %s (错误码: %d)", wecomResp.ErrMsg, wecomResp.ErrCode)
	}

	logx.Infof("[INFO] 企业微信Markdown通知发送成功\n")
	return nil
}

// buildWecomMarkdownContent 构建企业微信Markdown格式内容
func buildWecomMarkdownContent(client interface{}, keyword string) string {
	var content string
	
	// 根据客户端类型构建Markdown内容
	switch c := client.(type) {
	case map[string]interface{}:
		content = fmt.Sprintf(
			"## 【%s】客户端上线通知\n\n"+
				"> **外网IP**: `%v`\n"+
				"> **地址**: %v\n"+
				"> **进程**: `%v`\n"+
				"> **架构**: %v\n"+
				"> **内网IP**: `%v`\n"+
				"> **用户**: %v\n"+
				"> **时间**: <font color=\"info\">%s</font>\n",
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
		content = fmt.Sprintf(
			"## 【%s】客户端上线通知\n\n"+
				"> **时间**: <font color=\"info\">%s</font>\n"+
				"> **详情**: %v\n",
			keyword,
			time.Now().Format("2006-01-02 15:04:05"),
			client,
		)
	}
	
	return content
}

// WecomImageMessage 企业微信图片消息结构（可选）
type WecomImageMessage struct {
	MsgType string           `json:"msgtype"`
	Image   WecomImageContent `json:"image"`
}

// WecomImageContent 企业微信图片内容
type WecomImageContent struct {
	Base64 string `json:"base64"`
	MD5    string `json:"md5"`
}

// WecomNewsMessage 企业微信图文消息结构（可选）
type WecomNewsMessage struct {
	MsgType string           `json:"msgtype"`
	News    WecomNewsContent `json:"news"`
}

// WecomNewsContent 企业微信图文内容
type WecomNewsContent struct {
	Articles []WecomArticle `json:"articles"`
}

// WecomArticle 企业微信文章
type WecomArticle struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	PicURL      string `json:"picurl"`
}

// SendWecomNewsNotification 发送企业微信图文通知（可选功能）
func SendWecomNewsNotification(client interface{}, wxKey string, keyword string) error {
	if wxKey == "" {
		return fmt.Errorf("企业微信WxKey不能为空")
	}

	// 构建图文消息
	article := WecomArticle{
		Title:       fmt.Sprintf("【%s】客户端上线通知", keyword),
		Description: buildClientNotificationContent(client, keyword),
		URL:         "", // 可以添加详情页面链接
		PicURL:      "", // 可以添加图片链接
	}

	message := WecomNewsMessage{
		MsgType: "news",
		News: WecomNewsContent{
			Articles: []WecomArticle{article},
		},
	}

	// 构建webhook URL
	webhookURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%s", wxKey)

	// 序列化消息
	msgBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化企业微信图文消息失败: %v", err)
	}

	// 发送HTTP请求
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(webhookURL, "application/json", bytes.NewBuffer(msgBytes))
	if err != nil {
		return fmt.Errorf("发送企业微信图文请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("企业微信API返回错误状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var wecomResp WecomResponse
	if err := json.NewDecoder(resp.Body).Decode(&wecomResp); err != nil {
		return fmt.Errorf("解析企业微信响应失败: %v", err)
	}

	if wecomResp.ErrCode != 0 {
		return fmt.Errorf("企业微信图文通知发送失败: %s (错误码: %d)", wecomResp.ErrMsg, wecomResp.ErrCode)
	}

	logx.Infof("[INFO] 企业微信图文通知发送成功\n")
	return nil
}