package database

import (
	"BackendTemplate/pkg/utils/logx"
	"fmt"
)

// LogUpdate 统一 UPDATE 结果处理：err 告警、0 行降级为 Debug（多为客户端行
// 已删除后的残余连接心跳，属正常噪声）。热路径调用，仅失败路径有 IO（日志）。
// 背景：连接层 12+ 处裸调用丢弃 xorm Update 的 (int64, error)，磁盘满/库锁
// 超时/uid 行不存在时完全静默，表现为"在线状态幽灵翻转且无日志可查"。
func LogUpdate(affected int64, err error, ctx string) {
	if err != nil {
		logx.Warnf("⚠️ [DB] %s 更新失败: %v\n", ctx, err)
		return
	}
	if affected == 0 {
		logx.Debugf("[DB] %s 更新影响 0 行（记录可能已删除）\n", ctx)
	}
}

// SetClientOnline 客户端在线状态写库收敛入口（FirstBlood/重连/心跳恢复时调用）。
// 幂等；listener 非空时一并刷新监听器类型归属。语义与原各协议散落的
// Engine.Where("uid = ?").Update(&Clients{Online:"1"[, Listener:...]}) 完全一致，
// 仅增加结果日志。失败只告警不中断——状态标记写不应打断命令收发主链路。
func SetClientOnline(uid, listener string) {
	if Engine == nil {
		return
	}
	var (
		affected int64
		err      error
	)
	if listener != "" {
		affected, err = Engine.Where("uid = ?", uid).Update(&Clients{Online: "1", Listener: listener})
	} else {
		affected, err = Engine.Where("uid = ?", uid).Update(&Clients{Online: "1"})
	}
	LogUpdate(affected, err, fmt.Sprintf("SetClientOnline uid=%s listener=%s", uid, listener))
}
