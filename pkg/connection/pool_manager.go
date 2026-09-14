package connection

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

// 静态计数器，用于控制日志输出频率
// static_counter 健康检查计数（原子，防 HTTP 与 ticker 并发触发时数据竞争）
var static_counter int64
// 保护列表，防止健康检查期间误删正在初始化的连接
var protected_connections = make(map[string]time.Time)
var protected_mutex sync.RWMutex

// ConnectionPool 连接池配置
type ConnectionPoolConfig struct {
	MaxConnections    int           // 最大连接数
	MaxIdleTime       time.Duration // 最大空闲时间
	HealthCheckPeriod time.Duration // 健康检查周期
	ConnTimeout       time.Duration // 连接超时时间
	ReadTimeout       time.Duration // 读取超时时间
	WriteTimeout      time.Duration // 写入超时时间
}

// DefaultPoolConfig 默认连接池配置 - 隧道友好版本
var DefaultPoolConfig = &ConnectionPoolConfig{
	MaxConnections:    1000,
	MaxIdleTime:       60 * time.Minute, // 延长空闲时间
	HealthCheckPeriod: 5 * time.Minute,  // 健康检查间隔从30秒延长到5分钟
	ConnTimeout:       10 * time.Second,
	ReadTimeout:       10 * time.Minute, // 读取超时延长到10分钟
	WriteTimeout:      30 * time.Second,
}

// ConnectionInfo 连接信息
type ConnectionInfo struct {
	ID           string
	Conn         net.Conn
	LastActivity time.Time
	CreatedAt    time.Time
	ClientIP     string
	Protocol     string
	UID          string
	IsActive     bool
	BytesRead    int64
	BytesWritten int64
	ErrorCount   int
	mu           sync.RWMutex
}

// UpdateActivity 更新连接活动时间
func (ci *ConnectionInfo) UpdateActivity() {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.LastActivity = time.Now()
}

// IsIdle 检查连接是否空闲
func (ci *ConnectionInfo) IsIdle(maxIdleTime time.Duration) bool {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return time.Since(ci.LastActivity) > maxIdleTime
}

// AddError 增加错误计数
func (ci *ConnectionInfo) AddError() {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	ci.ErrorCount++
}

// GetErrorCount 读取错误计数（锁内）
func (ci *ConnectionInfo) GetErrorCount() int {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.ErrorCount
}

// GetStats 获取连接统计信息
func (ci *ConnectionInfo) GetStats() (time.Duration, int64, int64, int) {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	uptime := time.Since(ci.CreatedAt)
	// 字节计数用原子读（热路径写入为 atomic.AddInt64，避免持锁读放大）
	return uptime, atomic.LoadInt64(&ci.BytesRead), atomic.LoadInt64(&ci.BytesWritten), ci.ErrorCount
}

// EnhancedConnectionManager 增强的连接管理器
type EnhancedConnectionManager struct {
	config        *ConnectionPoolConfig
	configMu      sync.RWMutex // J2: 配置读写锁（UpdatePoolConfig 无锁改写 vs AddConnection/清理读竞争）
	connections   sync.Map // map[string]*ConnectionInfo
	uidIndex      sync.Map // map[string]string —— UID → 连接ID 二级索引（GetConnectionByUID O(1)，避免全表扫描）
	stats         *ConnectionStats
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.RWMutex
	healthCheckMu sync.Mutex // 串行化 performHealthCheck（HTTP handler 与 ticker 可并发触发）
}

// ConnectionStats 连接统计信息
type ConnectionStats struct {
	TotalConnections     int64
	ActiveConnections    int64
	IdleConnections      int64
	TotalBytesRead       int64
	TotalBytesWritten    int64
	TotalErrors          int64
	ConnectionsPerSecond float64
	LastUpdate           time.Time
	mu                   sync.RWMutex
}

// NewEnhancedConnectionManager 创建新的连接管理器
func NewEnhancedConnectionManager(config *ConnectionPoolConfig) *EnhancedConnectionManager {
	if config == nil {
		config = DefaultPoolConfig
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &EnhancedConnectionManager{
		config: config,
		stats: &ConnectionStats{
			LastUpdate: time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// 启动后台任务
	go manager.startHealthChecker()
	go manager.startStatsCollector()
	go manager.startIdleConnectionCleaner()

	return manager
}


// getConfig 快照读取配置（J2: 原子化配置访问）
func (m *EnhancedConnectionManager) getConfig() ConnectionPoolConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	return *m.config
}

// setConfig 更新配置（J2: 与读取方互斥）
func (m *EnhancedConnectionManager) setConfig(c ConnectionPoolConfig) {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	*m.config = c
}

// IsActiveSafe 锁内读取活跃状态（J2: 原无锁读与 RemoveConnection 写竞争）
func (ci *ConnectionInfo) IsActiveSafe() bool {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.IsActive
}

// UIDSafe 锁内读取 UID
func (ci *ConnectionInfo) UIDSafe() string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.UID
}

// ProtocolSafe 锁内读取协议
func (ci *ConnectionInfo) ProtocolSafe() string {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.Protocol
}

// LastActivitySafe 锁内读取最后活动时间
func (ci *ConnectionInfo) LastActivitySafe() time.Time {
	ci.mu.RLock()
	defer ci.mu.RUnlock()
	return ci.LastActivity
}
// AddConnection 添加连接到池中
func (manager *EnhancedConnectionManager) AddConnection(id, uid, protocol, clientIP string, conn net.Conn) error {
	// 检查连接数限制（【修复】计数与存储原子化：防止并发接入超 MaxConnections）
	cfg := manager.getConfig()
	manager.stats.mu.Lock()
	if manager.stats.ActiveConnections >= int64(cfg.MaxConnections) {
		manager.stats.mu.Unlock()
		// DNS 等无持久连接场景 conn 可能为 nil
		if conn != nil {
			conn.Close()
		}
		return fmt.Errorf("连接池已满，拒绝新连接")
	}
	manager.stats.ActiveConnections++
	manager.stats.TotalConnections++
	manager.stats.mu.Unlock()

	connInfo := &ConnectionInfo{
		ID:           id,
		Conn:         conn,
		LastActivity: time.Now(),
		CreatedAt:    time.Now(),
		ClientIP:     clientIP,
		Protocol:     protocol,
		UID:          uid,
		IsActive:     true,
	}

	// 设置连接超时
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(manager.getConfig().HealthCheckPeriod)
	}

	manager.connections.Store(id, connInfo)
	// P1-1: UID 二级索引（同一 UID 重连时覆盖为最新连接；旧连接移除时校验指向避免误删）
	manager.uidIndex.Store(uid, id)

	// 保护新连接不被健康检查误删（10秒保护期）
	protected_mutex.Lock()
	protected_connections[id] = time.Now().Add(10 * time.Second)
	protected_mutex.Unlock()

	logx.Infof("✅ 连接已添加到池: %s (UID: %s, 协议: %s, IP: %s) [保护期10秒]\n", id, uid, protocol, clientIP)
	return nil
}

// RemoveConnection 从池中移除连接
func (manager *EnhancedConnectionManager) RemoveConnection(id string) error {
	value, exists := manager.connections.LoadAndDelete(id)
	if !exists {
		return fmt.Errorf("连接不存在: %s", id)
	}

	connInfo := value.(*ConnectionInfo)
	// P1-1: 清理 uid 索引（仅当索引仍指向本连接时删除，避免误删重连后的新连接索引）
	connInfo.mu.RLock()
	uid := connInfo.UID
	connInfo.mu.RUnlock()
	if idxID, ok := manager.uidIndex.Load(uid); ok && idxID == id {
		manager.uidIndex.Delete(uid)
	}

	connInfo.mu.Lock()
	connInfo.IsActive = false
	if connInfo.Conn != nil {
		connInfo.Conn.Close()
	}
	connInfo.mu.Unlock()

	// 更新统计信息
	manager.stats.mu.Lock()
	manager.stats.ActiveConnections--
	manager.stats.mu.Unlock()

	logx.Infof("🗑️ 连接已从池中移除: %s\n", id)
	return nil
}

// GetConnection 获取连接
func (manager *EnhancedConnectionManager) GetConnection(id string) (*ConnectionInfo, bool) {
	value, exists := manager.connections.Load(id)
	if !exists {
		return nil, false
	}
	return value.(*ConnectionInfo), true
}

// GetConnectionByUID 根据UID获取连接（P1-1: uid 索引 O(1) 命中；索引失效时回退全表扫描保证行为兼容）
func (manager *EnhancedConnectionManager) GetConnectionByUID(uid string) (*ConnectionInfo, bool) {
	if id, ok := manager.uidIndex.Load(uid); ok {
		if connInfo, exists := manager.GetConnection(id.(string)); exists && connInfo.IsActiveSafe() {
			return connInfo, true
		}
	}

	// 回退路径：索引缺失/指向已失效连接（如旧连接移除时索引被覆盖前的竞态窗口）
	var result *ConnectionInfo
	found := false

	manager.connections.Range(func(key, value interface{}) bool {
		connInfo := value.(*ConnectionInfo)
		if connInfo.UIDSafe() == uid && connInfo.IsActiveSafe() {
			result = connInfo
			found = true
			return false // 停止迭代
		}
		return true
	})

	// 修复索引（若回退命中，说明索引遗漏，重建指向）
	if found {
		manager.uidIndex.Store(uid, result.ID)
	}
	return result, found
}

// GetAllConnections 获取所有连接
func (manager *EnhancedConnectionManager) GetAllConnections() map[string]*ConnectionInfo {
	result := make(map[string]*ConnectionInfo)

	manager.connections.Range(func(key, value interface{}) bool {
		id := key.(string)
		connInfo := value.(*ConnectionInfo)
		if connInfo.IsActiveSafe() {
			result[id] = connInfo
		}
		return true
	})

	return result
}

// GetActiveConnectionCount 获取活跃连接数
func (manager *EnhancedConnectionManager) GetActiveConnectionCount() int64 {
	manager.stats.mu.RLock()
	defer manager.stats.mu.RUnlock()
	return manager.stats.ActiveConnections
}

// GetConnectionsByProtocol 根据协议获取连接
func (manager *EnhancedConnectionManager) GetConnectionsByProtocol(protocol string) []*ConnectionInfo {
	var result []*ConnectionInfo

	manager.connections.Range(func(key, value interface{}) bool {
		connInfo := value.(*ConnectionInfo)
		if connInfo.ProtocolSafe() == protocol && connInfo.IsActiveSafe() {
			result = append(result, connInfo)
		}
		return true
	})

	return result
}

// UpdateConnectionActivity 更新连接活动时间（字节计数走原子累加，避免每消息全局锁竞争）
func (manager *EnhancedConnectionManager) UpdateConnectionActivity(id string, bytesRead, bytesWritten int64) {
	if connInfo, exists := manager.GetConnection(id); exists {
		connInfo.UpdateActivity()

		atomic.AddInt64(&connInfo.BytesRead, bytesRead)
		atomic.AddInt64(&connInfo.BytesWritten, bytesWritten)

		// 全局统计同样原子累加（原 stats.mu 每消息加锁，高并发热路径竞争点）
		atomic.AddInt64(&manager.stats.TotalBytesRead, bytesRead)
		atomic.AddInt64(&manager.stats.TotalBytesWritten, bytesWritten)
	}
}

// RecordConnectionError 记录连接错误 - 隧道友好版本
func (manager *EnhancedConnectionManager) RecordConnectionError(id string) {
	if connInfo, exists := manager.GetConnection(id); exists {
		connInfo.AddError()

		// 更新全局统计
		manager.stats.mu.Lock()
		manager.stats.TotalErrors++
		manager.stats.mu.Unlock()

		// 只有错误次数非常多时才断开连接（从10次提高到50次）
		errorCount := connInfo.GetErrorCount()
		if errorCount > 50 {
			logx.Warnf("⚠️ 连接错误次数过多，强制断开: %s (错误%d次)\n", id, errorCount)
			manager.RemoveConnection(id)
		}
	}
}

// 启动健康检查器
func (manager *EnhancedConnectionManager) startHealthChecker() {
	ticker := time.NewTicker(manager.getConfig().HealthCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-manager.ctx.Done():
			return
		case <-ticker.C:
			manager.performHealthCheck()
		}
	}
}

// 执行健康检查 - 隧道友好版本
// 可被 HTTP handler 与 ticker 并发触发：用 healthCheckMu 串行化，避免重复清理与计数竞争。
func (manager *EnhancedConnectionManager) performHealthCheck() {
	manager.healthCheckMu.Lock()
	defer manager.healthCheckMu.Unlock()

	var unhealthyConnections []string

	// 清理过期的保护连接
	protected_mutex.Lock()
	for id, expireTime := range protected_connections {
		if time.Now().After(expireTime) {
			delete(protected_connections, id)
		}
	}
	protected_mutex.Unlock()

	manager.connections.Range(func(key, value interface{}) bool {
		id := key.(string)
		connInfo := value.(*ConnectionInfo)

		if !connInfo.IsActiveSafe() {
			return true
		}

		// 检查是否在保护期内
		protected_mutex.RLock()
		_, isProtected := protected_connections[id]
		protected_mutex.RUnlock()
		
		if isProtected {
			// 跳过被保护的连接
			return true
		}

		// 检查连接是否仍然有效
		// 【死锁修复】锁内不得调用 LastActivitySafe()（其内部再次 RLock）。
		// RWMutex 在 writer 排队时会饿死后续读者：UpdateActivity 为热路径 writer，
		// 第二个 RLock 将永久阻塞且持锁不放，卡死健康检查与后续 RemoveConnection。
		connInfo.mu.RLock()
		conn := connInfo.Conn
		lastActivity := connInfo.LastActivity
		errorCount := connInfo.ErrorCount
		connInfo.mu.RUnlock()

		// 基本检查：连接对象是否存在
		if conn == nil {
			unhealthyConnections = append(unhealthyConnections, id)
			return true
		}

		// 隧道友好检查：只有在长时间无活动且错误过多时才认为不健康
		timeSinceLastActivity := time.Since(lastActivity)
		
		// 只有同时满足以下条件才认为连接不健康：
		// 1. 超过30分钟无活动 AND
		// 2. 错误次数超过20次
		if timeSinceLastActivity > 30*time.Minute && errorCount > 20 {
			logx.Infof("🏥 连接长时间无活动且错误过多，移除: %s (无活动%.1f分钟, 错误%d次)\n", 
				id, timeSinceLastActivity.Minutes(), errorCount)
			unhealthyConnections = append(unhealthyConnections, id)
		}

		return true
	})

	// 清理不健康的连接
	for _, id := range unhealthyConnections {
		manager.RemoveConnection(id)
	}

	// 输出当前连接池状态（每10次健康检查输出一次）
	if atomic.AddInt64(&static_counter, 1)%10 == 0 {
		activeCount := manager.GetActiveConnectionCount()
		protected_mutex.RLock()
		protectedCount := len(protected_connections)
		protected_mutex.RUnlock()
		logx.Infof("💓 连接池健康检查完成，当前活动连接: %d, 保护中: %d\n", activeCount, protectedCount)
	}
}

// 启动空闲连接清理器
func (manager *EnhancedConnectionManager) startIdleConnectionCleaner() {
	ticker := time.NewTicker(5 * time.Minute) // 每5分钟清理一次
	defer ticker.Stop()

	for {
		select {
		case <-manager.ctx.Done():
			return
		case <-ticker.C:
			manager.cleanIdleConnections()
		}
	}
}

// 清理空闲连接
func (manager *EnhancedConnectionManager) cleanIdleConnections() {
	var idleConnections []string

	manager.connections.Range(func(key, value interface{}) bool {
		id := key.(string)
		connInfo := value.(*ConnectionInfo)

		if connInfo.IsActiveSafe() && connInfo.IsIdle(manager.getConfig().MaxIdleTime) {
			idleConnections = append(idleConnections, id)
		}

		return true
	})

	for _, id := range idleConnections {
		logx.Infof("💤 清理空闲连接: %s\n", id)
		manager.RemoveConnection(id)
	}
}

// 启动统计信息收集器
func (manager *EnhancedConnectionManager) startStatsCollector() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	lastConnectionCount := int64(0)
	lastUpdateTime := time.Now()

	for {
		select {
		case <-manager.ctx.Done():
			return
		case <-ticker.C:
			manager.stats.mu.Lock()

			// 计算连接速率
			now := time.Now()
			timeDiff := now.Sub(lastUpdateTime).Seconds()
			connectionDiff := manager.stats.TotalConnections - lastConnectionCount

			if timeDiff > 0 {
				manager.stats.ConnectionsPerSecond = float64(connectionDiff) / timeDiff
			}

			manager.stats.LastUpdate = now
			lastConnectionCount = manager.stats.TotalConnections
			lastUpdateTime = now

			manager.stats.mu.Unlock()
		}
	}
}

// GetStats 获取统计信息
func (manager *EnhancedConnectionManager) GetStats() ConnectionStats {
	manager.stats.mu.RLock()
	defer manager.stats.mu.RUnlock()
	// 手动拷贝字段快照，避免拷贝含 mutex 的结构体
	return ConnectionStats{
		TotalConnections:     manager.stats.TotalConnections,
		ActiveConnections:    manager.stats.ActiveConnections,
		IdleConnections:      manager.stats.IdleConnections,
		TotalBytesRead:       atomic.LoadInt64(&manager.stats.TotalBytesRead),
		TotalBytesWritten:    atomic.LoadInt64(&manager.stats.TotalBytesWritten),
		TotalErrors:          manager.stats.TotalErrors,
		ConnectionsPerSecond: manager.stats.ConnectionsPerSecond,
		LastUpdate:           manager.stats.LastUpdate,
	}
}

// Close 关闭连接管理器
func (manager *EnhancedConnectionManager) Close() error {
	manager.cancel()

	// 关闭所有连接
	var allIDs []string
	manager.connections.Range(func(key, value interface{}) bool {
		allIDs = append(allIDs, key.(string))
		return true
	})

	for _, id := range allIDs {
		manager.RemoveConnection(id)
	}

	logx.Println("🔒 连接管理器已关闭")
	return nil
}

// SetConnectionTimeout 设置连接超时
func (manager *EnhancedConnectionManager) SetConnectionTimeout(conn net.Conn) error {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if err := tcpConn.SetReadDeadline(time.Now().Add(manager.config.ReadTimeout)); err != nil {
			return fmt.Errorf("设置读取超时失败: %v", err)
		}
		if err := tcpConn.SetWriteDeadline(time.Now().Add(manager.config.WriteTimeout)); err != nil {
			return fmt.Errorf("设置写入超时失败: %v", err)
		}
	}
	return nil
}

// 全局连接管理器实例
var GlobalConnectionManager *EnhancedConnectionManager

// InitializeConnectionManager 初始化全局连接管理器
func InitializeConnectionManager(config *ConnectionPoolConfig) error {
	// 重复初始化前先关闭旧实例（取消后台 goroutine：健康检查/空闲清理/统计收集），防泄漏
	if GlobalConnectionManager != nil {
		if err := GlobalConnectionManager.Close(); err != nil {
			logx.Warnf("⚠️ 关闭旧连接管理器失败: %v\n", err)
		}
	}
	GlobalConnectionManager = NewEnhancedConnectionManager(config)
	logx.Println("✅ 增强型连接管理器初始化成功")
	return nil
}
