package security

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

// ===== 🌍 地理位置检测模块 =====

// GeoLocationDetector 地理位置检测器
type GeoLocationDetector struct {
	cache           map[string]*LocationInfo
	cacheTTL        time.Duration
	cacheTimestamps map[string]time.Time
	providers       []GeoProvider
	timeout         time.Duration
	mu              sync.RWMutex
	fallbackRegion  string
}

// LocationInfo 位置信息
type LocationInfo struct {
	IP          string    `json:"ip"`
	Country     string    `json:"country"`
	CountryCode string    `json:"country_code"`
	Region      string    `json:"region"`
	City        string    `json:"city"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	ISP         string    `json:"isp"`
	IsChina     bool      `json:"is_china"`
	Timestamp   time.Time `json:"timestamp"`
}

// GeoProvider 地理位置服务提供商接口
type GeoProvider interface {
	Name() string
	Query(ip string) (*LocationInfo, error)
	IsAvailable() bool
}

// IPAPIProvider ip-api.com 提供商
type IPAPIProvider struct {
	BaseURL string
	Timeout time.Duration
}

// IPInfoProvider ipinfo.io 提供商
type IPInfoProvider struct {
	BaseURL string
	Token   string
	Timeout time.Duration
}

// IP2LocationProvider ip2location.com 提供商
type IP2LocationProvider struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// 全局地理位置检测器
var GlobalGeoDetector *GeoLocationDetector

// InitGeoLocationDetector 初始化地理位置检测器
func InitGeoLocationDetector() error {
	detector := &GeoLocationDetector{
		cache:           make(map[string]*LocationInfo),
		cacheTTL:        24 * time.Hour, // 缓存24小时
		cacheTimestamps: make(map[string]time.Time),
		timeout:         10 * time.Second,
		fallbackRegion:  "international", // 默认国际区域
	}

	// 初始化地理位置服务提供商
	detector.initProviders()

	GlobalGeoDetector = detector
	logx.Println("🌍 [地理位置] 检测器初始化完成")
	return nil
}

// initProviders 初始化地理位置服务提供商
func (gld *GeoLocationDetector) initProviders() {
	// 添加多个地理位置服务提供商，提高可靠性
	gld.providers = []GeoProvider{
		&IPAPIProvider{
			BaseURL: "http://ip-api.com/json/",
			Timeout: 8 * time.Second,
		},
		&IPInfoProvider{
			BaseURL: "https://ipinfo.io/",
			Token:   "", // 可选，无token也可使用基础功能
			Timeout: 8 * time.Second,
		},
		&IP2LocationProvider{
			BaseURL: "https://api.ip2location.com/v2/",
			APIKey:  "", // 可选，无API Key使用免费版本
			Timeout: 8 * time.Second,
		},
	}
}

// DetectLocation 检测IP地理位置
func (gld *GeoLocationDetector) DetectLocation(ip string) (*LocationInfo, error) {
	gld.mu.Lock()
	defer gld.mu.Unlock()

	// 检查缓存
	if info, exists := gld.cache[ip]; exists {
		if timestamp, ok := gld.cacheTimestamps[ip]; ok {
			if time.Since(timestamp) < gld.cacheTTL {
				logx.Infof("🎯 [地理位置] 缓存命中: %s -> %s\n", ip, info.Country)
				return info, nil
			}
		}
	}

	// 从提供商查询
	var lastErr error
	for _, provider := range gld.providers {
		if !provider.IsAvailable() {
			continue
		}

		info, err := provider.Query(ip)
		if err != nil {
			logx.Warnf("⚠️ [%s] 查询失败: %v\n", provider.Name(), err)
			lastErr = err
			continue
		}

		if info != nil {
			// 判断是否为中国IP
			info.IsChina = gld.isChinaIP(info)
			info.Timestamp = time.Now()

			// 更新缓存
			gld.cache[ip] = info
			gld.cacheTimestamps[ip] = time.Now()

			logx.Infof("🌍 [%s] 检测成功: %s -> %s (%s)\n",
				provider.Name(), ip, info.Country, gld.getRegionType(info))
			return info, nil
		}
	}

	// 所有提供商都失败，返回默认信息
	logx.Errorf("❌ [地理位置] 所有提供商查询失败: %v\n", lastErr)
	return gld.getFallbackLocation(ip), nil
}

// DetectClientRegion 检测客户端区域类型
func (gld *GeoLocationDetector) DetectClientRegion(clientIP string) string {
	location, err := gld.DetectLocation(clientIP)
	if err != nil || location == nil {
		logx.Warnf("⚠️ [区域检测] 检测失败，使用默认区域: %s\n", gld.fallbackRegion)
		return gld.fallbackRegion
	}

	if location.IsChina {
		return "china"
	}
	return "international"
}

// GetRecommendedProfiles 根据地理位置获取推荐的API配置
func (gld *GeoLocationDetector) GetRecommendedProfiles(region string) []string {
	switch region {
	case "china":
		return []string{"qq", "baidu", "douyin", "weibo"}
	case "international":
		return []string{"google", "facebook", "twitter", "instagram"}
	default:
		// 混合模式，国内外API都可用
		return []string{"google", "qq", "facebook", "baidu", "twitter", "douyin"}
	}
}

// isChinaIP 判断是否为中国IP
func (gld *GeoLocationDetector) isChinaIP(info *LocationInfo) bool {
	if info == nil {
		return false
	}

	// 通过国家代码判断
	countryCode := strings.ToUpper(info.CountryCode)
	if countryCode == "CN" {
		return true
	}

	// 通过国家名称判断
	country := strings.ToLower(info.Country)
	chinaNames := []string{
		"china", "中国", "中华人民共和国",
		"people's republic of china", "prc",
	}

	for _, name := range chinaNames {
		if strings.Contains(country, name) {
			return true
		}
	}

	return false
}

// getRegionType 获取区域类型描述
func (gld *GeoLocationDetector) getRegionType(info *LocationInfo) string {
	if info.IsChina {
		return "国内"
	}
	return "国外"
}

// getFallbackLocation 获取失败时的默认位置信息
func (gld *GeoLocationDetector) getFallbackLocation(ip string) *LocationInfo {
	return &LocationInfo{
		IP:          ip,
		Country:     "Unknown",
		CountryCode: "XX",
		Region:      gld.fallbackRegion,
		City:        "Unknown",
		ISP:         "Unknown",
		IsChina:     false, // 默认为国外
		Timestamp:   time.Now(),
	}
}

// ===== 地理位置服务提供商实现 =====

// IPAPIProvider 实现
func (p *IPAPIProvider) Name() string {
	return "IP-API"
}

func (p *IPAPIProvider) IsAvailable() bool {
	return true // ip-api.com 免费且稳定
}

func (p *IPAPIProvider) Query(ip string) (*LocationInfo, error) {
	client := &http.Client{Timeout: p.Timeout}

	url := fmt.Sprintf("%s%s?fields=status,country,countryCode,region,city,lat,lon,isp,query", p.BaseURL, ip)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var result struct {
		Status      string  `json:"status"`
		Country     string  `json:"country"`
		CountryCode string  `json:"countryCode"`
		Region      string  `json:"region"`
		City        string  `json:"city"`
		Lat         float64 `json:"lat"`
		Lon         float64 `json:"lon"`
		ISP         string  `json:"isp"`
		Query       string  `json:"query"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("查询失败: %s", result.Status)
	}

	return &LocationInfo{
		IP:          result.Query,
		Country:     result.Country,
		CountryCode: result.CountryCode,
		Region:      result.Region,
		City:        result.City,
		Latitude:    result.Lat,
		Longitude:   result.Lon,
		ISP:         result.ISP,
	}, nil
}

// IPInfoProvider 实现
func (p *IPInfoProvider) Name() string {
	return "IPInfo"
}

func (p *IPInfoProvider) IsAvailable() bool {
	return true // ipinfo.io 提供免费服务
}

func (p *IPInfoProvider) Query(ip string) (*LocationInfo, error) {
	client := &http.Client{Timeout: p.Timeout}

	url := fmt.Sprintf("%s%s/json", p.BaseURL, ip)
	if p.Token != "" {
		url += "?token=" + p.Token
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var result struct {
		IP       string `json:"ip"`
		Country  string `json:"country"`
		Region   string `json:"region"`
		City     string `json:"city"`
		Location string `json:"loc"` // "lat,lon" format
		Org      string `json:"org"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	// 解析经纬度
	var lat, lon float64
	if result.Location != "" {
		parts := strings.Split(result.Location, ",")
		if len(parts) == 2 {
			fmt.Sscanf(parts[0], "%f", &lat)
			fmt.Sscanf(parts[1], "%f", &lon)
		}
	}

	return &LocationInfo{
		IP:          result.IP,
		Country:     result.Country,
		CountryCode: result.Country, // IPInfo返回的是简写
		Region:      result.Region,
		City:        result.City,
		Latitude:    lat,
		Longitude:   lon,
		ISP:         result.Org,
	}, nil
}

// IP2LocationProvider 实现
func (p *IP2LocationProvider) Name() string {
	return "IP2Location"
}

func (p *IP2LocationProvider) IsAvailable() bool {
	return p.APIKey != "" // 需要API Key
}

func (p *IP2LocationProvider) Query(ip string) (*LocationInfo, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("需要API Key")
	}

	client := &http.Client{Timeout: p.Timeout}

	url := fmt.Sprintf("%s?ip=%s&key=%s&package=WS24", p.BaseURL, ip, p.APIKey)

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var result struct {
		CountryCode string  `json:"country_code"`
		CountryName string  `json:"country_name"`
		RegionName  string  `json:"region_name"`
		CityName    string  `json:"city_name"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		ISP         string  `json:"isp"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	return &LocationInfo{
		IP:          ip,
		Country:     result.CountryName,
		CountryCode: result.CountryCode,
		Region:      result.RegionName,
		City:        result.CityName,
		Latitude:    result.Latitude,
		Longitude:   result.Longitude,
		ISP:         result.ISP,
	}, nil
}

// ===== 工具函数 =====

// GetClientIP 从HTTP请求中获取客户端真实IP
func GetClientIP(r *http.Request) string {
	// 检查常见的代理头部
	headers := []string{
		"X-Forwarded-For",
		"X-Real-IP",
		"X-Client-IP",
		"CF-Connecting-IP", // Cloudflare
		"True-Client-IP",   // Akamai
	}

	for _, header := range headers {
		if ip := r.Header.Get(header); ip != "" {
			// X-Forwarded-For 可能包含多个IP，取第一个
			if strings.Contains(ip, ",") {
				ip = strings.TrimSpace(strings.Split(ip, ",")[0])
			}

			// 验证IP格式
			if net.ParseIP(ip) != nil && !isPrivateIP(ip) {
				return ip
			}
		}
	}

	// 从RemoteAddr获取
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// isPrivateIP 检查是否为私有IP
func isPrivateIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return true
	}

	// 检查是否为私有IP段
	privateBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
	}

	for _, block := range privateBlocks {
		_, cidr, _ := net.ParseCIDR(block)
		if cidr != nil && cidr.Contains(parsed) {
			return true
		}
	}

	return false
}

// ClearCache 清除地理位置缓存
func (gld *GeoLocationDetector) ClearCache() {
	gld.mu.Lock()
	defer gld.mu.Unlock()

	gld.cache = make(map[string]*LocationInfo)
	gld.cacheTimestamps = make(map[string]time.Time)
	logx.Println("🗑️ [地理位置] 缓存已清除")
}

// GetCacheStats 获取缓存统计信息
func (gld *GeoLocationDetector) GetCacheStats() map[string]interface{} {
	gld.mu.RLock()
	defer gld.mu.RUnlock()

	stats := map[string]interface{}{
		"cache_size": len(gld.cache),
		"cache_ttl":  gld.cacheTTL.String(),
		"providers":  len(gld.providers),
	}

	// 统计各地区数量
	regions := map[string]int{"china": 0, "international": 0}
	for _, info := range gld.cache {
		if info.IsChina {
			regions["china"]++
		} else {
			regions["international"]++
		}
	}
	stats["regions"] = regions

	return stats
}
