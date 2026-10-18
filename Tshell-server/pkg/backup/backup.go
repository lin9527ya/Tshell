// Package backup 数据库备份与恢复。
//
// 备份用 SQLite 的 `VACUUM INTO`：它是**在线一致性快照**，不需要停服务，
// 且会把 WAL 里未 checkpoint 的内容一并写入目标文件 —— 直接 `cp database.db`
// 会漏掉 WAL 中的数据，得到一个"看起来正常但少了最近写入"的库。
//
// 恢复必须替换整个数据库文件，只能在服务端未运行时进行（见 Restore 与
// docs/BACKUP_RESTORE.md）；服务端在线时提供的是备份的导出/导入，不是热恢复。
package backup

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 备份文件名：<prefix>-YYYYmmdd-HHMMSS.mmm.db。
// 带毫秒是必须的：保留策略按**文件名字典序**取最新的 N 份，只精确到秒时，
// 同一秒内的多份备份只能靠 -2/-3 这类后缀区分，而 '-'(0x2D) 排在 '.'(0x2E) 之前，
// 字典序就不再等于时间序 —— 保留策略会误删最新的那份（本包的用例复现过）。
const (
	nameTimeLayout = "20060102-150405.000"
	markerSuffix   = ".ok"
	dbSuffix       = ".db"
	// partialSuffix 备份写入中的临时后缀（校验通过后改名去掉它）
	partialSuffix = ".partial"
)

// ErrNotSQLite 备份子系统基于 SQLite 在线快照（VACUUM INTO + 文件替换），
// 不适用于 PostgreSQL。PostgreSQL 部署请使用 pg_dump / pg_basebackup。
var ErrNotSQLite = errors.New("备份子系统仅支持 SQLite；PostgreSQL 部署请使用 pg_dump")

// Options 备份配置（来自 config.backup.*）
type Options struct {
	// Driver 数据库类型（"sqlite3" 默认 / "postgres"）。PostgreSQL 下备份接口返回不可用。
	Driver   string
	DBPath   string        // 数据库文件路径（实际使用中的那个，非配置里的期望值）
	Dir      string        // 备份目录
	Keep     int           // 保留份数（按时间倒序保留最新的 Keep 份）
	Interval time.Duration // 自动备份间隔（<=0 表示不自动备份）
	Prefix   string        // 文件名前缀
}

// Info 一份备份的元信息
type Info struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	// Verified 是否通过完整性校验（创建时校验，结果记在 <name>.ok 里；
	// 文件大小与记录不一致时视为未校验 —— 例如拷贝过程被中断）
	Verified bool `json:"verified"`
	// Note 备份原因/备注（manual / scheduled / pre-restore 等）
	Note string `json:"note,omitempty"`
}

// Manager 备份管理器
type Manager struct {
	opts Options
}

// New 创建管理器（不启动任何后台任务）
func New(opts Options) *Manager {
	if opts.Dir == "" {
		opts.Dir = "./backups"
	}
	if opts.Prefix == "" {
		opts.Prefix = "tshell-backup"
	}
	if opts.Keep <= 0 {
		opts.Keep = 7
	}
	return &Manager{opts: opts}
}

// Options 返回当前配置（只读）
func (m *Manager) Options() Options { return m.opts }

// Create 生成一份备份并校验，然后按保留份数清理旧备份。
func (m *Manager) Create(note string) (Info, error) {
	if strings.EqualFold(m.opts.Driver, "postgres") {
		return Info{}, ErrNotSQLite
	}
	if m.opts.DBPath == "" {
		return Info{}, fmt.Errorf("数据库路径为空")
	}
	if _, err := os.Stat(m.opts.DBPath); err != nil {
		return Info{}, fmt.Errorf("数据库文件不可读: %w", err)
	}
	// 0700：备份里有完整凭据/战利品，不能给同机其他用户读
	if err := os.MkdirAll(m.opts.Dir, 0o700); err != nil {
		return Info{}, fmt.Errorf("创建备份目录失败: %w", err)
	}

	now := time.Now()
	name := m.uniqueName(now)
	dest := filepath.Join(m.opts.Dir, name)
	// 先写临时名，校验通过后再改名为最终名：
	// 备份过程中（VACUUM 可能持续数秒）列表里不会出现"半成品"，崩溃也不会留下
	// 一个看起来能用的 .db —— 未校验的备份比没有备份更危险。
	partial := dest + partialSuffix
	_ = os.Remove(partial)

	if err := vacuumInto(m.opts.DBPath, partial); err != nil {
		_ = os.Remove(partial)
		return Info{}, err
	}
	if err := os.Chmod(partial, 0o600); err != nil {
		_ = os.Remove(partial)
		return Info{}, fmt.Errorf("设置备份文件权限失败: %w", err)
	}

	info := Info{Name: name, Path: dest, CreatedAt: now, Note: note}
	if err := Verify(partial); err != nil {
		// 校验失败的备份没有保留价值，且必须让调用方知道（备份不可用 = 没有备份）
		_ = os.Remove(partial)
		return Info{}, fmt.Errorf("备份完整性校验失败（已删除）: %w", err)
	}
	st, err := os.Stat(partial)
	if err != nil {
		_ = os.Remove(partial)
		return Info{}, err
	}
	info.Size = st.Size()
	info.Verified = true
	// 标记先写（记录的是最终名），再改名：改名后 .db 与 .ok 必然成对出现
	if err := writeMarker(dest, info); err != nil {
		_ = os.Remove(partial)
		return Info{}, err
	}
	if err := os.Rename(partial, dest); err != nil {
		_ = os.Remove(partial)
		_ = os.Remove(dest + markerSuffix)
		return Info{}, err
	}
	return info, nil
}

// uniqueName 生成不与现有备份冲突的文件名。
// 同毫秒内连续备份（手动触发与定时备份撞车）时向后顺延毫秒，保持"名字即时间序"，
// 既不会互相覆盖，也不会让保留策略误删最新的一份。
func (m *Manager) uniqueName(now time.Time) string {
	for i := 0; i < 1000; i++ {
		name := fmt.Sprintf("%s-%s%s", m.opts.Prefix, now.Add(time.Duration(i)*time.Millisecond).Format(nameTimeLayout), dbSuffix)
		if _, err := os.Stat(filepath.Join(m.opts.Dir, name)); os.IsNotExist(err) {
			return name
		}
	}
	// 同毫秒 1000 次备份属于异常情况：退化为计数后缀（此时候选名已不再严格时间序）
	base := fmt.Sprintf("%s-%s", m.opts.Prefix, now.Format(nameTimeLayout))
	for i := 2; ; i++ {
		name := fmt.Sprintf("%s-%d%s", base, i, dbSuffix)
		if _, err := os.Stat(filepath.Join(m.opts.Dir, name)); os.IsNotExist(err) {
			return name
		}
	}
}

// vacuumInto 执行在线一致性快照
func vacuumInto(srcDB, dest string) error {
	// 以只读方式打开源库即可（VACUUM INTO 不修改源库内容，但需要能读 WAL）
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&mode=ro", srcDB)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	// 目标路径作为参数绑定（避免拼接带来的转义问题）
	if _, err := db.Exec("VACUUM INTO ?", dest); err != nil {
		return fmt.Errorf("VACUUM INTO 失败: %w", err)
	}
	return nil
}

// requiredTables 一份"可用的 Tshell 数据库"至少要有这些表。
// 校验它们是为了挡住"结构对但内容不是本系统"的文件（例如误传了别的 sqlite 库）。
var requiredTables = []string{"users", "clients", "listener"}

// Verify 校验备份文件：SQLite 完整性检查 + 必需表存在。
// 这是"备份能不能用来恢复"的唯一判据 —— 没校验过的备份等于没有备份。
func Verify(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.Size() == 0 {
		return fmt.Errorf("备份文件为空: %s", path)
	}
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		return fmt.Errorf("打开备份失败: %w", err)
	}
	defer db.Close()

	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("完整性检查执行失败: %w", err)
	}
	if !strings.EqualFold(result, "ok") {
		return fmt.Errorf("完整性检查未通过: %s", result)
	}
	for _, tbl := range requiredTables {
		var n int
		if err := db.QueryRow(
			"SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&n); err != nil {
			return fmt.Errorf("检查表 %s 失败: %w", tbl, err)
		}
		if n == 0 {
			return fmt.Errorf("备份缺少必需的表 %s（不是本系统的数据库？）", tbl)
		}
	}
	return nil
}

// List 列出备份目录中的备份（按时间倒序）
func (m *Manager) List() ([]Info, error) {
	entries, err := os.ReadDir(m.opts.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Info{}, nil
		}
		return nil, err
	}
	out := make([]Info, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, m.opts.Prefix) || !strings.HasSuffix(name, dbSuffix) {
			continue
		}
		path := filepath.Join(m.opts.Dir, name)
		st, err := e.Info()
		if err != nil {
			continue
		}
		info := Info{Name: name, Path: path, Size: st.Size(), CreatedAt: st.ModTime()}
		info.CreatedAt, info.Note = readMarker(path, info.CreatedAt)
		info.Verified = markerMatches(path, st.Size())
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// Delete 删除指定备份（含校验标记）
func (m *Manager) Delete(name string) error {
	path, err := m.resolve(name)
	if err != nil {
		return err
	}
	_ = os.Remove(path + markerSuffix)
	return os.Remove(path)
}

// Cleanup 按保留份数清理旧备份，返回被删除的文件名。
// 只动本管理器命名规则内的文件 —— 目录里放的其他东西一律不碰。
func (m *Manager) Cleanup() ([]string, error) {
	list, err := m.List()
	if err != nil {
		return nil, err
	}
	if len(list) <= m.opts.Keep {
		return nil, nil
	}
	removed := make([]string, 0, len(list)-m.opts.Keep)
	for _, info := range list[m.opts.Keep:] {
		if err := m.Delete(info.Name); err != nil {
			return removed, err
		}
		removed = append(removed, info.Name)
	}
	return removed, nil
}

// resolve 把备份名解析成目录内的路径（拒绝路径穿越与目录外的名字）
func (m *Manager) resolve(name string) (string, error) {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("非法的备份名: %q", name)
	}
	if !strings.HasPrefix(name, m.opts.Prefix) || !strings.HasSuffix(name, dbSuffix) {
		return "", fmt.Errorf("非法的备份名: %q", name)
	}
	path := filepath.Join(m.opts.Dir, name)
	// 双保险：解析后必须仍在备份目录内
	absDir, err := filepath.Abs(m.opts.Dir)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if filepath.Dir(absPath) != absDir {
		return "", fmt.Errorf("非法的备份名: %q", name)
	}
	return path, nil
}

// Open 按名字打开备份文件（下载用），返回文件句柄与元信息
func (m *Manager) Open(name string) (*os.File, Info, error) {
	path, err := m.resolve(name)
	if err != nil {
		return nil, Info{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, Info{}, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, Info{}, err
	}
	return f, Info{Name: name, Path: path, Size: st.Size(), CreatedAt: st.ModTime()}, nil
}

// Import 把外部上传的备份落到备份目录（先校验再落盘：坏文件不该占着位置）
func (m *Manager) Import(name string, content []byte) (Info, error) {
	if err := os.MkdirAll(m.opts.Dir, 0o700); err != nil {
		return Info{}, err
	}
	dest := filepath.Join(m.opts.Dir, name)
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return Info{}, fmt.Errorf("写入备份失败: %w", err)
	}
	if err := Verify(tmp); err != nil {
		_ = os.Remove(tmp)
		return Info{}, fmt.Errorf("上传的文件不是可用的备份: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return Info{}, err
	}
	st, err := os.Stat(dest)
	if err != nil {
		return Info{}, err
	}
	info := Info{Name: name, Path: dest, Size: st.Size(), CreatedAt: time.Now(), Verified: true, Note: "imported"}
	_ = writeMarker(dest, info)
	return info, nil
}

// 校验标记：<name>.ok 记录"这个文件在什么大小、什么时候通过了校验"。
// 列出时比对大小，能在不重跑 integrity_check 的前提下发现被截断/改动过的文件。
func writeMarker(dbPath string, info Info) error {
	content := fmt.Sprintf("size=%d\ntime=%s\nnote=%s\n", info.Size, info.CreatedAt.Format(time.RFC3339), info.Note)
	return os.WriteFile(dbPath+markerSuffix, []byte(content), 0o600)
}

// readMarker 读取标记里的创建时间与备注（缺失时回退文件时间）
func readMarker(dbPath string, fallback time.Time) (time.Time, string) {
	data, err := os.ReadFile(dbPath + markerSuffix)
	if err != nil {
		return fallback, ""
	}
	created, note := fallback, ""
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "time="):
			if t, err := time.Parse(time.RFC3339, strings.TrimPrefix(line, "time=")); err == nil {
				created = t
			}
		case strings.HasPrefix(line, "note="):
			note = strings.TrimPrefix(line, "note=")
		}
	}
	return created, note
}

// markerMatches 标记里记录的大小是否与当前文件一致
func markerMatches(dbPath string, size int64) bool {
	data, err := os.ReadFile(dbPath + markerSuffix)
	if err != nil {
		return false
	}
	var recorded int64
	if _, err := fmt.Sscanf(strings.TrimSpace(strings.Split(string(data), "\n")[0]), "size=%d", &recorded); err != nil {
		return false
	}
	return recorded == size
}
