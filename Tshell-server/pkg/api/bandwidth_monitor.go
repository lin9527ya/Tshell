package api

import (
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/net"
	"BackendTemplate/pkg/utils/logx"
)

// BandwidthMonitor 带宽监控器
type BandwidthMonitor struct {
	mu           sync.RWMutex
	lastStat     net.IOCountersStat
	lastTime     time.Time
	inboundBW    float64 // KB/s
	outboundBW   float64 // KB/s
	isInitialized bool
}

// 全局带宽监控器实例
var GlobalBandwidthMonitor *BandwidthMonitor

// InitBandwidthMonitor 初始化带宽监控器
func InitBandwidthMonitor() {
	GlobalBandwidthMonitor = &BandwidthMonitor{}
	// 启动监控协程
	go GlobalBandwidthMonitor.startMonitoring()
}

// startMonitoring 启动带宽监控
func (bm *BandwidthMonitor) startMonitoring() {
	ticker := time.NewTicker(2 * time.Second) // 每2秒更新一次
	defer ticker.Stop()

	// 获取初始值
	bm.updateBandwidth()

	for range ticker.C {
		bm.updateBandwidth()
	}
}

// updateBandwidth 更新带宽数据
func (bm *BandwidthMonitor) updateBandwidth() {
	netStats, err := net.IOCounters(false)
	if err != nil {
		logx.Infof("获取网络统计失败: %v\n", err)
		return
	}

	if len(netStats) == 0 {
		return
	}

	currentStat := netStats[0]
	currentTime := time.Now()

	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.isInitialized {
		// 计算时间差
		timeDiff := currentTime.Sub(bm.lastTime).Seconds()
		if timeDiff > 0 {
			// 计算字节差
			bytesSentDiff := currentStat.BytesSent - bm.lastStat.BytesSent
			bytesRecvDiff := currentStat.BytesRecv - bm.lastStat.BytesRecv

			// 计算速率 (bytes/s 转换为 KB/s)
			bm.outboundBW = float64(bytesSentDiff) / timeDiff / 1024
			bm.inboundBW = float64(bytesRecvDiff) / timeDiff / 1024
		}
	} else {
		bm.isInitialized = true
	}

	bm.lastStat = currentStat
	bm.lastTime = currentTime
}

// GetBandwidth 获取当前带宽使用情况
func (bm *BandwidthMonitor) GetBandwidth() (inbound, outbound string) {
	if bm == nil {
		return "0.00KB/s", "0.00KB/s"
	}

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	// 格式化输出
	inbound = formatBandwidth(bm.inboundBW)
	outbound = formatBandwidth(bm.outboundBW)
	
	return inbound, outbound
}

// formatBandwidth 格式化带宽显示
func formatBandwidth(kbps float64) string {
	if kbps < 1024 {
		return fmt.Sprintf("%.2fKB/s", kbps)
	} else {
		mbps := kbps / 1024
		return fmt.Sprintf("%.2fMB/s", mbps)
	}
}

// GetRawBandwidth 获取原始带宽数据 (KB/s)
func (bm *BandwidthMonitor) GetRawBandwidth() (inbound, outbound float64) {
	if bm == nil {
		return 0, 0
	}

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return bm.inboundBW, bm.outboundBW
}