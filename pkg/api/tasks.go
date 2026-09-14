package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/sendcommand"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ===== C14: 任务化命令 + 客户端分组 =====

// TaskRecord 批量任务记录（内存态 + 可扩展落库）
type TaskRecord struct {
	ID         string    `json:"id"`
	Command    string    `json:"command"`
	UIDs       []string  `json:"uids"`
	Status     string    `json:"status"` // running | done
	CreatedAt  time.Time `json:"created_at"`
	Results    map[string]string `json:"results"` // uid -> "sent" / 错误信息
}

var (
	taskMu      sync.Mutex
	taskRecords = make(map[string]*TaskRecord)
)

// BatchCommand 批量下发命令到多个客户端
func BatchCommand(c *gin.Context) {
	var req struct {
		UIDs    []string `json:"uids"`
		Command string   `json:"command"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.UIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uids 不能为空"})
		return
	}
	if req.Command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command 不能为空"})
		return
	}

	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	task := &TaskRecord{
		ID:        taskID,
		Command:   req.Command,
		UIDs:      req.UIDs,
		Status:    "running",
		CreatedAt: time.Now(),
		Results:   make(map[string]string),
	}
	taskMu.Lock()
	taskRecords[taskID] = task
	taskMu.Unlock()

	// 异步下发
	go func() {
		for _, uid := range req.UIDs {
			sendcommand.SendCommand(uid, req.Command)
			taskMu.Lock()
			task.Results[uid] = "已下发"
			taskMu.Unlock()
		}
		taskMu.Lock()
		task.Status = "done"
		taskMu.Unlock()
	}()

	c.JSON(http.StatusOK, gin.H{"status": 200, "task_id": taskID, "total": len(req.UIDs)})
}

// ListTasks 列出批量任务
func ListTasks(c *gin.Context) {
	taskMu.Lock()
	list := make([]*TaskRecord, 0, len(taskRecords))
	for _, t := range taskRecords {
		list = append(list, t)
	}
	taskMu.Unlock()
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": list})
}

// taskTTL 批量任务记录保留时长（超过即清理，防止内存无界增长）
const taskTTL = 1 * time.Hour

// startTaskCleanup 启动批量任务记录 TTL 清理 goroutine（由 main 启动一次）
func StartTaskCleanup() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			taskMu.Lock()
			cutoff := time.Now().Add(-taskTTL)
			for id, t := range taskRecords {
				if t.CreatedAt.Before(cutoff) {
					delete(taskRecords, id)
				}
			}
			taskMu.Unlock()
		}
	}()
}

// UpdateClientGroup 设置客户端分组
func UpdateClientGroup(c *gin.Context) {
	var req struct {
		Uid   string `json:"uid"`
		Group string `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uid 必填"})
		return
	}
	_, err := database.Engine.Where("uid = ?", req.Uid).Update(&database.Clients{Group: req.Group})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errors.SanitizeError(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "分组已更新"})
}

// BatchUpdateClientGroup 批量设置客户端分组（单次事务，替代前端逐客户端调用）
func BatchUpdateClientGroup(c *gin.Context) {
	var req struct {
		UIDs  []string `json:"uids"`
		Group string   `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.UIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uids 必填"})
		return
	}
	session := database.Engine.NewSession()
	defer session.Close()
	if err := session.Begin(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errors.SanitizeError(err)})
		return
	}
	for _, uid := range req.UIDs {
		if _, err := session.Where("uid = ?", uid).Update(&database.Clients{Group: req.Group}); err != nil {
			session.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"error": errors.SanitizeError(err)})
			return
		}
	}
	if err := session.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errors.SanitizeError(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": fmt.Sprintf("已为 %d 个客户端设置分组", len(req.UIDs))})
}

// GetClientTaskQueue 单会话任务队列可视化:命令下发时间线 + 队列长度
func GetClientTaskQueue(c *gin.Context) {
	uid := c.Param("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "缺少 uid"})
		return
	}
	records, lastPull, lastSeen := command.GlobalCmdHistory.Snapshot(uid)
	queueLen := command.CommandQueues.QueueLen(uid)
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{
		"uid":         uid,
		"queue_len":   queueLen,
		"commands":    records,
		"last_pull_at": lastPull.Format("2006-01-02 15:04:05"),
		"last_seen_at": lastSeen.Format("2006-01-02 15:04:05"),
	}})
}
