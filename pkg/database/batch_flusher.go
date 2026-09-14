package database

import "log"

// batchStmt 批量落库语句（供各后台 flusher 使用）
type batchStmt struct {
	sql  string
	args []interface{}
}

// execBatchSQL 将多条 UPDATE 放入单个事务执行：
// SQLite 每条无事务 UPDATE 都是独立写事务（WAL 下含 commit/fsync 开销），
// N 条合并为 1 个事务可显著降低写放大。
// 任一条失败 → 整体回滚并逐条回退执行（保持原实现"尽力写入"语义）。
func execBatchSQL(stmts []batchStmt) {
	if len(stmts) == 0 {
		return
	}

	// 逐条执行（事务失败时的回退路径；xorm Exec(sql, args...) 参数需整体展开）
	runEach := func() {
		for _, s := range stmts {
			args := append([]interface{}{s.sql}, s.args...)
			if _, e := Engine.Exec(args...); e != nil {
				log.Printf("execBatchSQL 回退执行失败: %v", e)
			}
		}
	}

	session := Engine.NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		// 事务开启失败（如锁等待超时）：回退为逐条执行
		log.Printf("execBatchSQL 事务开启失败: %v", err)
		runEach()
		return
	}

	ok := true
	for _, s := range stmts {
		args := append([]interface{}{s.sql}, s.args...)
		if _, e := session.Exec(args...); e != nil {
			ok = false
			log.Printf("execBatchSQL 事务内执行失败: %v", e)
			break
		}
	}
	if ok {
		if e := session.Commit(); e != nil {
			ok = false
			log.Printf("execBatchSQL 提交失败: %v", e)
		}
	}
	if !ok {
		_ = session.Rollback()
		// 回退：逐条执行（原实现语义）
		runEach()
	}
}
