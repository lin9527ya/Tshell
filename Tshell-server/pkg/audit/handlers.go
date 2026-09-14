package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

	c.JSON(http.StatusOK, gin.H{
		"status": http.StatusOK,
		"data": gin.H{
			"deleted_files": deletedCount,
			"cutoff_date":   cutoff,
		},
	})
}

// queryLogs 查询日志数据
func queryLogs(params LogQueryParams) ([]AuditLog, int, error) {
	if GlobalAuditLogger == nil {
		return nil, 0, fmt.Errorf("审计日志系统未初始化")
	}

	var allLogs []AuditLog

	// 获取日志目录中的所有文件
	files, err := filepath.Glob(filepath.Join(GlobalAuditLogger.logDir, "audit_*.log"))
	if err != nil {
		return nil, 0, err
	}

	// 按文件名倒序（最新日期在前），并限制扫描文件数（防全目录全量读取的内存/IO 放大）
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	const maxScanFiles = 30 // 最多扫描最近 30 个日志文件
	if len(files) > maxScanFiles {
		files = files[:maxScanFiles]
	}

	// 读取日志文件（倒序：新→旧）
	for _, file := range files {
		// 检查文件日期是否在查询范围内
		if params.StartTime != nil || params.EndTime != nil {
			filename := filepath.Base(file)
			dateStr := strings.TrimPrefix(filename, "audit_")
			dateStr = strings.TrimSuffix(dateStr, ".log")

			if fileDate, err := time.Parse("2006-01-02", dateStr); err == nil {
				if params.StartTime != nil && fileDate.Before(*params.StartTime) {
					continue
				}
				if params.EndTime != nil && fileDate.After(*params.EndTime) {
					continue
				}
			}
		}

		logs, err := readLogFile(file, params)
		if err != nil {
			continue // 跳过错误文件
		}
		allLogs = append(allLogs, logs...)
	}

	// 排序（按时间倒序）
	sort.Slice(allLogs, func(i, j int) bool {
		return allLogs[i].Timestamp.After(allLogs[j].Timestamp)
	})

	// 分页
	total := len(allLogs)
	start := (params.Page - 1) * params.PageSize
	end := start + params.PageSize

	if start >= total {
		return []AuditLog{}, total, nil
	}
	if end > total {
		end = total
	}

	return allLogs[start:end], total, nil
}

// readLogFile 读取单个日志文件
func readLogFile(filename string, params LogQueryParams) ([]AuditLog, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var logs []AuditLog
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var log AuditLog
		if err := json.Unmarshal([]byte(line), &log); err != nil {
			continue // 跳过无效行
		}

		// 应用过滤条件
		if !matchesFilters(log, params) {
			continue
		}

		logs = append(logs, log)
	}

	return logs, nil
}

// matchesFilters 检查日志是否匹配过滤条件
func matchesFilters(log AuditLog, params LogQueryParams) bool {
	// 时间范围
	if params.StartTime != nil && log.Timestamp.Before(*params.StartTime) {
		return false
	}
	if params.EndTime != nil && log.Timestamp.After(*params.EndTime) {
		return false
	}

	// 日志级别
	if params.Level != "" && log.Level != params.Level {
		return false
	}

	// 日志分类
	if params.Category != "" && log.Category != params.Category {
		return false
	}

	// 操作
	if params.Action != "" && !strings.Contains(strings.ToLower(log.Action), strings.ToLower(params.Action)) {
		return false
	}

	// 用户名
	if params.Username != "" && !strings.Contains(strings.ToLower(log.Username), strings.ToLower(params.Username)) {
		return false
	}

	// 客户端IP
	if params.ClientIP != "" && !strings.Contains(log.ClientIP, params.ClientIP) {
		return false
	}

	// 结果
	if params.Result != "" && log.Result != params.Result {
		return false
	}

	// 关键词搜索
	if params.Keyword != "" {
		keyword := strings.ToLower(params.Keyword)
		if !strings.Contains(strings.ToLower(log.Message), keyword) &&
			!strings.Contains(strings.ToLower(log.Action), keyword) &&
			!strings.Contains(strings.ToLower(log.Username), keyword) &&
			!strings.Contains(strings.ToLower(log.ClientIP), keyword) {
			return false
		}
	}

	return true
}

// generateStatistics 生成统计信息
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
