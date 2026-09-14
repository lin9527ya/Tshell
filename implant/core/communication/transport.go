package communication

import "sync/atomic"

// Transport 抽象各协议传输层，由各 adapter 实现。
// core 的命令派发与结果回送通过此接口与具体协议解耦，新增协议只需实现该接口。
type Transport interface {
	// Connect 建立连接/准备端点。host 为监听器地址（占位符在生成时被替换为真实值）。
	Connect(host string) error
	// Recv 取一条服务端下发命令的原始数据（持久传输阻塞读；轮询传输发起一次拉取）。
	Recv() ([]byte, error)
	// SendResult 回送已 base64 编码的加密结果包，由实现做协议特定的分帧与发送
	// （tcp: MetaInfo+normalData 分帧 + 分块写；http: POST body；kcp/ws/oss 各自处理）。
	SendResult(b64Packet []byte) error
	// Close 关闭连接/释放资源。
	Close() error
}

// CurrentTransport 由 adapter 在启动时注入其 Transport 实现，供共享派发函数回送结果。
// 【修复】原为裸全局变量：重连循环（runLoop）写入、心跳/结果 goroutine 并发读取，
// 存在数据竞争；改为 atomic.Pointer，读写均为原子操作。
var currentTransport atomic.Pointer[Transport]

// SetTransport 注入当前传输实现（adapter 重连成功后调用）。
func SetTransport(t Transport) {
	currentTransport.Store(&t)
}

// GetTransport 获取当前传输实现（无注入时返回 nil）。
func GetTransport() Transport {
	if p := currentTransport.Load(); p != nil {
		return *p
	}
	return nil
}
