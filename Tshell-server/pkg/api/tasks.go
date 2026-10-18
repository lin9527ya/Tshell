package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/metrics"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils/logx"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"xorm.io/xorm"
)

// ===== C14: 任务一等对象（DB 持久化：可查询 / 可取消 / 可审计 / 重启不丢） =====
//
// 旧实现是内存 map + 1h TTL、状态只有 running/done、Results 永远写"已下发"——
// 批量任务页的进度因此恒为 100%，也无法回答"谁在什么时候对哪个 beacon 下发了什么"。
// 现在每个 (任务组, uid) 一行落库；完成态由命令回包近似归属（协议无逐命令 ACK）。

const (
	taskStatusDispatched = "dispatched"
	taskStatusCompleted  = "completed"
	taskStatusFailed     = "failed"
	taskStatusCancelled  = "cancelled"
	taskStatusTimeout    = "timeout"

	// taskStaleTimeout 已下发但长时间无回显 → 标记 timeout（清理协程周期扫描）
	taskStaleTimeout = 6 * time.Hour
	// taskRetention 终态任务的保留时长
	taskRetention = 7 * 24 * time.Hour
	// taskResultMaxBytes 单条任务结果落库上限（大段输出截断）
	taskResultMaxBytes = 64 << 10
)

func newTaskBatchID() string { return fmt.Sprintf("task_%d", time.Now().UnixNano()) }

// operatorFromContext 取当前操作员用户名（审计与任务归属用；取不到返回空）。
func operatorFromContext(c *gin.Context) string {
	if u, exists := c.Get("username"); exists {
		if s, ok := u.(string); ok {
			return s
		}
	}
	return ""
}

// createTaskRows 为每个 uid 建一条 dispatched 任务，返回任务组 ID 与实际建成的行。
func createTaskRows(uids []string, operator, kind, command string) (string, []*database.Task) {
	if database.Engine == nil {
		return "", nil
	}
	batchID := newTaskBatchID()
	now := time.Now()
	rows := make([]*database.Task, 0, len(uids))
	for _, uid := range uids {
		uid = strings.TrimSpace(uid)
		if uid == "" {
			continue
		}
		task := &database.Task{
			BatchID: batchID, UID: uid, Operator: operator, Kind: kind,
			Command: command, Status: taskStatusDispatched, CreatedAt: now, CreatedUnix: now.Unix(),
		}
		if _, err := database.Engine.Insert(task); err != nil {
			logx.Warnf("[TASK] 任务落库失败 uid=%s: %v\n", uid, err)
			continue
		}
		metrics.TaskCreated(kind)
		rows = append(rows, task)
	}
	return batchID, rows
}

// finishTask 写入终态（只对 dispatched 生效，避免覆盖已终态；幂等）。
func finishTask(id int64, status, result, errMsg string) {
	if database.Engine == nil || id <= 0 {
		return
	}
	if len(result) > taskResultMaxBytes {
		result = result[:taskResultMaxBytes] + "\n...(已截断)"
	}
	// 读一次取 kind/创建时间供指标用；终态判定仍由下面 UPDATE 的 WHERE 原子保证
	var snapshot database.Task
	hasSnapshot, _ := database.Engine.ID(id).Get(&snapshot)

	res, err := database.Engine.Table(new(database.Task)).
		Where("id = ? AND status = ?", id, taskStatusDispatched).
		Update(map[string]interface{}{
			"status": status, "result": result, "error": errMsg, "finished_at": time.Now(),
		})
	if err != nil {
		logx.Warnf("[TASK] 更新任务终态失败 id=%d: %v\n", id, err)
		return
	}
	// res=0 表示任务已是终态（重复回调），不重复计数
	if res > 0 && hasSnapshot {
		metrics.TaskFinished(snapshot.Kind, status, time.Since(snapshot.CreatedAt))
	}
}

// OnTaskOutput 命令回包到达：把该 uid 最早一条 dispatched 任务标为 completed。
// 协议没有逐命令 ACK / 请求 ID，无法精确归属；按"先下发先回显"近似
// （同一 uid 上并发多条命令时可能错位——这是协议层限制，注释在此说明）。
func OnTaskOutput(uid, output string) {
	if database.Engine == nil || uid == "" {
		return
	}
	var task database.Task
	has, err := database.Engine.Where("uid = ? AND status = ?", uid, taskStatusDispatched).
		OrderBy("id").Get(&task)
	if err != nil || !has {
		return
	}
	finishTask(task.Id, taskStatusCompleted, output, "")
}

// taskResultLabel 给旧版任务视图（按 uid 的 results）用的状态文案。
func taskResultLabel(t database.Task) string {
	switch t.Status {
	case taskStatusCompleted:
		if strings.TrimSpace(t.Result) == "" {
			return "已完成（无回显）"
		}
		return t.Result
	case taskStatusFailed:
		return "失败: " + t.Error
	case taskStatusCancelled:
		return "已取消（命令可能已下发，无法撤回）"
	case taskStatusTimeout:
		return "超时（未收到回显）"
	default:
		return "已下发"
	}
}

// TaskRecord 批量任务视图（按 BatchID 聚合，保持旧前端契约）
type TaskRecord struct {
	ID        string            `json:"id"`
	Command   string            `json:"command"`
	UIDs      []string          `json:"uids"`
	Status    string            `json:"status"` // running | done
	CreatedAt time.Time         `json:"created_at"`
	Results   map[string]string `json:"results"` // uid -> 结果摘要/失败原因
}

// BatchCommand 批量下发命令到多个客户端（每个 uid 一条任务记录）
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

	// 协作：批量里任一 uid 被他人锁定 → 整批拒绝（避免"部分下发"造成的状态歧义）
	if !checkSessionLocks(c, req.UIDs) {
		return
	}

	batchID, rows := createTaskRows(req.UIDs, operatorFromContext(c), "batch", req.Command)
	if len(rows) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "任务创建失败（数据库不可用？）"})
		return
	}

	// 异步下发：逐 uid 记录真实投递结果（队列满/连接不存在 → failed）
	go func() {
		for _, task := range rows {
			if err := sendcommand.SendCommandChecked(task.UID, req.Command); err != nil {
				finishTask(task.Id, taskStatusFailed, "", err.Error())
			}
		}
	}()

	publishOperatorAction("", operatorFromContext(c), "batch",
		fmt.Sprintf("对 %d 台主机下发: %s", len(rows), truncateForEvent(req.Command)))

	c.JSON(http.StatusOK, gin.H{"status": 200, "task_id": batchID, "total": len(rows)})
}

// GetTaskRow 单条任务（GET /api/tasks/:id）——前端按 id 轮询终态用。
func GetTaskRow(c *gin.Context) {
	if database.Engine == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库不可用"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}
	var task database.Task
	has, err := database.Engine.ID(id).Get(&task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败: " + err.Error()})
		return
	}
	if !has {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": task})
}

// OnDownloadEnd 下载终态回调（由 pkg/command 在下载完成/中断时调用）。
// 下载没有"结束"报文，此前只有一行字节进度；这里把它收敛成任务的终态，
// 让任务中心能回答"这个文件下完了没有"。
func OnDownloadEnd(uid, filePath, status string) {
	if database.Engine == nil || uid == "" || filePath == "" {
		return
	}
	// 凭据制品优先：命中凭据请求的产物时按凭据链路收敛（校验/登记/任务/审计），
	// 不再当作普通下载处理（它本来也没有对应的 kind=download 任务）
	if matchCredArtifact(uid, filePath, status) {
		return
	}
	// 同一 uid 同一路径可能重复下载：只收敛最早一条仍在 dispatched 的记录
	var task database.Task
	has, err := database.Engine.Where("uid = ? AND kind = ? AND command = ? AND status = ?",
		uid, "download", "download "+filePath, taskStatusDispatched).
		OrderBy("id ASC").Get(&task)
	if err != nil || !has {
		return
	}
	if status == command.DownloadStatusCompleted {
		finishTask(task.Id, taskStatusCompleted, "已保存至服务端 Downloads 目录: "+filePath, "")
		return
	}
	finishTask(task.Id, taskStatusFailed, "", "下载未完成（客户端断开或长时间无数据）: "+filePath)
}

// ListTasks 批量任务视图（按 BatchID 聚合，兼容旧前端契约）。
// 数据来自 task 表；status=done 表示该组所有 uid 都已终态。
func ListTasks(c *gin.Context) {
	if database.Engine == nil {
		c.JSON(http.StatusOK, gin.H{"status": 200, "data": []TaskRecord{}})
		return
	}
	var rows []database.Task
	if err := database.Engine.OrderBy("id DESC").Limit(1000).Find(&rows); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败: " + err.Error()})
		return
	}

	order := make([]string, 0)
	byBatch := make(map[string]*TaskRecord)
	running := make(map[string]bool)
	for _, t := range rows {
		rec, ok := byBatch[t.BatchID]
		if !ok {
			rec = &TaskRecord{
				ID: t.BatchID, Command: t.Command, CreatedAt: t.CreatedAt,
				Results: make(map[string]string),
			}
			byBatch[t.BatchID] = rec
			order = append(order, t.BatchID)
		}
		rec.UIDs = append(rec.UIDs, t.UID)
		rec.Results[t.UID] = taskResultLabel(t)
		if t.Status == taskStatusDispatched {
			running[t.BatchID] = true
		}
	}
	list := make([]*TaskRecord, 0, len(order))
	for _, id := range order {
		rec := byBatch[id]
		if running[id] {
			rec.Status = "running"
		} else {
			rec.Status = "done"
		}
		list = append(list, rec)
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": list})
}

// ListTaskRows 任务明细（可过滤、分页）——任务中心用。
// GET /api/tasks?uid=&status=&kind=&page=&size=
func ListTaskRows(c *gin.Context) {
	if database.Engine == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库不可用"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 1000 {
		size = 100
	}
	// xorm.Session 不能 Clone，条件构造抽成闭包，统计与查询各建一个 session
	applyFilters := func() *xorm.Session {
		sess := database.Engine.Where("1=1")
		if uid := strings.TrimSpace(c.Query("uid")); uid != "" {
			sess = sess.And("uid = ?", uid)
		}
		if st := strings.TrimSpace(c.Query("status")); st != "" {
			sess = sess.And("status = ?", st)
		}
		if kind := strings.TrimSpace(c.Query("kind")); kind != "" {
			sess = sess.And("kind = ?", kind)
		}
		return sess
	}
	total, err := applyFilters().Count(new(database.Task))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "统计任务失败: " + err.Error()})
		return
	}
	var list []database.Task
	if err := applyFilters().OrderBy("id DESC").Limit(size, (page-1)*size).Find(&list); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询任务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{
		"total": total, "page": page, "size": size, "list": list,
	}})
}

// CancelTask 取消一条尚未回显的任务（POST /api/tasks/:id/cancel）。
// 说明：命令一旦下发无法从客户端撤回（协议无撤销），队列型客户端的命令块也没有
// 请求 ID、无法从队列中摘除 —— 这里只把任务标记为 cancelled，如实表达"不再等待回显"。
func CancelTask(c *gin.Context) {
	if database.Engine == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "数据库不可用"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}
	res, err := database.Engine.Table(new(database.Task)).
		Where("id = ? AND status = ?", id, taskStatusDispatched).
		Update(map[string]interface{}{"status": taskStatusCancelled, "finished_at": time.Now()})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "取消失败: " + err.Error()})
		return
	}
	if res == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "任务不存在或已结束（只能取消 dispatched 状态）"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": 200, "message": "任务已取消（已下发的命令无法撤回）"})
}

// StartTaskCleanup 周期清理：滞留的 dispatched 标 timeout；超期终态任务删除。
func StartTaskCleanup() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if database.Engine == nil {
				continue
			}
			// 超时收敛：批量 UPDATE 拿不到逐条的 kind，指标按"条数"计入 unknown
			// （这里只用于观察"有多少任务在滞留"，不参与按类型的吞吐分析）
			if n, err := database.Engine.Table(new(database.Task)).
				Where("status = ? AND created_unix < ?", taskStatusDispatched, time.Now().Add(-taskStaleTimeout).Unix()).
				Update(map[string]interface{}{"status": taskStatusTimeout, "finished_at": time.Now()}); err != nil {
				logx.Warnf("[TASK] 超时任务清理失败: %v\n", err)
			} else if n > 0 {
				for i := int64(0); i < n; i++ {
					metrics.TaskFinished("unknown", taskStatusTimeout, 0)
				}
			}
			if _, err := database.Engine.
				Where("status != ? AND created_unix < ?", taskStatusDispatched, time.Now().Add(-taskRetention).Unix()).
				Delete(new(database.Task)); err != nil {
				logx.Warnf("[TASK] 过期任务清理失败: %v\n", err)
			}
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
