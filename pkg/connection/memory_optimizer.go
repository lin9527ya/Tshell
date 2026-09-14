package connection

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"github.com/shirou/gopsutil/v3/mem"
	"BackendTemplate/pkg/utils/logx"
)

// MemoryOptimizer 内存优化器
type MemoryOptimizer struct {
	objectPools      map[string]*sync.Pool
	memoryStats      *MemoryStats
	gcTriggerLevel   float64
	compressionLevel int
	bufferSizes      map[string]int
	mu               sync.RWMutex
	config           *MemoryConfig
	physicalMemory   uint64 // P1-2: 物理总内存缓存（UsagePercent 分母）
}

// MemoryConfig 内存配置
type MemoryConfig struct {
	MaxMemoryUsage     int64   `json:"max_memory_usage"`    // 最大内存使用量（字节）
	GCTriggerPercent   float64 `json:"gc_trigger_percent"`  // 触发GC的内存使用百分比
	CompressionEnabled bool    `json:"compression_enabled"` // 是否启用压缩
	CompressionLevel   int     `json:"compression_level"`   // 压缩级别
	ObjectPoolEnabled  bool    `json:"object_pool_enabled"` // 是否启用对象池
	BufferPoolEnabled  bool    `json:"buffer_pool_enabled"` // 是否启用缓冲池
	MonitoringEnabled  bool    `json:"monitoring_enabled"`  // 是否启用内存监控
}

// MemoryStats 内存统计
type MemoryStats struct {
	AllocatedMemory  uint64    `json:"allocated_memory"`
	TotalMemory      uint64    `json:"total_memory"`
	UsagePercent     float64   `json:"usage_percent"`
	GCCount          uint32    `json:"gc_count"`
	LastGCTime       time.Time `json:"last_gc_time"`
	ObjectPoolHits   int64     `json:"object_pool_hits"`
	ObjectPoolMisses int64     `json:"object_pool_misses"`
	CompressionRatio float64   `json:"compression_ratio"`
	LastUpdate       time.Time `json:"last_update"`
	mu               sync.RWMutex
}

// BufferPool 缓冲池
type BufferPool struct {
	small  sync.Pool // 小缓冲区 (< 1KB)
	medium sync.Pool // 中缓冲区 (1KB - 64KB)
	large  sync.Pool // 大缓冲区 (> 64KB)
}

// CompressedData 压缩数据
type CompressedData struct {
	Data             []byte
	OriginalSize     int
	CompressedSize   int
	CompressionRatio float64
}

// GlobalMemoryOptimizer 全局内存优化器
var GlobalMemoryOptimizer *MemoryOptimizer

// InitMemoryOptimizer 初始化内存优化器
func InitMemoryOptimizer(config *MemoryConfig) error {
	if config == nil {
		config = getDefaultMemoryConfig()
	}

	optimizer := &MemoryOptimizer{
		objectPools:      make(map[string]*sync.Pool),
		memoryStats:      &MemoryStats{},
		gcTriggerLevel:   config.GCTriggerPercent,
		compressionLevel: config.CompressionLevel,
		config:           config,
		bufferSizes: map[string]int{
			"small":  1024,    // 1KB
			"medium": 65536,   // 64KB
			"large":  1048576, // 1MB
		},
	}

	// 初始化对象池
	if config.ObjectPoolEnabled {
		optimizer.initObjectPools()
	}

	// 启动内存监控
	if config.MonitoringEnabled {
		go optimizer.startMemoryMonitor()
	}

	// 设置GC参数
	optimizer.optimizeGCSettings()

	GlobalMemoryOptimizer = optimizer
	logx.Println("✅ 内存优化器初始化成功")
	return nil
}

// getDefaultMemoryConfig 获取默认内存配置
func getDefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		MaxMemoryUsage:     1024 * 1024 * 1024, // 1GB
		GCTriggerPercent:   80.0,
		CompressionEnabled: true,
		CompressionLevel:   6, // gzip默认级别
		ObjectPoolEnabled:  true,
		BufferPoolEnabled:  true,
		MonitoringEnabled:  true,
	}
}

// initObjectPools 初始化对象池
func (mo *MemoryOptimizer) initObjectPools() {
	// 字节缓冲池
	mo.objectPools["bytes"] = &sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 1024)
		},
	}

	// 字符串构建器池
	mo.objectPools["strings"] = &sync.Pool{
		New: func() interface{} {
			return &strings.Builder{}
		},
	}

	// 连接信息池
	mo.objectPools["connectionInfo"] = &sync.Pool{
		New: func() interface{} {
			return &ConnectionInfo{}
		},
	}

	// 缓冲区池
	mo.objectPools["bufferPool"] = &sync.Pool{
		New: func() interface{} {
			return &BufferPool{
				small: sync.Pool{
					New: func() interface{} {
						return make([]byte, mo.bufferSizes["small"])
					},
				},
				medium: sync.Pool{
					New: func() interface{} {
						return make([]byte, mo.bufferSizes["medium"])
					},
				},
				large: sync.Pool{
					New: func() interface{} {
						return make([]byte, mo.bufferSizes["large"])
					},
				},
			}
		},
	}

	logx.Println("📦 对象池初始化完成")
}

// GetBuffer 从缓冲池获取缓冲区
func (mo *MemoryOptimizer) GetBuffer(size int) []byte {
	if !mo.config.BufferPoolEnabled {
		return make([]byte, size)
	}

	var pool *sync.Pool
	// var targetSize int // 暂时不使用

	switch {
	case size <= mo.bufferSizes["small"]:
		if bufferPool, ok := mo.objectPools["bufferPool"]; ok {
			bp := bufferPool.Get().(*BufferPool)
			defer bufferPool.Put(bp)
			pool = &bp.small
			// targetSize = mo.bufferSizes["small"]
		}
	case size <= mo.bufferSizes["medium"]:
		if bufferPool, ok := mo.objectPools["bufferPool"]; ok {
			bp := bufferPool.Get().(*BufferPool)
			defer bufferPool.Put(bp)
			pool = &bp.medium
			// targetSize = mo.bufferSizes["medium"]
		}
	default:
		if bufferPool, ok := mo.objectPools["bufferPool"]; ok {
			bp := bufferPool.Get().(*BufferPool)
			defer bufferPool.Put(bp)
			pool = &bp.large
			// targetSize = mo.bufferSizes["large"]
		}
	}

	if pool != nil {
		// 【修复】按 cap 判定命中：池中存放的是未截断长度的切片
		if buf := pool.Get().([]byte); cap(buf) >= size {
			mo.memoryStats.mu.Lock()
			mo.memoryStats.ObjectPoolHits++
			mo.memoryStats.mu.Unlock()
			return buf[:size]
		}
	}

	// 如果池中没有合适的缓冲区，创建新的
	mo.memoryStats.mu.Lock()
	mo.memoryStats.ObjectPoolMisses++
	mo.memoryStats.mu.Unlock()

	return make([]byte, size)
}

// PutBuffer 将缓冲区返回到池中
func (mo *MemoryOptimizer) PutBuffer(buf []byte) {
	if !mo.config.BufferPoolEnabled {
		return
	}

	size := cap(buf)
	var pool *sync.Pool

	switch {
	case size <= mo.bufferSizes["small"]:
		if bufferPool, ok := mo.objectPools["bufferPool"]; ok {
			bp := bufferPool.Get().(*BufferPool)
			defer bufferPool.Put(bp)
			pool = &bp.small
		}
	case size <= mo.bufferSizes["medium"]:
		if bufferPool, ok := mo.objectPools["bufferPool"]; ok {
			bp := bufferPool.Get().(*BufferPool)
			defer bufferPool.Put(bp)
			pool = &bp.medium
		}
	default:
		if bufferPool, ok := mo.objectPools["bufferPool"]; ok {
			bp := bufferPool.Get().(*BufferPool)
			defer bufferPool.Put(bp)
			pool = &bp.large
		}
	}

	if pool != nil {
		// 【修复】保留完整切片长度入池（不清 len），GetBuffer 按 cap 判定可命中
		pool.Put(buf)
	}
}

// GetObject 从对象池获取对象
func (mo *MemoryOptimizer) GetObject(poolName string) interface{} {
	if !mo.config.ObjectPoolEnabled {
		return nil
	}

	mo.mu.RLock()
	pool, exists := mo.objectPools[poolName]
	mo.mu.RUnlock()

	if !exists {
		mo.memoryStats.mu.Lock()
		mo.memoryStats.ObjectPoolMisses++
		mo.memoryStats.mu.Unlock()
		return nil
	}

	mo.memoryStats.mu.Lock()
	mo.memoryStats.ObjectPoolHits++
	mo.memoryStats.mu.Unlock()

	return pool.Get()
}

// PutObject 将对象返回到池中
func (mo *MemoryOptimizer) PutObject(poolName string, obj interface{}) {
	if !mo.config.ObjectPoolEnabled {
		return
	}

	mo.mu.RLock()
	pool, exists := mo.objectPools[poolName]
	mo.mu.RUnlock()

	if exists {
		// 重置对象状态
		mo.resetObject(obj)
		pool.Put(obj)
	}
}

// resetObject 重置对象状态
func (mo *MemoryOptimizer) resetObject(obj interface{}) {
	switch v := obj.(type) {
	case *ConnectionInfo:
		*v = ConnectionInfo{} // 重置为零值
	case *strings.Builder:
		v.Reset()
	case []byte:
		// 对于切片，清空内容但保留容量
		obj = v[:0]
	}
}

// CompressData 压缩数据
func (mo *MemoryOptimizer) CompressData(data []byte) (*CompressedData, error) {
	if !mo.config.CompressionEnabled || len(data) == 0 {
		return &CompressedData{
			Data:             data,
			OriginalSize:     len(data),
			CompressedSize:   len(data),
			CompressionRatio: 1.0,
		}, nil
	}

	buf := mo.GetBuffer(len(data))
	defer mo.PutBuffer(buf)

	var compressedBuf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressedBuf, mo.compressionLevel)
	if err != nil {
		return nil, fmt.Errorf("创建gzip写入器失败: %v", err)
	}

	if _, err := writer.Write(data); err != nil {
		writer.Close()
		return nil, fmt.Errorf("压缩数据失败: %v", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("关闭gzip写入器失败: %v", err)
	}

	compressedData := compressedBuf.Bytes()
	ratio := float64(len(compressedData)) / float64(len(data))

	// 更新压缩统计
	mo.memoryStats.mu.Lock()
	mo.memoryStats.CompressionRatio = (mo.memoryStats.CompressionRatio + ratio) / 2
	mo.memoryStats.mu.Unlock()

	return &CompressedData{
		Data:             compressedData,
		OriginalSize:     len(data),
		CompressedSize:   len(compressedData),
		CompressionRatio: ratio,
	}, nil
}

// DecompressData 解压缩数据
func (mo *MemoryOptimizer) DecompressData(compressedData *CompressedData) ([]byte, error) {
	if !mo.config.CompressionEnabled || compressedData.CompressionRatio == 1.0 {
		return compressedData.Data, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(compressedData.Data))
	if err != nil {
		return nil, fmt.Errorf("创建gzip读取器失败: %v", err)
	}
	defer reader.Close()

	buf := mo.GetBuffer(compressedData.OriginalSize)
	defer mo.PutBuffer(buf)

	var decompressedBuf bytes.Buffer
	if _, err := io.Copy(&decompressedBuf, reader); err != nil {
		return nil, fmt.Errorf("解压缩数据失败: %v", err)
	}

	return decompressedBuf.Bytes(), nil
}

// optimizeGCSettings 优化垃圾回收设置
func (mo *MemoryOptimizer) optimizeGCSettings() {
	// 设置GC目标百分比
	debug.SetGCPercent(int(mo.gcTriggerLevel))

	// 【安全修复】不再调用 debug.SetMemoryLimit：1GB 硬上限对承载多客户端
	// + 文件传输的 C2 服务过低，超限后 Go 激进 GC 甚至直接 OOM。
	// 内存管理交给 Go 运行时自动 GC 策略。
	logx.Infof("🗑️ GC设置优化完成: 触发阈值=%0.1f%%（未设置硬性内存上限）\n", mo.gcTriggerLevel)
}

// startMemoryMonitor 启动内存监控（P1-2: 不再自动触发 runtime.GC——
// Go 运行时自动 GC 策略优于固定阈值全量 GC，超阈值仅告警，由运维介入）
func (mo *MemoryOptimizer) startMemoryMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		mo.updateMemoryStats()

		// 检查是否超过阈值（仅告警，不手动 GC）
		if mo.shouldTriggerGC() {
			logx.Warnf("⚠️ [MEMORY] 内存使用率 %.1f%% 超过阈值 %.1f%%，不触发手动 GC"+
				"（交由 Go 运行时自动回收），建议检查连接数/下载任务\n",
				mo.memoryStats.UsagePercent, mo.gcTriggerLevel)
		}
	}
}

// updateMemoryStats 更新内存统计（P1-2: 分母改用真实物理内存，
// 原硬编码 config.MaxMemoryUsage(1GB) 导致 UsagePercent 失真）
func (mo *MemoryOptimizer) updateMemoryStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 物理总内存缓存（gopsutil 查询代价高，30s 一次足够；失败回退配置值）
	if mo.physicalMemory == 0 {
		if vm, err := mem.VirtualMemory(); err == nil && vm.Total > 0 {
			mo.physicalMemory = vm.Total
		}
	}
	denominator := mo.physicalMemory
	if denominator == 0 {
		denominator = uint64(mo.config.MaxMemoryUsage)
	}

	mo.memoryStats.mu.Lock()
	defer mo.memoryStats.mu.Unlock()

	mo.memoryStats.AllocatedMemory = m.Alloc
	mo.memoryStats.TotalMemory = m.Sys
	mo.memoryStats.UsagePercent = float64(m.Alloc) / float64(denominator) * 100
	mo.memoryStats.GCCount = m.NumGC
	mo.memoryStats.LastUpdate = time.Now()

	// 记录最后一次GC时间
	if m.NumGC > 0 {
		mo.memoryStats.LastGCTime = time.Unix(0, int64(m.LastGC))
	}
}

// shouldTriggerGC 判断是否应该触发GC
func (mo *MemoryOptimizer) shouldTriggerGC() bool {
	mo.memoryStats.mu.RLock()
	defer mo.memoryStats.mu.RUnlock()

	return mo.memoryStats.UsagePercent > mo.gcTriggerLevel
}

// triggerOptimizedGC 触发优化的GC
func (mo *MemoryOptimizer) triggerOptimizedGC() {
	logx.Infof("🗑️ 内存使用率 %.1f%% 超过阈值 %.1f%%，触发GC\n",
		mo.memoryStats.UsagePercent, mo.gcTriggerLevel)

	start := time.Now()
	runtime.GC()
	duration := time.Since(start)

	logx.Infof("✅ GC完成，耗时: %v\n", duration)

	// 清理对象池
	mo.cleanupObjectPools()
}

// cleanupObjectPools 清理对象池内存
// 注意：所有池均定义了 New 函数，pool.Get() 永不返回 nil，
// 原"Get 直到 nil"的 drain 逻辑会形成死循环疯狂分配导致 OOM，已移除。
// 对象池内存交由 runtime.GC 回收即可。
func (mo *MemoryOptimizer) cleanupObjectPools() {
	mo.mu.Lock()
	defer mo.mu.Unlock()

	for name := range mo.objectPools {
		logx.Infof("🧹 清理对象池（已请求GC回收）: %s\n", name)
	}
}

// GetMemoryStats 获取内存统计信息
func (mo *MemoryOptimizer) GetMemoryStats() *MemoryStats {
	mo.memoryStats.mu.RLock()
	defer mo.memoryStats.mu.RUnlock()

	// J2: 返回字段副本（原整体复制含 sync.RWMutex → vet: copies lock value）
	stats := MemoryStats{
		AllocatedMemory:  mo.memoryStats.AllocatedMemory,
		TotalMemory:      mo.memoryStats.TotalMemory,
		UsagePercent:     mo.memoryStats.UsagePercent,
		GCCount:          mo.memoryStats.GCCount,
		LastGCTime:       mo.memoryStats.LastGCTime,
		ObjectPoolHits:   mo.memoryStats.ObjectPoolHits,
		ObjectPoolMisses: mo.memoryStats.ObjectPoolMisses,
		CompressionRatio: mo.memoryStats.CompressionRatio,
		LastUpdate:       mo.memoryStats.LastUpdate,
	}
	return &stats
}

// SetCompressionLevel 设置压缩级别
func (mo *MemoryOptimizer) SetCompressionLevel(level int) {
	if level >= 1 && level <= 9 {
		mo.mu.Lock()
		mo.compressionLevel = level
		mo.mu.Unlock()
		logx.Infof("📦 压缩级别已设置为: %d\n", level)
	}
}

// SetGCTriggerLevel 设置GC触发级别
func (mo *MemoryOptimizer) SetGCTriggerLevel(percent float64) {
	if percent > 0 && percent <= 100 {
		mo.mu.Lock()
		mo.gcTriggerLevel = percent
		mo.mu.Unlock()
		debug.SetGCPercent(int(percent))
		logx.Infof("🗑️ GC触发阈值已设置为: %.1f%%\n", percent)
	}
}

// ForceGC 强制触发GC
func (mo *MemoryOptimizer) ForceGC() {
	mo.triggerOptimizedGC()
}

// GetPoolStats 获取对象池统计
func (mo *MemoryOptimizer) GetPoolStats() map[string]interface{} {
	mo.memoryStats.mu.RLock()
	defer mo.memoryStats.mu.RUnlock()

	return map[string]interface{}{
		"hits":   mo.memoryStats.ObjectPoolHits,
		"misses": mo.memoryStats.ObjectPoolMisses,
		"hit_rate": float64(mo.memoryStats.ObjectPoolHits) /
			float64(mo.memoryStats.ObjectPoolHits+mo.memoryStats.ObjectPoolMisses) * 100,
		"compression_ratio": mo.memoryStats.CompressionRatio,
	}
}
