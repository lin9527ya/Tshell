package database

import (
	"sync"
	"time"
)

// ShellContent 异步批量落库（P1-5）：
// 原实现在每条命令输出上同步执行 UPDATE（高频 shell 会话 + 多客户端时 SQLite 写锁竞争严重），
// 改为缓冲 channel + 后台批量刷盘 goroutine：同一 uid 的多次追加合并为一条 UPDATE，
// 调用路径只做入队，不阻塞。前端 shell 内容读取按秒级轮询，500ms 落库延迟无感知。
type shellAppend struct {
	uid     string
	content string
}

const (
	shellQueueSize    = 8192
	shellFlushPeriod  = 500 * time.Millisecond
	shellFlushMaxUids = 256
)

var (
	shellOnce sync.Once
	shellCh   = make(chan shellAppend, shellQueueSize)
)

// AppendShellContent 增量追加 shell 内容（异步批量：同 uid 内容合并落库）。
// 队列满时丢弃本次追加（高负载保护；前端展示层有独立的输出缓冲，不依赖本持久化实时性）。
func AppendShellContent(uid string, content string) {
	shellOnce.Do(func() {
		go shellFlusher()
	})
	select {
	case shellCh <- shellAppend{uid: uid, content: content}:
	default:
	}
}

// shellFlusher 批量刷盘：按 uid 合并追加后逐条 UPDATE（同 uid 内容保持追加顺序）。
func shellFlusher() {
	ticker := time.NewTicker(shellFlushPeriod)
	defer ticker.Stop()
	pending := make(map[string]string)
	pendingCount := 0

	flush := func() {
		if len(pending) == 0 {
			return
		}
		// 【P1】批量合并为单个事务，减少 SQLite 写事务提交次数
		const maxShellContentSize = 5 * 1024 * 1024 // 与 ShellContent 字段上限一致
		stmts := make([]batchStmt, 0, len(pending))
		for uid, content := range pending {
			stmts = append(stmts, batchStmt{
				sql:  "UPDATE `shell` SET `shell_content` = substr(`shell_content` || ?, -? ) WHERE `uid` = ?",
				args: []interface{}{content, maxShellContentSize, uid},
			})
		}
		execBatchSQL(stmts)
		pending = make(map[string]string)
		pendingCount = 0
	}

	for {
		select {
		case a := <-shellCh:
			if _, ok := pending[a.uid]; !ok {
				pendingCount++
			}
			pending[a.uid] += a.content
			if pendingCount >= shellFlushMaxUids {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
