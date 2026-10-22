package command

import "sync"

// execCapable 记录"该客户端支持 EXEC_DONE / EXD1 任务头"（按 uid）。
//
// 客户端在首次成功收包后上报 EXEC_CAP；服务端重启丢失 map 后，收到任意一条
// EXEC_DONE 也会重新标记（见 reply_handlers.go），所以不需要持久化。
var execCapable sync.Map // uid -> struct{}

// execOutputReady 记录客户端已经实际发过 EXEC_OUTPUT（type=98）。
// 在收到第一条 98 之前，服务端保持"exec 队首即停"的保守归属，
// 避免能力上报与启用命令之间的窗口里把 type-0 输出误归给 console 任务。
var execOutputReady sync.Map // uid -> struct{}

// MarkExecClientCapable 标记客户端支持显式执行完成回执。
func MarkExecClientCapable(uid string) {
	if uid == "" {
		return
	}
	execCapable.Store(uid, struct{}{})
}

// IsExecClientCapable 查询客户端能力；旧客户端返回 false，保持原有
// "输出即完成"的任务语义，避免回归。
func IsExecClientCapable(uid string) bool {
	_, ok := execCapable.Load(uid)
	return ok
}

// ClearExecClientCapability 客户端离线/清理时移除能力标记（重连会重新上报）。
func ClearExecClientCapability(uid string) {
	execCapable.Delete(uid)
	execOutputReady.Delete(uid)
}

// MarkExecOutputReady 标记客户端已开始使用 EXEC_OUTPUT(98)。
func MarkExecOutputReady(uid string) {
	if uid == "" {
		return
	}
	execOutputReady.Store(uid, struct{}{})
}

// IsExecOutputReady 客户端是否已实际发过 EXEC_OUTPUT。
func IsExecOutputReady(uid string) bool {
	_, ok := execOutputReady.Load(uid)
	return ok
}
