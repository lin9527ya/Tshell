package security

import (
	"hash/fnv"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"BackendTemplate/pkg/utils/logx"
	"github.com/gin-gonic/gin"
)

// 速率限制配置
const (
	DEFAULT_LOGIN_RATE_LIMIT = 5                // 每个IP每分钟最多5次登录尝试
	DEFAULT_API_RATE_LIMIT   = 100              // 每个IP每分钟最多100次API请求
	DEFAULT_CLEANUP_INTERVAL = 5 * time.Minute  // 清理间隔
	DEFAULT_BLOCK_DURATION   = 15 * time.Minute // 默认封禁时长
	DEFAULT_SLIDING_WINDOW   = 1 * time.Minute  // 滑动窗口大小

	rateLimiterShards       = 16   // 分段锁分片数
	maxLoginEntriesPerShard = 1024 // 每分片登录记录上限（防 map 无界增长的内存 DoS）
	maxAPIEntriesPerShard   = 4096 // 每分片 API 记录上限
)

// 访问记录
type AccessRecord struct {
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Blocked   bool      `json:"blocked"`
	BlockedAt time.Time `json:"blocked_at"`
}

// IP白名单管理器
type IPWhitelistManager struct {
	mu        sync.RWMutex
	whitelist map[string]bool
	networks  []*net.IPNet // 预解析的网络段，避免每请求重复 ParseCIDR
}

// 速率限制器：分段锁（按 IP 哈希分片），避免热路径全局锁竞争
type RateLimiter struct {
	shards         [rateLimiterShards]*rateLimiterShard
	ipWhitelist    *IPWhitelistManager
	cfgMu          sync.RWMutex // 保护下列配置字段（管理员低频修改）
	loginRateLimit int
	apiRateLimit   int
	blockDuration  time.Duration
	slidingWindow  time.Duration
	enabled        bool
	throttled      int64
	cleanupTicker  *time.Ticker
	stopCh         chan struct{}
}

// rateLimiterShard 单个分片：独立锁 + 独立 map（容量受限）
type rateLimiterShard struct {
	mu            sync.RWMutex
	loginAttempts map[string]*AccessRecord
	apiRequests   map[string]*AccessRecord
	maxLogin      int
	maxAPI        int
}

func newShard(maxLogin, maxAPI int) *rateLimiterShard {
	return &rateLimiterShard{
		loginAttempts: make(map[string]*AccessRecord),
		apiRequests:   make(map[string]*AccessRecord),
		maxLogin:      maxLogin,
		maxAPI:        maxAPI,
	}
}

// 创建新的速率限制器
func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		ipWhitelist:    NewIPWhitelistManager(),
		loginRateLimit: DEFAULT_LOGIN_RATE_LIMIT,
		apiRateLimit:   DEFAULT_API_RATE_LIMIT,
		blockDuration:  DEFAULT_BLOCK_DURATION,
		slidingWindow:  DEFAULT_SLIDING_WINDOW,
		enabled:        true,
		stopCh:         make(chan struct{}),
	}
	for i := range rl.shards {
		rl.shards[i] = newShard(maxLoginEntriesPerShard, maxAPIEntriesPerShard)
	}

	// 启动清理定时器
	rl.cleanupTicker = time.NewTicker(DEFAULT_CLEANUP_INTERVAL)
	go rl.cleanupExpiredRecords()

	// 初始化默认白名单
	rl.initDefaultWhitelist()

	return rl
}

// shardFor 按 IP 哈希选择分片
func (rl *RateLimiter) shardFor(ip string) *rateLimiterShard {
	h := fnv.New32a()
	h.Write([]byte(ip))
	return rl.shards[h.Sum32()%rateLimiterShards]
}

// 创建IP白名单管理器
func NewIPWhitelistManager() *IPWhitelistManager {
	return &IPWhitelistManager{
		whitelist: make(map[string]bool),
	}
}

// 初始化默认白名单
func (rl *RateLimiter) initDefaultWhitelist() {
	// 【安全修复】仅保留本地回环默认豁免。原实现将 192.168/16、10/8、172.16/12
	// 全量加入白名单并完全跳过登录/API 限流——同网段任意主机（含已沦陷的相邻机器）
	// 可无限爆破。私网豁免改为经配置 security.whitelist_ips 显式声明。
	rl.ipWhitelist.AddIP("127.0.0.1")
	rl.ipWhitelist.AddIP("::1")

	logx.Println("✅ 默认IP白名单初始化完成（仅回环；私网豁免请经配置 security.whitelist_ips 显式声明）")
}

// RebuildWhitelist 从配置重建白名单：回环 + 配置项（IP 或 CIDR 网段）。
// 由服务启动（main.go）与配置热更新（config/handlers.go applyConfigUpdates）调用。
func (rl *RateLimiter) RebuildWhitelist(entries []string) {
	wm := rl.ipWhitelist
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.whitelist = make(map[string]bool)
	wm.networks = nil
	wm.whitelist["127.0.0.1"] = true
	wm.whitelist["::1"] = true

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if strings.Contains(entry, "/") {
			_, ipnet, err := net.ParseCIDR(entry)
			if err != nil {
				logx.Errorf("❌ 无效的白名单网段: %s, 错误: %v\n", entry, err)
				continue
			}
			wm.whitelist[entry] = true
			wm.networks = append(wm.networks, ipnet)
		} else if ip := net.ParseIP(entry); ip != nil {
			wm.whitelist[entry] = true
		} else {
			logx.Errorf("❌ 无效的白名单IP: %s\n", entry)
		}
	}

	logx.Infof("✅ IP白名单已重建: %d 个条目\n", len(wm.whitelist))
}

// 添加IP到白名单
func (wm *IPWhitelistManager) AddIP(ip string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.whitelist[ip] = true
	logx.Infof("✅ IP %s 已添加到白名单\n", ip)
}

// 添加网络段到白名单（预解析为 *net.IPNet，运行时只做 Contains）
func (wm *IPWhitelistManager) AddNetwork(network string) {
	_, ipnet, err := net.ParseCIDR(network)
	if err != nil {
		logx.Errorf("❌ 无效的网络段: %s, 错误: %v\n", network, err)
		return
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.whitelist[network] = true
	wm.networks = append(wm.networks, ipnet)
	logx.Infof("✅ 网络段 %s 已添加到白名单\n", network)
}

// 检查IP是否在白名单中
func (wm *IPWhitelistManager) IsWhitelisted(ip string) bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	// 直接IP匹配
	if wm.whitelist[ip] {
		return true
	}

	// 网络段匹配（预解析缓存，无重复 ParseCIDR）
	clientIP := net.ParseIP(ip)
	if clientIP == nil {
		return false
	}
	for _, ipnet := range wm.networks {
		if ipnet.Contains(clientIP) {
			return true
		}
	}

	return false
}

// Count 白名单条目数（自身锁保护）
func (wm *IPWhitelistManager) Count() int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return len(wm.whitelist)
}

// 从白名单移除IP
func (wm *IPWhitelistManager) RemoveIP(ip string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	delete(wm.whitelist, ip)
	// CIDR 还要从预解析网络段里删掉，否则 delete(map) 只是删了展示项，
	// IsWhitelisted 仍会命中 networks，表现为“移除后依然放行”。
	if _, target, err := net.ParseCIDR(ip); err == nil {
		networks := wm.networks[:0]
		for _, n := range wm.networks {
			if n.String() != target.String() {
				networks = append(networks, n)
			}
		}
		wm.networks = networks
	}
	logx.Infof("🗑️ IP %s 已从白名单移除\n", ip)
}

// 获取客户端真实IP
// 安全：仅当直连对端为回环/内网（可信代理）时才信任 X-Forwarded-For 等转发头，
// 否则攻击者可伪造 XFF 绕过限流。公网直连一律用 RemoteAddr。
func getRealIP(c *gin.Context) string {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		host = c.Request.RemoteAddr
	}

	peerIP := net.ParseIP(host)
	if peerIP != nil && (peerIP.IsLoopback() || peerIP.IsPrivate()) {
		// 可信代理后，取转发头
		if ip := c.GetHeader("X-Forwarded-For"); ip != "" {
			// X-Forwarded-For 可能包含多个IP，取第一个
			parts := strings.Split(ip, ",")
			return strings.TrimSpace(parts[0])
		}
		if ip := c.GetHeader("X-Real-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
		if ip := c.GetHeader("X-Client-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
	}

	return host
}

// 检查登录速率限制
func (rl *RateLimiter) CheckLoginRateLimit(ip string) (bool, time.Duration) {
	// 白名单IP跳过限制
	if rl.ipWhitelist.IsWhitelisted(ip) {
		return true, 0
	}

	rl.cfgMu.RLock()
	loginRateLimit, slidingWindow, blockDuration := rl.loginRateLimit, rl.slidingWindow, rl.blockDuration
	rl.cfgMu.RUnlock()

	shard := rl.shardFor(ip)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now()
	record, exists := shard.loginAttempts[ip]

	if !exists {
		// 容量上限：超限先淘汰最旧记录，防无界增长
		if len(shard.loginAttempts) >= shard.maxLogin {
			evictOldest(shard.loginAttempts)
		}
		shard.loginAttempts[ip] = &AccessRecord{
			Count:     1,
			FirstSeen: now,
			LastSeen:  now,
			Blocked:   false,
		}
		return true, 0
	}

	// 检查是否被封禁
	if record.Blocked && now.Sub(record.BlockedAt) < blockDuration {
		remaining := blockDuration - now.Sub(record.BlockedAt)
		atomic.AddInt64(&rl.throttled, 1)
		return false, remaining
	}

	// 重置封禁状态
	if record.Blocked && now.Sub(record.BlockedAt) >= blockDuration {
		record.Blocked = false
		record.Count = 1
		record.FirstSeen = now
	}

	// 检查滑动窗口
	if now.Sub(record.FirstSeen) > slidingWindow {
		// 重置计数器
		record.Count = 1
		record.FirstSeen = now
	} else {
		record.Count++
	}

	record.LastSeen = now

	// 检查是否超过限制
	if record.Count > loginRateLimit {
		record.Blocked = true
		record.BlockedAt = now
		atomic.AddInt64(&rl.throttled, 1)
		logx.Infof("🚫 IP %s 因登录尝试过频被封禁 %v\n", ip, blockDuration)
		return false, blockDuration
	}

	return true, 0
}

// 检查API速率限制
func (rl *RateLimiter) CheckAPIRateLimit(ip string) (bool, time.Duration) {
	// 白名单IP跳过限制
	if rl.ipWhitelist.IsWhitelisted(ip) {
		return true, 0
	}

	rl.cfgMu.RLock()
	apiRateLimit, slidingWindow := rl.apiRateLimit, rl.slidingWindow
	rl.cfgMu.RUnlock()

	shard := rl.shardFor(ip)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now()
	record, exists := shard.apiRequests[ip]

	if !exists {
		// 容量上限
		if len(shard.apiRequests) >= shard.maxAPI {
			evictOldest(shard.apiRequests)
		}
		shard.apiRequests[ip] = &AccessRecord{
			Count:     1,
			FirstSeen: now,
			LastSeen:  now,
			Blocked:   false,
		}
		return true, 0
	}

	// 检查滑动窗口
	if now.Sub(record.FirstSeen) > slidingWindow {
		record.Count = 1
		record.FirstSeen = now
	} else {
		record.Count++
	}

	record.LastSeen = now

	// 检查是否超过限制
	if record.Count > apiRateLimit {
		remaining := slidingWindow - now.Sub(record.FirstSeen)
		if remaining <= 0 {
			remaining = slidingWindow
		}
		atomic.AddInt64(&rl.throttled, 1)
		return false, remaining
	}

	return true, 0
}

// UnblockIP 解除指定 IP 的封禁
func (rl *RateLimiter) UnblockIP(ip string) {
	shard := rl.shardFor(ip)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if record, exists := shard.loginAttempts[ip]; exists {
		record.Blocked = false
		record.Count = 0
		delete(shard.loginAttempts, ip)
	}
}

// GetBlockedIPs 返回被封禁的 IP 列表（含封禁详情）
func (rl *RateLimiter) GetBlockedIPs() []map[string]interface{} {
	blockedIPs := make([]map[string]interface{}, 0)
	for _, shard := range rl.shards {
		shard.mu.RLock()
		for ip, record := range shard.loginAttempts {
			if record.Blocked {
				blockedIPs = append(blockedIPs, map[string]interface{}{
					"ip":         ip,
					"blocked_at": record.BlockedAt.Format("2006-01-02 15:04:05"),
					"attempts":   record.Count,
					"first_seen": record.FirstSeen.Format("2006-01-02 15:04:05"),
					"last_seen":  record.LastSeen.Format("2006-01-02 15:04:05"),
				})
			}
		}
		shard.mu.RUnlock()
	}
	return blockedIPs
}

// UpdateConfig 更新限流配置（仅 >0 的字段生效）
func (rl *RateLimiter) UpdateConfig(loginRateLimit, apiRateLimit int, blockDuration time.Duration) {
	rl.UpdateConfigFull(loginRateLimit, apiRateLimit, blockDuration, 0)
}

// UpdateConfigFull 更新限流配置，含滑动窗口（仅 >0 的字段生效）。
func (rl *RateLimiter) UpdateConfigFull(loginRateLimit, apiRateLimit int, blockDuration, slidingWindow time.Duration) {
	rl.cfgMu.Lock()
	defer rl.cfgMu.Unlock()
	if loginRateLimit > 0 {
		rl.loginRateLimit = loginRateLimit
	}
	if apiRateLimit > 0 {
		rl.apiRateLimit = apiRateLimit
	}
	if blockDuration > 0 {
		rl.blockDuration = blockDuration
	}
	if slidingWindow > 0 {
		rl.slidingWindow = slidingWindow
	}
}

// SetEnabled 开启/关闭限流（config.security.enable_rate_limit）。
func (rl *RateLimiter) SetEnabled(enabled bool) {
	rl.cfgMu.Lock()
	rl.enabled = enabled
	rl.cfgMu.Unlock()
}

// IsEnabled 返回限流是否启用。
func (rl *RateLimiter) IsEnabled() bool {
	rl.cfgMu.RLock()
	defer rl.cfgMu.RUnlock()
	return rl.enabled
}

// GetConfigSnapshot 返回当前配置快照（cfgMu 保护）
func (rl *RateLimiter) GetConfigSnapshot() (loginRateLimit, apiRateLimit int, blockDuration time.Duration) {
	rl.cfgMu.RLock()
	defer rl.cfgMu.RUnlock()
	return rl.loginRateLimit, rl.apiRateLimit, rl.blockDuration
}

// GetRecentActiveIPs 返回 cutoff 之后活跃的 IP 列表（逐分片遍历）
func (rl *RateLimiter) GetRecentActiveIPs(cutoff time.Time) []map[string]interface{} {
	recentIPs := make([]map[string]interface{}, 0)
	for _, shard := range rl.shards {
		shard.mu.RLock()
		for ip, record := range shard.apiRequests {
			if record.LastSeen.After(cutoff) {
				recentIPs = append(recentIPs, map[string]interface{}{
					"ip":         ip,
					"requests":   record.Count,
					"last_seen":  record.LastSeen.Format("2006-01-02 15:04:05"),
					"first_seen": record.FirstSeen.Format("2006-01-02 15:04:05"),
				})
			}
		}
		shard.mu.RUnlock()
	}
	return recentIPs
}

// GetCurrentCount 返回指定 IP 的 API 请求计数
func (rl *RateLimiter) GetCurrentCount(ip string) int {
	shard := rl.shardFor(ip)
	shard.mu.RLock()
	defer shard.mu.RUnlock()
	if record, exists := shard.apiRequests[ip]; exists {
		return record.Count
	}
	return 0
}

// evictOldest 淘汰 map 中 LastSeen 最旧的记录（容量上限用）
func evictOldest(m map[string]*AccessRecord) {
	var oldestKey string
	var oldest time.Time
	first := true
	for k, rec := range m {
		if first || rec.LastSeen.Before(oldest) {
			oldestKey, oldest, first = k, rec.LastSeen, false
		}
	}
	if !first {
		delete(m, oldestKey)
	}
}

// 记录登录失败
func (rl *RateLimiter) RecordLoginFailure(ip string) {
	shard := rl.shardFor(ip)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	now := time.Now()
	if record, exists := shard.loginAttempts[ip]; exists {
		record.LastSeen = now
	}
}

// 清理过期记录（逐分片，stopCh 可退出）
func (rl *RateLimiter) cleanupExpiredRecords() {
	for {
		select {
		case <-rl.stopCh:
			return
		case <-rl.cleanupTicker.C:
			now := time.Now()
			for _, shard := range rl.shards {
				shard.mu.Lock()
				for ip, record := range shard.loginAttempts {
					if now.Sub(record.LastSeen) > 24*time.Hour {
						delete(shard.loginAttempts, ip)
					}
				}
				for ip, record := range shard.apiRequests {
					if now.Sub(record.LastSeen) > time.Hour {
						delete(shard.apiRequests, ip)
					}
				}
				shard.mu.Unlock()
			}
		}
	}
}

// 获取统计信息
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.cfgMu.RLock()
	loginRateLimit := rl.loginRateLimit
	apiRateLimit := rl.apiRateLimit
	blockDuration := rl.blockDuration
	enabled := rl.enabled
	rl.cfgMu.RUnlock()

	blockedIPs := make([]string, 0)
	loginTracked := 0
	apiTracked := 0

	for _, shard := range rl.shards {
		shard.mu.RLock()
		loginTracked += len(shard.loginAttempts)
		apiTracked += len(shard.apiRequests)
		for ip, record := range shard.loginAttempts {
			if record.Blocked {
				blockedIPs = append(blockedIPs, ip)
			}
		}
		shard.mu.RUnlock()
	}

	return map[string]interface{}{
		"login_attempts_tracked": loginTracked,
		"api_requests_tracked":   apiTracked,
		"blocked_ips":            blockedIPs,
		"whitelist_count":        rl.ipWhitelist.Count(),
		"login_rate_limit":       loginRateLimit,
		"api_rate_limit":         apiRateLimit,
		"block_duration":         blockDuration.String(),
		"enabled":                enabled,
		"throttled_requests":     atomic.LoadInt64(&rl.throttled),
	}
}

// 停止速率限制器（清理 goroutine 随 stopCh 退出）
func (rl *RateLimiter) Stop() {
	if rl.cleanupTicker != nil {
		rl.cleanupTicker.Stop()
	}
	select {
	case <-rl.stopCh:
	default:
		close(rl.stopCh)
	}
}

// 全局速率限制器实例
var GlobalRateLimiter *RateLimiter

// 初始化全局速率限制器
func init() {
	GlobalRateLimiter = NewRateLimiter()
	logx.Println("🚀 速率限制器初始化完成")
}
