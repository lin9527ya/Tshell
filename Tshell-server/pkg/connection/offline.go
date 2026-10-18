package connection

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"fmt"
	"strconv"
	"sync"
	"time"

	"BackendTemplate/pkg/utils/logx"
)

// ClientActivity 记录客户端活动状态（用于统一离线检测）。
type ClientActivity struct {
	UID           string
	Listener      string
	LastActive    time.Time
	SleepSeconds  int
	JitterPercent int    // beacon 抖动百分比（CS 模型，阈值按最坏拍间隔计算）
	Cleanup       func() // 离线时清理 per-protocol 连接 map（如 TCPClientManger）
}

// ClientState 对外只读的客户端状态快照（SmartOfflineCheck 等消费）
type ClientState struct {
	LastActive    time.Time
	Listener      string
	SleepSeconds  int
	JitterPercent int
}

// OfflineDetector 统一离线检测器。单 goroutine 定期检查所有客户端的活动时间，
// 超出阈值则标记离线。替代各协议各自 duplicated 的 checkHeartbeats goroutine。
type OfflineDetector struct {
	mu      sync.RWMutex
	clients map[string]*ClientActivity
	stopCh  chan struct{}
	running bool
}

// GlobalOfflineDetector 全局实例
var GlobalOfflineDetector *OfflineDetector

// InitOfflineDetector 初始化全局离线检测器
func InitOfflineDetector() {
	GlobalOfflineDetector = &OfflineDetector{
		clients: make(map[string]*ClientActivity),
		stopCh:  make(chan struct{}),
		running: true,
	}
	go GlobalOfflineDetector.run()
	logx.Println("✅ 统一离线检测器已启动")
}

// Register 注册客户端到离线检测器（FirstBlood 时调用）。cleanup 可选，离线时清理 per-protocol 连接 map。
// 【统一心跳】sleepSeconds 为客户端上报的真实 beacon sleep（旧客户端无上报时回退监听器配置）。
func (d *OfflineDetector) Register(uid, listener string, sleepSeconds int, cleanup ...func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := &ClientActivity{
		UID:           uid,
		Listener:      listener,
		LastActive:    time.Now(),
		SleepSeconds:  sleepSeconds,
		JitterPercent: beaconJitterPercent(),
	}
	if len(cleanup) > 0 {
		c.Cleanup = cleanup[0]
	}
	d.clients[uid] = c
}

// beaconJitterPercent 当前全局 beacon 抖动百分比（config client.jitter，0-50 钳制）
func beaconJitterPercent() int {
	if g := config.GetGlobalConfig(); g != nil {
		j := g.Client.Jitter
		if j < 0 {
			j = 0
		}
		if j > 50 {
			j = 50
		}
		return j
	}
	return 0
}

// ResolveSleepSeconds 解析客户端上报的 beacon sleep 秒数。
// MetaInfo 版本 >= 4 时 osArray[4] 携带客户端真实 sleep（CS 单一事实来源）；
// 旧客户端无此字段时回退监听器配置 interval；再回退默认 5s。
func ResolveSleepSeconds(osArray []string, fallbackListenerInterval string) int {
	// 优先客户端上报（唯一可信来源）
	if len(osArray) > 4 {
		if v, err := strconv.Atoi(osArray[4]); err == nil && v > 0 {
			if v > 300 {
				v = 300
			}
			return v
		}
	}
	// 旧客户端：回退监听器心跳间隔
	if v, err := strconv.Atoi(fallbackListenerInterval); err == nil && v > 0 {
		return v
	}
	return 5
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
	d.mu.Lock()
	c, ok := d.clients[uid]
	if ok {
		delete(d.clients, uid)
	}
	d.mu.Unlock()
	if !ok {
		return false
	}
	if c.Cleanup != nil {
		c.Cleanup()
	}
	MarkOffline(uid, c.Listener, reason)
	return true
}

// Touch 更新客户端最后活动时间（任何活动：心跳/命令/结果/轮询时调用）
func (d *OfflineDetector) Touch(uid string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c, ok := d.clients[uid]; ok {
		c.LastActive = time.Now()
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
	d.mu.Lock()
	defer d.mu.Unlock()
	if c, ok := d.clients[uid]; ok {
		c.SleepSeconds = sleepSeconds
	}
}

// Unregister 注销客户端（连接断开/清理时调用）
func (d *OfflineDetector) Unregister(uid string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.clients, uid)
}

// GetClientState 获取客户端状态快照（SmartOfflineCheck 等统一消费，
// 阈值/节奏的唯一事实来源在检测器，不再各自从 DB 字段推算）
func (d *OfflineDetector) GetClientState(uid string) (ClientState, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if c, ok := d.clients[uid]; ok {
		return ClientState{
			LastActive:    c.LastActive,
			Listener:      c.Listener,
			SleepSeconds:  c.SleepSeconds,
			JitterPercent: c.JitterPercent,
		}, true
	}
	return ClientState{}, false
}

// GetLastActive 获取客户端最后活动时间（兼容旧调用方）
func (d *OfflineDetector) GetLastActive(uid string) (time.Time, string, bool) {
	s, ok := d.GetClientState(uid)
	return s.LastActive, s.Listener, ok
}

// IsRegistered 检查客户端是否已注册（即在线）
func (d *OfflineDetector) IsRegistered(uid string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.clients[uid]
	return ok
}

// run 单 goroutine 定期检查所有客户端
func (d *OfflineDetector) run() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.checkAll()
		}
	}
}

// checkAll 检查所有客户端是否超时
func (d *OfflineDetector) checkAll() {
	d.mu.Lock()
	now := time.Now()
	var expired []*ClientActivity
	for uid, c := range d.clients {
		_, _, critical := Thresholds(c.SleepSeconds, c.JitterPercent)
		if now.Sub(c.LastActive) > critical {
			expired = append(expired, c)
			delete(d.clients, uid)
		}
	}
	d.mu.Unlock()
	for _, c := range expired {
		idle := now.Sub(c.LastActive)
		_, _, critical := Thresholds(c.SleepSeconds, c.JitterPercent)
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
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		close(d.stopCh)
		d.running = false
	}
}
