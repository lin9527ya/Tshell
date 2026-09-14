package proxy

import (
	"errors"
	"fmt"
	"io"
	"net"
)

// relayEndState 把转发结束时的正常连接关闭(io.EOF/net.ErrClosed)降级为"正常关闭"，
// 避免半关闭 teardown 在日志里刷 "use of closed network connection" 噪音；真实错误照常返回。
func relayEndState(err error) string {
	if err == nil {
		return "正常"
	}
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
		return "正常关闭"
	}
	return fmt.Sprintf("错误: %v", err)
}
