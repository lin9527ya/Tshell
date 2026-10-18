package api

import (
	"BackendTemplate/pkg/collab"
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/metrics"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ===== 多操作员协作 =====
//
// 三件事：
//  1. 在线状态（presence）：任一已认证请求都会 Touch 续期（见 AuthMiddleware），
//     超 PresenceTTL 视为离线；
//  2. 会话锁：操作员在动某台 beacon 前显式声明占用，别人再对它下命令会被拒（409）；
//  3. 操作广播：锁与写操作经事件总线推给所有前端（operator_action 事件）。
//
// 锁是"软"的：无人持锁时行为与从前完全一致，不会因为引入协作而阻断单操作员流程。

// ListOperators 在线操作员（GET /api/collab/operators）
func ListOperators(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": collab.Global.Online()})
}

// ListSessionLocks 当前会话锁（GET /api/collab/locks）
func ListSessionLocks(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": collab.Global.Locks()})
}

// AcquireSessionLock 声明占用会话（POST /api/collab/locks/:uid）
// 幂等：同一操作员重复调用为续期；被他人持有时 409 并返回持有人。
func AcquireSessionLock(c *gin.Context) {
	uid := c.Param("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "error": "缺少 uid"})
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body) // body 可选

	operator := operatorFromContext(c)
	lock, ok, holder := collab.Global.Acquire(uid, operator, body.Reason)
	if !ok {
		c.JSON(http.StatusConflict, gin.H{
			"status":    http.StatusConflict,
			"error":     "会话已被 " + holder + " 锁定",
			"locked_by": holder,
			"data":      lock,
		})
		return
	}
	publishOperatorAction(uid, operator, "lock", "占用会话")
	auditLogEvent("SESSION_LOCK", "SUCCESS", operator+" 锁定会话 "+uid, map[string]interface{}{"uid": uid})
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": lock})
}

// ReleaseSessionLock 释放会话（DELETE /api/collab/locks/:uid）
// 仅持有人可释放；未持有（含已过期）返回 404，便于前端清理本地状态。
func ReleaseSessionLock(c *gin.Context) {
	uid := c.Param("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": 400, "error": "缺少 uid"})
		return
	}
	operator := operatorFromContext(c)
	if !collab.Global.Release(uid, operator) {
		c.JSON(http.StatusNotFound, gin.H{"status": 404, "error": "未持有该会话锁（或已过期）"})
		return
	}
	publishOperatorAction(uid, operator, "unlock", "释放会话")
	auditLogEvent("SESSION_LOCK", "SUCCESS", operator+" 释放会话 "+uid, map[string]interface{}{"uid": uid})
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

// checkSessionLock 会话锁软校验：被其他操作员持有时拒绝本次写操作。
// 返回 false 时已写好 409 响应，调用方直接 return 即可。
func checkSessionLock(c *gin.Context, uid string) bool {
	holder := collab.Global.Holder(uid)
	if holder == "" {
		return true
	}
	if operator := operatorFromContext(c); operator == holder {
		return true
	}
	c.JSON(http.StatusConflict, gin.H{
		"status":    http.StatusConflict,
		"error":     "会话已被 " + holder + " 锁定，请先与其协调或等待锁过期",
		"locked_by": holder,
	})
	return false
}

// checkSessionLocks 批量场景：任一 uid 被他人锁定即整体拒绝（返回被占用的那个）。
func checkSessionLocks(c *gin.Context, uids []string) bool {
	operator := operatorFromContext(c)
	for _, uid := range uids {
		holder := collab.Global.Holder(uid)
		if holder != "" && holder != operator {
			c.JSON(http.StatusConflict, gin.H{
				"status":     http.StatusConflict,
				"error":      "会话 " + uid + " 已被 " + holder + " 锁定，批量下发已取消",
				"locked_by":  holder,
				"locked_uid": uid,
			})
			return false
		}
	}
	return true
}

// truncateForEvent 截断广播内容（命令可能很长，广播只用于"看到有人在做什么"）
func truncateForEvent(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}

// publishOperatorAction 广播操作员动作（锁/命令/高危操作），供前端实时展示。
func publishOperatorAction(uid, operator, action, detail string) {
	if operator == "" {
		return
	}
	metrics.OperatorAction(action)
	command.PublishEvent(command.EventOperatorAction, uid, map[string]interface{}{
		"operator": operator,
		"action":   action,
		"detail":   detail,
		"at":       time.Now().Unix(),
	})
}
