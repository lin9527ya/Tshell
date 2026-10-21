package database

import (
	"fmt"
	"strings"
	"time"

	"xorm.io/xorm"
)

// ===== 领域存储层（阶段三 §3.7）=====
//
// 为什么把 xorm 链式查询收口到这里：
//
//  1. **同一个实体只能有一种查法**。收口前"按地址查监听器"在不同文件里被写成
//     `Where("listen_address = ?")`、`Like("%"+addr)`、`Find` 后自己遍历三种；
//     webdelivery.go 里 "id → public_key → listening_port" 这条三段式查找链更是
//     被复制了六遍。这类重复是漂移 bug 的温床（改一处忘一处），收到一个方法里
//     就只可能对一次。
//  2. **级联删除有单一出口**：删投递要同时清访问日志、删客户端要同时清备注/下载/
//     战利品，散在调用方必然漏。
//  3. 业务层不再拼 SQL，回归守卫（store_access_test.go）用棘轮锁住这条边界。
//
// 命名约定：`Xxxs()` 返回领域入口（复数形式，避免与同包实体类型 Listener/Clients
// 等重名）；实体类型名本身保持不变。
//
// 统一错误：引擎未初始化时返回 ErrStoreUnavailable，业务层用 errors.Is 区分
// "环境问题"与"数据问题"。

// ErrStoreUnavailable 数据库引擎未初始化
var ErrStoreUnavailable = fmt.Errorf("数据库引擎未初始化")

func nowStamp() string { return time.Now().Format("2006-01-02 15:04:05") }

// ===== 监听器 =====

// ListenerStore 监听器领域访问
type ListenerStore struct{}

// Listeners 监听器领域方法入口
func Listeners() ListenerStore { return ListenerStore{} }

// ExistsByAddress 按监听地址精确判重。
//
// **必须精确匹配**：原实现用 LIKE 子串匹配，"0.0.0.0:808" 会撞上已存在的
// "0.0.0.0:8080"（地址里的 % / _ 还会被当通配符）。其余接口一律按 listen_address
// 精确查库，判重也必须精确，否则前端看到"已存在"却删不掉。
func (ListenerStore) ExistsByAddress(addr string) (bool, error) {
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	return s.Where("listen_address = ?", addr).Exist(&Listener{})
}

// GetByAddress 按监听地址精确取一条
func (ListenerStore) GetByAddress(addr string) (Listener, bool, error) {
	var l Listener
	s := DB()
	if s == nil {
		return l, false, ErrStoreUnavailable
	}
	has, err := s.Where("listen_address = ?", addr).Get(&l)
	return l, has, err
}

// GetByID 按主键取一条
func (ListenerStore) GetByID(id int64) (Listener, bool, error) {
	var l Listener
	s := DB()
	if s == nil {
		return l, false, ErrStoreUnavailable
	}
	has, err := s.ID(id).Get(&l)
	return l, has, err
}

// Insert 新建监听器
func (ListenerStore) Insert(l *Listener) error {
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	_, err := s.Insert(l)
	return err
}

// UpdateAllCols 全列更新（编辑保存用：调用方已把"留空即保留"合并进结构体）。
//
// id 必须显式传入，不能从 l.Id 取：调用方构造的更新结构体通常**只填要改的列**，
// Id 是零值 —— 用 l.Id 会变成 ID(0) 更新零行（表现为"保存了但没生效"，
// 而 affected 又不报错）。这里把 id 做成参数，让这个坑在签名上就暴露。
func (ListenerStore) UpdateAllCols(id int64, l *Listener) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).AllCols().Update(l)
}

// UpdateBeaconOverrides 单独更新监听器级 beacon 覆盖值（nil = 未配置/NULL）。
//
// 不能只依赖 UpdateAllCols：xorm 对 nil 指针字段会跳过，无法把已有值写回 NULL，
// 于是"清空为继承"会静默失效。这里用参数化 UPDATE，nil 参数明确落成 SQL NULL。
func (ListenerStore) UpdateBeaconOverrides(id int64, sleepSeconds, jitterPercent *int) error {
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	// 直接用 Exec 而不是 xorm 的 map Update：nil 参数在这里就是 SQL NULL，
	// 行为在 SQLite/PostgreSQL 上一致（map Update 会把 nil 当成"跳过该列"）。
	var sleepVal, jitterVal interface{}
	if sleepSeconds != nil {
		sleepVal = *sleepSeconds
	}
	if jitterPercent != nil {
		jitterVal = *jitterPercent
	}
	_, err := s.Exec("UPDATE listener SET beacon_sleep_seconds = ?, beacon_jitter_percent = ? WHERE id = ?",
		sleepVal, jitterVal, id)
	return err
}

// UpdateStatus 只改状态列（避免全列更新把密钥等字段写成零值）
func (ListenerStore) UpdateStatus(addr string, status int) error {
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	_, err := s.Where("listen_address = ?", addr).Update(&Listener{Status: status})
	return err
}

// ListOpen 启用中的监听器（status=1）
func (ListenerStore) ListOpen() ([]Listener, error) {
	var out []Listener
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Where("status = ?", 1).Find(&out)
}

// ExistsByConnectAddress 按 connect_address 判存在
func (ListenerStore) ExistsByConnectAddress(addr string) (bool, error) {
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	return s.Where("connect_address = ?", addr).Exist(&Listener{})
}

// List 全部监听器（按 id 升序，保证前端展示顺序稳定）
func (ListenerStore) List() ([]Listener, error) {
	var out []Listener
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Asc("id").Find(&out)
}

// ListByType 按协议类型取监听器
func (ListenerStore) ListByType(listenerType string) ([]Listener, error) {
	var out []Listener
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Where("type = ?", listenerType).Asc("id").Find(&out)
}

// UpdateColumnsByAddress 按地址只更新指定列
func (ListenerStore) UpdateColumnsByAddress(addr string, cols []string, patch *Listener) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("listen_address = ?", addr).Cols(cols...).Update(patch)
}

// Delete 删除监听器记录
func (ListenerStore) Delete(addr string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("listen_address = ?", addr).Delete(&Listener{})
}

// FindByConnectOrListen 按连接地址或监听地址查（客户端归属解析：同类型多实例时
// 按类型查只能取到任意一行，必须按地址精确匹配）
func (ListenerStore) FindByConnectOrListen(addr string) (Listener, bool, error) {
	var l Listener
	s := DB()
	if s == nil {
		return l, false, ErrStoreUnavailable
	}
	if addr == "" {
		return l, false, nil
	}
	has, err := s.Where("connect_address = ? OR listen_address = ?", addr, addr).Get(&l)
	return l, has, err
}

// LatestByType 按类型取最新一条（查不到具体地址时的既有兜底行为）
func (ListenerStore) LatestByType(listenerType string) (Listener, bool, error) {
	var l Listener
	s := DB()
	if s == nil {
		return l, false, ErrStoreUnavailable
	}
	has, err := s.Where("type = ?", listenerType).Desc("id").Get(&l)
	return l, has, err
}

// ExistsByTypeAndAddrVariants 监听器存在性检查：类型匹配 + 地址命中四种写法之一
// （connect_address / listen_address / "0.0.0.0:port" / ":port"）。
//
// 为什么收口：这四种变体的拼接规则原来直接写在调用处的 SQL 字符串里，还带两次
// LastIndex 取端口——写错一个拼接就是"投递开启时总报监听器不存在"。放进方法里
// 只写一次，也便于单测。
func (ListenerStore) ExistsByTypeAndAddrVariants(listenerType, addr string) (bool, error) {
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return s.Where("type = ? AND (connect_address = ? OR listen_address = ?)",
			listenerType, addr, addr).Exist(&Listener{})
	}
	wildcard := "0.0.0.0" + addr[idx:]
	justPort := ":" + addr[idx+1:]
	return s.Where(
		"type = ? AND (connect_address = ? OR listen_address = ? OR listen_address = ? OR listen_address = ?)",
		listenerType, addr, addr, wildcard, justPort,
	).Exist(&Listener{})
}

// OpenByType 类型 + 已开启状态（socks5/隧道找"可用的那个监听器"用）
func (ListenerStore) OpenByType(listenerType string) (Listener, bool, error) {
	var l Listener
	s := DB()
	if s == nil {
		return l, false, ErrStoreUnavailable
	}
	has, err := s.Where("type = ? AND status = ?", listenerType, 1).Get(&l)
	return l, has, err
}

// ===== 客户端（会话）=====

// ClientStore 客户端领域访问
type ClientStore struct{}

// Clients_ 客户端领域方法入口（Clients 已是实体类型名）
func Clients_() ClientStore { return ClientStore{} }

// Count 客户端总数
func (ClientStore) Count() (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Count(&Clients{})
}

// Page 分页取客户端（按 id 倒序 = 最新上线在前）
func (ClientStore) Page(offset, limit int) ([]Clients, error) {
	var out []Clients
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Desc("id").Limit(limit, offset).Find(&out)
}

// All 全部客户端
func (ClientStore) All() ([]Clients, error) {
	var out []Clients
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Find(&out)
}

// GetByUID 按 uid 取一条
func (ClientStore) GetByUID(uid string) (Clients, bool, error) {
	var c Clients
	s := DB()
	if s == nil {
		return c, false, ErrStoreUnavailable
	}
	has, err := s.Where("uid = ?", uid).Get(&c)
	return c, has, err
}

// OnlineByUIDs 在给定 uid 集合里筛出在线的（批量操作前的守卫）
func (ClientStore) OnlineByUIDs(uids []string) ([]Clients, error) {
	var out []Clients
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	if len(uids) == 0 {
		return out, nil
	}
	return out, s.In("uid", uids).Where("online = ?", "1").Find(&out)
}

// OnlineByListener 某监听器上在线的客户端
func (ClientStore) OnlineByListener(listenerType string) ([]Clients, error) {
	var out []Clients
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Where("listener = ? AND online = ?", listenerType, "1").Find(&out)
}

// UpdateColumns 只更新指定列（避免全列更新把心跳/地址等写成零值）
func (ClientStore) UpdateColumns(uid string, cols []string, patch *Clients) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("uid = ?", uid).Cols(cols...).Update(patch)
}

// UpdateFields 更新结构体的非零字段（xorm Update 语义：零值列不写）
func (ClientStore) UpdateFields(uid string, patch *Clients) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("uid = ?", uid).Update(patch)
}

// DeleteByUID 删除客户端记录
func (ClientStore) DeleteByUID(uid string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("uid = ?", uid).Delete(new(Clients))
}

// MaxID 当前最大客户端 id（新建记录时分配"心跳序号"用）
func (ClientStore) MaxID() (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	var maxID int64
	_, err := s.SQL("SELECT COALESCE(MAX(id), 0) FROM clients").Get(&maxID)
	return maxID, err
}

// PurgeByUID 删除客户端及其**全部**从属数据（级联删除的唯一出口）。
//
// 为什么必须收口：原实现把这段写在两处，一处删 4 张表、另一处删 3 张表，
// 战利品（loot）两边都没删 —— 客户端删掉后凭据仍留在战利品库里，
// 靠"来源会话"字段指向一个已不存在的 uid。这类"漏一张表"的 bug 在
// 散落的删除代码里基本无法靠 review 发现。
//
// 返回各表删除条数，便于调用方打日志定位。
func (ClientStore) PurgeByUID(uid string) map[string]int64 {
	out := make(map[string]int64, 5)
	dels := []struct {
		name string
		fn   func(string) (int64, error)
	}{
		{"clients", ClientStore{}.DeleteByUID},
		{"downloads", DownloadStore{}.DeleteByUID},
		{"notes", NoteStore{}.DeleteByUID},
		{"shell", ShellStore{}.DeleteByUID},
		{"loot", LootStore{}.DeleteByUID},
	}
	for _, d := range dels {
		n, err := d.fn(uid)
		if err != nil {
			continue // 单表失败不阻断其余：残留总比"停下不删"好收拾
		}
		out[d.name] = n
	}
	return out
}

// SetOnline 置在线状态
func (ClientStore) SetOnline(uid, online string) (int64, error) {
	return ClientStore{}.UpdateColumns(uid, []string{"online"}, &Clients{Online: online})
}

// ===== 客户端备注 =====

// NoteStore 备注领域访问
type NoteStore struct{}

// NotesStore_ 备注领域方法入口（Notes 已是实体类型名）
func NotesStore_() NoteStore { return NoteStore{} }

// GetByUID 取备注
func (NoteStore) GetByUID(uid string) (Notes, bool, error) {
	var n Notes
	s := DB()
	if s == nil {
		return n, false, ErrStoreUnavailable
	}
	has, err := s.Where("uid = ?", uid).Get(&n)
	return n, has, err
}

// Upsert 写入备注（不存在则插入）。
// 收口前"先查再决定 insert/update"这段在多个文件重复，且有人漏了 update 分支。
func (NoteStore) Upsert(uid, note string) error {
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	has, err := s.Where("uid = ?", uid).Exist(&Notes{})
	if err != nil {
		return err
	}
	if has {
		_, err = s.Where("uid = ?", uid).Cols("note").Update(&Notes{Note: note})
		return err
	}
	_, err = s.Insert(&Notes{Uid: uid, Note: note})
	return err
}

// DeleteByUID 删客户端时清理备注
func (NoteStore) DeleteByUID(uid string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("uid = ?", uid).Delete(new(Notes))
}

// ===== 客户端下载记录 =====

// DownloadStore 下载记录领域访问
type DownloadStore struct{}

// DownloadsStore_ 下载领域方法入口（Downloads 已是实体类型名）
func DownloadsStore_() DownloadStore { return DownloadStore{} }

// ListByUID 某客户端的下载记录
func (DownloadStore) ListByUID(uid string) ([]Downloads, error) {
	var out []Downloads
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Where("uid = ?", uid).Asc("id").Find(&out)
}

// ListAll 全部下载记录（报告导出用）
func (DownloadStore) ListAll() ([]Downloads, error) {
	var out []Downloads
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Asc("id").Find(&out)
}

// DeleteByUID 删客户端时清理下载记录
func (DownloadStore) DeleteByUID(uid string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("uid = ?", uid).Delete(new(Downloads))
}

// ExistsByPath 该 uid 下是否已有这个文件路径的下载记录
func (DownloadStore) ExistsByPath(uid, path string) (bool, error) {
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	return s.Where("uid = ? AND file_path = ?", uid, path).Exist(&Downloads{})
}

// Insert 新增一条下载记录
func (DownloadStore) Insert(rec *Downloads) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Insert(rec)
}

// DeleteByPath 按 uid + 文件路径删除（下载列表里删除单条用）
func (DownloadStore) DeleteByPath(uid, path string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("uid = ? AND file_path = ?", uid, path).Delete(&Downloads{})
}

// Upsert 记录/更新一个下载（同 uid + 文件路径视为同一条，续传时更新进度）
func (DownloadStore) Upsert(rec *Downloads) error {
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	has, err := s.Where("uid = ? AND file_path = ?", rec.Uid, rec.FilePath).Exist(&Downloads{})
	if err != nil {
		return err
	}
	if has {
		_, err = s.Where("uid = ? AND file_path = ?", rec.Uid, rec.FilePath).
			Cols("file_size", "downloaded_size").Update(rec)
		return err
	}
	_, err = s.Insert(rec)
	return err
}

// ===== 客户端命令历史（Shell）=====

// ShellStore 客户端命令历史领域访问
type ShellStore struct{}

// Shells 命令历史领域方法入口（Shell 已是实体类型名）
func Shells() ShellStore { return ShellStore{} }

// GetByUID 取命令历史
func (ShellStore) GetByUID(uid string) (Shell, bool, error) {
	var sh Shell
	s := DB()
	if s == nil {
		return sh, false, ErrStoreUnavailable
	}
	has, err := s.Where("uid = ?", uid).Get(&sh)
	return sh, has, err
}

// UpdateContent 更新命令历史内容
func (ShellStore) UpdateContent(uid, content string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("uid = ?", uid).Cols("shell_content").Update(&Shell{ShellContent: content})
}

// DeleteByUID 删客户端时清理命令历史
func (ShellStore) DeleteByUID(uid string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("uid = ?", uid).Delete(new(Shell))
}

// ===== 战利品 / 凭据 =====

// LootStore 战利品领域访问。
//
// 收口的价值：口令入库必须去重（同一台机器重复收集不该刷屏），"去重规则"散在
// 调用方时必然一人一个写法。
type LootStore struct{}

// LootStore_ 战利品领域方法入口（Loot 已是实体类型名）
func LootStore_() LootStore { return LootStore{} }

// ListBySources 按来源集合取战利品（凭据制品列表用：source 属于给定集合，
// uid 为空表示不限会话），按 id 倒序取前 limit 条。
func (LootStore) ListBySources(sources []string, uid string, limit int) ([]Loot, error) {
	out := make([]Loot, 0)
	s := DB()
	if s == nil {
		return out, ErrStoreUnavailable
	}
	if len(sources) == 0 {
		return out, nil
	}
	sess := s.In("source", sources)
	if uid != "" {
		sess = sess.And("uid = ?", uid)
	}
	if err := sess.Desc("id").Limit(limit).Find(&out); err != nil {
		return out, err
	}
	if err := decryptLootRows(out); err != nil {
		return nil, err
	}
	return out, nil
}

// AddAutoIngest 命令产出自动入库（去重键与手动录入**刻意不同**）。
//
// 去重键 = uid + type + host + secret + source：对应"同一会话把同一条提取命令
// （如 wifi 连跑两次）重复执行"，此时 user/realm 可能为空或写法不一，
// 用它们参与去重会漏判。
//
// 与 AddIfNotEmpty（手动/上报路径，键 = uid+user+realm+secret）的差异是**故意的**，
// 不是遗留不一致 —— 两条路径的"什么算重复"本来就不同。把差异写在两个命名方法里，
// 比塞进一个带 flag 的方法更容易看懂。
func (LootStore) AddAutoIngest(l *Loot) (bool, error) {
	if strings.TrimSpace(l.Secret) == "" {
		return false, nil
	}
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	// 去重改用**指纹列**：Secret 已静态加密，AES-GCM 每次密文不同，
	// 继续拿 Secret 比较会让同一口令的重复收集全部"不重复"，去重形同失效。
	fp, err := lootFp(l.Secret)
	if err != nil {
		return false, err
	}
	has, err := s.Where("uid = ? AND type = ? AND host = ? AND secret_fp = ? AND source = ?",
		l.Uid, l.Type, l.Host, fp, l.Source).Exist(&Loot{})
	if err != nil {
		return false, err
	}
	if has {
		return false, nil
	}
	if l.CreatedAt == "" {
		l.CreatedAt = nowStamp()
	}
	sealed, err := sealLootSecret(l.Secret)
	if err != nil {
		return false, err
	}
	l.SecretFp = fp
	l.Secret = sealed
	_, err = s.Insert(l)
	return err == nil, err
}

// Insert 直接插入一条战利品（手动录入路径，不做去重 —— 用户明确要加就该加）
func (LootStore) Insert(l *Loot) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	if l.CreatedAt == "" {
		l.CreatedAt = nowStamp()
	}
	// 仍然补算指纹：手动录入与自动入库可能撞上同一条口令，
	// 有指纹列才能让后续去重（以及"两条是否同一口令"的排查）成立。
	fp, err := lootFp(l.Secret)
	if err != nil {
		return 0, err
	}
	sealed, err := sealLootSecret(l.Secret)
	if err != nil {
		return 0, err
	}
	l.SecretFp = fp
	l.Secret = sealed
	return s.Insert(l)
}

// PageFiltered 分页筛选战利品（uid / type 为空表示不筛）
func (LootStore) PageFiltered(uid, lootType string, offset, limit int) ([]Loot, int64, error) {
	var items []Loot
	var total int64
	s := DB()
	if s == nil {
		return nil, 0, ErrStoreUnavailable
	}
	build := func() *xorm.Session {
		sess := s.Where("1=1")
		if uid != "" {
			sess = sess.And("uid = ?", uid)
		}
		if lootType != "" && lootType != "all" {
			sess = sess.And("type = ?", lootType)
		}
		return sess
	}
	// xorm v1.3.9 的 Count 只接受结构体指针：传 *int64 会返回 "unsupported type"，
	// 且计数由返回值给出（与 TaskStore.PageFiltered 同形）
	var err error
	if total, err = build().Table("loot").Count(new(Loot)); err != nil {
		return nil, 0, err
	}
	if err := build().Table("loot").Desc("id").Limit(limit, offset).Find(&items); err != nil {
		return nil, 0, err
	}
	if err := decryptLootRows(items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// AddIfNotEmpty 有非空凭据就入库，返回是否真的插入了。
//
// 空密钥不入库：否则战利品列表会被一堆空记录淹没。重复条目静默跳过
// （同一台机器重复收集是常态，不该刷屏也不该报错）。
func (LootStore) AddIfNotEmpty(uid, lootType, user, realm, host, secret, note, source string) (bool, error) {
	if strings.TrimSpace(secret) == "" {
		return false, nil
	}
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	fp, err := lootFp(secret)
	if err != nil {
		return false, err
	}
	exists, err := s.Where("uid = ? AND user = ? AND realm = ? AND secret_fp = ?",
		uid, user, realm, fp).Exist(&Loot{})
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	sealed, err := sealLootSecret(secret)
	if err != nil {
		return false, err
	}
	_, err = s.Insert(&Loot{
		Uid: uid, Type: lootType, User: user, Realm: realm,
		Host: host, Secret: sealed, SecretFp: fp, Note: note, Source: source,
		CreatedAt: nowStamp(),
	})
	return err == nil, err
}

// ListByUID 某客户端的战利品
func (LootStore) ListByUID(uid string) ([]Loot, error) {
	var out []Loot
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	if err := s.Where("uid = ?", uid).Asc("id").Find(&out); err != nil {
		return nil, err
	}
	if err := decryptLootRows(out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAll 全部战利品（按 id 倒序 = 最新在前）
func (LootStore) ListAll() ([]Loot, error) {
	var out []Loot
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	if err := s.Desc("id").Find(&out); err != nil {
		return nil, err
	}
	if err := decryptLootRows(out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetByID 按主键取一条
func (LootStore) GetByID(id int64) (Loot, bool, error) {
	var l Loot
	s := DB()
	if s == nil {
		return l, false, ErrStoreUnavailable
	}
	has, err := s.ID(id).Get(&l)
	if err != nil || !has {
		return l, has, err
	}
	// decryptLootRows 作用于切片，所以包一层再写回
	rows := []Loot{l}
	if err := decryptLootRows(rows); err != nil {
		return l, has, err
	}
	l.Secret = rows[0].Secret
	return l, has, nil
}

// DeleteByID 删除一条
func (LootStore) DeleteByID(id int64) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).Delete(new(Loot))
}

// PurgeOlderThan 删除 created_at 早于 cutoff 的战果（保留期清理用）。
//
// CreatedAt 是固定宽度、零填充的 "2006-01-02 15:04:05" 文本，
// 字典序即时间序，所以可以直接用字符串比较，不必解析成时间再比。
func (LootStore) PurgeOlderThan(cutoff time.Time) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("created_at < ?", cutoff.Format("2006-01-02 15:04:05")).Delete(new(Loot))
}

// DeleteByUID 删客户端时清理其战利品
func (LootStore) DeleteByUID(uid string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("uid = ?", uid).Delete(new(Loot))
}
