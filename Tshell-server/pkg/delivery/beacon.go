package delivery

import (
	"strconv"
	"strings"

	"BackendTemplate/pkg/config"
)

// Beacon 生成期节奏参数（CS 语义）：sleep 秒数 + jitter 百分比。
//
// 优先级（监听器 > 画像 > 全局 > 硬默认）：
//   sleep:  override > listener > profile.sleeptime_ms/1000 > client.sleep > 5
//   jitter: override > listener > client.jitter > 20
//
// 这里集中解析，载荷生成（PatchClientBinary）与监听器列表的"生效值"共用同一套
// 逻辑，UI 显示与真正编译进客户端占位符的值不会漂移。
const (
	BeaconUnset         = -1 // "未配置"哨兵（jitter 的 0 是合法值，不能用 0 表示未设置）
	BeaconSleepMin      = 1
	BeaconSleepMax      = 300
	BeaconJitterMin     = 0
	BeaconJitterMax     = 50
	BeaconDefaultSleep  = 5
	BeaconDefaultJitter = 20
)

// ResolveBeaconSleep 计算最终写入客户端的 beacon sleep（秒）。
// override 为空/非法时依次回落到监听器值、画像 sleeptime_ms、全局 client.sleep。
func ResolveBeaconSleep(override string, listenerSleep, profileSleepMs int) int {
	global := BeaconDefaultSleep
	if sleepSeconds, _ := config.ClientTiming(); sleepSeconds > 0 {
		global = sleepSeconds
	}
	return resolveBeaconSleep(override, listenerSleep, profileSleepMs, global)
}

// resolveBeaconSleep 可测内核：全局值显式传入，测试不依赖包级配置。
func resolveBeaconSleep(override string, listenerSleep, profileSleepMs, globalSleep int) int {
	if v, ok := parseBeaconOverride(override, BeaconSleepMin, BeaconSleepMax); ok {
		return v
	}
	if listenerSleep >= BeaconSleepMin && listenerSleep <= BeaconSleepMax {
		return listenerSleep
	}
	if profileSleepMs > 0 {
		// 画像 sleeptime_ms 是毫秒 → 秒，向下取整且至少 1s
		sec := profileSleepMs / 1000
		if sec < BeaconSleepMin {
			sec = BeaconSleepMin
		}
		if sec > BeaconSleepMax {
			sec = BeaconSleepMax
		}
		return sec
	}
	if globalSleep >= BeaconSleepMin && globalSleep <= BeaconSleepMax {
		return globalSleep
	}
	return BeaconDefaultSleep
}

// ResolveBeaconJitter 计算最终写入客户端的 jitter 百分比。
// override 为空/非法时依次回落到监听器值、全局 client.jitter。
func ResolveBeaconJitter(override string, listenerJitter int) int {
	global := BeaconDefaultJitter
	if _, jitterPercent := config.ClientTiming(); jitterPercent >= BeaconJitterMin && jitterPercent <= BeaconJitterMax {
		global = jitterPercent
	}
	return resolveBeaconJitter(override, listenerJitter, global)
}

// resolveBeaconJitter 可测内核：显式 0 是合法配置，必须被保留。
func resolveBeaconJitter(override string, listenerJitter, globalJitter int) int {
	if v, ok := parseBeaconOverride(override, BeaconJitterMin, BeaconJitterMax); ok {
		return v
	}
	if listenerJitter >= BeaconJitterMin && listenerJitter <= BeaconJitterMax {
		return listenerJitter
	}
	if globalJitter >= BeaconJitterMin && globalJitter <= BeaconJitterMax {
		return globalJitter
	}
	return BeaconDefaultJitter
}

// parseBeaconOverride 解析字符串覆盖值（PatchOptions 里的显式参数）。
// 空串、非数字、越界都视为"未提供"，由调用链继续回落。
func parseBeaconOverride(s string, min, max int) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < min || v > max {
		return 0, false
	}
	return v, true
}
