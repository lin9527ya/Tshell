package connection

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GetConnectionPoolStats 获取连接池统计信息
func GetConnectionPoolStats(c *gin.Context) {
	if GlobalConnectionManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "连接管理器未初始化",
		})
		return
	}

	stats := GlobalConnectionManager.GetStats()
	allConnections := GlobalConnectionManager.GetAllConnections()

	// 按协议统计连接数
	protocolStats := make(map[string]int)
	for _, conn := range allConnections {
		protocolStats[conn.Protocol]++
	}

	poolCfg := GlobalConnectionManager.GetConfig()
	maxConnections := poolCfg.MaxConnections
	if maxConnections <= 0 {
		maxConnections = 1
	}
	utilization := float64(stats.ActiveConnections) / float64(maxConnections)
	healthStatus := "HEALTHY"
	if utilization >= 0.9 {
		healthStatus = "DEGRADED"
	}
	response := gin.H{
		"pool_stats": gin.H{
			"total_connections":      stats.TotalConnections,
			"active_connections":     stats.ActiveConnections,
			"idle_connections":       stats.IdleConnections,
			"total_bytes_read":       stats.TotalBytesRead,
			"total_bytes_written":    stats.TotalBytesWritten,
			"total_errors":           stats.TotalErrors,
			"connections_per_second": stats.ConnectionsPerSecond,
			"last_update":            stats.LastUpdate,
		},
		"protocol_stats":  protocolStats,
		"max_connections": maxConnections,
		"utilization":     utilization,
		"health_status":   healthStatus,
		// 回传当前生效的超时配置，前端据此回填（此前只能回填 max_connections / max_idle_time，
		// 超时项保存后刷新又从默认值覆盖，看不出实际生效值）
		"config": gin.H{
			"max_connections":     maxConnections,
			"max_idle_time":       poolCfg.MaxIdleTime.String(),
			"health_check_period": poolCfg.HealthCheckPeriod.String(),
			"conn_timeout":        poolCfg.ConnTimeout.String(),
			"read_timeout":        poolCfg.ReadTimeout.String(),
			"write_timeout":       poolCfg.WriteTimeout.String(),
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data":   response,
	})
}

// GetWorkerPoolStats 获取工作池统计信息
func GetWorkerPoolStats(c *gin.Context) {
	if GlobalWorkerPool == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "工作池未初始化",
		})
		return
	}

	stats := GlobalWorkerPool.GetStats()
	workerInfo := GlobalWorkerPool.GetWorkerInfo()
	queueSize := GlobalWorkerPool.GetQueueSize()

	// 除零保护：workerCount=0 时 maxQueueSize 为缓冲下限，利用率按 0 计
	queueUtilization := 0.0
	if GlobalWorkerPool.maxQueueSize > 0 {
		queueUtilization = float64(queueSize) / float64(GlobalWorkerPool.maxQueueSize)
	}

	response := gin.H{
		"pool_stats": gin.H{
			"total_tasks":        stats.TotalTasks,
			"completed_tasks":    stats.CompletedTasks,
			"failed_tasks":       stats.FailedTasks,
			"active_workers":     stats.ActiveWorkers,
			"queued_tasks":       stats.QueuedTasks,
			"average_task_time":  stats.AverageTaskTime,
			"total_task_time":    stats.TotalTaskTime,
			"worker_utilization": stats.WorkerUtilization,
			"last_update":        stats.LastUpdate,
		},
		"worker_info":    workerInfo,
		"queue_size":     queueSize,
		"max_queue_size": GlobalWorkerPool.maxQueueSize,
		// J2: worker_count 经锁内 GetWorkerInfo 读取（原无锁读与 ResizePool 写竞争）
		"worker_count":      len(workerInfo),
		"queue_utilization": queueUtilization,
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data":   response,
	})
}

// GetConnectionDetails 获取连接详细信息
func GetConnectionDetails(c *gin.Context) {
	if GlobalConnectionManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "连接管理器未初始化",
		})
		return
	}

	// 获取查询参数
	protocol := c.Query("protocol")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	allConnections := GlobalConnectionManager.GetAllConnections()
	var filteredConnections []*ConnectionInfo

	// 按协议过滤
	for _, conn := range allConnections {
		if protocol == "" || conn.Protocol == protocol {
			filteredConnections = append(filteredConnections, conn)
		}
	}

	// 分页
	total := len(filteredConnections)
	start := (page - 1) * pageSize
	end := start + pageSize

	if start >= total {
		filteredConnections = []*ConnectionInfo{}
	} else {
		if end > total {
			end = total
		}
		filteredConnections = filteredConnections[start:end]
	}

	// 构建响应数据
	// 空列表序列化为 []（nil slice 会变 null，前端 Array.isArray 判定失败）
	connectionData := make([]gin.H, 0)
	for _, conn := range filteredConnections {
		uptime, bytesRead, bytesWritten, errorCount := conn.GetStats()

		connectionData = append(connectionData, gin.H{
			"id":         conn.ID,
			"uid":        conn.UID,
			"protocol":   conn.Protocol,
			"client_ip":  conn.ClientIP,
			"created_at": conn.CreatedAt,
			// 【并发修复】LastActivity/IsActive 在 ci.mu 保护下被热路径修改，
			// 必须经 Safe 读取避免数据竞争
			"last_activity": conn.LastActivitySafe(),
			"is_active":     conn.IsActiveSafe(),
			"uptime":        uptime,
			"bytes_read":    bytesRead,
			"bytes_written": bytesWritten,
			"error_count":   errorCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"connections": connectionData,
			"pagination": gin.H{
				"page":        page,
				"page_size":   pageSize,
				"total":       total,
				"total_pages": (total + pageSize - 1) / pageSize,
			},
		},
	})
}

// CloseConnection 关闭指定连接
func CloseConnection(c *gin.Context) {
	if GlobalConnectionManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "连接管理器未初始化",
		})
		return
	}

	connectionID := c.Param("id")
	if connectionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "连接ID不能为空",
		})
		return
	}

	err := GlobalConnectionManager.RemoveConnection(connectionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "连接已关闭",
	})
}

// UpdatePoolConfig 更新连接池配置
func UpdatePoolConfig(c *gin.Context) {
	var req struct {
		MaxConnections    *int    `json:"max_connections"`
		MaxIdleTime       *string `json:"max_idle_time"`       // 格式: "30m", "1h"
		HealthCheckPeriod *string `json:"health_check_period"` // 格式: "30s", "1m"
		ConnTimeout       *string `json:"conn_timeout"`        // 格式: "10s", "30s"
		ReadTimeout       *string `json:"read_timeout"`        // 格式: "30s", "1m"
		WriteTimeout      *string `json:"write_timeout"`       // 格式: "30s", "1m"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "参数错误: " + err.Error(),
		})
		return
	}

	if GlobalConnectionManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "连接管理器未初始化",
		})
		return
	}

	// J2: 快照读取 → 修改 → 原子写回（原直接无锁改写 config 指针 → 与 AddConnection/清理读竞争）
	newCfg := GlobalConnectionManager.getConfig()
	if req.MaxConnections != nil && *req.MaxConnections > 0 {
		newCfg.MaxConnections = *req.MaxConnections
	}
	// 其余字段此前只解析不应用：页面保存成功但实际配置没变。
	durationFields := []struct {
		name string
		raw  *string
		dst  *time.Duration
	}{
		{"max_idle_time", req.MaxIdleTime, &newCfg.MaxIdleTime},
		{"health_check_period", req.HealthCheckPeriod, &newCfg.HealthCheckPeriod},
		{"conn_timeout", req.ConnTimeout, &newCfg.ConnTimeout},
		{"read_timeout", req.ReadTimeout, &newCfg.ReadTimeout},
		{"write_timeout", req.WriteTimeout, &newCfg.WriteTimeout},
	}
	for _, f := range durationFields {
		if f.raw == nil {
			continue
		}
		d, err := time.ParseDuration(*f.raw)
		if err != nil || d <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"status": http.StatusBadRequest,
				"error":  f.name + " 必须是大于 0 的 duration（如 30s、5m）",
			})
			return
		}
		*f.dst = d
	}

	GlobalConnectionManager.setConfig(newCfg)
	cfg := GlobalConnectionManager.getConfig()

	c.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "配置更新成功",
		"config": gin.H{
			"max_connections":     cfg.MaxConnections,
			"max_idle_time":       cfg.MaxIdleTime.String(),
			"health_check_period": cfg.HealthCheckPeriod.String(),
			"conn_timeout":        cfg.ConnTimeout.String(),
			"read_timeout":        cfg.ReadTimeout.String(),
			"write_timeout":       cfg.WriteTimeout.String(),
		},
	})
}

// ResizeWorkerPool 调整工作池大小
func ResizeWorkerPool(c *gin.Context) {
	var req struct {
		WorkerCount int `json:"worker_count" binding:"required,min=1,max=1000"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "参数错误: " + err.Error(),
		})
		return
	}

	if GlobalWorkerPool == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "工作池未初始化",
		})
		return
	}

	err := GlobalWorkerPool.ResizePool(req.WorkerCount)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   http.StatusOK,
		"message":  "工作池大小调整成功",
		"new_size": req.WorkerCount,
	})
}

// GetConnectionsByProtocol 根据协议获取连接
func GetConnectionsByProtocol(c *gin.Context) {
	if GlobalConnectionManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "连接管理器未初始化",
		})
		return
	}

	protocol := c.Param("protocol")
	if protocol == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "协议参数不能为空",
		})
		return
	}

	connections := GlobalConnectionManager.GetConnectionsByProtocol(protocol)

	// 空列表序列化为 []（nil slice 会变 null，前端 Array.isArray 判定失败）
	connectionData := make([]gin.H, 0)
	for _, conn := range connections {
		uptime, bytesRead, bytesWritten, errorCount := conn.GetStats()

		connectionData = append(connectionData, gin.H{
			"id":         conn.ID,
			"uid":        conn.UID,
			"client_ip":  conn.ClientIP,
			"created_at": conn.CreatedAt,
			// 【并发修复】LastActivity 在 ci.mu 保护下被热路径修改，必须经 Safe 读取
			"last_activity": conn.LastActivitySafe(),
			"uptime":        uptime,
			"bytes_read":    bytesRead,
			"bytes_written": bytesWritten,
			"error_count":   errorCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"protocol":    protocol,
			"count":       len(connections),
			"connections": connectionData,
		},
	})
}

// PerformHealthCheck 手动执行健康检查
func PerformHealthCheck(c *gin.Context) {
	if GlobalConnectionManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "连接管理器未初始化",
		})
		return
	}

	// 触发健康检查
	GlobalConnectionManager.performHealthCheck()

	c.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"message": "健康检查已执行",
	})
}

// GetConnectionByUID 根据UID获取连接
func GetConnectionByUID(c *gin.Context) {
	if GlobalConnectionManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "连接管理器未初始化",
		})
		return
	}

	uid := c.Param("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "UID参数不能为空",
		})
		return
	}

	conn, exists := GlobalConnectionManager.GetConnectionByUID(uid)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"status": http.StatusNotFound,
			"error":  "连接不存在",
		})
		return
	}

	uptime, bytesRead, bytesWritten, errorCount := conn.GetStats()

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"id":            conn.ID,
			"uid":           conn.UID,
			"protocol":      conn.Protocol,
			"client_ip":     conn.ClientIP,
			"created_at":    conn.CreatedAt,
			"last_activity": conn.LastActivity,
			"is_active":     conn.IsActive,
			"uptime":        uptime,
			"bytes_read":    bytesRead,
			"bytes_written": bytesWritten,
			"error_count":   errorCount,
		},
	})
}
