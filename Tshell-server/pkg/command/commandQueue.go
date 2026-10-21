package command

import (
	"encoding/binary"
	"sync"
)

// clientQueue 单个客户端的下行命令队列（独立锁，消除全局锁争用）
type clientQueue struct {
	mu   sync.Mutex
	data [][]byte
}

type ClientCommandQueue struct {
	queues sync.Map // clientID -> *clientQueue（分片锁，各客户端互不阻塞）
}

var CommandQueues = &ClientCommandQueue{}

// QueueLen 返回客户端当前队列长度(任务可视化用)
func (c *ClientCommandQueue) QueueLen(clientID string) int {
	v, exists := c.queues.Load(clientID)
	if !exists {
		return 0
	}
	cq := v.(*clientQueue)
	cq.mu.Lock()
	defer cq.mu.Unlock()
	return len(cq.data)
}

// Stats 汇总当前所有客户端队列的积压情况（指标用）：
// 返回 (积压总数, 单客户端最大积压, 有积压的客户端数)。
// 遍历 sync.Map 是 O(客户端数)，仅在指标抓取（默认 15s 一次）时调用。
func (c *ClientCommandQueue) Stats() (depth, maxPerClient, clients int64) {
	c.queues.Range(func(_, v interface{}) bool {
		cq, ok := v.(*clientQueue)
		if !ok {
			return true
		}
		cq.mu.Lock()
		n := int64(len(cq.data))
		cq.mu.Unlock()
		depth += n
		if n > maxPerClient {
			maxPerClient = n
		}
		if n > 0 {
			clients++
		}
		return true
	})
	return depth, maxPerClient, clients
}

// MaxQueuePerClient 单个客户端命令队列上限（块），防止大文件上传全量驻留内存
const MaxQueuePerClient = 1024

// AddCommand 追加命令到客户端队列。
// 队列满时返回 false（背压信号，不丢弃旧命令）——调用方据此告警/放弃，
// 避免旧实现"丢最旧"导致分块上传/长命令静默损坏。
func (c *ClientCommandQueue) AddCommand(clientID string, command []byte) bool {
	v, _ := c.queues.LoadOrStore(clientID, &clientQueue{})
	cq := v.(*clientQueue)
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if len(cq.data) >= MaxQueuePerClient {
		return false
	}
	cq.data = append(cq.data, command)
	// 任务队列可视化:记录入队命令
	if GlobalCmdHistory != nil {
		GlobalCmdHistory.Record(clientID, string(command))
	}
	return true
}

// GetCommand 取出并移除队列首条命令
func (c *ClientCommandQueue) GetCommand(clientID string) (command []byte, ok bool) {
	v, exists := c.queues.Load(clientID)
	if !exists {
		return []byte{}, false
	}
	cq := v.(*clientQueue)
	cq.mu.Lock()
	defer cq.mu.Unlock()

	if len(cq.data) == 0 {
		return []byte{}, false
	}

	// queue[0]=nil 释放引用，防止底层数组残留已弹出命令（长连接积压的内存泄漏）
	command = cq.data[0]
	cq.data[0] = nil
	// 任务队列可视化:标记已取走
	if GlobalCmdHistory != nil {
		GlobalCmdHistory.MarkRemoved(clientID, string(command))
	}
	newData := cq.data[1:]
	if len(newData) == 0 {
		cq.data = nil // 空队列整体释放底层数组
	} else {
		cq.data = newData
	}
	return command, true
}

// GetCommands 批量取出并移除至多 max 条命令（保序；不足则全取）。
// 供轮询协议(HTTP/DNS)批量下发使用（CMD_BATCH, B5）:每轮单条拉取时批量任务
// 每条命令一个完整 RTT,分块上传等连续任务极慢。
func (c *ClientCommandQueue) GetCommands(clientID string, max int) [][]byte {
	if max <= 0 {
		return nil
	}
	v, exists := c.queues.Load(clientID)
	if !exists {
		return nil
	}
	cq := v.(*clientQueue)
	cq.mu.Lock()
	defer cq.mu.Unlock()

	n := len(cq.data)
	if n == 0 {
		return nil
	}
	if n > max {
		n = max
	}
	out := make([][]byte, n)
	copy(out, cq.data[:n])
	// 任务队列可视化:标记已取走
	if GlobalCmdHistory != nil {
		for _, cmd := range out {
			GlobalCmdHistory.MarkRemoved(clientID, string(cmd))
		}
	}
	// 已取部分置 nil 释放引用(同 GetCommand 的内存泄漏防护),余量前移
	for i := 0; i < n; i++ {
		cq.data[i] = nil
	}
	newData := cq.data[n:]
	if len(newData) == 0 {
		cq.data = nil // 空队列整体释放底层数组
	} else {
		cq.data = newData
	}
	return out
}

// GetCommandCoalescingInput 取出队首命令；若队首是交互式终端输入（cmdType 为
// interactiveInputType），则把紧随其后的同类命令 payload 一并取出拼成一条。
//
// 动机：轮询传输（http/https）一轮 GET 只取一条命令，而 Web 终端是「每敲一个字符
// 就入队一条 INTERACTIVE_SHELL_INPUT」。不合并时，打字期间积压在队列里的每个字符
// 都要单独等到下一轮轮询才下发，交互式终端表现为「输入有 1 个轮询间隔的延迟」。
// 合并后一轮 GET 就能把已敲的字符整批送下去，字符顺序与字节内容完全不变。
//
// 只合并「队首连续同类」命令：中间夹了别的命令就停下，保证与其它命令的相对顺序不变。
// 每个被取走的条目仍逐条记入任务队列历史（MarkRemoved），与 GetCommand 行为一致。
func (c *ClientCommandQueue) GetCommandCoalescingInput(clientID string, interactiveInputType uint32) (command []byte, ok bool) {
	v, exists := c.queues.Load(clientID)
	if !exists {
		return []byte{}, false
	}
	cq := v.(*clientQueue)
	cq.mu.Lock()

	n := len(cq.data)
	if n == 0 {
		cq.mu.Unlock()
		return []byte{}, false
	}

	first := cq.data[0]
	// 队首不是交互式输入：与 GetCommand 完全一致，只弹一条
	if !hasCmdType(first, interactiveInputType) {
		cq.data[0] = nil
		if n == 1 {
			cq.data = nil
		} else {
			cq.data = cq.data[1:]
		}
		cq.mu.Unlock()
		if GlobalCmdHistory != nil {
			GlobalCmdHistory.MarkRemoved(clientID, string(first))
		}
		return first, true
	}

	// 队首是交互式输入：连续同类全部弹出，payload 按字节拼接
	merged := first
	popped := make([][]byte, 0, 4)
	popped = append(popped, first)
	take := 1
	for take < n {
		next := cq.data[take]
		if !hasCmdType(next, interactiveInputType) {
			break
		}
		merged = append(merged, next[4:]...)
		popped = append(popped, next)
		take++
	}
	for i := 0; i < take; i++ {
		cq.data[i] = nil // 释放引用，防底层数组残留
	}
	rest := cq.data[take:]
	if len(rest) == 0 {
		cq.data = nil
	} else {
		cq.data = rest
	}
	cq.mu.Unlock()

	if GlobalCmdHistory != nil {
		for _, cmd := range popped {
			GlobalCmdHistory.MarkRemoved(clientID, string(cmd))
		}
	}
	return merged, true
}

// hasCmdType 判断一条待发送命令的类型头是否为 typ（命令至少要有 4 字节类型头）。
func hasCmdType(cmd []byte, typ uint32) bool {
	if len(cmd) < 4 {
		return false
	}
	return binary.BigEndian.Uint32(cmd[:4]) == typ
}

// DeleteClient 删除指定客户端的命令队列（断线/删除客户端时调用）
func (c *ClientCommandQueue) DeleteClient(clientID string) {
	c.queues.Delete(clientID)
}
