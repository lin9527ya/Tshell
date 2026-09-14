package database

import (
	"sync"
	"time"
)

// 下载进度节流落库：
// 原实现每个下载分块执行一次 UPDATE downloads SET downloaded_size = downloaded_size + ?，
// 大文件 = 数千次独立写事务（SQLite 单写者，写竞争放大）。
// 改为内存累加 + 周期批量刷盘：分块到达只做内存加法，flusher 每 500ms 按 uid 合并一次 UPDATE。
type downloadProgressUpdate struct {
	uid  string
	size int64
}

const (
	dlProgressQueueSize = 8192
	dlProgressPeriod    = 500 * time.Millisecond
)

var (
	dlProgressOnce sync.Once
	dlProgressCh   = make(chan downloadProgressUpdate, dlProgressQueueSize)
)

// QueueDownloadProgress 累计下载进度（内存累加，由后台 flusher 批量落库）。
// 队列满时丢弃本次累加：进度显示略滞后，不影响下载数据正确性。
func QueueDownloadProgress(uid string, size int64) {
	dlProgressOnce.Do(func() {
		go downloadProgressFlusher()
	})
	select {
	case dlProgressCh <- downloadProgressUpdate{uid: uid, size: size}:
	default:
	}
}

// downloadProgressFlusher 周期将累计的下载进度批量刷入数据库（按 uid 合并）。
func downloadProgressFlusher() {
	ticker := time.NewTicker(dlProgressPeriod)
	defer ticker.Stop()
	pending := make(map[string]int64)

	flush := func() {
		if len(pending) == 0 {
			return
		}
		// 【P1】批量合并为单个事务，减少 SQLite 写事务提交次数
		stmts := make([]batchStmt, 0, len(pending))
		for uid, size := range pending {
			stmts = append(stmts, batchStmt{
				sql:  `UPDATE downloads SET downloaded_size = downloaded_size + ? WHERE uid = ?;`,
				args: []interface{}{size, uid},
			})
		}
		execBatchSQL(stmts)
		pending = make(map[string]int64)
	}

	for {
		select {
		case u := <-dlProgressCh:
			pending[u.uid] += u.size
		case <-ticker.C:
			flush()
		}
	}
}
