package security

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

// TrafficObfuscator 流量混淆器
type TrafficObfuscator struct {
	patterns      []ObfuscationPattern
	domainFronts  []string
	userAgents    []string
	mu            sync.RWMutex
	mimicProfiles map[string]*MimicProfile
	// ===== 🔄 动态协议切换相关字段 =====
	currentProfile   string                   // 当前使用的协议配置
	switchRules      []ProtocolSwitchRule     // 协议切换规则
	detectionHistory []DetectionEvent         // 检测历史记录
	switchScheduler  *ProtocolSwitchScheduler // 协议切换调度器
	networkQuality   *NetworkQualityMonitor   // 网络质量监控
	failureThreshold int                      // 失败阈值
	maxRetryCount    int                      // 最大重试次数
}

// ObfuscationPattern 混淆模式
type ObfuscationPattern struct {
	Name        string
	ContentType string
	Headers     map[string]string
	Transform   func([]byte) []byte
}

// MimicProfile 模拟配置
type MimicProfile struct {
	Name        string
	UserAgent   string
	Headers     map[string]string
	URIPatterns []string
	Timing      TimingProfile
}

// TimingProfile 时序特征
type TimingProfile struct {
	MinDelay   time.Duration
	MaxDelay   time.Duration
	Jitter     float64
	BurstCount int
	BurstDelay time.Duration
}

// DomainFronting 域前置配置
type DomainFronting struct {
	FrontDomain string
	RealDomain  string
	SNI         string
	HostHeader  string
	TLSConfig   *tls.Config
}

// ===== 🔄 动态协议切换相关结构体 =====

// ProtocolSwitchRule 协议切换规则
type ProtocolSwitchRule struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Trigger       SwitchTrigger     `json:"trigger"`
	TargetProfile string            `json:"target_profile"`
	Priority      int               `json:"priority"`
	Enabled       bool              `json:"enabled"`
	Conditions    []SwitchCondition `json:"conditions"`
	Cooldown      time.Duration     `json:"cooldown"` // 冷却时间
	LastUsed      time.Time         `json:"last_used"`
}

// SwitchTrigger 切换触发器类型
type SwitchTrigger string

const (
	TriggerDetection      SwitchTrigger = "detection"       // 检测到威胁
	TriggerFailure        SwitchTrigger = "failure"         // 连接失败
	TriggerTrafficPattern SwitchTrigger = "traffic_pattern" // 流量模式异常
	TriggerScheduled      SwitchTrigger = "scheduled"       // 定时切换
	TriggerNetworkQuality SwitchTrigger = "network_quality" // 网络质量
	TriggerManual         SwitchTrigger = "manual"          // 手动切换
)

// SwitchCondition 切换条件
type SwitchCondition struct {
	Type      ConditionType `json:"type"`
	Operator  string        `json:"operator"` // ">", "<", "==", "!=", "contains"
	Value     interface{}   `json:"value"`
	Threshold float64       `json:"threshold"`
}

// ConditionType 条件类型
type ConditionType string

const (
	ConditionFailureRate    ConditionType = "failure_rate"
	ConditionResponseTime   ConditionType = "response_time"
	ConditionDetectionScore ConditionType = "detection_score"
	ConditionTimeOfDay      ConditionType = "time_of_day"
	ConditionNetworkLatency ConditionType = "network_latency"
	ConditionBandwidth      ConditionType = "bandwidth"
	ConditionSuccessRate    ConditionType = "success_rate"
)

// DetectionEvent 检测事件
type DetectionEvent struct {
	Timestamp      time.Time     `json:"timestamp"`
	Profile        string        `json:"profile"`
	DetectionType  string        `json:"detection_type"`
	Severity       int           `json:"severity"` // 1-10
	Description    string        `json:"description"`
	SourceIP       string        `json:"source_ip"`
	TargetEndpoint string        `json:"target_endpoint"`
	ResponseCode   int           `json:"response_code"`
	Latency        time.Duration `json:"latency"`
}

// ProtocolSwitchScheduler 协议切换调度器
type ProtocolSwitchScheduler struct {
	schedules  []SwitchSchedule
	isRunning  bool
	stopChan   chan bool
	obfuscator *TrafficObfuscator
	mu         sync.RWMutex
}

// SwitchSchedule 切换计划
type SwitchSchedule struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	CronExpr      string        `json:"cron_expr"`      // cron表达式
	TargetProfile string        `json:"target_profile"` // 目标协议
	Duration      time.Duration `json:"duration"`       // 持续时间
	Enabled       bool          `json:"enabled"`
	NextRun       time.Time     `json:"next_run"`
}

// NetworkQualityMonitor 网络质量监控器
type NetworkQualityMonitor struct {
	metrics        map[string]*NetworkMetrics
	isMonitoring   bool
	monitorChan    chan bool
	updateInterval time.Duration
	mu             sync.RWMutex
}

// NetworkMetrics 网络指标
type NetworkMetrics struct {
	Profile        string        `json:"profile"`
	Latency        time.Duration `json:"latency"`
	Bandwidth      float64       `json:"bandwidth"`    // MB/s
	SuccessRate    float64       `json:"success_rate"` // 0-1
	FailureRate    float64       `json:"failure_rate"` // 0-1
	ResponseTime   time.Duration `json:"response_time"`
	LastUpdate     time.Time     `json:"last_update"`
	SampleCount    int           `json:"sample_count"`
	TotalRequests  int           `json:"total_requests"`
	FailedRequests int           `json:"failed_requests"`
}

// SwitchDecision 切换决策
type SwitchDecision struct {
	ShouldSwitch   bool          `json:"should_switch"`
	TargetProfile  string        `json:"target_profile"`
	Reason         string        `json:"reason"`
	Confidence     float64       `json:"confidence"` // 0-1
	TriggerRule    string        `json:"trigger_rule"`
	EstimatedDelay time.Duration `json:"estimated_delay"`
}

// GlobalTrafficObfuscator 全局流量混淆器
var GlobalTrafficObfuscator *TrafficObfuscator

// InitTrafficObfuscator 初始化流量混淆器
func InitTrafficObfuscator() error {
	obfuscator := &TrafficObfuscator{
		patterns:      make([]ObfuscationPattern, 0),
		domainFronts:  make([]string, 0),
		userAgents:    getCommonUserAgents(),
		mimicProfiles: make(map[string]*MimicProfile),
		// ===== 🔄 动态协议切换初始化 =====
		currentProfile:   "chrome", // 默认使用Chrome配置
		switchRules:      make([]ProtocolSwitchRule, 0),
		detectionHistory: make([]DetectionEvent, 0),
		failureThreshold: 3, // 连续3次失败后切换
		maxRetryCount:    5, // 最大重试次数
	}

	// 初始化混淆模式
	obfuscator.initObfuscationPatterns()

	// 初始化模拟配置
	obfuscator.initMimicProfiles()

	// 初始化域前置配置
	obfuscator.initDomainFronting()

	// ===== 🔄 初始化动态协议切换功能 =====
	obfuscator.initProtocolSwitchRules()
	obfuscator.initNetworkQualityMonitor()
	obfuscator.initProtocolSwitchScheduler()

	// ===== 🔐 初始化协议指纹随机化 =====
	err := InitProtocolFingerprintRandomizer()
	if err != nil {
		logx.Warnf("⚠️ [协议指纹] 初始化失败: %v\n", err)
	}

	// ===== 🌍 初始化地理位置检测 =====
	err = InitGeoLocationDetector()
	if err != nil {
		logx.Warnf("⚠️ [地理位置] 初始化失败: %v\n", err)
	}

	// 启动网络质量监控
	go obfuscator.networkQuality.startMonitoring()

	// 启动协议切换调度器
	go obfuscator.switchScheduler.start()

	GlobalTrafficObfuscator = obfuscator
	logx.Println("🔄 [动态协议切换] 初始化完成，当前协议:", obfuscator.currentProfile)
	return nil
}

// initObfuscationPatterns 初始化混淆模式
func (to *TrafficObfuscator) initObfuscationPatterns() {
	// ===== 🌐 基础混淆模式 =====

	// HTTP API 模式
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "json_api",
		ContentType: "application/json",
		Headers: map[string]string{
			"Accept":           "application/json, text/plain, */*",
			"X-API-Key":        "{{RAND:32}}",
			"X-Client-Version": "2.1.0",
		},
		Transform: to.transformAsJSON,
	})

	// 图片上传模式
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "image_upload",
		ContentType: "multipart/form-data",
		Headers: map[string]string{
			"Accept": "image/webp,image/apng,image/*,*/*;q=0.8",
		},
		Transform: to.transformAsImage,
	})

	// Google Analytics 模式
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "google_analytics",
		ContentType: "text/plain",
		Headers: map[string]string{
			"Origin":  "https://www.google-analytics.com",
			"Referer": "https://www.google-analytics.com/",
		},
		Transform: to.transformAsAnalytics,
	})

	// CDN 静态资源模式
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "cdn_resource",
		ContentType: "application/javascript",
		Headers: map[string]string{
			"Accept":          "application/javascript, text/javascript, */*",
			"Accept-Encoding": "gzip, deflate, br",
			"Cache-Control":   "max-age=3600",
		},
		Transform: to.transformAsJS,
	})

	// ===== 🔍 高级流量特征混淆模式 =====

	// WebSocket 升级混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "websocket_upgrade",
		ContentType: "text/plain",
		Headers: map[string]string{
			"Upgrade":                "websocket",
			"Connection":             "Upgrade",
			"Sec-WebSocket-Key":      "{{WSKEY}}",
			"Sec-WebSocket-Version":  "13",
			"Sec-WebSocket-Protocol": "chat, superchat",
		},
		Transform: to.transformAsWebSocketHandshake,
	})

	// 视频流混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "video_streaming",
		ContentType: "video/mp4",
		Headers: map[string]string{
			"Accept":        "video/webm,video/ogg,video/*;q=0.9,application/ogg;q=0.7,audio/*;q=0.6,*/*;q=0.5",
			"Range":         "bytes=0-1023",
			"Content-Range": "bytes 0-1023/2048",
		},
		Transform: to.transformAsVideoChunk,
	})

	// DNS over HTTPS (DoH) 混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "dns_over_https",
		ContentType: "application/dns-message",
		Headers: map[string]string{
			"Accept":        "application/dns-message",
			"Cache-Control": "no-cache",
		},
		Transform: to.transformAsDNSQuery,
	})

	// 基于云存储的混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "cloud_storage",
		ContentType: "application/octet-stream",
		Headers: map[string]string{
			"X-Amz-Content-Sha256": "{{RAND:64}}",
			"X-Amz-Date":           time.Now().UTC().Format("20060102T150405Z"),
			"Authorization":        "AWS4-HMAC-SHA256 Credential={{RAND:20}}",
		},
		Transform: to.transformAsCloudStorage,
	})

	// 匿名 FTP 混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "ftp_anonymous",
		ContentType: "application/octet-stream",
		Headers: map[string]string{
			"Content-Disposition":       "attachment; filename=\"data.zip\"",
			"Content-Transfer-Encoding": "binary",
		},
		Transform: to.transformAsFTPData,
	})

	// MQTT 消息混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "mqtt_message",
		ContentType: "application/mqtt",
		Headers: map[string]string{
			"X-MQTT-Version": "3.1.1",
			"X-Client-ID":    "{{RAND:16}}",
		},
		Transform: to.transformAsMQTTMessage,
	})

	// 匿名网络流量混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "tor_like_traffic",
		ContentType: "application/x-tor-stream",
		Headers: map[string]string{
			"X-Tor-Version": "0.4.7.10",
			"X-Circuit-ID":  "{{RAND:8}}",
		},
		Transform: to.transformAsTorTraffic,
	})

	// P2P 文件分享混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "p2p_file_sharing",
		ContentType: "application/x-bittorrent",
		Headers: map[string]string{
			"X-Peer-ID":  "{{RAND:20}}",
			"X-InfoHash": "{{RAND:40}}",
			"User-Agent": "BitTorrent/7.10.5",
		},
		Transform: to.transformAsP2PTraffic,
	})

	// 游戏协议混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "game_protocol",
		ContentType: "application/x-game-data",
		Headers: map[string]string{
			"X-Game-Version":  "1.2.3",
			"X-Player-ID":     "{{RAND:12}}",
			"X-Session-Token": "{{RAND:32}}",
		},
		Transform: to.transformAsGameData,
	})

	// 健康检查混淆
	to.patterns = append(to.patterns, ObfuscationPattern{
		Name:        "health_check",
		ContentType: "text/plain",
		Headers: map[string]string{
			"User-Agent":     "HealthChecker/1.0",
			"X-Health-Check": "true",
		},
		Transform: to.transformAsHealthCheck,
	})
}

// initMimicProfiles 初始化模拟配置
func (to *TrafficObfuscator) initMimicProfiles() {
	// Chrome 浏览器配置
	to.mimicProfiles["chrome"] = &MimicProfile{
		Name:      "chrome",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Headers: map[string]string{
			"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
			"Accept-Language":           "en-US,en;q=0.5",
			"Accept-Encoding":           "gzip, deflate, br",
			"DNT":                       "1",
			"Connection":                "keep-alive",
			"Upgrade-Insecure-Requests": "1",
		},
		URIPatterns: []string{
			"/api/v1/search",
			"/static/js/main.js",
			"/images/logo.png",
			"/css/styles.css",
		},
		Timing: TimingProfile{
			MinDelay:   100 * time.Millisecond,
			MaxDelay:   2 * time.Second,
			Jitter:     0.3,
			BurstCount: 3,
			BurstDelay: 50 * time.Millisecond,
		},
	}

	// 移动端配置
	to.mimicProfiles["mobile"] = &MimicProfile{
		Name:      "mobile",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
		Headers: map[string]string{
			"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			"Accept-Language": "en-US,en;q=0.5",
			"Accept-Encoding": "gzip, deflate",
			"Connection":      "keep-alive",
		},
		URIPatterns: []string{
			"/m/api/data",
			"/mobile/app.js",
			"/touch/interface",
		},
		Timing: TimingProfile{
			MinDelay:   200 * time.Millisecond,
			MaxDelay:   5 * time.Second,
			Jitter:     0.5,
			BurstCount: 2,
			BurstDelay: 100 * time.Millisecond,
		},
	}

	// ===== 🌟 国际化API伪装配置 =====
	// ===== 🇨🇳 国内流行API (保留4个) =====

	// QQ API伪装配置
	to.mimicProfiles["qq"] = &MimicProfile{
		Name:      "qq",
		UserAgent: "Mozilla/5.0 (Linux; Android 12; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/103.0.0.0 Mobile Safari/537.36 QQ/8.9.28.10090 V1_AND_SQ_8.9.28_4018_YYB_D",
		Headers: map[string]string{
			"Accept":             "application/json, text/plain, */*",
			"Accept-Language":    "zh-CN,zh;q=0.9,en;q=0.8",
			"Accept-Encoding":    "gzip, deflate, br",
			"X-Requested-With":   "com.tencent.mobileqq",
			"Origin":             "https://qun.qq.com",
			"Referer":            "https://qun.qq.com/member.html",
			"sec-ch-ua":          "\"Chromium\";v=\"103\", \"QQ\";v=\"8\", \"Not)A;Brand\";v=\"24\"",
			"sec-ch-ua-mobile":   "?1",
			"sec-ch-ua-platform": "\"Android\"",
		},
		URIPatterns: []string{
			"/cgi-bin/qun_mgr/search_group_members",
			"/cgi-bin/group_digest_auth/group_digest_auth",
			"/cgi-bin/httpsqs/get_group_msg",
			"/cgi-bin/httpsqs/send_group_msg",
			"/cgi-bin/qun_mgr/get_group_list",
			"/cgi-bin/qun_mgr/get_group_info",
		},
		Timing: TimingProfile{
			MinDelay:   250 * time.Millisecond,
			MaxDelay:   4 * time.Second,
			Jitter:     0.6,
			BurstCount: 3,
			BurstDelay: 150 * time.Millisecond,
		},
	}

	// 抖音API伪装配置
	to.mimicProfiles["douyin"] = &MimicProfile{
		Name:      "douyin",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 aweme/28.8.0 JsSdk/1.0 NetType/WIFI Channel/App Store BytedanceWebview/d8a21c6",
		Headers: map[string]string{
			"Accept":              "application/json, text/plain, */*",
			"Accept-Language":     "zh-CN,zh;q=0.9",
			"Accept-Encoding":     "gzip, deflate, br",
			"X-Requested-With":    "com.ss.android.ugc.aweme",
			"Origin":              "https://www.douyin.com",
			"Referer":             "https://www.douyin.com/",
			"X-TT-TOKEN":          "{{RAND:32}}",
			"X-Secsdk-Csrf-Token": "{{RAND:40}}",
			"sec-ch-ua":           "\"Chromium\";v=\"116\", \"Not)A;Brand\";v=\"24\", \"Aweme\";v=\"28\"",
			"sec-ch-ua-mobile":    "?1",
			"sec-ch-ua-platform":  "\"iOS\"",
		},
		URIPatterns: []string{
			"/aweme/v1/web/aweme/feed/",
			"/aweme/v1/web/aweme/detail/",
			"/aweme/v1/web/comment/list/",
			"/aweme/v1/web/aweme/post/",
			"/aweme/v1/web/general/search/single/",
			"/aweme/v1/web/im/user/talk/",
		},
		Timing: TimingProfile{
			MinDelay:   200 * time.Millisecond,
			MaxDelay:   2 * time.Second,
			Jitter:     0.5,
			BurstCount: 4,
			BurstDelay: 100 * time.Millisecond,
		},
	}

	// 微博API伪装配置
	to.mimicProfiles["weibo"] = &MimicProfile{
		Name:      "weibo",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 Weibo (iPhone13,2__weibo__12.12.1__iphone__os17.1)",
		Headers: map[string]string{
			"Accept":             "application/json, text/plain, */*",
			"Accept-Language":    "zh-CN,zh;q=0.9",
			"Accept-Encoding":    "gzip, deflate, br",
			"X-Requested-With":   "com.sina.weibo",
			"Origin":             "https://m.weibo.cn",
			"Referer":            "https://m.weibo.cn/",
			"X-XSRF-TOKEN":       "{{RAND:32}}",
			"sec-ch-ua":          "\"Chromium\";v=\"116\", \"Not)A;Brand\";v=\"24\", \"Weibo\";v=\"12\"",
			"sec-ch-ua-mobile":   "?1",
			"sec-ch-ua-platform": "\"iOS\"",
		},
		URIPatterns: []string{
			"/api/container/getIndex",
			"/api/statuses/friends_timeline",
			"/api/comments/show",
			"/api/statuses/update",
			"/api/statuses/upload_pic",
			"/api/chat/send",
		},
		Timing: TimingProfile{
			MinDelay:   300 * time.Millisecond,
			MaxDelay:   3 * time.Second,
			Jitter:     0.4,
			BurstCount: 2,
			BurstDelay: 250 * time.Millisecond,
		},
	}

	// ===== 🇺🇸 国外流行API (新4个) =====

	// Google API伪装配置
	to.mimicProfiles["google"] = &MimicProfile{
		Name:      "google",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Headers: map[string]string{
			"Accept":             "application/json, text/plain, */*",
			"Accept-Language":    "en-US,en;q=0.9",
			"Accept-Encoding":    "gzip, deflate, br",
			"X-Requested-With":   "XMLHttpRequest",
			"Origin":             "https://accounts.google.com",
			"Referer":            "https://accounts.google.com/",
			"X-Client-Data":      "{{RAND:40}}",
			"X-Goog-AuthUser":    "0",
			"sec-ch-ua":          "\"Chromium\";v=\"120\", \"Not)A;Brand\";v=\"24\", \"Google Chrome\";v=\"120\"",
			"sec-ch-ua-platform": "\"Windows\"",
		},
		URIPatterns: []string{
			"/o/oauth2/v2/auth",
			"/api/accounts/ListAccounts",
			"/gmail/v1/users/me/messages",
			"/drive/v3/files",
			"/calendar/v3/calendars/primary/events",
			"/youtube/v3/search",
		},
		Timing: TimingProfile{
			MinDelay:   150 * time.Millisecond,
			MaxDelay:   2 * time.Second,
			Jitter:     0.3,
			BurstCount: 3,
			BurstDelay: 100 * time.Millisecond,
		},
	}

	// Facebook/Meta API伪装配置
	to.mimicProfiles["facebook"] = &MimicProfile{
		Name:      "facebook",
		UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1 FBAN/FBIOS;FBDV/iPhone13,2;FBMD/iPhone;FBSN/iOS;FBSV/17.1.1;FBSS/3;FBID/phone;FBLC/en_US;FBOP/5",
		Headers: map[string]string{
			"Accept":               "application/json, text/plain, */*",
			"Accept-Language":      "en-US,en;q=0.9",
			"Accept-Encoding":      "gzip, deflate, br",
			"X-FB-Connection-Type": "WIFI",
			"X-FB-Sim-HNI":         "{{RAND:6}}",
			"X-FB-Net-HNI":         "{{RAND:6}}",
			"Origin":               "https://m.facebook.com",
			"Referer":              "https://m.facebook.com/",
			"sec-ch-ua":            "\"Chromium\";v=\"120\", \"Not)A;Brand\";v=\"24\", \"Facebook\";v=\"450\"",
			"sec-ch-ua-mobile":     "?1",
		},
		URIPatterns: []string{
			"/api/graphql/",
			"/ajax/bz",
			"/api/graphqlbatch/",
			"/rsrc.php/v3/yn/l/0",
			"/ajax/webstorage/process_keys/",
			"/api/fb_messaging/attachment/upload",
		},
		Timing: TimingProfile{
			MinDelay:   200 * time.Millisecond,
			MaxDelay:   3 * time.Second,
			Jitter:     0.5,
			BurstCount: 2,
			BurstDelay: 180 * time.Millisecond,
		},
	}

	// Twitter/X API伪装配置
	to.mimicProfiles["twitter"] = &MimicProfile{
		Name:      "twitter",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Headers: map[string]string{
			"Accept":                    "*/*",
			"Accept-Language":           "en-US,en;q=0.9",
			"Accept-Encoding":           "gzip, deflate, br",
			"Authorization":             "Bearer {{RAND:110}}",
			"X-Twitter-Auth-Type":       "OAuth2Session",
			"X-Twitter-Client-Language": "en",
			"X-Csrf-Token":              "{{RAND:160}}",
			"Origin":                    "https://twitter.com",
			"Referer":                   "https://twitter.com/",
			"sec-ch-ua":                 "\"Chromium\";v=\"120\", \"Not)A;Brand\";v=\"24\", \"Google Chrome\";v=\"120\"",
			"sec-ch-ua-platform":        "\"Windows\"",
		},
		URIPatterns: []string{
			"/i/api/1.1/statuses/home_timeline.json",
			"/i/api/graphql/V7H0Ap3_Hh2FyS75OCDO3Q/UserTweets",
			"/i/api/2/timeline/conversation/",
			"/i/api/1.1/statuses/update.json",
			"/i/api/1.1/media/upload.json",
			"/i/api/graphql/favMd4_NTlRLIAPu6gQaw/CreateTweet",
		},
		Timing: TimingProfile{
			MinDelay:   180 * time.Millisecond,
			MaxDelay:   2500 * time.Millisecond,
			Jitter:     0.4,
			BurstCount: 3,
			BurstDelay: 120 * time.Millisecond,
		},
	}

	// Instagram API伪装配置
	to.mimicProfiles["instagram"] = &MimicProfile{
		Name:      "instagram",
		UserAgent: "Instagram 302.0.0.23.103 Android (33/13; 420dpi; 1080x2340; samsung; SM-G991B; o1s; exynos2100; en_US; 516766307)",
		Headers: map[string]string{
			"Accept":               "*/*",
			"Accept-Language":      "en-US,en;q=0.9",
			"Accept-Encoding":      "gzip, deflate",
			"X-IG-App-ID":          "936619743392459",
			"X-IG-Device-ID":       "{{RAND:36}}",
			"X-IG-Android-ID":      "android-{{RAND:16}}",
			"X-Bloks-Version-ID":   "5f56efad68e1edec7801f630b5c122704ec5378adbee6609a448f105f559a9a0",
			"X-FB-Connection-Type": "WIFI",
			"X-Requested-With":     "com.instagram.android",
			"sec-ch-ua":            "\"Chromium\";v=\"120\", \"Instagram\";v=\"302\", \"Not)A;Brand\";v=\"24\"",
			"sec-ch-ua-mobile":     "?1",
		},
		URIPatterns: []string{
			"/api/v1/feed/timeline/",
			"/api/v1/media/configure/",
			"/api/v1/upload/photo/",
			"/api/v1/direct_v2/inbox/",
			"/api/v1/accounts/current_user/",
			"/api/v1/discover/explore/",
		},
		Timing: TimingProfile{
			MinDelay:   250 * time.Millisecond,
			MaxDelay:   4 * time.Second,
			Jitter:     0.6,
			BurstCount: 2,
			BurstDelay: 200 * time.Millisecond,
		},
	}

	// 百度API伪装配置（增强原有配置）
	to.mimicProfiles["baidu"] = &MimicProfile{
		Name:      "baidu",
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		Headers: map[string]string{
			"Accept":           "application/json, text/javascript, */*; q=0.01",
			"Accept-Language":  "zh-CN,zh;q=0.9",
			"Accept-Encoding":  "gzip, deflate, br",
			"X-Requested-With": "XMLHttpRequest",
			"Origin":           "https://www.baidu.com",
			"Referer":          "https://www.baidu.com/",
			"Cookie":           "BAIDUID={{RAND:32}}:FG=1; BIDUPSID={{RAND:32}}",
		},
		URIPatterns: []string{
			"/sugrec",
			"/s",
			"/baidu",
			"/api/search",
			"/api/suggest",
			"/api/translate",
		},
		Timing: TimingProfile{
			MinDelay:   100 * time.Millisecond,
			MaxDelay:   2 * time.Second,
			Jitter:     0.3,
			BurstCount: 3,
			BurstDelay: 50 * time.Millisecond,
		},
	}
}

// initDomainFronting 初始化域前置
func (to *TrafficObfuscator) initDomainFronting() {
	// 常用CDN域名
	cdnDomains := []string{
		"ajax.googleapis.com",
		"cdn.jsdelivr.net",
		"unpkg.com",
		"cdnjs.cloudflare.com",
		"fonts.googleapis.com",
		"www.googletagmanager.com",
	}

	to.domainFronts = append(to.domainFronts, cdnDomains...)
}

// ObfuscateRequest 混淆HTTP请求
func (to *TrafficObfuscator) ObfuscateRequest(data []byte, profile string) (*http.Request, error) {
	return to.ObfuscateRequestWithClientIP(data, profile, "")
}

// ObfuscateRequestWithClientIP 基于客户端IP混淆HTTP请求
// 注意：不在整个函数上持读锁，避免与 EvaluateSwitchNeed/SwitchProtocol 的加锁形成
// 递归读锁（Go RWMutex 写者优先时会自死锁），以及手动 RUnlock/RLock 与 defer 叠加的 double-unlock。
func (to *TrafficObfuscator) ObfuscateRequestWithClientIP(data []byte, profile string, clientIP string) (*http.Request, error) {

	// 🌍 基于地理位置的智能协议选择
	if clientIP != "" && GlobalGeoDetector != nil {
		region := GlobalGeoDetector.DetectClientRegion(clientIP)
		recommendedProfiles := GlobalGeoDetector.GetRecommendedProfiles(region)

		// 如果没有指定协议或指定的协议不适合当前地区，选择推荐的协议
		if profile == "" || !contains(recommendedProfiles, profile) {
			if len(recommendedProfiles) > 0 {
				profile = recommendedProfiles[randomInt(0, len(recommendedProfiles))]
				logx.Infof("🌍 [智能选择] 客户端地区: %s, 选择协议: %s\n", region, profile)
			}
		}
	}

	// 🔄 动态协议切换逻辑
	// 如果没有指定协议，使用当前激活的协议
	if profile == "" {
		profile = to.GetCurrentProfile() // 通过加锁方法读取，避免裸读 currentProfile
	}

	// 检查是否需要切换协议（EvaluateSwitchNeed / SwitchProtocol 各自加锁，此处不持锁）
	decision := to.EvaluateSwitchNeed()
	if decision.ShouldSwitch && decision.Confidence > 0.7 {
		if err := to.SwitchProtocol(decision.TargetProfile, decision.Reason); err == nil {
			profile = decision.TargetProfile
			logx.Infof("🔄 [自动切换] 已切换到: %s (置信度: %.2f)\n", profile, decision.Confidence)
		}
	}

	// 选择混淆模式（短读锁保护 mimicProfiles map 读取）
	to.mu.RLock()
	pattern := to.selectPattern()
	mimicProfile := to.mimicProfiles[profile]
	to.mu.RUnlock()
	if mimicProfile == nil {
		mimicProfile = to.mimicProfiles["chrome"] // 默认使用Chrome（init-only，裸读安全）
		logx.Warnf("⚠️ [协议选择] 指定的协议 %s 不存在，使用默认Chrome\n", profile)
	}

	// 转换数据
	obfuscatedData := pattern.Transform(data)

	// 创建请求
	req, err := http.NewRequest("POST", to.generateURI(mimicProfile), bytes.NewReader(obfuscatedData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置头部
	req.Header.Set("User-Agent", mimicProfile.UserAgent)
	req.Header.Set("Content-Type", pattern.ContentType)

	for k, v := range materializeDynamicHeaders(pattern.Headers) {
		req.Header.Set(k, v)
	}

	for k, v := range materializeDynamicHeaders(mimicProfile.Headers) {
		req.Header.Set(k, v)
	}

	// 添加随机头部
	to.addRandomHeaders(req)

	// 记录使用统计（用于网络质量监控）
	startTime := time.Now()
	go func() {
		// 模拟请求执行时间（实际应该在请求完成后调用）
		time.Sleep(100 * time.Millisecond)
		to.UpdateNetworkMetrics(profile, true, time.Since(startTime), time.Since(startTime))
	}()

	return req, nil
}

// CreateFrontedRequest 创建域前置请求
func (to *TrafficObfuscator) CreateFrontedRequest(data []byte, targetHost string) (*http.Request, *tls.Config, error) {
	// 选择CDN域名
	frontDomain := to.selectRandomDomain()

	// 创建基础请求
	req, err := to.ObfuscateRequest(data, "chrome")
	if err != nil {
		return nil, nil, err
	}

	// 修改请求用于域前置
	req.URL.Host = frontDomain
	req.URL.Scheme = "https"
	req.Host = targetHost // 设置Host头为真实目标

	// 创建TLS配置
	tlsConfig := &tls.Config{
		ServerName:         frontDomain, // SNI使用前置域名
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS12,
	}

	return req, tlsConfig, nil
}

// MimicNormalTraffic 模拟正常流量
func (to *TrafficObfuscator) MimicNormalTraffic(data []byte) []byte {
	// 随机选择模拟类型
	mimicTypes := []string{"json", "form", "image", "script"}
	mimicType := mimicTypes[randomInt(0, len(mimicTypes))]

	switch mimicType {
	case "json":
		return to.mimicJSONTraffic(data)
	case "form":
		return to.mimicFormTraffic(data)
	case "image":
		return to.mimicImageTraffic(data)
	case "script":
		return to.mimicScriptTraffic(data)
	default:
		return to.mimicJSONTraffic(data)
	}
}

// 选择混淆模式
func (to *TrafficObfuscator) selectPattern() ObfuscationPattern {
	if len(to.patterns) == 0 {
		return ObfuscationPattern{
			Name:        "default",
			ContentType: "application/json",
			Transform:   to.transformAsJSON,
		}
	}
	return to.patterns[randomInt(0, len(to.patterns))]
}

// 生成随机URI
func (to *TrafficObfuscator) generateURI(profile *MimicProfile) string {
	if len(profile.URIPatterns) == 0 {
		return "/api/v1/data"
	}

	basePattern := profile.URIPatterns[randomInt(0, len(profile.URIPatterns))]

	// 添加随机查询参数
	params := url.Values{}
	params.Add("t", fmt.Sprintf("%d", time.Now().Unix()))
	params.Add("r", generateRandomString(8))
	params.Add("v", "1.0")

	return fmt.Sprintf("%s?%s", basePattern, params.Encode())
}

// 添加随机头部
func (to *TrafficObfuscator) addRandomHeaders(req *http.Request) {
	// 随机添加一些常见的头部
	commonHeaders := map[string][]string{
		"X-Forwarded-For": {"192.168.1.100", "10.0.0.1", "172.16.0.1"},
		"X-Real-IP":       {"203.0.113.1", "198.51.100.1", "192.0.2.1"},
		"X-Client-ID":     {generateRandomString(16)},
		"X-Session-ID":    {generateRandomString(32)},
		"X-Timestamp":     {fmt.Sprintf("%d", time.Now().Unix())},
	}

	// 随机选择1-3个头部添加
	count := randomInt(1, 4)
	keys := make([]string, 0, len(commonHeaders))
	for k := range commonHeaders {
		keys = append(keys, k)
	}

	for i := 0; i < count && i < len(keys); i++ {
		key := keys[randomInt(0, len(keys))]
		values := commonHeaders[key]
		value := values[randomInt(0, len(values))]
		req.Header.Set(key, value)
	}
}

// 选择随机域名
func (to *TrafficObfuscator) selectRandomDomain() string {
	if len(to.domainFronts) == 0 {
		return "ajax.googleapis.com"
	}
	return to.domainFronts[randomInt(0, len(to.domainFronts))]
}

// 数据转换函数
func (to *TrafficObfuscator) transformAsJSON(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	jsonData := fmt.Sprintf(`{"data":"%s","timestamp":%d,"version":"1.0"}`, encoded, time.Now().Unix())
	return []byte(jsonData)
}

func (to *TrafficObfuscator) transformAsImage(data []byte) []byte {
	// 创建伪造的multipart form data
	boundary := generateRandomString(16)
	encoded := base64.StdEncoding.EncodeToString(data)

	formData := fmt.Sprintf("--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"image.jpg\"\r\nContent-Type: image/jpeg\r\n\r\n%s\r\n--%s--\r\n",
		boundary, encoded, boundary)

	return []byte(formData)
}

func (to *TrafficObfuscator) transformAsAnalytics(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	// 模拟Google Analytics数据
	gaData := fmt.Sprintf("v=1&tid=UA-12345678-1&cid=%s&t=event&ec=custom&ea=data&el=%s",
		generateRandomString(16), url.QueryEscape(encoded))
	return []byte(gaData)
}

func (to *TrafficObfuscator) transformAsJS(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	// 伪装成JavaScript代码
	jsData := fmt.Sprintf("var config = {data: '%s', timestamp: %d}; window.appConfig = config;",
		encoded, time.Now().Unix())
	return []byte(jsData)
}

// 流量模拟函数
func (to *TrafficObfuscator) mimicJSONTraffic(data []byte) []byte {
	return to.transformAsJSON(data)
}

func (to *TrafficObfuscator) mimicFormTraffic(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	formData := url.Values{}
	formData.Add("data", encoded)
	formData.Add("action", "submit")
	formData.Add("token", generateRandomString(32))
	return []byte(formData.Encode())
}

func (to *TrafficObfuscator) mimicImageTraffic(data []byte) []byte {
	return to.transformAsImage(data)
}

func (to *TrafficObfuscator) mimicScriptTraffic(data []byte) []byte {
	return to.transformAsJS(data)
}

// 工具函数
func getCommonUserAgents() []string {
	return []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
	}
}

// ApplyTimingProfile 应用时序特征
func (to *TrafficObfuscator) ApplyTimingProfile(profile *MimicProfile) {
	if profile == nil || profile.Timing.MinDelay == 0 {
		return
	}

	// 计算延迟时间
	baseDelay := profile.Timing.MinDelay
	maxDelay := profile.Timing.MaxDelay
	if maxDelay < baseDelay {
		maxDelay = baseDelay * 2
	}

	// 添加抖动
	jitterRange := time.Duration(float64(maxDelay-baseDelay) * profile.Timing.Jitter)
	jitter := time.Duration(randomInt(0, int(jitterRange)))

	delay := baseDelay + jitter
	time.Sleep(delay)
}

// ===== 🔄 动态协议切换核心功能实现 =====

// initProtocolSwitchRules 初始化协议切换规则
func (to *TrafficObfuscator) initProtocolSwitchRules() {
	// 检测威胁时切换规则
	to.switchRules = append(to.switchRules, ProtocolSwitchRule{
		ID:            "detection_threat",
		Name:          "检测到威胁时切换",
		Trigger:       TriggerDetection,
		TargetProfile: "wechat", // 切换到微信伪装
		Priority:      10,
		Enabled:       true,
		Cooldown:      5 * time.Minute,
		Conditions: []SwitchCondition{
			{Type: ConditionDetectionScore, Operator: ">", Threshold: 7.0},
		},
	})

	// 连接失败时切换规则
	to.switchRules = append(to.switchRules, ProtocolSwitchRule{
		ID:            "connection_failure",
		Name:          "连接失败时切换",
		Trigger:       TriggerFailure,
		TargetProfile: "qq", // 切换到QQ伪装
		Priority:      8,
		Enabled:       true,
		Cooldown:      3 * time.Minute,
		Conditions: []SwitchCondition{
			{Type: ConditionFailureRate, Operator: ">", Threshold: 0.3},
		},
	})

	// 网络质量差时切换规则
	to.switchRules = append(to.switchRules, ProtocolSwitchRule{
		ID:            "poor_network",
		Name:          "网络质量差时切换",
		Trigger:       TriggerNetworkQuality,
		TargetProfile: "douyin", // 切换到抖音伪装
		Priority:      6,
		Enabled:       true,
		Cooldown:      10 * time.Minute,
		Conditions: []SwitchCondition{
			{Type: ConditionNetworkLatency, Operator: ">", Threshold: 1000}, // 1秒延迟
			{Type: ConditionSuccessRate, Operator: "<", Threshold: 0.8},
		},
	})

	// 定时切换规则 - 夜间切换到微博
	to.switchRules = append(to.switchRules, ProtocolSwitchRule{
		ID:            "night_schedule",
		Name:          "夜间定时切换",
		Trigger:       TriggerScheduled,
		TargetProfile: "weibo",
		Priority:      4,
		Enabled:       true,
		Cooldown:      2 * time.Hour,
		Conditions: []SwitchCondition{
			{Type: ConditionTimeOfDay, Operator: ">", Value: 22}, // 22点后
			{Type: ConditionTimeOfDay, Operator: "<", Value: 6},  // 6点前
		},
	})
}

// initNetworkQualityMonitor 初始化网络质量监控器
func (to *TrafficObfuscator) initNetworkQualityMonitor() {
	to.networkQuality = &NetworkQualityMonitor{
		metrics:        make(map[string]*NetworkMetrics),
		isMonitoring:   false,
		monitorChan:    make(chan bool),
		updateInterval: 30 * time.Second, // 30秒更新一次
	}

	// 为每个协议配置初始化网络指标
	for profileName := range to.mimicProfiles {
		to.networkQuality.metrics[profileName] = &NetworkMetrics{
			Profile:        profileName,
			Latency:        0,
			Bandwidth:      0,
			SuccessRate:    1.0,
			FailureRate:    0.0,
			ResponseTime:   0,
			LastUpdate:     time.Now(),
			SampleCount:    0,
			TotalRequests:  0,
			FailedRequests: 0,
		}
	}
}

// initProtocolSwitchScheduler 初始化协议切换调度器
func (to *TrafficObfuscator) initProtocolSwitchScheduler() {
	to.switchScheduler = &ProtocolSwitchScheduler{
		schedules:  make([]SwitchSchedule, 0),
		isRunning:  false,
		stopChan:   make(chan bool),
		obfuscator: to,
	}

	// 添加定时切换计划
	to.switchScheduler.schedules = append(to.switchScheduler.schedules, SwitchSchedule{
		ID:            "daily_morning",
		Name:          "每日早晨切换",
		CronExpr:      "0 8 * * *", // 每天8点
		TargetProfile: "chrome",
		Duration:      12 * time.Hour,
		Enabled:       true,
		NextRun:       time.Now().Add(24 * time.Hour),
	})

	to.switchScheduler.schedules = append(to.switchScheduler.schedules, SwitchSchedule{
		ID:            "daily_evening",
		Name:          "每日晚间切换",
		CronExpr:      "0 20 * * *", // 每天20点
		TargetProfile: "mobile",
		Duration:      12 * time.Hour,
		Enabled:       true,
		NextRun:       time.Now().Add(24 * time.Hour),
	})
}

// SwitchProtocol 切换协议
func (to *TrafficObfuscator) SwitchProtocol(targetProfile string, reason string) error {
	to.mu.Lock()
	defer to.mu.Unlock()

	// 检查目标协议是否存在
	if _, exists := to.mimicProfiles[targetProfile]; !exists {
		return fmt.Errorf("目标协议配置不存在: %s", targetProfile)
	}

	// 记录切换前的协议
	oldProfile := to.currentProfile

	// 执行协议切换
	to.currentProfile = targetProfile

	// 记录切换事件
	logx.Infof("🔄 [协议切换] %s -> %s, 原因: %s\n", oldProfile, targetProfile, reason)

	// 更新网络指标
	if metrics, exists := to.networkQuality.metrics[targetProfile]; exists {
		metrics.LastUpdate = time.Now()
	}

	return nil
}

// EvaluateSwitchNeed 评估是否需要切换协议
func (to *TrafficObfuscator) EvaluateSwitchNeed() *SwitchDecision {
	to.mu.RLock()
	defer to.mu.RUnlock()

	// 按优先级排序检查规则
	for _, rule := range to.switchRules {
		if !rule.Enabled {
			continue
		}

		// 检查冷却时间
		if time.Since(rule.LastUsed) < rule.Cooldown {
			continue
		}

		// 评估规则条件
		if to.evaluateRuleConditions(&rule) {
			return &SwitchDecision{
				ShouldSwitch:   true,
				TargetProfile:  rule.TargetProfile,
				Reason:         fmt.Sprintf("触发规则: %s", rule.Name),
				Confidence:     to.calculateConfidence(&rule),
				TriggerRule:    rule.ID,
				EstimatedDelay: 100 * time.Millisecond,
			}
		}
	}

	return &SwitchDecision{
		ShouldSwitch: false,
		Reason:       "无需切换",
	}
}

// evaluateRuleConditions 评估规则条件
func (to *TrafficObfuscator) evaluateRuleConditions(rule *ProtocolSwitchRule) bool {
	currentMetrics := to.networkQuality.metrics[to.currentProfile]
	if currentMetrics == nil {
		return false
	}

	// 检查所有条件
	for _, condition := range rule.Conditions {
		if !to.evaluateSingleCondition(condition, currentMetrics) {
			return false // 任一条件不满足则规则不触发
		}
	}

	return len(rule.Conditions) > 0 // 至少有一个条件且全部满足
}

// evaluateSingleCondition 评估单个条件
func (to *TrafficObfuscator) evaluateSingleCondition(condition SwitchCondition, metrics *NetworkMetrics) bool {
	var currentValue float64

	// 获取当前指标值
	switch condition.Type {
	case ConditionFailureRate:
		currentValue = metrics.FailureRate
	case ConditionResponseTime:
		currentValue = float64(metrics.ResponseTime.Milliseconds())
	case ConditionNetworkLatency:
		currentValue = float64(metrics.Latency.Milliseconds())
	case ConditionSuccessRate:
		currentValue = metrics.SuccessRate
	case ConditionTimeOfDay:
		currentValue = float64(time.Now().Hour())
	default:
		return false
	}

	// 根据操作符进行比较
	switch condition.Operator {
	case ">":
		return currentValue > condition.Threshold
	case "<":
		return currentValue < condition.Threshold
	case ">=":
		return currentValue >= condition.Threshold
	case "<=":
		return currentValue <= condition.Threshold
	case "==":
		return currentValue == condition.Threshold
	case "!=":
		return currentValue != condition.Threshold
	default:
		return false
	}
}

// calculateConfidence 计算切换置信度
func (to *TrafficObfuscator) calculateConfidence(rule *ProtocolSwitchRule) float64 {
	// 基础置信度基于规则优先级
	baseConfidence := float64(rule.Priority) / 10.0

	// 根据历史成功率调整
	currentMetrics := to.networkQuality.metrics[to.currentProfile]
	targetMetrics := to.networkQuality.metrics[rule.TargetProfile]

	if currentMetrics != nil && targetMetrics != nil {
		// 如果目标协议历史表现更好，增加置信度
		if targetMetrics.SuccessRate > currentMetrics.SuccessRate {
			baseConfidence += 0.2
		}
		if targetMetrics.ResponseTime < currentMetrics.ResponseTime {
			baseConfidence += 0.1
		}
	}

	// 限制在0-1范围内
	if baseConfidence > 1.0 {
		baseConfidence = 1.0
	}
	if baseConfidence < 0.0 {
		baseConfidence = 0.0
	}

	return baseConfidence
}

// UpdateNetworkMetrics 更新网络指标
func (to *TrafficObfuscator) UpdateNetworkMetrics(profile string, success bool, latency time.Duration, responseTime time.Duration) {
	to.mu.Lock()
	defer to.mu.Unlock()

	if metrics, exists := to.networkQuality.metrics[profile]; exists {
		metrics.TotalRequests++
		if !success {
			metrics.FailedRequests++
		}

		// 更新成功率和失败率
		metrics.SuccessRate = float64(metrics.TotalRequests-metrics.FailedRequests) / float64(metrics.TotalRequests)
		metrics.FailureRate = float64(metrics.FailedRequests) / float64(metrics.TotalRequests)

		// 更新延迟（使用移动平均）
		if metrics.SampleCount == 0 {
			metrics.Latency = latency
			metrics.ResponseTime = responseTime
		} else {
			// 简单移动平均
			metrics.Latency = (metrics.Latency + latency) / 2
			metrics.ResponseTime = (metrics.ResponseTime + responseTime) / 2
		}

		metrics.SampleCount++
		metrics.LastUpdate = time.Now()
	}
}

// RecordDetectionEvent 记录检测事件
func (to *TrafficObfuscator) RecordDetectionEvent(event DetectionEvent) {
	to.mu.Lock()
	defer to.mu.Unlock()

	event.Timestamp = time.Now()
	to.detectionHistory = append(to.detectionHistory, event)

	// 保持历史记录在合理范围内
	if len(to.detectionHistory) > 1000 {
		to.detectionHistory = to.detectionHistory[100:] // 保留最近900条
	}

	// 高严重度事件立即触发协议切换评估
	if event.Severity >= 8 {
		go func() {
			decision := to.EvaluateSwitchNeed()
			if decision.ShouldSwitch {
				err := to.SwitchProtocol(decision.TargetProfile, decision.Reason)
				if err != nil {
					logx.Errorf("❌ [协议切换] 切换失败: %v\n", err)
				}
			}
		}()
	}

	logx.Infof("🚨 [检测事件] %s - %s (严重度: %d)\n", event.DetectionType, event.Description, event.Severity)
}

// GetCurrentProfile 获取当前使用的协议配置
func (to *TrafficObfuscator) GetCurrentProfile() string {
	to.mu.RLock()
	defer to.mu.RUnlock()
	return to.currentProfile
}

// GetNetworkMetrics 获取网络指标
func (to *TrafficObfuscator) GetNetworkMetrics(profile string) *NetworkMetrics {
	to.mu.RLock()
	defer to.mu.RUnlock()
	return to.networkQuality.metrics[profile]
}

// GetAllProfiles 获取所有可用的协议配置
func (to *TrafficObfuscator) GetAllProfiles() []string {
	to.mu.RLock()
	defer to.mu.RUnlock()

	profiles := make([]string, 0, len(to.mimicProfiles))
	for name := range to.mimicProfiles {
		profiles = append(profiles, name)
	}
	return profiles
}

// ===== 网络质量监控器方法 =====

// startMonitoring 开始监控网络质量
func (nqm *NetworkQualityMonitor) startMonitoring() {
	nqm.mu.Lock()
	nqm.isMonitoring = true
	nqm.mu.Unlock()

	logx.Println("🔍 [网络监控] 开始监控网络质量")

	ticker := time.NewTicker(nqm.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-nqm.monitorChan:
			logx.Println("🔍 [网络监控] 停止监控")
			nqm.mu.Lock()
			nqm.isMonitoring = false
			nqm.mu.Unlock()
			return
		case <-ticker.C:
			// 定期检查网络质量并更新指标
			nqm.performQualityCheck()
		}
	}
}

// performQualityCheck 执行网络质量检查
func (nqm *NetworkQualityMonitor) performQualityCheck() {
	// 这里可以实现实际的网络质量检测逻辑
	// 目前只是更新时间戳
	nqm.mu.Lock()
	defer nqm.mu.Unlock()

	for _, metrics := range nqm.metrics {
		// 模拟网络质量波动
		if metrics.SampleCount > 0 {
			// 添加一些随机波动来模拟真实网络环境
			jitter := time.Duration(randomInt(-50, 50)) * time.Millisecond
			metrics.Latency += jitter
			if metrics.Latency < 0 {
				metrics.Latency = 10 * time.Millisecond
			}
		}
		metrics.LastUpdate = time.Now()
	}
}

// ===== 协议切换调度器方法 =====

// start 启动协议切换调度器
func (pss *ProtocolSwitchScheduler) start() {
	pss.mu.Lock()
	pss.isRunning = true
	pss.mu.Unlock()

	logx.Println("⏰ [切换调度] 启动协议切换调度器")

	ticker := time.NewTicker(1 * time.Minute) // 每分钟检查一次
	defer ticker.Stop()

	for {
		select {
		case <-pss.stopChan:
			logx.Println("⏰ [切换调度] 停止调度器")
			pss.mu.Lock()
			pss.isRunning = false
			pss.mu.Unlock()
			return
		case <-ticker.C:
			pss.checkSchedules()
		}
	}
}

// checkSchedules 检查调度计划
func (pss *ProtocolSwitchScheduler) checkSchedules() {
	pss.mu.RLock()
	defer pss.mu.RUnlock()

	now := time.Now()
	for i := range pss.schedules {
		schedule := &pss.schedules[i]
		if schedule.Enabled && now.After(schedule.NextRun) {
			// 执行切换
			err := pss.obfuscator.SwitchProtocol(schedule.TargetProfile, fmt.Sprintf("定时切换: %s", schedule.Name))
			if err != nil {
				logx.Errorf("❌ [定时切换] 失败: %v\n", err)
			} else {
				logx.Infof("⏰ [定时切换] 成功切换到: %s\n", schedule.TargetProfile)
			}

			// 计算下次运行时间（简化实现，实际应该解析cron表达式）
			schedule.NextRun = now.Add(24 * time.Hour) // 24小时后
		}
	}
}

// ===== 🔍 高级流量特征混淆转换函数 =====

// generateWebSocketKey 生成WebSocket密钥
func generateWebSocketKey() string {
	key := make([]byte, 16)
	rand.Read(key)
	return base64.StdEncoding.EncodeToString(key)
}

// transformAsWebSocketHandshake WebSocket握手混淆
func (to *TrafficObfuscator) transformAsWebSocketHandshake(data []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(data)
	// 模拟WebSocket升级请求的体内容
	handshakeData := fmt.Sprintf("GET /chat HTTP/1.1\r\nHost: example.com\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: chat\r\nOrigin: https://example.com\r\n\r\n%s",
		generateWebSocketKey(), encoded)
	return []byte(handshakeData)
}

// transformAsVideoChunk 视频块混淆
func (to *TrafficObfuscator) transformAsVideoChunk(data []byte) []byte {
	// 模拟视频数据块，在前面添加伪造的视频头部
	header := []byte{0x00, 0x00, 0x00, 0x1C, 0x66, 0x74, 0x79, 0x70} // MP4 头部的部分
	padding := make([]byte, randomInt(50, 200))                      // 随机填充
	rand.Read(padding)

	encoded := base64.StdEncoding.EncodeToString(data)
	result := append(header, padding...)
	result = append(result, []byte(encoded)...)
	return result
}

// transformAsDNSQuery DNS查询混淆
func (to *TrafficObfuscator) transformAsDNSQuery(data []byte) []byte {
	// 模拟DNS over HTTPS查询数据包
	header := []byte{0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00} // DNS头部
	domain := "example.com"                                                                  // 伪造域名
	queryData := fmt.Sprintf("%s%s", string(header), domain)

	encoded := base64.StdEncoding.EncodeToString(data)
	result := fmt.Sprintf("%s_%s", queryData, encoded)
	return []byte(result)
}

// transformAsCloudStorage 云存储混淆
func (to *TrafficObfuscator) transformAsCloudStorage(data []byte) []byte {
	// 模拟AWS S3或类似云存储的数据上传
	bucketName := "my-data-bucket-" + generateRandomString(8)
	objectKey := fmt.Sprintf("data/%d/%s.bin", time.Now().Unix(), generateRandomString(16))

	encoded := base64.StdEncoding.EncodeToString(data)
	s3Data := fmt.Sprintf(`{
		"bucket": "%s",
		"key": "%s",
		"data": "%s",
		"timestamp": %d,
		"checksum": "%s"
	}`, bucketName, objectKey, encoded, time.Now().Unix(), generateRandomString(32))

	return []byte(s3Data)
}

// transformAsFTPData FTP数据混淆
func (to *TrafficObfuscator) transformAsFTPData(data []byte) []byte {
	// 模拟FTP文件传输数据
	filename := fmt.Sprintf("document_%d.zip", time.Now().Unix())
	ftpHeader := fmt.Sprintf("STOR %s\r\n", filename)

	encoded := base64.StdEncoding.EncodeToString(data)
	result := ftpHeader + encoded + "\r\n226 Transfer complete\r\n"
	return []byte(result)
}

// transformAsMQTTMessage MQTT消息混淆
func (to *TrafficObfuscator) transformAsMQTTMessage(data []byte) []byte {
	// 模拟MQTT PUBLISH消息
	topic := "sensors/temperature/" + generateRandomString(8)
	mqttHeader := []byte{0x30, 0x00} // MQTT PUBLISH头部

	encoded := base64.StdEncoding.EncodeToString(data)
	mqttData := fmt.Sprintf(`{
		"topic": "%s",
		"payload": "%s",
		"qos": 1,
		"retain": false,
		"timestamp": %d
	}`, topic, encoded, time.Now().Unix())

	result := append(mqttHeader, []byte(mqttData)...)
	return result
}

// transformAsTorTraffic Tor流量混淆
func (to *TrafficObfuscator) transformAsTorTraffic(data []byte) []byte {
	// 模拟Tor网络流量（多层加密模拟）
	layer1 := base64.StdEncoding.EncodeToString(data)
	layer2 := base64.StdEncoding.EncodeToString([]byte(layer1))
	layer3 := base64.StdEncoding.EncodeToString([]byte(layer2))

	torFrame := fmt.Sprintf(`{
		"version": 3,
		"circuit_id": "%s",
		"stream_id": "%s",
		"command": "RELAY_DATA",
		"data": "%s",
		"digest": "%s"
	}`, generateRandomString(8), generateRandomString(8), layer3, generateRandomString(20))

	return []byte(torFrame)
}

// transformAsP2PTraffic P2P流量混淆
func (to *TrafficObfuscator) transformAsP2PTraffic(data []byte) []byte {
	// 模拟BitTorrent协议数据包
	peerID := generateRandomString(20)
	infoHash := generateRandomString(40)
	pieceIndex := randomInt(0, 1000)

	encoded := base64.StdEncoding.EncodeToString(data)
	p2pData := fmt.Sprintf(`{
		"msg_type": "piece",
		"peer_id": "%s",
		"info_hash": "%s",
		"piece_index": %d,
		"block_offset": %d,
		"block_data": "%s",
		"timestamp": %d
	}`, peerID, infoHash, pieceIndex, randomInt(0, 16384), encoded, time.Now().Unix())

	return []byte(p2pData)
}

// transformAsGameData 游戏数据混淆
func (to *TrafficObfuscator) transformAsGameData(data []byte) []byte {
	// 模拟网络游戏数据包
	playerID := generateRandomString(12)
	sessionID := generateRandomString(16)
	actionType := []string{"move", "attack", "chat", "item_use", "skill_cast"}[randomInt(0, 5)]

	encoded := base64.StdEncoding.EncodeToString(data)
	gamePacket := fmt.Sprintf(`{
		"packet_type": "game_action",
		"player_id": "%s",
		"session_id": "%s",
		"action": "%s",
		"sequence": %d,
		"payload": "%s",
		"checksum": "%s"
	}`, playerID, sessionID, actionType, randomInt(1000, 9999), encoded, generateRandomString(8))

	return []byte(gamePacket)
}

// transformAsHealthCheck 健康检查混淆
func (to *TrafficObfuscator) transformAsHealthCheck(data []byte) []byte {
	// 模拟系统健康检查数据
	status := []string{"healthy", "degraded", "checking"}[randomInt(0, 3)]
	serviceID := generateRandomString(10)

	encoded := base64.StdEncoding.EncodeToString(data)
	healthData := fmt.Sprintf(`{
		"status": "%s",
		"service_id": "%s",
		"timestamp": %d,
		"metrics": {
			"cpu_usage": %.2f,
			"memory_usage": %.2f,
			"response_time": %d
		},
		"details": "%s"
	}`, status, serviceID, time.Now().Unix(),
		float64(randomInt(10, 90)), float64(randomInt(20, 80)), randomInt(50, 500), encoded)

	return []byte(healthData)
}

// ===== 🔀 流量模式动态选择增强 =====

// selectPatternByProfile 根据协议配置选择适合的混淆模式
func (to *TrafficObfuscator) selectPatternByProfile(profileName string) ObfuscationPattern {
	// 根据不同的API伪装配置选择最适合的混淆模式
	profilePatterns := map[string][]string{
		"wechat":          {"json_api", "websocket_upgrade", "image_upload"},
		"qq":              {"json_api", "websocket_upgrade", "p2p_file_sharing"},
		"douyin":          {"video_streaming", "json_api", "image_upload"},
		"weibo":           {"json_api", "image_upload", "cdn_resource"},
		"dingtalk":        {"json_api", "cloud_storage", "health_check"},
		"tencent_meeting": {"websocket_upgrade", "video_streaming", "json_api"},
		"baidu":           {"json_api", "google_analytics", "cdn_resource"},
		"chrome":          {"json_api", "cdn_resource", "dns_over_https"},
		"mobile":          {"json_api", "image_upload", "game_protocol"},
	}

	if preferredPatterns, exists := profilePatterns[profileName]; exists {
		// 从首选模式中随机选择
		patternName := preferredPatterns[randomInt(0, len(preferredPatterns))]
		for _, pattern := range to.patterns {
			if pattern.Name == patternName {
				return pattern
			}
		}
	}

	// 如果没有找到首选模式，随机选择
	return to.selectPattern()
}

// AdaptiveObfuscateRequest 自适应混淆请求
func (to *TrafficObfuscator) AdaptiveObfuscateRequest(data []byte, profile string, trafficType string) (*http.Request, error) {
	to.mu.RLock()
	defer to.mu.RUnlock()

	// 根据流量类型和协议配置选择最佳混淆模式
	var pattern ObfuscationPattern
	switch trafficType {
	case "command":
		pattern = to.selectPatternByProfile(profile)
	case "file_transfer":
		// 文件传输优先使用云存储或FTP模式
		pattern = to.getPatternByName("cloud_storage")
		if pattern.Name == "" {
			pattern = to.getPatternByName("ftp_anonymous")
		}
	case "video":
		// 视频数据使用视频流模式
		pattern = to.getPatternByName("video_streaming")
	case "heartbeat":
		// 心跳数据使用健康检查模式
		pattern = to.getPatternByName("health_check")
	default:
		pattern = to.selectPatternByProfile(profile)
	}

	// 如果未找到模式，使用默认模式
	if pattern.Name == "" {
		pattern = to.selectPattern()
	}

	// 使用选定的模式创建请求
	return to.createObfuscatedRequest(data, profile, pattern)
}

// getPatternByName 根据名称获取混淆模式
func (to *TrafficObfuscator) getPatternByName(name string) ObfuscationPattern {
	for _, pattern := range to.patterns {
		if pattern.Name == name {
			return pattern
		}
	}
	return ObfuscationPattern{} // 返回空模式
}

// createObfuscatedRequest 创建混淆请求
func (to *TrafficObfuscator) createObfuscatedRequest(data []byte, profile string, pattern ObfuscationPattern) (*http.Request, error) {
	mimicProfile := to.mimicProfiles[profile]
	if mimicProfile == nil {
		mimicProfile = to.mimicProfiles["chrome"]
	}

	// 转换数据
	obfuscatedData := pattern.Transform(data)

	// 创建请求
	req, err := http.NewRequest("POST", to.generateURI(mimicProfile), bytes.NewReader(obfuscatedData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	// 设置头部
	req.Header.Set("User-Agent", mimicProfile.UserAgent)
	req.Header.Set("Content-Type", pattern.ContentType)

	for k, v := range materializeDynamicHeaders(pattern.Headers) {
		req.Header.Set(k, v)
	}

	for k, v := range materializeDynamicHeaders(mimicProfile.Headers) {
		req.Header.Set(k, v)
	}

	// 添加随机头部和流量特征混淆
	to.addRandomHeaders(req)
	to.addTrafficObfuscationHeaders(req, pattern.Name)

	// ===== 🔐 应用协议指纹随机化 =====
	if GlobalFingerprintRandomizer != nil {
		GlobalFingerprintRandomizer.ApplyHTTPFingerprint(req)
	}

	logx.Debugf("🔍 [流量混淆] 使用模式: %s, 协议: %s\n", pattern.Name, profile)

	return req, nil
}

// addTrafficObfuscationHeaders 添加流量混淆头部
func (to *TrafficObfuscator) addTrafficObfuscationHeaders(req *http.Request, patternName string) {
	// 添加与模式相关的特定头部
	switch patternName {
	case "websocket_upgrade":
		req.Header.Set("Sec-WebSocket-Extensions", "permessage-deflate; client_max_window_bits")
	case "video_streaming":
		req.Header.Set("Accept-Ranges", "bytes")
		req.Header.Set("Content-Encoding", "identity")
	case "dns_over_https":
		req.Header.Set("Accept", "application/dns-message")
		req.Header.Set("Content-Type", "application/dns-message")
	case "cloud_storage":
		req.Header.Set("X-Amz-Storage-Class", "STANDARD")
		req.Header.Set("X-Amz-Server-Side-Encryption", "AES256")
	case "game_protocol":
		req.Header.Set("X-Game-Server", "us-west-"+generateRandomString(3))
		req.Header.Set("X-Client-Platform", "PC")
	case "health_check":
		req.Header.Set("X-Health-Check-Version", "2.0")
		req.Header.Set("X-Monitor-ID", generateRandomString(16))
	}

	// 添加通用的反检测头部
	req.Header.Set("X-Request-ID", generateRandomString(32))
	req.Header.Set("X-Trace-ID", generateRandomString(16))
	req.Header.Set("X-Span-ID", generateRandomString(16))
}

// ===== 🌍 地理位置相关功能 =====

// contains 检查列表中是否包含指定元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// SwitchProfileByRegion 根据地区切换协议配置
func (to *TrafficObfuscator) SwitchProfileByRegion(clientIP string) error {
	if GlobalGeoDetector == nil {
		return fmt.Errorf("地理位置检测器未初始化")
	}

	region := GlobalGeoDetector.DetectClientRegion(clientIP)
	recommendedProfiles := GlobalGeoDetector.GetRecommendedProfiles(region)

	if len(recommendedProfiles) == 0 {
		return fmt.Errorf("无可用的推荐协议")
	}

	// 选择第一个推荐的协议
	targetProfile := recommendedProfiles[0]
	err := to.SwitchProtocol(targetProfile, fmt.Sprintf("地区自适应切换: %s", region))
	if err != nil {
		return err
	}

	logx.Infof("🌍 [地区切换] IP: %s, 地区: %s, 协议: %s\n", clientIP, region, targetProfile)
	return nil
}

// GetRegionalProfilesStats 获取各地区协议使用统计
func (to *TrafficObfuscator) GetRegionalProfilesStats() map[string]interface{} {
	stats := map[string]interface{}{
		"current_profile":    to.GetCurrentProfile(),
		"available_profiles": to.GetAllProfiles(),
	}

	if GlobalGeoDetector != nil {
		stats["geo_cache"] = GlobalGeoDetector.GetCacheStats()
		stats["recommended_profiles"] = map[string][]string{
			"china":         GlobalGeoDetector.GetRecommendedProfiles("china"),
			"international": GlobalGeoDetector.GetRecommendedProfiles("international"),
		}
	}

	return stats
}
