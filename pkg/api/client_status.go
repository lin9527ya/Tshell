package api

import (
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// SmartOfflineCheckRequest 智能离线检测请求
type SmartOfflineCheckRequest struct {
	UID string `json:"uid" binding:"required"`
}

// SmartOfflineCheckResponse 智能离线检测响应
type SmartOfflineCheckResponse struct {
	Status    string                 `json:"status"`     // "online", "offline", "possibly_busy", "likely_offline"
	Message   string                 `json:"message"`    // 状态描述
	CheckTime time.Time              `json:"check_time"` // 检测时间
	Details   map[string]interface{} `json:"details"`    // 详细信息
}

// SmartOfflineCheck 智能离线检测：一律基于统一离线检测器（OfflineDetector）的状态与阈值，
// 消除原先与服务端检测器重复实现、两套阈值/两套清理互相打架的问题。
// 客户端在检测器中注册即在线；未注册且库中不是离线态时，先等待一个心跳周期再判。
func SmartOfflineCheck(c *gin.Context) {
	var req SmartOfflineCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	uid := req.UID
	checkTime := time.Now()

	client, err := database.GetClientByUid(uid)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":     "error",
			"message":    "客户端不存在: " + uid,
			"check_time": checkTime,
		})
		return
	}

	if client.Online == "2" {
		c.JSON(http.StatusOK, SmartOfflineCheckResponse{
			Status:    "offline",
			Message:   "客户端已离线（数据库状态）",
			CheckTime: checkTime,
			Details: map[string]interface{}{
				"database_status": "offline",
				"last_seen":       client.FirstStart,
				"listener_type":   client.Listener,
				"algorithm":       "unified_offline_detector",
			},
		})
		return
	}

	// 统一来源：离线检测器持有的状态（LastActive / sleep / jitter）与阈值算法
	if connection.GlobalOfflineDetector == nil {
		c.JSON(http.StatusOK, SmartOfflineCheckResponse{
			Status:    "online",
			Message:   "统一离线检测器未初始化，视为在线",
			CheckTime: checkTime,
			Details:   map[string]interface{}{"algorithm": "unified_offline_detector"},
		})
		return
	}

	state, registered := connection.GlobalOfflineDetector.GetClientState(uid)
	if !registered {
		// 未注册：等待一个默认心跳周期，期间不直接判离线，避免客户端刚起连、
		// FirstBlood 尚未落库时误标离线（旧逻辑会立即判离线）。
		_, _, critical := connection.Thresholds(5, 0)
		if t, err := time.Parse("2006-01-02 15:04:05", client.FirstStart); err == nil && time.Since(t) < critical {
			c.JSON(http.StatusOK, SmartOfflineCheckResponse{
				Status:    "possibly_busy",
				Message:   "客户端已上线，等待首个心跳注册后再判",
				CheckTime: checkTime,
				Details: map[string]interface{}{
					"connection_exists": false,
					"listener_type":     client.Listener,
					"algorithm":         "unified_offline_detector",
				},
			})
			return
		}

		connection.GlobalOfflineDetector.ForceOffline(uid, "未注册超时")
		c.JSON(http.StatusOK, SmartOfflineCheckResponse{
			Status:    "offline",
			Message:   "客户端未在统一检测器中注册，已标记离线",
			CheckTime: checkTime,
			Details: map[string]interface{}{
				"connection_exists": false,
				"listener_type":     client.Listener,
				"algorithm":         "unified_offline_detector",
			},
		})
		return
	}

	// 已注册：统一阈值判定
	quick, warning, critical := connection.Thresholds(state.SleepSeconds, state.JitterPercent)
	timeSince := time.Since(state.LastActive)

	var status, message string
	switch {
	case timeSince <= quick:
		status, message = "online", "客户端在线"
	case timeSince <= warning:
		status, message = "possibly_busy", "可能正在执行命令或网络延迟"
	case timeSince <= critical:
		status, message = "likely_offline", "很可能已离线"
	default:
		// 超出统一 critical → 直接走统一离线处理（含 per-protocol cleanup）
		status, message = "offline", "确认离线"
		connection.GlobalOfflineDetector.ForceOffline(uid, "smart检查超阈值")
	}

	c.JSON(http.StatusOK, SmartOfflineCheckResponse{
		Status:    status,
		Message:   message,
		CheckTime: checkTime,
		Details: map[string]interface{}{
			"registered":                 true,
			"listener_type":              state.Listener,
			"last_activity":              state.LastActive,
			"time_since_activity":        timeSince.Seconds(),
			"sleep_seconds":              state.SleepSeconds,
			"jitter_percent":             state.JitterPercent,
			"quick_threshold_seconds":    quick.Seconds(),
			"warning_threshold_seconds":  warning.Seconds(),
			"critical_threshold_seconds": critical.Seconds(),
			"algorithm":                  "unified_offline_detector",
		},
	})
}
