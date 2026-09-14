package communication

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/encrypt"
	"encoding/json"
	"io"
	"net/http"

	"BackendTemplate/pkg/utils/logx"
)

// XTerm会话广播函数，由api包在初始化时设置（用于交互式终端输出路由）
var BroadcastToXTermSessions func(uid string, data []byte)

// HVNC会话广播函数，由api包在初始化时设置
var BroadcastToHVNCSessions func(uid, sid string, frame []byte)

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

	// 客户端回包统一处理（四协议共用 pkg/command/reply_handlers.go，B4 收敛）：
	// 随之补齐 HTTP 通道原缺失的密钥轮换确认（KeyRotateAck 121，轮换流程闭环）；
	// 截图判定统一为严格图像魔数 + data URI 前缀（原宽松的大数据包兜底移除）
	replyType, data, err := command.SplitReplyPayload(dataBytes)
	if err != nil {
		// 畸形短载荷：记日志但仍按原逻辑返回 200 伪装响应（不暴露校验细节、避免重试风暴）
		logx.Warnf("⚠️ [HTTP-POST] %v, UID: %s\n", err, uid)
	} else {
		logx.Infof("📨 [%s-POST] 处理消息 - UID: %s, 消息类型: %d, 数据长度: %d\n", listenerType, uid, replyType, len(data))
		if err := command.HandleClientReply(listenerType, uid, replyType, data, BroadcastToXTermSessions, BroadcastToHVNCSessions); err != nil {
			logx.Warnf("⚠️ [%s-POST] 处理回包失败 (type=%d): %v\n", listenerType, replyType, err)
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

