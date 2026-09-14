package security

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

// ===== 🔐 协议指纹随机化模块 =====

// ProtocolFingerprintRandomizer 协议指纹随机化器
type ProtocolFingerprintRandomizer struct {
	tlsConfigs     map[string]*tls.Config
	httpProfiles   map[string]*HTTPFingerprint
	tcpProfiles    map[string]*TCPFingerprint
	currentProfile string
	rotationTicker *time.Ticker
	mu             sync.RWMutex
}

// HTTPFingerprint HTTP指纹配置
type HTTPFingerprint struct {
	UserAgent        string            `json:"user_agent"`
	AcceptLanguage   string            `json:"accept_language"`
	AcceptEncoding   string            `json:"accept_encoding"`
	Accept           string            `json:"accept"`
	Connection       string            `json:"connection"`
	UpgradeInsecure  string            `json:"upgrade_insecure"`
	CacheControl     string            `json:"cache_control"`
	CustomHeaders    map[string]string `json:"custom_headers"`
	HeaderOrder      []string          `json:"header_order"`
	HTTPVersion      string            `json:"http_version"`
	KeepAlive        bool              `json:"keep_alive"`
	CompressionLevel int               `json:"compression_level"`
}

// TCPFingerprint TCP指纹配置
type TCPFingerprint struct {
	WindowSize     uint16  `json:"window_size"`
	WindowScale    uint8   `json:"window_scale"`
	MSS            uint16  `json:"mss"`
	SACKPermitted  bool    `json:"sack_permitted"`
	Timestamp      bool    `json:"timestamp"`
	TCPOptions     []uint8 `json:"tcp_options"`
	InitialCWND    uint32  `json:"initial_cwnd"`
	ConnectTimeout int     `json:"connect_timeout"`
	KeepAlive      bool    `json:"keep_alive"`
	NoDelay        bool    `json:"no_delay"`
}

// TLSFingerprint TLS指纹配置
type TLSFingerprint struct {
	Version            uint16   `json:"version"`
	CipherSuites       []uint16 `json:"cipher_suites"`
	CompressionMethods []uint8  `json:"compression_methods"`
	Extensions         []uint16 `json:"extensions"`
	EllipticCurves     []uint16 `json:"elliptic_curves"`
	PointFormats       []uint8  `json:"point_formats"`
	SignatureAlgos     []uint16 `json:"signature_algos"`
	ALPNProtocols      []string `json:"alpn_protocols"`
	ServerName         string   `json:"server_name"`
	SessionTicket      bool     `json:"session_ticket"`
	Renegotiation      uint8    `json:"renegotiation"`
}

// FingerprintRotationConfig 指纹轮换配置
type FingerprintRotationConfig struct {
	Enabled          bool           `json:"enabled"`
	RotationInterval time.Duration  `json:"rotation_interval"`
	RandomOrder      bool           `json:"random_order"`
	ProfileWeights   map[string]int `json:"profile_weights"`
}

// GlobalFingerprintRandomizer 全局协议指纹随机化器
var GlobalFingerprintRandomizer *ProtocolFingerprintRandomizer

// InitProtocolFingerprintRandomizer 初始化协议指纹随机化器
func InitProtocolFingerprintRandomizer() error {
	randomizer := &ProtocolFingerprintRandomizer{
		tlsConfigs:     make(map[string]*tls.Config),
		httpProfiles:   make(map[string]*HTTPFingerprint),
		tcpProfiles:    make(map[string]*TCPFingerprint),
		currentProfile: "default",
	}

	// 初始化各种协议指纹配置
	randomizer.initTLSFingerprints()
	randomizer.initHTTPFingerprints()
	randomizer.initTCPFingerprints()

	// 启动自动轮换
	randomizer.startRotation(30 * time.Minute) // 每30分钟轮换一次

	GlobalFingerprintRandomizer = randomizer
	logx.Println("🔐 [协议指纹] 随机化器初始化完成")
	return nil
}

// initTLSFingerprints 初始化TLS指纹配置
func (pfr *ProtocolFingerprintRandomizer) initTLSFingerprints() {
	// Chrome TLS指纹
	pfr.tlsConfigs["chrome"] = &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		NextProtos:         []string{"h2", "http/1.1"},
		InsecureSkipVerify: true,
		ServerName:         "",
	}

	// Firefox TLS指纹
	pfr.tlsConfigs["firefox"] = &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
		NextProtos:         []string{"h2", "http/1.1"},
		InsecureSkipVerify: true,
		ServerName:         "",
	}

	// Safari TLS指纹
	pfr.tlsConfigs["safari"] = &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		NextProtos:         []string{"h2", "http/1.1"},
		InsecureSkipVerify: true,
		ServerName:         "",
	}

	// Edge TLS指纹
	pfr.tlsConfigs["edge"] = &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		},
		NextProtos:         []string{"h2", "http/1.1"},
		InsecureSkipVerify: true,
		ServerName:         "",
	}

	// 随机TLS指纹
	pfr.tlsConfigs["random"] = pfr.generateRandomTLSConfig()
}

// initHTTPFingerprints 初始化HTTP指纹配置
func (pfr *ProtocolFingerprintRandomizer) initHTTPFingerprints() {
	// Chrome HTTP指纹
	pfr.httpProfiles["chrome"] = &HTTPFingerprint{
		UserAgent:        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		AcceptLanguage:   "en-US,en;q=0.9",
		AcceptEncoding:   "gzip, deflate, br",
		Accept:           "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8",
		Connection:       "keep-alive",
		UpgradeInsecure:  "1",
		CacheControl:     "max-age=0",
		HTTPVersion:      "HTTP/2.0",
		KeepAlive:        true,
		CompressionLevel: 6,
		HeaderOrder:      []string{"Host", "User-Agent", "Accept", "Accept-Language", "Accept-Encoding", "Connection", "Upgrade-Insecure-Requests", "Cache-Control"},
		CustomHeaders: map[string]string{
			"sec-ch-ua":          "\"Chromium\";v=\"120\", \"Not)A;Brand\";v=\"24\", \"Google Chrome\";v=\"120\"",
			"sec-ch-ua-mobile":   "?0",
			"sec-ch-ua-platform": "\"Windows\"",
			"Sec-Fetch-Dest":     "document",
			"Sec-Fetch-Mode":     "navigate",
			"Sec-Fetch-Site":     "none",
			"Sec-Fetch-User":     "?1",
		},
	}

	// Firefox HTTP指纹
	pfr.httpProfiles["firefox"] = &HTTPFingerprint{
		UserAgent:        "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		AcceptLanguage:   "en-US,en;q=0.5",
		AcceptEncoding:   "gzip, deflate, br",
		Accept:           "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		Connection:       "keep-alive",
		UpgradeInsecure:  "1",
		HTTPVersion:      "HTTP/2.0",
		KeepAlive:        true,
		CompressionLevel: 6,
		HeaderOrder:      []string{"Host", "User-Agent", "Accept", "Accept-Language", "Accept-Encoding", "Connection", "Upgrade-Insecure-Requests"},
		CustomHeaders: map[string]string{
			"DNT":            "1",
			"Sec-Fetch-Dest": "document",
			"Sec-Fetch-Mode": "navigate",
			"Sec-Fetch-Site": "none",
			"Sec-Fetch-User": "?1",
		},
	}

	// Safari HTTP指纹
	pfr.httpProfiles["safari"] = &HTTPFingerprint{
		UserAgent:        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
		AcceptLanguage:   "en-US,en;q=0.9",
		AcceptEncoding:   "gzip, deflate, br",
		Accept:           "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		Connection:       "keep-alive",
		UpgradeInsecure:  "1",
		HTTPVersion:      "HTTP/2.0",
		KeepAlive:        true,
		CompressionLevel: 6,
		HeaderOrder:      []string{"Host", "Accept", "User-Agent", "Accept-Language", "Accept-Encoding", "Connection", "Upgrade-Insecure-Requests"},
		CustomHeaders:    map[string]string{},
	}

	// Mobile Chrome HTTP指纹
	pfr.httpProfiles["mobile_chrome"] = &HTTPFingerprint{
		UserAgent:        "Mozilla/5.0 (Linux; Android 10; SM-G973F) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
		AcceptLanguage:   "en-US,en;q=0.9",
		AcceptEncoding:   "gzip, deflate, br",
		Accept:           "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8",
		Connection:       "keep-alive",
		UpgradeInsecure:  "1",
		HTTPVersion:      "HTTP/2.0",
		KeepAlive:        true,
		CompressionLevel: 6,
		HeaderOrder:      []string{"Host", "User-Agent", "Accept", "Accept-Language", "Accept-Encoding", "Connection"},
		CustomHeaders: map[string]string{
			"sec-ch-ua":          "\"Chromium\";v=\"120\", \"Not)A;Brand\";v=\"24\", \"Google Chrome\";v=\"120\"",
			"sec-ch-ua-mobile":   "?1",
			"sec-ch-ua-platform": "\"Android\"",
		},
	}

	// 随机HTTP指纹
	pfr.httpProfiles["random"] = pfr.generateRandomHTTPProfile()
}

// initTCPFingerprints 初始化TCP指纹配置
func (pfr *ProtocolFingerprintRandomizer) initTCPFingerprints() {
	// Windows TCP指纹
	pfr.tcpProfiles["windows"] = &TCPFingerprint{
		WindowSize:     65535,
		WindowScale:    8,
		MSS:            1460,
		SACKPermitted:  true,
		Timestamp:      true,
		TCPOptions:     []uint8{2, 4, 5, 8, 3, 3},
		InitialCWND:    10,
		ConnectTimeout: 3000, // 3秒
		KeepAlive:      true,
		NoDelay:        true,
	}

	// Linux TCP指纹
	pfr.tcpProfiles["linux"] = &TCPFingerprint{
		WindowSize:     29200,
		WindowScale:    7,
		MSS:            1460,
		SACKPermitted:  true,
		Timestamp:      true,
		TCPOptions:     []uint8{2, 4, 5, 8, 3, 3},
		InitialCWND:    10,
		ConnectTimeout: 3000,
		KeepAlive:      true,
		NoDelay:        false,
	}

	// macOS TCP指纹
	pfr.tcpProfiles["macos"] = &TCPFingerprint{
		WindowSize:     65535,
		WindowScale:    6,
		MSS:            1460,
		SACKPermitted:  true,
		Timestamp:      true,
		TCPOptions:     []uint8{2, 4, 5, 8, 3, 3},
		InitialCWND:    10,
		ConnectTimeout: 5000,
		KeepAlive:      true,
		NoDelay:        true,
	}

	// Android TCP指纹
	pfr.tcpProfiles["android"] = &TCPFingerprint{
		WindowSize:     32768,
		WindowScale:    8,
		MSS:            1440,
		SACKPermitted:  true,
		Timestamp:      true,
		TCPOptions:     []uint8{2, 4, 5, 8, 3, 3},
		InitialCWND:    10,
		ConnectTimeout: 4000,
		KeepAlive:      true,
		NoDelay:        false,
	}

	// 随机TCP指纹
	pfr.tcpProfiles["random"] = pfr.generateRandomTCPProfile()
}

// generateRandomTLSConfig 生成随机TLS配置
func (pfr *ProtocolFingerprintRandomizer) generateRandomTLSConfig() *tls.Config {
	// 随机选择TLS版本
	versions := []uint16{tls.VersionTLS12, tls.VersionTLS13}
	minVersion := versions[randomInt(0, len(versions))]
	maxVersion := uint16(tls.VersionTLS13)

	// 随机选择密码套件
	allCiphers := []uint16{
		tls.TLS_AES_128_GCM_SHA256,
		tls.TLS_AES_256_GCM_SHA384,
		tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}

	// 随机选择4-8个密码套件
	cipherCount := randomInt(4, 9)
	selectedCiphers := make([]uint16, 0, cipherCount)

	for i := 0; i < cipherCount && i < len(allCiphers); i++ {
		selectedCiphers = append(selectedCiphers, allCiphers[randomInt(0, len(allCiphers))])
	}

	// 随机选择ALPN协议
	alpnProtocols := []string{"h2", "http/1.1"}
	if randomInt(0, 2) == 0 {
		alpnProtocols = []string{"http/1.1"}
	}

	return &tls.Config{
		MinVersion:         minVersion,
		MaxVersion:         maxVersion,
		CipherSuites:       selectedCiphers,
		NextProtos:         alpnProtocols,
		InsecureSkipVerify: true,
		ServerName:         "",
	}
}

// generateRandomHTTPProfile 生成随机HTTP配置
func (pfr *ProtocolFingerprintRandomizer) generateRandomHTTPProfile() *HTTPFingerprint {
	// 随机用户代理
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + fmt.Sprintf("%d.0.0.0", randomInt(110, 125)) + " Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + fmt.Sprintf("%d.0.0.0", randomInt(110, 125)) + " Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:" + fmt.Sprintf("%d.0", randomInt(100, 125)) + ") Gecko/20100101 Firefox/" + fmt.Sprintf("%d.0", randomInt(100, 125)),
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/" + fmt.Sprintf("%d.1", randomInt(15, 18)) + " Safari/605.1.15",
	}

	// 随机语言设置
	languages := []string{
		"en-US,en;q=0.9",
		"en-US,en;q=0.5",
		"zh-CN,zh;q=0.9,en;q=0.8",
		"en-GB,en;q=0.9",
		"fr-FR,fr;q=0.9,en;q=0.8",
	}

	// 随机编码设置
	encodings := []string{
		"gzip, deflate, br",
		"gzip, deflate",
		"br, gzip, deflate",
		"gzip, deflate, br, zstd",
	}

	return &HTTPFingerprint{
		UserAgent:        userAgents[randomInt(0, len(userAgents))],
		AcceptLanguage:   languages[randomInt(0, len(languages))],
		AcceptEncoding:   encodings[randomInt(0, len(encodings))],
		Accept:           "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		Connection:       "keep-alive",
		UpgradeInsecure:  "1",
		HTTPVersion:      "HTTP/2.0",
		KeepAlive:        true,
		CompressionLevel: randomInt(1, 9),
		HeaderOrder:      []string{"Host", "User-Agent", "Accept", "Accept-Language", "Accept-Encoding", "Connection"},
		CustomHeaders:    make(map[string]string),
	}
}

// generateRandomTCPProfile 生成随机TCP配置
func (pfr *ProtocolFingerprintRandomizer) generateRandomTCPProfile() *TCPFingerprint {
	// 随机窗口大小
	windowSizes := []uint16{8192, 16384, 32768, 65535}
	windowSize := windowSizes[randomInt(0, len(windowSizes))]

	// 随机MSS
	mssValues := []uint16{1460, 1440, 1380, 1360}
	mss := mssValues[randomInt(0, len(mssValues))]

	return &TCPFingerprint{
		WindowSize:     windowSize,
		WindowScale:    uint8(randomInt(6, 10)),
		MSS:            mss,
		SACKPermitted:  randomInt(0, 2) == 1,
		Timestamp:      randomInt(0, 2) == 1,
		TCPOptions:     []uint8{2, 4, 5, 8, 3, 3},
		InitialCWND:    uint32(randomInt(8, 16)),
		ConnectTimeout: randomInt(2000, 6000),
		KeepAlive:      randomInt(0, 2) == 1,
		NoDelay:        randomInt(0, 2) == 1,
	}
}

// startRotation 启动自动轮换
func (pfr *ProtocolFingerprintRandomizer) startRotation(interval time.Duration) {
	pfr.rotationTicker = time.NewTicker(interval)
	go func() {
		for range pfr.rotationTicker.C {
			pfr.rotateToRandomProfile()
		}
	}()
	logx.Infof("🔄 [指纹轮换] 已启动，间隔: %v\n", interval)
}

// rotateToRandomProfile 轮换到随机配置
func (pfr *ProtocolFingerprintRandomizer) rotateToRandomProfile() {
	pfr.mu.Lock()
	defer pfr.mu.Unlock()

	// 选择可用的配置
	profiles := []string{"chrome", "firefox", "safari", "edge", "mobile_chrome", "random"}
	newProfile := profiles[randomInt(0, len(profiles))]

	// 避免连续使用相同配置
	for newProfile == pfr.currentProfile && len(profiles) > 1 {
		newProfile = profiles[randomInt(0, len(profiles))]
	}

	oldProfile := pfr.currentProfile
	pfr.currentProfile = newProfile

	// 如果选择了随机配置，重新生成
	if newProfile == "random" {
		pfr.tlsConfigs["random"] = pfr.generateRandomTLSConfig()
		pfr.httpProfiles["random"] = pfr.generateRandomHTTPProfile()
		pfr.tcpProfiles["random"] = pfr.generateRandomTCPProfile()
	}

	logx.Infof("🔄 [指纹轮换] %s -> %s\n", oldProfile, newProfile)
}

// GetCurrentTLSConfig 获取当前TLS配置
func (pfr *ProtocolFingerprintRandomizer) GetCurrentTLSConfig() *tls.Config {
	pfr.mu.RLock()
	defer pfr.mu.RUnlock()

	config, exists := pfr.tlsConfigs[pfr.currentProfile]
	if !exists {
		return pfr.tlsConfigs["chrome"] // 默认配置
	}
	return config
}

// GetCurrentHTTPProfile 获取当前HTTP配置
func (pfr *ProtocolFingerprintRandomizer) GetCurrentHTTPProfile() *HTTPFingerprint {
	pfr.mu.RLock()
	defer pfr.mu.RUnlock()

	profile, exists := pfr.httpProfiles[pfr.currentProfile]
	if !exists {
		return pfr.httpProfiles["chrome"] // 默认配置
	}
	return profile
}

// GetCurrentTCPProfile 获取当前TCP配置
func (pfr *ProtocolFingerprintRandomizer) GetCurrentTCPProfile() *TCPFingerprint {
	pfr.mu.RLock()
	defer pfr.mu.RUnlock()

	profile, exists := pfr.tcpProfiles[pfr.currentProfile]
	if !exists {
		return pfr.tcpProfiles["windows"] // 默认配置
	}
	return profile
}

// ApplyHTTPFingerprint 应用HTTP指纹到请求
func (pfr *ProtocolFingerprintRandomizer) ApplyHTTPFingerprint(req *http.Request) {
	profile := pfr.GetCurrentHTTPProfile()

	// 应用基本头部
	req.Header.Set("User-Agent", profile.UserAgent)
	req.Header.Set("Accept", profile.Accept)
	req.Header.Set("Accept-Language", profile.AcceptLanguage)
	req.Header.Set("Accept-Encoding", profile.AcceptEncoding)
	req.Header.Set("Connection", profile.Connection)

	if profile.UpgradeInsecure != "" {
		req.Header.Set("Upgrade-Insecure-Requests", profile.UpgradeInsecure)
	}

	if profile.CacheControl != "" {
		req.Header.Set("Cache-Control", profile.CacheControl)
	}

	// 应用自定义头部
	for key, value := range profile.CustomHeaders {
		req.Header.Set(key, value)
	}

	// 添加随机指纹混淆头部
	pfr.addRandomFingerprintHeaders(req)

	logx.Infof("🔐 [HTTP指纹] 已应用配置: %s\n", pfr.currentProfile)
}

// addRandomFingerprintHeaders 添加随机指纹混淆头部
func (pfr *ProtocolFingerprintRandomizer) addRandomFingerprintHeaders(req *http.Request) {
	// 随机添加一些指纹混淆头部
	randomHeaders := map[string][]string{
		"X-Forwarded-For":  {"127.0.0.1", "192.168.1.1", "10.0.0.1"},
		"X-Real-IP":        {"203.0.113.1", "198.51.100.1", "192.0.2.1"},
		"X-Originating-IP": {"172.16.0.1", "10.1.1.1", "192.168.0.1"},
		"X-Remote-IP":      {"203.0.113.2", "198.51.100.2"},
		"X-Client-IP":      {"192.168.1.100", "10.0.0.100"},
	}

	// 随机选择1-2个头部添加
	count := randomInt(1, 3)
	keys := make([]string, 0, len(randomHeaders))
	for k := range randomHeaders {
		keys = append(keys, k)
	}

	for i := 0; i < count && i < len(keys); i++ {
		key := keys[randomInt(0, len(keys))]
		values := randomHeaders[key]
		value := values[randomInt(0, len(values))]

		// 避免重复设置
		if req.Header.Get(key) == "" {
			req.Header.Set(key, value)
		}
	}

	// 添加随机的浏览器特征头部
	if randomInt(0, 3) == 0 {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}

	if randomInt(0, 4) == 0 {
		req.Header.Set("X-CSRF-Token", generateRandomString(32))
	}

	if randomInt(0, 5) == 0 {
		req.Header.Set("X-Request-ID", generateRandomString(16))
	}
}

// SwitchToProfile 切换到指定配置
func (pfr *ProtocolFingerprintRandomizer) SwitchToProfile(profileName string) error {
	pfr.mu.Lock()
	defer pfr.mu.Unlock()

	// 检查配置是否存在
	if _, exists := pfr.tlsConfigs[profileName]; !exists {
		return fmt.Errorf("协议指纹配置不存在: %s", profileName)
	}

	oldProfile := pfr.currentProfile
	pfr.currentProfile = profileName

	// 如果切换到随机配置，重新生成
	if profileName == "random" {
		pfr.tlsConfigs["random"] = pfr.generateRandomTLSConfig()
		pfr.httpProfiles["random"] = pfr.generateRandomHTTPProfile()
		pfr.tcpProfiles["random"] = pfr.generateRandomTCPProfile()
	}

	logx.Infof("🔐 [指纹切换] %s -> %s\n", oldProfile, profileName)
	return nil
}

// GetCurrentProfile 获取当前配置名称
func (pfr *ProtocolFingerprintRandomizer) GetCurrentProfile() string {
	pfr.mu.RLock()
	defer pfr.mu.RUnlock()
	return pfr.currentProfile
}

// GetAvailableProfiles 获取所有可用配置
func (pfr *ProtocolFingerprintRandomizer) GetAvailableProfiles() []string {
	pfr.mu.RLock()
	defer pfr.mu.RUnlock()

	profiles := make([]string, 0, len(pfr.tlsConfigs))
	for name := range pfr.tlsConfigs {
		profiles = append(profiles, name)
	}
	return profiles
}

// CreateCustomTLSConfig 创建自定义TLS配置
func (pfr *ProtocolFingerprintRandomizer) CreateCustomTLSConfig(profileName string, config *tls.Config) {
	pfr.mu.Lock()
	defer pfr.mu.Unlock()

	pfr.tlsConfigs[profileName] = config
	logx.Infof("🔐 [自定义配置] 已添加TLS配置: %s\n", profileName)
}

// CreateCustomHTTPProfile 创建自定义HTTP配置
func (pfr *ProtocolFingerprintRandomizer) CreateCustomHTTPProfile(profileName string, profile *HTTPFingerprint) {
	pfr.mu.Lock()
	defer pfr.mu.Unlock()

	pfr.httpProfiles[profileName] = profile
	logx.Infof("🔐 [自定义配置] 已添加HTTP配置: %s\n", profileName)
}

// CreateCustomTCPProfile 创建自定义TCP配置
func (pfr *ProtocolFingerprintRandomizer) CreateCustomTCPProfile(profileName string, profile *TCPFingerprint) {
	pfr.mu.Lock()
	defer pfr.mu.Unlock()

	pfr.tcpProfiles[profileName] = profile
	logx.Infof("🔐 [自定义配置] 已添加TCP配置: %s\n", profileName)
}

// StopRotation 停止自动轮换
func (pfr *ProtocolFingerprintRandomizer) StopRotation() {
	if pfr.rotationTicker != nil {
		pfr.rotationTicker.Stop()
		pfr.rotationTicker = nil
		logx.Println("🔄 [指纹轮换] 已停止")
	}
}

// ForceRotation 强制立即轮换
func (pfr *ProtocolFingerprintRandomizer) ForceRotation() {
	pfr.rotateToRandomProfile()
}

// GetFingerprintInfo 获取当前指纹信息
func (pfr *ProtocolFingerprintRandomizer) GetFingerprintInfo() map[string]interface{} {
	pfr.mu.RLock()
	defer pfr.mu.RUnlock()

	info := map[string]interface{}{
		"current_profile":    pfr.currentProfile,
		"available_profiles": pfr.GetAvailableProfiles(),
		"rotation_active":    pfr.rotationTicker != nil,
	}

	// 添加当前配置的详细信息
	if httpProfile := pfr.httpProfiles[pfr.currentProfile]; httpProfile != nil {
		info["http_user_agent"] = httpProfile.UserAgent
		info["http_version"] = httpProfile.HTTPVersion
	}

	if tlsConfig := pfr.tlsConfigs[pfr.currentProfile]; tlsConfig != nil {
		info["tls_min_version"] = tlsConfig.MinVersion
		info["tls_max_version"] = tlsConfig.MaxVersion
		info["tls_cipher_count"] = len(tlsConfig.CipherSuites)
	}

	return info
}
