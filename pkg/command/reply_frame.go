package command

import (
	"encoding/binary"
	"fmt"
)

// SplitReplyPayload 拆分客户端回包载荷（replyType(4)|data），全协议共用。
// 统一边界校验：解密后至少 4 字节类型头，杜绝各协议手写 dataBytes[:4] 的
// 越界 panic（OSS 曾因此短包 panic → recover 终止整个监听 goroutine）。
func SplitReplyPayload(payload []byte) (replyType uint32, data []byte, err error) {
	if len(payload) < 4 {
		return 0, nil, fmt.Errorf("回包长度不足: 需要至少4字节，实际%d字节", len(payload))
	}
	return binary.BigEndian.Uint32(payload[:4]), payload[4:], nil
}
