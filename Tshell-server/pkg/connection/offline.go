package connection

import (
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"BackendTemplate/pkg/utils/logx"
)

// ClientActivity 记录客户端活动状态（用于统一离线检测）。
type ClientActivity struct {
	UID      string
	Listener string

	lastActive    atomic.Int64 // UnixNano
	sleepSeconds  atomic.Int32
	jitterPercent atomic.Int32

	Cleanup func() // 离线时清理 per-protocol 连接 map（如 TCPClientManger）
}

func (c *ClientActivity) touch() { c.lastActive.Store(time.Now().UnixNano()) }
func (c *ClientActivity) lastActiveTime() time.Time { return time.Unix(0, c.lastActive.Load()) }
func (c *ClientActivity) setLastActive(t time.Time) { c.lastActive.Store(t.UnixNano()) }
func (c *ClientActivity) beacon() (sleepSeconds, jitterPercent int) {
	return int(c.sleepSeconds.Load()), int(c.jitterPercent.Load())
}

// ClientState 对外只读的客户端状态快照（SmartOfflineCheck 等消费）
type ClientState struct {
	LastActive    time.Time
	Listener      string
	SleepSeconds  int
	JitterPercent int
}

// OfflineDetector 统一离线检测器。单 goroutine 按"最早离线截止时间"调度：
// 睡到该时刻再检查，阈值一到立即判离线（不再靠固定 ticker 轮询）。
// 替代各协议各自 duplicated 的 checkHeartbeats goroutine。
type OfflineDetector struct {
	clients  sync.Map // uid -> *ClientActivity
	stopCh   chan struct{}
	// wakeCh 截止时间可能提前时唤醒调度器（Register / UpdateSleep）。
	// 缓冲 1 + 非阻塞发送：唤醒合并，调度器来不及处理也不会积压。
	wakeCh   chan struct{}
	stopOnce sync.Once
}

// GlobalOfflineDetector 全局实例
var GlobalOfflineDetector *OfflineDetector

// InitOfflineDetector 初始化全局离线检测器
func InitOfflineDetector() {
	GlobalOfflineDetector = &OfflineDetector{
		stopCh: make(chan struct{}),
		wakeCh: make(chan struct{}, 1),
	}
	go GlobalOfflineDetector.run()
	logx.Println("✅ 统一离线检测器已启动")
}

// Register 注册客户端到离线检测器（FirstBlood/重连时调用）。cleanup 可选，离线时清理 per-protocol 连接 map。
//
// 【统一心跳】sleepSeconds 为客户端上报的真实 beacon sleep（旧客户端无上报时回退监听器配置）；
// jitterPercent 为监听器级 beacon 抖动（与生成载荷时编译进客户端的值一致），传 -1 表示
// 监听器未配置、回落到全局 client.jitter。显式 0 是合法配置（不抖动），必须保留。
func (d *OfflineDetector) Register(uid, listener string, sleepSeconds, jitterPercent int, cleanup ...func()) {
	if jitterPercent < 0 {
		jitterPercent = beaconJitterPercent()
	}
	if jitterPercent > 50 {
		jitterPercent = 50
	}
	c := &ClientActivity{UID: uid, Listener: listener}
	c.touch()
	c.sleepSeconds.Store(int32(sleepSeconds))
	c.jitterPercent.Store(int32(jitterPercent))
	if len(cleanup) > 0 {
		c.Cleanup = cleanup[0]
	}
	d.clients.Store(uid, c)
	// 新客户端可能带来更早的截止时间，让调度器重新计时
	d.wake()
}

// beaconJitterPercent 当前全局 beacon 抖动百分比（config client.jitter，0-50 钳制）
func beaconJitterPercent() int {
	_, j := config.ClientTiming()
	if j < 0 {
		j = 0
	}
	if j > 50 {
		j = 50
	}
	return j
}

// ResolveSleepSeconds 解析客户端上报的 beacon sleep 秒数。
//
// MetaInfo 版本 >= 4 时 osArray[4] 携带客户端真实 sleep（生成期占位符值，
// CS 单一事实来源）。旧客户端没有该字段时按**生成时的实际来源**回退：
// 监听器显式 sleep > 画像 sleeptime_ms > 全局 client.sleep > 5s。
//
// 不再使用监听器 heartbeat_interval：该字段只是历史遗留，和新版生成链
// （监听器 > 画像 > 全局）不一致，继续参与只会让"旧客户端"的阈值取决于
// 一个早已不代表真实节奏的配置。
func ResolveSleepSeconds(osArray []string, listenerSleepSeconds, profileSleepSeconds, globalSleepSeconds int) int {
	// 优先客户端上报（唯一可信来源）
	if len(osArray) > 4 {
		if v, err := strconv.Atoi(osArray[4]); err == nil && v > 0 {
			if v > 300 {
				v = 300
			}
			return v
		}
	}
	for _, v := range []int{listenerSleepSeconds, profileSleepSeconds, globalSleepSeconds} {
		if v >= 1 && v <= 300 {
			return v
		}
	}
	return 5
}

// ProfileSleepSeconds 返回画像 sleeptime_ms 对应的秒数（不存在/未配置=0）。
func ProfileSleepSeconds(profileName string) int {
	if profileName == "" {
		return 0
	}
	p := c2profile.Get(profileName)
	if p == nil || p.SleeptimeMs <= 0 {
		return 0
	}
	sec := p.SleeptimeMs / 1000
	if sec < 1 {
		sec = 1
	}
	if sec > 300 {
		sec = 300
	}
	return sec
}

// GlobalSleepSeconds 返回全局 client.sleep（0=未配置，交给 ResolveSleepSeconds 回退）。
func GlobalSleepSeconds() int {
	sleep, _ := config.ClientTiming()
	return sleep
}

// ResolveListenerSleep 是各协议 FirstBlood 的统一入口：按
// 客户端上报 > 监听器显式 sleep > 画像 sleeptime_ms > 全局 client.sleep > 5 解析。
func ResolveListenerSleep(osArray []string, beaconCfg database.ListenerBeaconConfig) int {
	return ResolveSleepSeconds(osArray, beaconCfg.SleepSeconds, ProfileSleepSeconds(beaconCfg.Profile), GlobalSleepSeconds())
}

// Thresholds 统一离线判定阈值（唯一实现，检测器与 SmartOfflineCheck 共用）：
// expected = sleep × (1 + jitter%)（CS 模型最坏拍间隔）；
// quick = 1.5×expected / warning = 3×expected / critical = 4.5×expected；
// sleep<5 时 critical 固定 20s（避免误判长时间命令执行）。
func Thresholds(sleepSeconds, jitterPercent int) (quick, warning, critical time.Duration) {
	if sleepSeconds <= 0 {
		sleepSeconds = 5
	}
	if jitterPercent < 0 || jitterPercent > 50 {
		jitterPercent = 0
	}
	expected := time.Duration(sleepSeconds*(100+jitterPercent)) * 10 * time.Millisecond
	quick = expected * 3 / 2
	warning = expected * 3
	critical = expected * 9 / 2
	if sleepSeconds < 5 && critical < 20*time.Second {
		critical = 20 * time.Second
	}
	return
}

// MarkOffline 统一离线处理（幂等）：落库 Online=2、发布离线事件、清理命令队列
// 与监听类型映射。所有离线写点（检测器/断连 defer/SmartOfflineCheck）统一走这里，
// 消除散落的 Online="2" 直写与状态震荡。
func MarkOffline(uid, listener, reason string) {
	if database.DB() != nil {
		affected, err := database.DB().Where("uid = ?", uid).Update(&database.Clients{Online: "2"})
		database.LogUpdate(affected, err, "MarkOffline uid="+uid)
	}
	logx.Infof("🔴 [%s] 客户端 %s 标记离线（%s）\n", listener, uid, reason)
	if command.GlobalEventBus != nil {
		command.PublishEvent(command.EventClientOffline, uid, map[string]interface{}{
			"listener": listener,
			"reason":   reason,
		})
	}
	command.CleanupClientQueues(uid)
	MuClientListenerType.Lock()
	delete(ClientListenerType, uid)
	MuClientListenerType.Unlock()
}

// ForceOffline 对单个客户端立即执行离线处理（SmartOfflineCheck 确认离线时调用），
// 与检测器超时路径完全一致（含 per-protocol cleanup 回调）。返回是否确有该注册。
func (d *OfflineDetector) ForceOffline(uid, reason string) bool {
	v, ok := d.clients.Load(uid)
	if !ok {
		return false
	}
	d.clients.Delete(uid)
	c := v.(*ClientActivity)
	if c.Cleanup != nil {
		c.Cleanup()
	}
	MarkOffline(uid, c.Listener, reason)
	return true
}

// Touch 更新客户端最后活动时间（任何活动：心跳/命令/结果/轮询时调用）
func (d *OfflineDetector) Touch(uid string) {
	if v, ok := d.clients.Load(uid); ok {
		v.(*ClientActivity).touch()
	}
}

// UpdateSleep 更新客户端的 sleep（SLEEP 命令调整时调用；1-300s 钳制，与客户端一致）
func (d *OfflineDetector) UpdateSleep(uid string, sleepSeconds int) {
	if sleepSeconds < 1 {
		sleepSeconds = 1
	}
	if sleepSeconds > 300 {
		sleepSeconds = 300
	}
	if v, ok := d.clients.Load(uid); ok {
		v.(*ClientActivity).sleepSeconds.Store(int32(sleepSeconds))
	}
	// sleep 调小会让离线截止时间提前，必须唤醒调度器重算
	d.wake()
}

// Unregister 注销客户端（连接断开/清理时调用）
func (d *OfflineDetector) Unregister(uid string) {
	d.clients.Delete(uid)
}

// GetClientState 获取客户端状态快照（SmartOfflineCheck 等统一消费，
// 阈值/节奏的唯一事实来源在检测器，不再各自从 DB 字段推算）
func (d *OfflineDetector) GetClientState(uid string) (ClientState, bool) {
	v, ok := d.clients.Load(uid)
	if !ok {
		return ClientState{}, false
	}
	c := v.(*ClientActivity)
	sleepSeconds, jitterPercent := c.beacon()
	return ClientState{
		LastActive:    c.lastActiveTime(),
		Listener:      c.Listener,
		SleepSeconds:  sleepSeconds,
		JitterPercent: jitterPercent,
	}, true
}

// GetLastActive 获取客户端最后活动时间（兼容旧调用方）
func (d *OfflineDetector) GetLastActive(uid string) (time.Time, string, bool) {
	s, ok := d.GetClientState(uid)
	return s.LastActive, s.Listener, ok
}

// IsRegistered 检查客户端是否已注册（即在线）
func (d *OfflineDetector) IsRegistered(uid string) bool {
	_, ok := d.clients.Load(uid)
	return ok
}

// run 截止时间驱动的调度器：睡到"最早一个客户端的离线截止时间"再检查，
// 而不是固定 ticker 轮询 —— 阈值一到就判离线（原 5s ticker 最多晚 5s）。
//
// Touch 只更新 LastActive、不唤醒调度器：截止时间只会往后推，调度器可能在旧
// 截止时间提前醒来，checkAll 发现无人过期后按新的 LastActive 重新计时。代价是
// 每个客户端每个阈值周期最多一次多余唤醒，远小于固定轮询的全量扫描开销。
func (d *OfflineDetector) run() {
	for {
		delay, ok := d.nextCheckDelay()
		if !ok {
			// 当前没有客户端：等到下一个 Register
			select {
			case <-d.stopCh:
				return
			case <-d.wakeCh:
			}
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-d.stopCh:
			timer.Stop()
			return
		case <-d.wakeCh:
			timer.Stop()
		case <-timer.C:
			d.checkAll()
		}
	}
}

// nextCheckDelay 返回距"最早离线截止时间"的时长；没有客户端时 ok=false。
// 上限 1 分钟：即使阈值很长（sleep 300s → 约 27min）也周期性重算一次，
// 防御时间跳变或异常数据导致的超长挂起。
func (d *OfflineDetector) nextCheckDelay() (time.Duration, bool) {
	var earliest time.Time
	d.clients.Range(func(_, value any) bool {
		c := value.(*ClientActivity)
		sleepSeconds, jitterPercent := c.beacon()
		_, _, critical := Thresholds(sleepSeconds, jitterPercent)
		deadline := c.lastActiveTime().Add(critical)
		if earliest.IsZero() || deadline.Before(earliest) {
			earliest = deadline
		}
		return true
	})
	if earliest.IsZero() {
		return 0, false
	}
	delay := time.Until(earliest)
	if delay < 0 {
		delay = 0
	}
	if delay > time.Minute {
		delay = time.Minute
	}
	return delay, true
}

// wake 非阻塞唤醒调度器重新计算截止时间（合并多次唤醒）。
func (d *OfflineDetector) wake() {
	if d.wakeCh == nil {
		return
	}
	select {
	case d.wakeCh <- struct{}{}:
	default:
	}
}

// checkAll 检查所有客户端是否超时
func (d *OfflineDetector) checkAll() {
	now := time.Now()
	var expired []*ClientActivity
	d.clients.Range(func(key, value any) bool {
		c := value.(*ClientActivity)
		sleepSeconds, jitterPercent := c.beacon()
		_, _, critical := Thresholds(sleepSeconds, jitterPercent)
		if now.Sub(c.lastActiveTime()) < critical {
			return true
		}
		// CompareAndDelete：只删"扫描时看到的那个实例"，避免误删期间重连写入的新条目。
		if !d.clients.CompareAndDelete(key, c) {
			return true
		}
		// 删除前一刻可能刚有 Touch：再确认一次；若已活跃则放回（期间若有新
		// Register 写入新条目，LoadOrStore 不会覆盖它）。
		if time.Since(c.lastActiveTime()) < critical {
			d.clients.LoadOrStore(key, c)
			return true
		}
		expired = append(expired, c)
		return true
	})
	for _, c := range expired {
		// 扫描删除后、真正落库前客户端可能已重连（新实例重新注册）：
		// 此时不能再用旧实例判离线，否则会把新连接的队列/在线状态一起清掉。
		if _, reconnected := d.clients.Load(c.UID); reconnected {
			continue
		}
		sleepSeconds, jitterPercent := c.beacon()
		idle := now.Sub(c.lastActiveTime())
		_, _, critical := Thresholds(sleepSeconds, jitterPercent)
		logx.Infof("🔴 [%s] 客户端 %s 统一检测离线（%.1f秒无活动，阈值%.1f秒）\n",
			c.Listener, c.UID, idle.Seconds(), critical.Seconds())
		if c.Cleanup != nil {
			c.Cleanup()
		}
		MarkOffline(c.UID, c.Listener, fmt.Sprintf("统一检测:%.0fs无活动", idle.Seconds()))
	}
}

// Stop 停止检测器
func (d *OfflineDetector) Stop() {
	d.stopOnce.Do(func() {
		if d.stopCh != nil {
			close(d.stopCh)
		}
	})
}
