package command

import (
	"sync"
)

type PidQueue struct {
	mutex  sync.Mutex
	Queues map[string]chan string
}

var VarPidQueue = &PidQueue{Queues: make(map[string]chan string)}

func (q *PidQueue) Add(uid string, pids string) {
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

	// 发送最新的 pids 数据
	q.Queues[uid] <- pids
}

// 获取或创建 UID 队列的方法
func (q *PidQueue) GetOrCreateQueue(uid string) chan string {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if _, exists := q.Queues[uid]; !exists {
		q.Queues[uid] = make(chan string, 1) // 带缓冲区的通道，防止阻塞
	}
	return q.Queues[uid]
}
