package database

import (
	"fmt"
	"time"

	"xorm.io/xorm"
)

// ===== 领域存储：投递（WebDelivery）/ 访问日志 / 隧道 / 仪表盘聚合 =====
//
// 与 domain_core.go 同属 §3.7 收口范围，按领域拆文件避免单文件过长。

// ===== 投递（WebDelivery）=====

// DeliveryStore 载荷投递领域访问
type DeliveryStore struct{}

// Deliveries 投递领域方法入口
func Deliveries() DeliveryStore { return DeliveryStore{} }

// List 全部投递（按 id 升序）
func (DeliveryStore) List() ([]WebDelivery, error) {
	var out []WebDelivery
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Asc("id").Find(&out)
}

// Insert 新建投递
func (DeliveryStore) Insert(d *WebDelivery) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Insert(d)
}

// FindAny 三段式查找：id → public_key → listening_port。
//
// 为什么必须收口：这段"按三种可能的标识依次查"的逻辑在 webdelivery.go 里被复制了
// 六遍，每一处都要自己决定"哪个字段先判空、命中后怎么继续"。复制六遍意味着改一处
// 忘五处，而这类查找错位的症状是"接口偶发返回 404"——很难定位。
//
// 入参零值表示该标识不可用，按 id → key → port 顺序取第一个非零值查。
func (DeliveryStore) FindAny(id int64, publicKey, port string) (WebDelivery, bool, error) {
	var d WebDelivery
	s := DB()
	if s == nil {
		return d, false, ErrStoreUnavailable
	}
	if id > 0 {
		has, err := s.ID(id).Get(&d)
		if has || err != nil {
			return d, has, err
		}
	}
	if publicKey != "" {
		has, err := s.Where("public_key = ?", publicKey).Get(&d)
		if has || err != nil {
			return d, has, err
		}
	}
	if port != "" {
		has, err := s.Where("listening_port = ?", port).Get(&d)
		return d, has, err
	}
	return d, false, nil
}

// GetByID 按主键取
func (DeliveryStore) GetByID(id int64) (WebDelivery, bool, error) {
	var d WebDelivery
	s := DB()
	if s == nil {
		return d, false, ErrStoreUnavailable
	}
	has, err := s.ID(id).Get(&d)
	return d, has, err
}

// GetByPort 按监听端口取
func (DeliveryStore) GetByPort(port string) (WebDelivery, bool, error) {
	var d WebDelivery
	s := DB()
	if s == nil {
		return d, false, ErrStoreUnavailable
	}
	has, err := s.Where("listening_port = ?", port).Get(&d)
	return d, has, err
}

// SetStatusByKey 按公开标识置状态
func (DeliveryStore) SetStatusByKey(key string, status int) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("public_key = ?", key).Update(&WebDelivery{Status: status})
}

// DeleteByPort 按端口删除
func (DeliveryStore) DeleteByPort(port string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("listening_port = ?", port).Delete(&WebDelivery{})
}

// GetByPublicKey 按公开标识取
func (DeliveryStore) GetByPublicKey(key string) (WebDelivery, bool, error) {
	var d WebDelivery
	s := DB()
	if s == nil {
		return d, false, ErrStoreUnavailable
	}
	has, err := s.Where("public_key = ?", key).Get(&d)
	return d, has, err
}

// PortInUse 该监听端口是否已被启用中的投递占用
func (DeliveryStore) PortInUse(port string) (bool, error) {
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	return s.Where("listening_port = ? AND status = 1", port).Exist(&WebDelivery{})
}

// ListByStatus 按状态取投递
func (DeliveryStore) ListByStatus(status int) ([]WebDelivery, error) {
	var out []WebDelivery
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Where("status = ?", status).Find(&out)
}

// UpdateColumns 只更新指定列
func (DeliveryStore) UpdateColumns(id int64, cols []string, patch *WebDelivery) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).Cols(cols...).Update(patch)
}

// UpdateEditable 更新投递的可编辑列（不含 id / public_key / status / 命中统计）。
//
// 列名必须用数据库里的实际列名：xorm 的 SnakeMapper 把缩写拆成了
// o_s / u_r_i_path / u_a_allow / stager_u_r_l / shellcode_u_r_l。
// 用结构体 + Cols 更新时这些列很容易写错（OpenWebDelivery 里就用了不存在的
// "stager_url"/"shellcode_url"，错误被 `_, _ =` 吞掉）。
func (DeliveryStore) UpdateEditable(id int64, patch *WebDelivery) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	upd := map[string]interface{}{
		"listener_config":    patch.ListenerConfig,
		"o_s":                patch.OS,
		"arch":               patch.Arch,
		"listening_port":     patch.ListeningPort,
		"file_name":          patch.FileName,
		"pass":               patch.Pass,
		"download_path":      patch.DownloadPath,
		"save_filename":      patch.SaveFilename,
		"bind_mode":          patch.BindMode,
		"public_host":        patch.PublicHost,
		"public_scheme":      patch.PublicScheme,
		"base_path":          patch.BasePath,
		"u_r_i_path":         patch.URIPath,
		"content_type":       patch.ContentType,
		"stage_content_type": patch.StageContentType,
		"access_token":       patch.AccessToken,
		"u_a_allow":          patch.UAAllow,
		"max_downloads":      patch.MaxDownloads,
		"one_time":           patch.OneTime,
		"crypto_mode":        patch.CryptoMode,
		"delivery_key":       patch.DeliveryKey,
		"note":               patch.Note,
		"server_address":     patch.ServerAddress,
		"stager_u_r_l":       patch.StagerURL,
		"shellcode_u_r_l":    patch.ShellcodeURL,
	}
	return s.Table("web_delivery").Where("id = ?", id).Update(upd)
}

// MarkOneTimeUsed 持久化一次性资源的消费状态。
//
// 只允许三列，避免把列名拼接引入 SQL 注入。内存里的 CAS 只负责并发互斥，
// 真正防止"服务重启后一次性 URL 复活"的是这里落库。
func (DeliveryStore) MarkOneTimeUsed(id int64, publicKey, column string) error {
	switch column {
	case "full_used", "stager_used", "sc_used":
	default:
		return fmt.Errorf("非法的一次性状态列: %s", column)
	}
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	q := s.Table("web_delivery")
	switch {
	case id > 0:
		q = q.Where("id = ?", id)
	case publicKey != "":
		q = q.Where("public_key = ?", publicKey)
	default:
		return fmt.Errorf("缺少投递标识")
	}
	_, err := q.Update(map[string]interface{}{column: 1})
	return err
}

// UpdateHitStats 写入命中统计。调用方已经用原子计数串行化，这里只做持久化。
func (DeliveryStore) UpdateHitStats(id int64, publicKey string, hits int64, at, ip string) error {
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	q := s.Table("web_delivery")
	switch {
	case id > 0:
		q = q.Where("id = ?", id)
	case publicKey != "":
		q = q.Where("public_key = ?", publicKey)
	default:
		return fmt.Errorf("缺少投递标识")
	}
	_, err := q.Update(map[string]interface{}{
		"hit_count":   hits,
		"last_hit_at": at,
		"last_hit_ip": ip,
	})
	return err
}

// SetStatusByPort 按端口置状态
func (DeliveryStore) SetStatusByPort(port string, status int) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("listening_port = ?", port).Cols("status").Update(&WebDelivery{Status: status})
}

// SetStatusByID 按 id 置状态
func (DeliveryStore) SetStatusByID(id int64, status int) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).Cols("status").Update(&WebDelivery{Status: status})
}

// ListOpen status=1 的投递（启动时恢复用）
func (DeliveryStore) ListOpen() ([]WebDelivery, error) {
	return DeliveryStore{}.ListByStatus(1)
}

// UpdateByPort 按端口更新若干列（shared 模式下同端口多行同步）
func (DeliveryStore) UpdateByPort(port string, patch *WebDelivery) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("listening_port = ?", port).Update(patch)
}

// DeleteCascade 删除投递并清理其访问日志（级联删除的唯一出口）
func (DeliveryStore) DeleteCascade(d WebDelivery) error {
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	if d.ID > 0 {
		if _, err := s.ID(d.ID).Delete(&WebDelivery{}); err != nil {
			return err
		}
		_, _ = s.Where("delivery_id = ?", d.ID).Delete(&DeliveryAccessLog{})
	}
	if d.ListeningPort != "" {
		_, err := s.Where("listening_port = ?", d.ListeningPort).Delete(&WebDelivery{})
		return err
	}
	return nil
}

// DeleteByPublicKey 按公开标识删除
func (DeliveryStore) DeleteByPublicKey(key string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("public_key = ?", key).Delete(&WebDelivery{})
}

// ===== 投递访问日志 =====

// DeliveryLogStore 投递访问日志领域访问
type DeliveryLogStore struct{}

// DeliveryLogs 访问日志领域方法入口
func DeliveryLogs() DeliveryLogStore { return DeliveryLogStore{} }

// Recent 最近 limit 条访问日志（按 id 倒序）。
//
// 注意用 "id" 而不是 "i_d"：原实现写的是 Desc("i_d")，而本表的主键列在
// xorm 标签里被显式钉成 'id'（`xorm:"'id' pk autoincr"`），i_d 这个列并不存在，
// 于是这条查询**一直报错**、`err == nil` 判定为假，社会工程报告里的"点击列表"
// 永远是空的。收口到方法里并用测试钉住（TestDeliveryLogRecentOrdering）。
func (DeliveryLogStore) Recent(limit int) ([]DeliveryAccessLog, error) {
	var out []DeliveryAccessLog
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Desc("id").Limit(limit).Find(&out)
}

// Page 最近的访问日志：deliveryID>0 按投递筛，否则 publicKey 非空按标识筛
func (DeliveryLogStore) Page(limit int, deliveryID int64, publicKey string) ([]DeliveryAccessLog, error) {
	var out []DeliveryAccessLog
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	q := s.Desc("id").Limit(limit)
	if deliveryID > 0 {
		q = q.Where("delivery_id = ?", deliveryID)
	} else if publicKey != "" {
		q = q.Where("public_key = ?", publicKey)
	}
	return out, q.Find(&out)
}

// Record 记录一次投递访问
func (DeliveryLogStore) Record(log *DeliveryAccessLog) error {
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	_, err := s.Insert(log)
	return err
}

// CountByDelivery 某投递的访问次数
func (DeliveryLogStore) CountByDelivery(deliveryID int64) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("delivery_id = ?", deliveryID).Count(&DeliveryAccessLog{})
}

// ===== 隧道代理 =====

// TunnelStore 隧道代理领域访问
type TunnelStore struct{}

// Tunnels 隧道领域方法入口
func Tunnels() TunnelStore { return TunnelStore{} }

// List 全部隧道（按 id 升序）
func (TunnelStore) List() ([]TunnelProxy, error) {
	var out []TunnelProxy
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Asc("id").Find(&out)
}

// GetByID 按主键取
func (TunnelStore) GetByID(id int64) (TunnelProxy, bool, error) {
	var t TunnelProxy
	s := DB()
	if s == nil {
		return t, false, ErrStoreUnavailable
	}
	has, err := s.ID(id).Get(&t)
	return t, has, err
}

// PortInUse 服务端端口是否已被其它隧道占用（excludeID>0 时排除自身）
func (TunnelStore) PortInUse(port int, excludeID int64) (bool, error) {
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	sess := s.Where("server_port = ?", port)
	if excludeID > 0 {
		sess = sess.And("id != ?", excludeID)
	}
	return sess.Exist(&TunnelProxy{})
}

// ===== 仪表盘聚合 =====

// DashboardStore 仪表盘统计领域访问
type DashboardStore struct{}

// Dashboard 仪表盘统计入口
func Dashboard() DashboardStore { return DashboardStore{} }

// ClientCounts 客户端总数与在线数。
//
// 原实现把这两项合并成一条聚合 SQL 以减少 5 秒轮询下的全表扫描次数，
// 失败时回退为两次 Count。收口后把"聚合 + 回退"整体放进领域层：
// 调用方不该关心用哪种方式统计，只该拿到两个数字。
func (DashboardStore) ClientCounts() (total, online int64, err error) {
	s := DB()
	if s == nil {
		return 0, 0, ErrStoreUnavailable
	}
	var agg struct {
		Total  int64 `xorm:"total"`
		Online int64 `xorm:"online"`
	}
	_, aggErr := s.SQL(
		"SELECT COUNT(*) AS total, COALESCE(SUM(CASE WHEN online='1' THEN 1 ELSE 0 END),0) AS online FROM clients",
	).Get(&agg)
	if aggErr == nil {
		return agg.Total, agg.Online, nil
	}
	// 回退：聚合失败（方言差异等）时退化为两次 Count，保证仪表盘仍可用
	total, err = s.Count(&Clients{})
	if err != nil {
		return 0, 0, err
	}
	online, err = s.Where("online = ?", "1").Count(&Clients{})
	if err != nil {
		return total, 0, err
	}
	return total, online, nil
}

// OpenListenerCount 启用中的监听器数量
func (DashboardStore) OpenListenerCount() (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("status = ?", 1).Count(&Listener{})
}

// ===== SOCKS5 代理 =====

// Socks5Store SOCKS5 代理领域访问
type Socks5Store struct{}

// Socks5s SOCKS5 领域方法入口
func Socks5s() Socks5Store { return Socks5Store{} }

// List 全部 SOCKS5 记录（按 id 升序）
func (Socks5Store) List() ([]Socks5, error) {
	var out []Socks5
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Asc("id").Find(&out)
}

// Insert 新建
func (Socks5Store) Insert(r *Socks5) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Insert(r)
}

// GetByID 按主键取
func (Socks5Store) GetByID(id int64) (Socks5, bool, error) {
	var r Socks5
	s := DB()
	if s == nil {
		return r, false, ErrStoreUnavailable
	}
	has, err := s.ID(id).Get(&r)
	return r, has, err
}

// UpdateAllCols 全列更新（编辑保存用；id 显式传入，理由同 ListenerStore.UpdateAllCols）
func (Socks5Store) UpdateAllCols(id int64, r *Socks5) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).AllCols().Update(r)
}

// ListByStatus 按状态取
func (Socks5Store) ListByStatus(status int) ([]Socks5, error) {
	var out []Socks5
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Where("status = ?", status).Find(&out)
}

// SetStatus 改状态并刷新 UpdatedAt
//
// 收口的意义：状态列必须与 UpdatedAt 一起改，原来 6 处调用各自决定要不要带
// UpdatedAt，漏一处就会让"最后更新时间"永久停在上一次。
func (Socks5Store) SetStatus(id int64, status int) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).Update(&Socks5{Status: status, UpdatedAt: time.Now()})
}

// DeleteByID 删除
func (Socks5Store) DeleteByID(id int64) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).Delete(&Socks5{})
}

// PortInUse 服务端端口是否已被其它记录占用（excludeID>0 时排除自身）
func (Socks5Store) PortInUse(port string, excludeID int64) (bool, error) {
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	sess := s.Where("socks5port = ?", port)
	if excludeID > 0 {
		sess = sess.And("id != ?", excludeID)
	}
	return sess.Exist(&Socks5{})
}

// PortsInUse 端口占用检查：返回给定 server_port 集合里已存在的隧道（含 socks5 端口集合）。
//
// 建隧道时要同时避开"已有隧道的 server_port"和"Agent 端口 = server_port+1000"
// 以及 socks5 端口，三组查询原来散在调用方；收成一个方法后调用方只需给出
// 要检查的端口集合。
func (TunnelStore) PortsInUse(ports []int) ([]TunnelProxy, error) {
	var out []TunnelProxy
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	if len(ports) == 0 {
		return out, nil
	}
	args := make([]interface{}, 0, len(ports))
	for _, p := range ports {
		args = append(args, p)
	}
	return out, s.In("server_port", args...).Find(&out)
}

// PortConflict 端口冲突检查：给定端口集合里是否已有隧道占用（excludeID>0 时排除自身）。
//
// 用 xorm 的 In(column, args...)：Where("col IN (?)", slice) 在本版本不会展开切片，
// 会把整个切片当成单个参数交给驱动而报 "unsupported type" —— 错误若被忽略就会
// 静默放行冲突。这里显式返回错误，调用方按 fail-closed 处理（查不动就当占用）。
func (TunnelStore) PortConflict(ports []int, excludeID int64) (TunnelProxy, bool, error) {
	var t TunnelProxy
	s := DB()
	if s == nil {
		return t, false, ErrStoreUnavailable
	}
	if len(ports) == 0 {
		return t, false, nil
	}
	args := make([]interface{}, 0, len(ports))
	for _, p := range ports {
		args = append(args, p)
	}
	sess := s.In("server_port", args...)
	if excludeID > 0 {
		sess = sess.And("id != ?", excludeID)
	}
	has, err := sess.Get(&t)
	return t, has, err
}

// PortConflict 端口冲突检查（SOCKS5 侧；ports 为字符串形式的端口）
func (Socks5Store) PortConflict(ports []string, excludeID int64) (Socks5, bool, error) {
	var r Socks5
	s := DB()
	if s == nil {
		return r, false, ErrStoreUnavailable
	}
	if len(ports) == 0 {
		return r, false, nil
	}
	args := make([]interface{}, 0, len(ports))
	for _, p := range ports {
		args = append(args, p)
	}
	sess := s.In("socks5port", args...)
	if excludeID > 0 {
		sess = sess.And("id != ?", excludeID)
	}
	has, err := sess.Get(&r)
	return r, has, err
}

// Insert 新建隧道
func (TunnelStore) Insert(t *TunnelProxy) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Insert(t)
}

// UpdateColumns 只更新指定列
func (TunnelStore) UpdateColumns(id int64, cols []string, patch *TunnelProxy) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).Cols(cols...).Update(patch)
}

// UpdateAll 全列更新
func (TunnelStore) UpdateAll(id int64, patch *TunnelProxy) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).Update(patch)
}

// UpdateMap 用 Table+Where 形式按 map 更新（规避 xorm ID 方法的表引用问题，保持原行为）
func (TunnelStore) UpdateMap(id int64, patch map[string]interface{}) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Table(&TunnelProxy{}).Where("id = ?", id).Update(patch)
}

// DeleteByID 删除
func (TunnelStore) DeleteByID(id int64) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).Delete(&TunnelProxy{})
}

// GetByUIDs 按 uid 集合批量取客户端（隧道列表里补客户端信息用）
func (ClientStore) GetByUIDs(uids []string) ([]Clients, error) {
	var out []Clients
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	if len(uids) == 0 {
		return out, nil
	}
	args := make([]interface{}, 0, len(uids))
	for _, u := range uids {
		args = append(args, u)
	}
	return out, s.In("uid", args...).Find(&out)
}

// OnlineByUID 取"在线"的客户端（不存在或离线时 has=false）
func (ClientStore) OnlineByUID(uid string) (Clients, bool, error) {
	var c Clients
	s := DB()
	if s == nil {
		return c, false, ErrStoreUnavailable
	}
	has, err := s.Where("uid = ? AND online = ?", uid, "1").Get(&c)
	return c, has, err
}

// ===== 任务（命令下发记录）=====

// TaskStore 任务领域访问
type TaskStore struct{}

// Tasks 任务领域方法入口
func Tasks() TaskStore { return TaskStore{} }

// Insert 新建任务
func (TaskStore) Insert(t *Task) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Insert(t)
}

// GetByID 按主键取
func (TaskStore) GetByID(id int64) (Task, bool, error) {
	var t Task
	s := DB()
	if s == nil {
		return t, false, ErrStoreUnavailable
	}
	has, err := s.ID(id).Get(&t)
	return t, has, err
}

// NextDispatched 取该会话最早的一条"已下发"任务（FIFO 消费）
func (TaskStore) NextDispatched(uid, dispatched string) (Task, bool, error) {
	var t Task
	s := DB()
	if s == nil {
		return t, false, ErrStoreUnavailable
	}
	has, err := s.Where("uid = ? AND status = ?", uid, dispatched).OrderBy("id").Get(&t)
	return t, has, err
}

// NextDispatchedByKind 取该 uid 最早一条指定 kind 的 dispatched 任务。
// 插件/内存执行（EXEC_DONE）按 taskID 精确定位失败时用它回退。
func (TaskStore) NextDispatchedByKind(uid, kind, dispatched string) (Task, bool, error) {
	var t Task
	s := DB()
	if s == nil {
		return t, false, ErrStoreUnavailable
	}
	has, err := s.Where("uid = ? AND kind = ? AND status = ?", uid, kind, dispatched).
		OrderBy("id").Get(&t)
	return t, has, err
}

// NextDispatchedExcludingKind 取该 uid 最早一条非指定 kind 的 dispatched 任务。
// 能力客户端的 exec 输出走 EXEC_OUTPUT（不进任务归属），所以 console/batch
// 可以安全地跳过 exec 队首。
func (TaskStore) NextDispatchedExcludingKind(uid, kind, dispatched string) (Task, bool, error) {
	var t Task
	s := DB()
	if s == nil {
		return t, false, ErrStoreUnavailable
	}
	has, err := s.Where("uid = ? AND kind <> ? AND status = ?", uid, kind, dispatched).
		OrderBy("id").Get(&t)
	return t, has, err
}

// FindDispatchedDownload 查是否已有同样的下载任务在排队（避免重复下发）
func (TaskStore) FindDispatchedDownload(uid, filePath, dispatched string) (Task, bool, error) {
	var t Task
	s := DB()
	if s == nil {
		return t, false, ErrStoreUnavailable
	}
	has, err := s.Where("uid = ? AND kind = ? AND command = ? AND status = ?",
		uid, "download", "download "+filePath, dispatched).OrderBy("id ASC").Get(&t)
	return t, has, err
}

// CompleteIfDispatched 把"已下发"的任务置为完成。
//
// status 条件写在 WHERE 里（而不是先查后改）是有意的：并发下两个回包同时到达时，
// 只有一个能命中，另一个 affected=0 —— 这正是"重复回包不覆盖结果"的保证。
func (TaskStore) CompleteIfDispatched(id int64, dispatched, status, result, errMsg string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	res, err := s.Table(new(Task)).
		Where("id = ? AND status = ?", id, dispatched).
		Update(map[string]interface{}{
			"status": status, "result": result, "error": errMsg, "finished_at": time.Now(),
		})
	if err != nil {
		return 0, err
	}
	return res, nil
}

// CancelIfDispatched 取消"已下发"的任务
func (TaskStore) CancelIfDispatched(id int64, dispatched, cancelled string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	res, err := s.Table(new(Task)).
		Where("id = ? AND status = ?", id, dispatched).
		Update(map[string]interface{}{"status": cancelled, "finished_at": time.Now()})
	if err != nil {
		return 0, err
	}
	return res, nil
}

// Recent 最近 limit 条任务（按 id 倒序）
func (TaskStore) Recent(limit int) ([]Task, error) {
	var out []Task
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.OrderBy("id DESC").Limit(limit).Find(&out)
}

// PageFiltered 分页筛选任务（uid/status/kind 为空表示不筛）
//
// 三个筛选条件各自独立，原来在调用方用一个闭包拼 session（xorm.Session 不能
// Clone，所以统计与查询各建一个）。收进领域层后调用方只传条件。
func (TaskStore) PageFiltered(uid, status, kind string, offset, limit int) ([]Task, int64, error) {
	var items []Task
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
		if status != "" {
			sess = sess.And("status = ?", status)
		}
		if kind != "" {
			sess = sess.And("kind = ?", kind)
		}
		return sess
	}
	var err error
	if total, err = build().Count(&Task{}); err != nil {
		return nil, 0, err
	}
	if err := build().Desc("id").Limit(limit, offset).Find(&items); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// MarkStaleAsTimeout 把超时仍未完成的任务标记为 timeout，返回影响行数
func (TaskStore) MarkStaleAsTimeout(dispatched, timeoutStatus string, beforeUnix int64) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	res, err := s.Table(new(Task)).
		Where("status = ? AND created_unix < ?", dispatched, beforeUnix).
		Update(map[string]interface{}{"status": timeoutStatus, "finished_at": time.Now()})
	if err != nil {
		return 0, err
	}
	return res, nil
}

// PurgeFinishedBefore 清理已完成且早于 beforeUnix 的任务
func (TaskStore) PurgeFinishedBefore(dispatched string, beforeUnix int64) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("status != ? AND created_unix < ?", dispatched, beforeUnix).Delete(new(Task))
}

// SetClientGroup 设置客户端分组（原写在 tasks.go 里，属客户端领域）
func (ClientStore) SetClientGroup(uid, group string) (int64, error) {
	return ClientStore{}.UpdateColumns(uid, []string{"group"}, &Clients{Group: group})
}

// ===== 插件 =====

// PluginStore 插件领域访问
type PluginStore struct{}

// Plugins 插件领域方法入口
func Plugins() PluginStore { return PluginStore{} }

// List 全部插件
func (PluginStore) List() ([]Plugin, error) {
	var out []Plugin
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Find(&out)
}

// GetByName 按插件名取
func (PluginStore) GetByName(name string) (Plugin, bool, error) {
	var p Plugin
	s := DB()
	if s == nil {
		return p, false, ErrStoreUnavailable
	}
	has, err := s.Where("name = ?", name).Get(&p)
	return p, has, err
}

// Upsert 按名字插入或更新（id 存在则按 id 更新）
func (PluginStore) Upsert(p *Plugin) error {
	s := DB()
	if s == nil {
		return ErrStoreUnavailable
	}
	if p.Id > 0 {
		_, err := s.ID(p.Id).Update(p)
		return err
	}
	_, err := s.Insert(p)
	return err
}

// Insert 新建插件
func (PluginStore) Insert(p *Plugin) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Insert(p)
}

// DeleteByName 按名字删除
func (PluginStore) DeleteByName(name string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("name = ?", name).Delete(&Plugin{})
}

// ===== 动态 stager 用到的投递按端口取（复用 DeliveryStore）=====

// GetByPortOrEmpty 按端口取投递；不存在返回零值且 found=false
func (DeliveryStore) GetByPortOrEmpty(port string) (WebDelivery, bool) {
	d, has, _ := DeliveryStore{}.GetByPort(port)
	return d, has
}

// ===== 动态 stager 令牌 =====

// StagerTokenStore 动态 stager 令牌领域访问
type StagerTokenStore struct{}

// StagerTokens 令牌领域方法入口
func StagerTokens() StagerTokenStore { return StagerTokenStore{} }

// DeleteByID 删除令牌
func (StagerTokenStore) DeleteByID(id int64) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.ID(id).Delete(new(StagerToken))
}
