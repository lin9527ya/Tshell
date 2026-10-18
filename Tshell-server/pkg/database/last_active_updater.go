package database

import (
	"sync"
	"time"
)

// 客户端 LastActive 异步批量落库：
// 原实现在每个"客户端列表"请求里对每个在线客户端逐条同步 UPDATE（N+1 写放大），
// 改为缓冲 channel + 后台批量刷盘 goroutine，请求路径只做入队，不阻塞。
type lastActiveUpdate struct {
	uid string
	ts  string
}

const (
	lastActiveQueueSize   = 4096
	lastActiveFlushBatch  = 128
	lastActiveFlushPeriod = 500 * time.Millisecond
)

var (
	lastActiveOnce sync.Once
	lastActiveCh   = make(chan lastActiveUpdate, lastActiveQueueSize)
)

// QueueLastActiveUpdate 异步排队更新客户端最后活跃时间（uid 唯一，重复入队只保留最新值）。
// 队列满时丢弃本次更新：心跳时间持续推进，下次列表刷新会重新入队，属可接受的近似。
func QueueLastActiveUpdate(uid, ts string) {
	lastActiveOnce.Do(func() {
		go lastActiveFlusher()
	})
	select {
	case lastActiveCh <- lastActiveUpdate{uid: uid, ts: ts}:
	default:
	}
}

// lastActiveFlusher 批量刷盘：按 uid 合并去重后执行 UPDATE，减少 SQLite 写事务次数。
func lastActiveFlusher() {
	ticker := time.NewTicker(lastActiveFlushPeriod)
	defer ticker.Stop()
	pending := make(map[string]string)

	flush := func() {
		if len(pending) == 0 {
			return
		}
		// 【P1】批量合并为单个事务，减少 SQLite 写事务提交次数
		stmts := make([]batchStmt, 0, len(pending))
		for uid, ts := range pending {
			stmts = append(stmts, batchStmt{
				sql:  `UPDATE clients SET last_active = ? WHERE uid = ?`,
				args: []interface{}{ts, uid},
			})
		}
		execBatchSQL(stmts)
		pending = make(map[string]string)
	}

	for {
		select {
		case u := <-lastActiveCh:
			pending[u.uid] = u.ts
			if len(pending) >= lastActiveFlushBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
