package database

import (
	"database/sql"
	"time"

	"xorm.io/xorm"
)

// Store 存储访问层的统一接口。
//
// 业务代码（pkg/api 等）只依赖这个接口，不再直接引用具体的 `*xorm.Engine`：
//   - 具体实现在 SQLite / PostgreSQL 之间切换（见 pkg/database/dialect.go）；
//   - 所有访问都经过 DB()，便于以后加统一日志/重试/指标，或在替换 ORM 时只需实现本接口；
//   - 回归守卫 `store_access_test.go` 禁止业务代码重新直接引用 database.Engine。
//
// 接口刻意对齐 xorm 的 fluent 风格：Where/ID/Table 返回 *xorm.Session 做链式调用，
// 改动面最小、行为与改造前完全一致。领域方法（如 GetClientByUID）放在本包的
// 领域文件里逐步沉淀，业务层优先用领域方法、其次用通用查询。
type Store interface {
	xorm.Interface

	// 会话与生命周期
	NewSession() *xorm.Session
	Sync2(beans ...interface{}) error
	Sync(beans ...interface{}) error
	IsTableExist(beanOrTableName interface{}) (bool, error)
	Close() error
	Ping() error

	// 连接池
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
	SetConnMaxLifetime(time.Duration)
}

// DB 返回当前存储实例（未初始化时返回 nil）。
//
// 注意返回的是接口：引擎未初始化时返回未类型化的 nil，调用方写
// `if database.DB() == nil` 才成立（直接返回 *xorm.Engine(nil) 会得到"非 nil 接口"）。
func DB() Store {
	engineMu.RLock()
	defer engineMu.RUnlock()
	if Engine == nil {
		return nil
	}
	return Engine
}

// DBStats 返回底层连接池统计（连接数/等待次数），供指标采集使用。
// 放在包里而不是让业务代码调 Engine.DB()，避免把 xorm 的 core.DB 类型带出去。
func DBStats() (sql.DBStats, bool) {
	engineMu.RLock()
	e := Engine
	engineMu.RUnlock()
	if e == nil || e.DB() == nil {
		return sql.DBStats{}, false
	}
	return e.DB().Stats(), true
}
