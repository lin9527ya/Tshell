package command

import (
	"BackendTemplate/pkg/utils"
	"sync"
)

// ===== B5: 轮询协议批量命令下发 =====
//
// 能力协商: 客户端在 MetaInfo 的 osInfo 追加第 6 段 "b1"(客户端 core/utils/utils.go
// MakeMetaInfo 生成)。旧客户端无此段 → 永远单条下发,零行为变化(fail-closed);
// 旧服务端遇到第 6 段也只按 \t split 取前几段,同样无影响(双向兼容)。

// batchCapableClients 上报 b1 能力标记的客户端集合(per-uid)
var batchCapableClients = struct {
	sync.RWMutex
	m map[string]bool
}{m: make(map[string]bool)}

// batchMaxCount 单轮最多打包的命令条数(与客户端 dispatch 的解析上限无耦合,
// 但不宜过大:批量帧随单条响应下发,过大易触发中间设备截断)
const batchMaxCount = 16

// BatchMaxCount 对外暴露单轮批量上限(取用方传参用)
func BatchMaxCount() int { return batchMaxCount }

// batchMaxBytes 单轮批量帧累计载荷上限(字节,防大命令(上传分块等)拼出超大响应)
const batchMaxBytes = 1024 * 1024

// MarkBatchCapable 记录客户端的批量能力(FirstBlood/重连解析 MetaInfo 时调用)。
// capable=false 或 uid 为空时清除标记(客户端重连降级/清理场景)。
func MarkBatchCapable(uid string, capable bool) {
	if uid == "" {
		return
	}
	batchCapableClients.Lock()
	defer batchCapableClients.Unlock()
	if capable {
		batchCapableClients.m[uid] = true
	} else {
		delete(batchCapableClients.m, uid)
	}
}

// IsBatchCapable 客户端是否上报了 b1 能力
func IsBatchCapable(uid string) bool {
	batchCapableClients.RLock()
	defer batchCapableClients.RUnlock()
	return batchCapableClients.m[uid]
}

// ParseBatchCapability 从 MetaInfo 的 osArray 判定批量能力(osInfo 第 6 段 == "b1")
func ParseBatchCapability(osArray []string) bool {
	return len(osArray) > 5 && osArray[5] == "b1"
}

// BuildBatchFrame 将多条命令打包为 CMD_BATCH 帧:
// CMD_BATCH(4) | count(4) | N×[len(4)|cmd]。cmds 为空返回 nil。
func BuildBatchFrame(cmds [][]byte) []byte {
	if len(cmds) == 0 {
		return nil
	}
	total := len(cmds)
	if total > batchMaxCount {
		total = batchMaxCount
	}
	// 先按字节上限筛选(保序,超出即停)
	// sizes 记录纯命令体长度(len(cmd),不含长度字段自身)——读回方按此值
	// 读命令体,若含长度字段自身会越界吞掉下一条的长度头
	sizes := make([]int, 0, total)
	bytes := 4 + 4 // cmdType + count
	n := 0
	for _, cmd := range cmds[:total] {
		if bytes+4+len(cmd) > batchMaxBytes {
			break
		}
		sizes = append(sizes, len(cmd))
		bytes += 4 + len(cmd)
		n++
	}
	if n == 0 {
		return nil
	}
	frame := make([]byte, 0, bytes)
	frame = append(frame, utils.WriteInt(CMD_BATCH)...)
	frame = append(frame, utils.WriteInt(n)...)
	for i := 0; i < n; i++ {
		frame = append(frame, utils.WriteInt(sizes[i])...)
		frame = append(frame, cmds[i]...)
	}
	return frame
}
