package reconnect

import (
	"implant/core/config"
	"implant/core/debuglog"
	"math/rand"
	"sync"
	"time"
)

// ConnectionState 连接状态枚举
type ConnectionState int

const (
	STATE_DISCONNECTED   ConnectionState = iota // 断开连接
	STATE_CONNECTING                            // 正在连接
	STATE_AUTHENTICATING                        // 身份验证中（FirstBlood阶段）
	STATE_CONNECTED                             // 已连接
	STATE_RECONNECTING                          // 重连中
	STATE_FAILED                                // 连接失败
)

func (s ConnectionState) String() string {
	switch s {
	case STATE_DISCONNECTED:
		return "DISCONNECTED"
	case STATE_CONNECTING:
		return "CONNECTING"
	case STATE_AUTHENTICATING:
		return "AUTHENTICATING"
	case STATE_CONNECTED:
		return "CONNECTED"
	case STATE_RECONNECTING:
		return "RECONNECTING"
	case STATE_FAILED:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}

// ConnectionQuality 连接质量评估
type ConnectionQuality struct {
	avgLatency     time.Duration
	packetLoss     float64
	disconnectRate float64 // 每小时断线次数
	lastCheck      time.Time
	mu             sync.RWMutex
}

// CircuitState 熔断器状态
type CircuitState int

const (
	CIRCUIT_CLOSED    CircuitState = iota // 正常重连
	CIRCUIT_OPEN                          // 停止重连
	CIRCUIT_HALF_OPEN                     // 尝试恢复
)

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	maxFailures  int
	resetTimeout time.Duration
	failureCount int
	lastFailTime time.Time
	state        CircuitState
	mu           sync.RWMutex
}

// ReconnectManager 智能重连管理器
type ReconnectManager struct {
	retryCount       int
	consecutiveFails int
	successCount     int
	lastRetryTime    time.Time
	lastSuccessTime  time.Time
	state            ConnectionState
	stateChanged     time.Time
	quality          *ConnectionQuality
	circuitBreaker   *CircuitBreaker
	mu               sync.RWMutex
}

// NewReconnectManager 创建新的重连管理器
func NewReconnectManager() *ReconnectManager {
	return &ReconnectManager{
		state:        STATE_DISCONNECTED,
		stateChanged: time.Now(),
		quality: &ConnectionQuality{
			lastCheck: time.Now(),
		},
		circuitBreaker: &CircuitBreaker{
			maxFailures:  config.ReconnectConfig.MaxRetries,
			resetTimeout: config.ReconnectConfig.ResetTimeout,
			state:        CIRCUIT_CLOSED,
		},
	}
}

// GetNextDelay 计算下一次重连延时（智能退避算法）
func (rm *ReconnectManager) GetNextDelay() time.Duration {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	// 🔧 修复：限制最大重试次数防止指数溢出
	actualRetryCount := rm.retryCount
	if actualRetryCount > 30 { // 防止指数退避算法溢出
		actualRetryCount = 30
		debuglog.Printf("⚠️ [重连算法] 重试次数过高 (%d)，限制为30以防止计算溢出\n", rm.retryCount)
	}

	// 基础延时：指数退避
	baseDelay := config.ReconnectConfig.BaseDelay
	delay := baseDelay * time.Duration(1<<uint(actualRetryCount))

	// 限制最大延时
	if delay > config.ReconnectConfig.MaxDelay {
		delay = config.ReconnectConfig.MaxDelay
	}

	// 添加详细的调试日志
	debuglog.Printf("📊 [重连算法] retryCount=%d (实际=%d), baseDelay=%v, 指数延时=%v, 限制后=%v\n",
		rm.retryCount, actualRetryCount, baseDelay, baseDelay*time.Duration(1<<uint(actualRetryCount)), delay)

	// 根据连接质量调整延时
	if rm.quality.packetLoss > 0.05 || rm.quality.avgLatency > 2*time.Second {
		// 网络质量差，延长等待时间
		originalDelay := delay
		delay = delay * 2
		debuglog.Printf("🌐 [网络质量] 质量差(丢包:%.1f%%, 延时:%v)，延时调整: %v -> %v\n",
			rm.quality.packetLoss*100, rm.quality.avgLatency, originalDelay, delay)
	} else if rm.quality.disconnectRate > 10 {
		// 频繁断线，使用谨慎策略
		originalDelay := delay
		delay = delay * 3 / 2
		debuglog.Printf("⚡ [频繁断线] 断线率:%.1f/小时，谨慎延时: %v -> %v\n",
			rm.quality.disconnectRate, originalDelay, delay)
	}

	// 添加智能抖动（使用配置的抖动百分比）
	jitterPercent := config.ReconnectConfig.JitterPercent
	if jitterPercent > config.ReconnectConfig.MaxJitterPercent {
		jitterPercent = config.ReconnectConfig.MaxJitterPercent
	}

	if jitterPercent > 0 {
		maxJitter := delay * time.Duration(jitterPercent) / 100

		// 🔧 修复：防止随机数生成溢出导致panic
		jitterRange := int64(maxJitter * 2)
		if jitterRange <= 0 {
			// 如果计算结果<=0，使用固定的小抖动值
			jitterRange = int64(time.Second) // 1秒作为最大抖动
			debuglog.Printf("⚠️ [智能抖动] 抖动计算溢出，使用固定抖动: %v\n", time.Duration(jitterRange))
		}

		var jitter time.Duration
		if jitterRange > 0 {
			jitter = time.Duration(rand.Int63n(jitterRange)) - maxJitter
		} else {
			jitter = 0
		}

		originalDelay := delay
		delay += jitter
		debuglog.Printf("🎲 [智能抖动] %d%%抖动: %v -> %v (抖动:%v, 范围:%v)\n",
			jitterPercent, originalDelay, delay, jitter, time.Duration(jitterRange))
	}

	// 确保最小延时
	if delay < 100*time.Millisecond {
		delay = 100 * time.Millisecond
	}

	debuglog.Printf("⏰ [最终延时] %v (retryCount=%d)\n", delay, rm.retryCount)
	return delay
}

// CanAttemptReconnect 检查是否可以尝试重连（熔断器机制）
func (rm *ReconnectManager) CanAttemptReconnect() bool {
	rm.circuitBreaker.mu.RLock()
	defer rm.circuitBreaker.mu.RUnlock()

	switch rm.circuitBreaker.state {
	case CIRCUIT_CLOSED:
		return true
	case CIRCUIT_OPEN:
		// 检查是否超过重置时间
		if time.Since(rm.circuitBreaker.lastFailTime) > rm.circuitBreaker.resetTimeout {
			rm.circuitBreaker.mu.RUnlock()
			rm.circuitBreaker.mu.Lock()
			rm.circuitBreaker.state = CIRCUIT_HALF_OPEN
			rm.circuitBreaker.mu.Unlock()
			rm.circuitBreaker.mu.RLock()
			return true
		}
		return false
	case CIRCUIT_HALF_OPEN:
		return true
	}
	return false
}

// OnReconnectAttempt 重连尝试事件
func (rm *ReconnectManager) OnReconnectAttempt() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.lastRetryTime = time.Now()
	rm.setState(STATE_CONNECTING)
}

// OnReconnectFailed 重连失败事件
func (rm *ReconnectManager) OnReconnectFailed() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.retryCount++
	rm.consecutiveFails++
	rm.successCount = 0
	rm.setState(STATE_FAILED)

	// 🔄 优化：连续失败10次后重置指数退避算法，避免重连间隔过长
	const MAX_RETRY_BEFORE_RESET = 10
	if rm.consecutiveFails >= MAX_RETRY_BEFORE_RESET {
		debuglog.Printf("🔄 [重连管理] 连续失败%d次，重置指数退避算法 (retry:%d->0, fails:%d->0)\n",
			rm.consecutiveFails, rm.retryCount, rm.consecutiveFails)
		rm.retryCount = 0
		rm.consecutiveFails = 0
	}

	// 更新熔断器
	rm.circuitBreaker.mu.Lock()
	rm.circuitBreaker.failureCount++
	rm.circuitBreaker.lastFailTime = time.Now()

	// 检查是否需要开启熔断器
	if rm.circuitBreaker.failureCount >= rm.circuitBreaker.maxFailures {
		rm.circuitBreaker.state = CIRCUIT_OPEN
		debuglog.Printf("🚨 [重连管理] 熔断器开启：失败次数 %d，将在 %v 后重试\n",
			rm.circuitBreaker.failureCount, rm.circuitBreaker.resetTimeout)
	}
	rm.circuitBreaker.mu.Unlock()
}

// OnReconnectSuccess 重连成功事件
func (rm *ReconnectManager) OnReconnectSuccess() {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.successCount++
	rm.lastSuccessTime = time.Now()
	rm.setState(STATE_CONNECTED)

	// ✅ 修复：重连成功后立即重置指数退避算法
	// 原来需要连续成功多次才重置，导致测试场景中延迟累积
	if rm.retryCount > 0 || rm.consecutiveFails > 0 {
		debuglog.Printf("🔄 [重连管理] 重连成功，重置指数退避算法 (retry:%d->0, fails:%d->0)\n",
			rm.retryCount, rm.consecutiveFails)
		rm.retryCount = 0
		rm.consecutiveFails = 0
	}

	// 重置熔断器
	rm.circuitBreaker.mu.Lock()
	rm.circuitBreaker.failureCount = 0
	rm.circuitBreaker.state = CIRCUIT_CLOSED
	rm.circuitBreaker.mu.Unlock()
}

// OnAuthenticating 进入身份验证状态
func (rm *ReconnectManager) OnAuthenticating() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.setState(STATE_AUTHENTICATING)
}

// setState 设置连接状态（内部方法，需要已持有锁）
func (rm *ReconnectManager) setState(newState ConnectionState) {
	if rm.state != newState {
		debuglog.Printf("🔄 [连接状态] %s -> %s\n", rm.state.String(), newState.String())
		rm.state = newState
		rm.stateChanged = time.Now()
	}
}

// GetState 获取当前连接状态
func (rm *ReconnectManager) GetState() ConnectionState {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.state
}

// UpdateQuality 更新连接质量信息
func (rm *ReconnectManager) UpdateQuality(latency time.Duration, packetLoss float64) {
	rm.quality.mu.Lock()
	defer rm.quality.mu.Unlock()

	// 使用指数加权移动平均更新延时
	alpha := 0.3 // 平滑因子
	if rm.quality.avgLatency == 0 {
		rm.quality.avgLatency = latency
	} else {
		rm.quality.avgLatency = time.Duration(float64(rm.quality.avgLatency)*(1-alpha) + float64(latency)*alpha)
	}

	rm.quality.packetLoss = packetLoss
	rm.quality.lastCheck = time.Now()
}

// GetStats 获取重连统计信息
func (rm *ReconnectManager) GetStats() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rm.circuitBreaker.mu.RLock()
	circuitState := rm.circuitBreaker.state
	failureCount := rm.circuitBreaker.failureCount
	rm.circuitBreaker.mu.RUnlock()

	rm.quality.mu.RLock()
	avgLatency := rm.quality.avgLatency
	packetLoss := rm.quality.packetLoss
	rm.quality.mu.RUnlock()

	return map[string]interface{}{
		"state":             rm.state.String(),
		"retry_count":       rm.retryCount,
		"consecutive_fails": rm.consecutiveFails,
		"success_count":     rm.successCount,
		"circuit_state":     circuitState,
		"failure_count":     failureCount,
		"avg_latency_ms":    avgLatency.Milliseconds(),
		"packet_loss":       packetLoss,
		"last_success":      rm.lastSuccessTime,
		"last_retry":        rm.lastRetryTime,
	}
}
