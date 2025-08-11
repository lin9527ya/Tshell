package command

import "sync"

type ClientCommandQueue struct {
	mu     sync.Mutex          // 互斥锁，保证并发安全
	queues map[string][][]byte // 客户端ID -> 命令队列
}

var CommandQueues = &ClientCommandQueue{
	queues: make(map[string][][]byte),
}

func (c *ClientCommandQueue) AddCommand(clientID string, command []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 检查是否存在clientID的队列，如果不存在则初始化
	if _, exists := c.queues[clientID]; !exists {
		c.queues[clientID] = [][]byte{} // 初始化为空切片
	}

	// 将命令添加到对应的队列
	c.queues[clientID] = append(c.queues[clientID], command)
}
func (c *ClientCommandQueue) GetCommand(clientID string) (command []byte, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 获取队列，如果不存在则初始化
	queue, exists := c.queues[clientID]
	if !exists {
		c.queues[clientID] = [][]byte{}
		return []byte{}, false
	}

	// 如果队列为空，返回空命令和false
	if len(queue) == 0 {
		return []byte{}, false
	}

	// 返回并移除队列中的第一个命令
	command, c.queues[clientID] = queue[0], queue[1:]
	return command, true
}
