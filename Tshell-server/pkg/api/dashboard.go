package api

import (
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"BackendTemplate/pkg/errors"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"BackendTemplate/pkg/utils/logx"
	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

// DashboardStats 仪表盘统计数据
type DashboardStats struct {
	MonitorCount    int        `json:"monitor_count"`     // 监听器数
	ConnectionCount int        `json:"connection_count"`  // 在线连接数
	UserCount       int        `json:"user_count"`        // 客户端总数
	OnlineUserCount int        `json:"online_user_count"` // 在线客户端数
	ConfigInfo      ConfigInfo `json:"config_info"`       // 配置信息
	SystemInfo      SystemInfo `json:"system_info"`       // 系统信息
}

// ConfigInfo 配置信息
type ConfigInfo struct {
	LogLevel    string `json:"log_level"`     // 日志等级
	LogFilePath string `json:"log_file_path"` // 日志文件路径
	WebPort     int    `json:"web_port"`      // WEB端口
	BasicAuth   bool   `json:"basic_auth"`    // Basic认证是否开启
	Version     string `json:"version"`       // 版本
}

// SystemInfo 系统信息
type SystemInfo struct {
	CPUUsage       float64 `json:"cpu_usage"`       // CPU使用率
	DiskUsage      float64 `json:"disk_usage"`      // 磁盘使用率
	MemoryUsage    float64 `json:"memory_usage"`    // 内存使用率
	VirtualMemory  float64 `json:"virtual_memory"`  // 虚拟内存使用率
	TCPConnections int     `json:"tcp_connections"` // TCP连接数
	UDPConnections int     `json:"udp_connections"` // UDP连接数
	OutboundBW     string  `json:"outbound_bw"`     // 流出带宽
	InboundBW      string  `json:"inbound_bw"`      // 流入带宽
}

// GetDashboardStats 获取仪表盘统计数据
func GetDashboardStats(c *gin.Context) {
	stats := DashboardStats{}

	// 检查数据库连接状态
	if database.DB() == nil {
		errors.AbortWithError(c, errors.NewInternalError("数据库连接未初始化"))
		return
	}

	// P0-1A: 监听器数量（小表，单次 Count 保留）
	listenerCount, err := database.Dashboard().OpenListenerCount()
	if err != nil {
		logx.Infof("获取监听器数据失败: %v\n", err)
		stats.MonitorCount = 0
	} else {
		stats.MonitorCount = int(listenerCount)
	}

	// P0-1A: 客户端总数 + 在线数合并为单次聚合查询（原 2 次全表扫描，5s 轮询放大）
	//
	// "聚合失败则退化为两次 Count" 这条回退已收进领域层
	// （database.DashboardStore.ClientCounts）。原来调用方还有一份同样的回退代码，
	// 与领域层那份并存只会两边漂移，已删。
	totalN, onlineN, aggErr := database.Dashboard().ClientCounts()
	if aggErr != nil {
		logx.Infof("统计 clients 失败（聚合与回退均失败）: %v", aggErr)
		stats.UserCount = 0
		stats.ConnectionCount = 0
		stats.OnlineUserCount = 0
	} else {
		stats.UserCount = int(totalN)
		stats.ConnectionCount = int(onlineN)
		stats.OnlineUserCount = int(onlineN)
	}

	// 获取配置信息
	appConfig := config.GetGlobalConfig()
	if appConfig != nil {
		webPort := appConfig.Server.Port
		// -p 可覆盖配置端口；Hub 在启动时记录实际监听端口，优先用它
		if actual := delivery.GlobalHub.MainPort(); actual > 0 {
			webPort = actual
		}
		stats.ConfigInfo = ConfigInfo{
			LogLevel:    appConfig.Logging.Level,
			LogFilePath: getAbsoluteLogPath(appConfig.Logging.AuditLogDir),
			WebPort:     webPort,
			BasicAuth:   false,                         // 当前项目使用JWT认证，不是Basic认证
			Version:     appConfig.Application.Version, // 从配置文件获取版本号
		}
	} else {
		// 如果配置获取失败，使用默认值
		stats.ConfigInfo = ConfigInfo{
			LogLevel:    "info",
			LogFilePath: "./logs",
			WebPort:     8089,
			BasicAuth:   false,
			Version:     "1.0.0",
		}
	}

	// 获取系统信息
	systemInfo, err := getSystemInfoCached()
	if err != nil {
		logx.Infof("获取系统信息失败: %v\n", err)
		// 使用默认的系统信息
		stats.SystemInfo = SystemInfo{
			CPUUsage:       0.0,
			DiskUsage:      0.0,
			MemoryUsage:    0.0,
			VirtualMemory:  0.0,
			TCPConnections: 0,
			UDPConnections: 0,
			OutboundBW:     "0.00MB/s",
			InboundBW:      "0.00KB/s",
		}
	} else {
		stats.SystemInfo = systemInfo
	}

	errors.RespondWithSuccess(c, stats)
}

// getAbsoluteLogPath 获取日志文件的绝对路径
func getAbsoluteLogPath(logDir string) string {
	absPath, err := filepath.Abs(logDir)
	if err != nil {
		return logDir // 如果获取失败，返回原路径
	}
	return absPath
}

// 系统信息缓存：cpu.Percent 需采样阻塞 1s，net.Connections 遍历 /proc，开销大，
// 该端点同时被路由守卫 token 验证与仪表盘轮询高频调用，5s TTL 缓存可消除重复开销。
// 【并发优化】命中路径无锁读（atomic.Pointer），仅刷新路径经单飞锁串行——
// 原实现刷新期间持全局 Mutex 1s+，并发请求全部串行等待。
// P1-2B: 后台 10s 预采，请求路径仅原子读；net.Connections 降至 30s 一次
var (
	systemInfoCachePtr atomic.Pointer[SystemInfo]
	systemInfoCacheAt  atomic.Int64 // UnixNano 缓存时间戳
	systemInfoLoadMu   sync.Mutex   // 仅冷启动/兜底刷新持有（单飞）
	// P1-2B: 后台预采与按需刷新分离的连接计数缓存（30s 周期，避免每 10s 遍历 /proc）
	cachedTCPConns atomic.Int64
	cachedUDPConns atomic.Int64
)

const systemInfoCacheTTL = 10 * time.Second

// InitDashboardCollector 启动后台系统信息预采（10s 周期 + 30s 连接数周期）
// 由 main.go 在依赖就绪后调用一次；重复调用无副作用
var dashboardCollectorOnce sync.Once

func InitDashboardCollector() {
	dashboardCollectorOnce.Do(func() {
		// 立即预采一次，避免冷启动空窗
		if info, err := getSystemInfoWithConnSampling(true); err == nil {
			cp := info
			systemInfoCachePtr.Store(&cp)
			systemInfoCacheAt.Store(time.Now().UnixNano())
		}
		go func() {
			tick10 := time.NewTicker(10 * time.Second)
			tick30 := time.NewTicker(30 * time.Second)
			defer tick10.Stop()
			defer tick30.Stop()
			sampleConn := false
			for {
				select {
				case <-tick10.C:
					info, err := getSystemInfoWithConnSampling(sampleConn)
					if err == nil {
						cp := info
						systemInfoCachePtr.Store(&cp)
						systemInfoCacheAt.Store(time.Now().UnixNano())
					}
					sampleConn = false
				case <-tick30.C:
					sampleConn = true
				}
			}
		}()
	})
}

// getSystemInfoWithConnSampling 采样系统信息；sampleConn=true 时才遍历 net.Connections
func getSystemInfoWithConnSampling(sampleConn bool) (SystemInfo, error) {
	info, err := getSystemInfoBase()
	if err != nil {
		return info, err
	}
	if sampleConn {
		if conns, e := net.Connections("all"); e == nil {
			tcp, udp := 0, 0
			for _, c := range conns {
				if c.Type == 1 && c.Status == "ESTABLISHED" {
					tcp++
				} else if c.Type == 2 {
					udp++
				}
			}
			cachedTCPConns.Store(int64(tcp))
			cachedUDPConns.Store(int64(udp))
		}
	}
	info.TCPConnections = int(cachedTCPConns.Load())
	info.UDPConnections = int(cachedUDPConns.Load())
	return info, nil
}

// getSystemInfoBase 采集 CPU/内存/磁盘/带宽（不含连接数遍历）
func getSystemInfoBase() (SystemInfo, error) {
	var info SystemInfo
	cpuPercent, err := cpu.Percent(time.Second, false)
	if err == nil && len(cpuPercent) > 0 {
		info.CPUUsage = cpuPercent[0]
	}
	if memInfo, err := mem.VirtualMemory(); err == nil {
		info.MemoryUsage = memInfo.UsedPercent
		if swapInfo, err := mem.SwapMemory(); err == nil {
			info.VirtualMemory = swapInfo.UsedPercent
		}
	}
	diskInfo, err := disk.Usage("/")
	if runtime.GOOS == "windows" {
		diskInfo, err = disk.Usage("C:\\")
	}
	if err == nil {
		info.DiskUsage = diskInfo.UsedPercent
	}
	if GlobalBandwidthMonitor != nil {
		inbound, outbound := GlobalBandwidthMonitor.GetBandwidth()
		info.InboundBW = inbound
		info.OutboundBW = outbound
	} else {
		info.OutboundBW = "0.00KB/s"
		info.InboundBW = "0.00KB/s"
	}
	return info, nil
}

// getSystemInfoCached 请求路径：仅原子读后台预采结果；冷启动空窗才单飞同步采样一次
func getSystemInfoCached() (SystemInfo, error) {
	if p := systemInfoCachePtr.Load(); p != nil {
		if cachedAt := systemInfoCacheAt.Load(); cachedAt > 0 &&
			time.Since(time.Unix(0, cachedAt)) < systemInfoCacheTTL {
			return *p, nil
		}
	}

	// 冷启动或后台尚未就绪：单飞同步采样一次（仅此分支阻塞 1s）
	systemInfoLoadMu.Lock()
	defer systemInfoLoadMu.Unlock()

	if p := systemInfoCachePtr.Load(); p != nil {
		if cachedAt := systemInfoCacheAt.Load(); cachedAt > 0 &&
			time.Since(time.Unix(0, cachedAt)) < systemInfoCacheTTL {
			return *p, nil
		}
	}

	info, err := getSystemInfoWithConnSampling(false)
	if err != nil {
		return info, err
	}
	cp := info
	systemInfoCachePtr.Store(&cp)
	systemInfoCacheAt.Store(time.Now().UnixNano())
	return info, nil
}

// getSystemInfo 保留兼容：等价 getSystemInfoWithConnSampling(true)
func getSystemInfo() (SystemInfo, error) {
	return getSystemInfoWithConnSampling(true)
}
