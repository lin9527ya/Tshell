package command

import "sync"

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

// DeleteClient 删除指定客户端的命令队列（断线/删除客户端时调用）
func (c *ClientCommandQueue) DeleteClient(clientID string) {
	c.queues.Delete(clientID)
}
