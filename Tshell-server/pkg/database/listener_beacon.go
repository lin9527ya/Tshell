package database

import "strings"

// ListenerBeaconConfig 监听器级 beacon 配置（生成载荷与离线判定共用）。
//
// 未配置用 -1 表示（jitter 的 0 是合法值，不能用 0 当"未设置"）。
type ListenerBeaconConfig struct {
	SleepSeconds      int    // 1-300；-1 = 未配置（继承画像/全局）
	JitterPercent     int    // 0-50；-1 = 未配置（继承全局）
	HeartbeatTimeout  string // 兼容旧客户端/UI（遗留字段）
	HeartbeatInterval string // 旧客户端离线判定的 sleep 回退
	Profile           string // 绑定的 C2 画像名（空=default）
}

// GetListenerBeaconConfigByAddr 按地址定位监听器后取其 beacon 配置。
//
// 定位语义与 GetListenerHeartbeatConfigByAddr 完全一致（见 findListenerByAddr），
// 只是额外返回被解除引用后的 sleep/jitter 与画像名，供载荷生成和离线检测使用。
// 查不到记录 / Engine 未初始化时返回"全部继承"的默认配置。
func GetListenerBeaconConfigByAddr(addr, listenerType string) ListenerBeaconConfig {
	cfg := ListenerBeaconConfig{
		SleepSeconds:      -1,
		JitterPercent:     -1,
		HeartbeatTimeout:  "30",
		HeartbeatInterval: defaultHeartbeatInterval(),
	}
	l, ok := findListenerByAddr(addr, listenerType)
	if !ok {
		return cfg
	}
	if l.HeartbeatTimeout != "" {
		cfg.HeartbeatTimeout = l.HeartbeatTimeout
	}
	if l.HeartbeatInterval != "" {
		cfg.HeartbeatInterval = l.HeartbeatInterval
	}
	cfg.Profile = l.Profile
	if l.BeaconSleepSeconds != nil && *l.BeaconSleepSeconds >= 1 && *l.BeaconSleepSeconds <= 300 {
		cfg.SleepSeconds = *l.BeaconSleepSeconds
	}
	if l.BeaconJitterPercent != nil && *l.BeaconJitterPercent >= 0 && *l.BeaconJitterPercent <= 50 {
		cfg.JitterPercent = *l.BeaconJitterPercent
	}
	return cfg
}

// findListenerByAddr 按地址定位监听器（多实例同类型时必须按地址，不能按类型取任意一行）。
//
// 依次尝试：connect_address 精确 → listen_address 精确 → 同类型下按端口匹配
// （监听地址常绑通配地址，而连接的 LocalAddr 是具体 IP，只能按端口对）。
// 定位不到或 addr 为空时回退按类型（status=1），保持既有行为。
func findListenerByAddr(addr, listenerType string) (Listener, bool) {
	if Engine == nil {
		return Listener{}, false
	}
	if addr == "" {
		return listenerByType(listenerType)
	}
	var listener Listener
	has, err := Engine.Where("connect_address = ? OR listen_address = ?", addr, addr).Get(&listener)
	if err == nil && !has {
		// 端口兜底：取最后一个 ':' 之后的部分（IPv6 字面量也适用）
		if i := strings.LastIndex(addr, ":"); i >= 0 && i < len(addr)-1 {
			port := addr[i+1:]
			has, _ = Engine.Where("type = ? AND listen_address LIKE ?", listenerType, "%:"+port).Get(&listener)
		}
	}
	if err != nil || !has {
		return listenerByType(listenerType)
	}
	return listener, true
}

// listenerByType 取该类型下任意一个活跃监听器（按类型兜底的既有行为）。
func listenerByType(listenerType string) (Listener, bool) {
	if Engine == nil {
		return Listener{}, false
	}
	var listener Listener
	has, err := Engine.Where("type = ? AND status = 1", listenerType).Get(&listener)
	if err != nil || !has {
		return Listener{}, false
	}
	return listener, true
}
