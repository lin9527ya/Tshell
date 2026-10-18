package audit

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/utils/logx"

	"xorm.io/xorm"
)

// ===== 审计日志落库（与文件双写）=====
//
// 文件保留"人读 + 原始导出"；库用于过滤/分页查询与统计。
// 原查询路径是"读最近 30 个文件 → 全量 JSON 解析 → 内存过滤排序 → 切片分页"，
// 规模上 O(全部日志)，且落盘队列满时整批丢弃 —— 落库后查询走索引，审计不再随负载丢失。

// errAuditDBNotReady 数据库未初始化（审计查询/写入直接返回，不影响调用方主链路）
var errAuditDBNotReady = fmt.Errorf("审计日志库未初始化")

// persistBatch 将一批审计日志写入 AuditLogDB（由 flushBatch 在文件落盘后调用）。
// 失败仅记 debug 日志，不影响文件主链路。
func (al *AuditLogger) persistBatch(batch []AuditLog) {
	if database.DB() == nil || len(batch) == 0 {
		return
	}
	rows := make([]database.AuditLogDB, 0, len(batch))
	for _, l := range batch {
		rows = append(rows, auditLogToDB(l))
	}
	if _, err := database.DB().Insert(&rows); err == nil {
		return
	}
	// 整批失败（多为 LogID 唯一索引冲突，如重放）：逐条插入并跳过冲突项
	for i := range rows {
		if _, err := database.DB().Insert(&rows[i]); err != nil {
			logx.Debugf("[AUDIT] 日志入库失败（已跳过）: %s: %v\n", rows[i].LogID, err)
		}
	}
}

func auditLogToDB(l AuditLog) database.AuditLogDB {
	details := ""
	if len(l.Details) > 0 {
		if b, err := json.Marshal(l.Details); err == nil {
			details = string(b)
		}
	}
	return database.AuditLogDB{
		LogID: l.ID, Timestamp: l.Timestamp,
		Level: string(l.Level), Category: string(l.Category), Action: l.Action,
		UserID: l.UserID, Username: l.Username, ClientIP: l.ClientIP, UserAgent: l.UserAgent,
		RequestURI: l.RequestURI, Method: l.Method, StatusCode: l.StatusCode,
		Duration: l.Duration.Milliseconds(), Result: l.Result,
		Message: l.Message, ErrorMsg: l.ErrorMsg, Details: details,
		CreatedAt: time.Now(), TsUnix: l.Timestamp.Unix(),
	}
}

func dbToAuditLog(r database.AuditLogDB) AuditLog {
	out := AuditLog{
		ID: r.LogID, Timestamp: r.Timestamp,
		Level: LogLevel(r.Level), Category: LogCategory(r.Category), Action: r.Action,
		UserID: r.UserID, Username: r.Username, ClientIP: r.ClientIP, UserAgent: r.UserAgent,
		RequestURI: r.RequestURI, Method: r.Method, StatusCode: r.StatusCode,
		Duration: time.Duration(r.Duration) * time.Millisecond,
		Result:   r.Result, Message: r.Message, ErrorMsg: r.ErrorMsg,
	}
	if r.Details != "" {
		_ = json.Unmarshal([]byte(r.Details), &out.Details)
	}
	return out
}

// applyLogFilters 把 LogQueryParams 映射到 SQL 条件（语义与旧的文件版 matchesFilters 一致：
// 时间/级别/分类/结果精确匹配，操作/用户/IP/关键词为包含匹配）。
func applyLogFilters(sess *xorm.Session, params LogQueryParams) *xorm.Session {
	// 范围查询走数值列（DATETIME 列在 SQLite 上与时间/字符串参数比较不可靠）
	if params.StartTime != nil {
		sess = sess.And("ts_unix >= ?", params.StartTime.Unix())
	}
	if params.EndTime != nil {
		sess = sess.And("ts_unix <= ?", params.EndTime.Unix())
	}
	if params.Level != "" {
		sess = sess.And("level = ?", string(params.Level))
	}
	if params.Category != "" {
		sess = sess.And("category = ?", string(params.Category))
	}
	if params.Result != "" {
		sess = sess.And("result = ?", params.Result)
	}
	if params.Action != "" {
		sess = sess.And("action LIKE ?", "%"+params.Action+"%")
	}
	if params.Username != "" {
		sess = sess.And("username LIKE ?", "%"+params.Username+"%")
	}
	if params.ClientIP != "" {
		sess = sess.And("client_ip LIKE ?", "%"+params.ClientIP+"%")
	}
	if kw := strings.TrimSpace(params.Keyword); kw != "" {
		like := "%" + kw + "%"
		sess = sess.And("(message LIKE ? OR action LIKE ? OR username LIKE ? OR client_ip LIKE ?)",
			like, like, like, like)
	}
	return sess
}

// queryLogsFromDB 从库中按条件查询并分页（时间倒序）。
func queryLogsFromDB(params LogQueryParams) ([]AuditLog, int, error) {
	if database.DB() == nil {
		return nil, 0, errAuditDBNotReady
	}
	total, err := applyLogFilters(database.DB().Where("1=1"), params).Count(new(database.AuditLogDB))
	if err != nil {
		return nil, 0, err
	}
	var rows []database.AuditLogDB
	if err := applyLogFilters(database.DB().Where("1=1"), params).
		OrderBy("ts_unix DESC, id DESC").
		Limit(params.PageSize, (params.Page-1)*params.PageSize).
		Find(&rows); err != nil {
		return nil, 0, err
	}
	out := make([]AuditLog, 0, len(rows))
	for _, r := range rows {
		out = append(out, dbToAuditLog(r))
	}
	return out, int(total), nil
}

// cleanupDBBefore 删除早于 cutoff 的库内审计日志（与文件清理同步执行）。
func cleanupDBBefore(cutoff time.Time) (int64, error) {
	if database.DB() == nil {
		return 0, nil
	}
	return database.DB().Where("ts_unix < ?", cutoff.Unix()).Delete(new(database.AuditLogDB))
}
