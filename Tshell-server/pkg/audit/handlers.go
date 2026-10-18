package audit

import (
	"BackendTemplate/pkg/database"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

// LogQueryParams 日志查询参数
type LogQueryParams struct {
	StartTime *time.Time  `form:"start_time"`
	EndTime   *time.Time  `form:"end_time"`
	Level     LogLevel    `form:"level"`
	Category  LogCategory `form:"category"`
	Action    string      `form:"action"`
	Username  string      `form:"username"`
	ClientIP  string      `form:"client_ip"`
	Result    string      `form:"result"`
	Page      int         `form:"page" binding:"min=1"`
	PageSize  int         `form:"page_size" binding:"min=1,max=1000"`
	Keyword   string      `form:"keyword"`
}

// LogQueryResponse 日志查询响应
type LogQueryResponse struct {
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Logs     []AuditLog `json:"logs"`
}

// LogStatistics 日志统计信息
type LogStatistics struct {
	TotalLogs      int                 `json:"total_logs"`
	LogsByLevel    map[LogLevel]int    `json:"logs_by_level"`
	LogsByCategory map[LogCategory]int `json:"logs_by_category"`
	LogsByResult   map[string]int      `json:"logs_by_result"`
	TopUsers       []UserStats         `json:"top_users"`
	TopIPs         []IPStats           `json:"top_ips"`
	TodayLogs      int                 `json:"today_logs"`
	RecentFailures int                 `json:"recent_failures"`
}

type UserStats struct {
	Username string `json:"username"`
	Count    int    `json:"count"`
}

type IPStats struct {
	IP    string `json:"ip"`
	Count int    `json:"count"`
}

// GetAuditLogs 获取审计日志
func GetAuditLogs(c *gin.Context) {
	var params LogQueryParams

	// 设置默认值
	params.Page = 1
	params.PageSize = 20

	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "参数错误: " + err.Error(),
		})
		return
	}

	// 验证时间参数
	if params.StartTime != nil && params.EndTime != nil && params.StartTime.After(*params.EndTime) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "开始时间不能晚于结束时间",
		})
		return
	}

	logs, total, err := queryLogs(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "查询日志失败: " + err.Error(),
		})
		return
	}

	response := LogQueryResponse{
		Total:    total,
		Page:     params.Page,
		PageSize: params.PageSize,
		Logs:     logs,
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data":   response,
	})
}

// GetAuditStatistics 获取审计日志统计信息
func GetAuditStatistics(c *gin.Context) {
	timeRange := c.DefaultQuery("time_range", "24h") // 24h, 7d, 30d

	var startTime time.Time
	switch timeRange {
	case "24h":
		startTime = time.Now().Add(-24 * time.Hour)
	case "7d":
		startTime = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		startTime = time.Now().Add(-30 * 24 * time.Hour)
	default:
		startTime = time.Now().Add(-24 * time.Hour)
	}

	stats, err := generateStatistics(startTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "生成统计信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data":   stats,
	})
}

// ExportAuditLogs 导出审计日志
func ExportAuditLogs(c *gin.Context) {
	var params LogQueryParams

	if err := c.ShouldBindQuery(&params); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "参数错误: " + err.Error(),
		})
		return
	}

	// 导出所有符合条件的日志，不分页
	params.Page = 1
	params.PageSize = 10000

	logs, _, err := queryLogs(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "查询日志失败: " + err.Error(),
		})
		return
	}

	// 设置响应头
	filename := fmt.Sprintf("audit_logs_%s.json", time.Now().Format("2006-01-02_15-04-05"))
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	// 输出JSON
	c.JSON(http.StatusOK, gin.H{
		"export_time": time.Now(),
		"total":       len(logs),
		"logs":        logs,
	})
}

// CleanupOldLogs 清理旧日志
func CleanupOldLogs(c *gin.Context) {
	var req struct {
		Days int `json:"days" binding:"required,min=1"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": http.StatusBadRequest,
			"error":  "参数错误: " + err.Error(),
		})
		return
	}

	if GlobalAuditLogger == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "审计日志系统未初始化",
		})
		return
	}

	// 执行清理
	cutoff := time.Now().AddDate(0, 0, -req.Days)
	deletedCount := 0

	err := filepath.Walk(GlobalAuditLogger.logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.ModTime().Before(cutoff) {
			if filepath.Ext(path) == ".log" {
				if err := os.Remove(path); err == nil {
					deletedCount++
				}
			}
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "清理日志失败: " + err.Error(),
		})
		return
	}

	// 库内记录与文件同一保留期
	dbDeleted, dbErr := cleanupDBBefore(cutoff)
	if dbErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": http.StatusInternalServerError,
			"error":  "清理审计记录失败: " + dbErr.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"deleted_files":   deletedCount,
			"deleted_records": dbDeleted,
			"cutoff_date":     cutoff,
		},
	})
}

// queryLogs 查询日志数据
func queryLogs(params LogQueryParams) ([]AuditLog, int, error) {
	// 查询走库（原实现：遍历最近 30 个文件全量解析→内存过滤排序→切片分页）
	return queryLogsFromDB(params)
}

// QueryLoginFailures 返回 since 之后的登录失败记录（时间倒序，最多 limit 条）。
// 与 /api/audit/logs 同源（落库后走库），供 IOC 报告等聚合使用；失败返回 nil。
func QueryLoginFailures(since time.Time, limit int) []string {
	if database.DB() == nil || limit <= 0 {
		return nil
	}
	logs, _, err := queryLogs(LogQueryParams{
		StartTime: &since,
		Category:  CategoryAuth,
		Result:    "FAILURE",
		Page:      1,
		PageSize:  limit,
	})
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(logs))
	for _, l := range logs {
		out = append(out, fmt.Sprintf("%s %s@%s", l.Timestamp.Format("2006-01-02 15:04:05"), l.Username, l.ClientIP))
	}
	return out
}


func generateStatistics(startTime time.Time) (*LogStatistics, error) {
	params := LogQueryParams{
		StartTime: &startTime,
		Page:      1,
		PageSize:  10000, // 获取所有记录用于统计
	}

	logs, _, err := queryLogs(params)
	if err != nil {
		return nil, err
	}

	stats := &LogStatistics{
		LogsByLevel:    make(map[LogLevel]int),
		LogsByCategory: make(map[LogCategory]int),
		LogsByResult:   make(map[string]int),
	}

	userCount := make(map[string]int)
	ipCount := make(map[string]int)
	today := time.Now().Truncate(24 * time.Hour)

	for _, log := range logs {
		stats.TotalLogs++

		// 按级别统计
		stats.LogsByLevel[log.Level]++

		// 按分类统计
		stats.LogsByCategory[log.Category]++

		// 按结果统计
		stats.LogsByResult[log.Result]++

		// 用户统计
		if log.Username != "" {
			userCount[log.Username]++
		}

		// IP统计
		if log.ClientIP != "" {
			ipCount[log.ClientIP]++
		}

		// 今日日志
		if log.Timestamp.After(today) {
			stats.TodayLogs++
		}

		// 最近失败
		if log.Result == "FAILURE" && log.Timestamp.After(time.Now().Add(-1*time.Hour)) {
			stats.RecentFailures++
		}
	}

	// Top 10 用户
	var userStats []UserStats
	for user, count := range userCount {
		userStats = append(userStats, UserStats{Username: user, Count: count})
	}
	sort.Slice(userStats, func(i, j int) bool {
		return userStats[i].Count > userStats[j].Count
	})
	if len(userStats) > 10 {
		userStats = userStats[:10]
	}
	stats.TopUsers = userStats

	// Top 10 IP
	var ipStats []IPStats
	for ip, count := range ipCount {
		ipStats = append(ipStats, IPStats{IP: ip, Count: count})
	}
	sort.Slice(ipStats, func(i, j int) bool {
		return ipStats[i].Count > ipStats[j].Count
	})
	if len(ipStats) > 10 {
		ipStats = ipStats[:10]
	}
	stats.TopIPs = ipStats

	return stats, nil
}
