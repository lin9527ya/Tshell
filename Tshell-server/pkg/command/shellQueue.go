package command

import (
	"sync"
)

type ShellQueue struct {
	mutex  sync.Mutex
	Queues map[string]chan string
}

var VarShellQueue = &ShellQueue{Queues: make(map[string]chan string)}

func (q *ShellQueue) Add(uid string, shellResponse string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if _, exists := q.Queues[uid]; !exists {
		q.Queues[uid] = make(chan string, 1)
	}
	// 如果通道已满，则先清除旧数据以避免阻塞
	select {
	case <-q.Queues[uid]: // 清空旧数据
	default: // 若通道为空，继续发送
	}

	// 发送最新的shell响应数据
	q.Queues[uid] <- shellResponse
}

// 获取或创建 UID 队列的方法
func (q *ShellQueue) GetOrCreateQueue(uid string) chan string {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if _, exists := q.Queues[uid]; !exists {
		q.Queues[uid] = make(chan string, 1) // 带缓冲区的通道，防止阻塞
	}
	return q.Queues[uid]
}
// Remove 删除指定客户端的通道（断线/删除客户端时调用）
func (q *ShellQueue) Remove(uid string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	delete(q.Queues, uid)
}
