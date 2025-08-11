package command

import "sync"

type FileContentQueue struct {
	mutex  sync.Mutex
	Queues map[string]map[string]chan string
}

var VarFileContentQueue = &FileContentQueue{Queues: make(map[string]map[string]chan string)}

func (q *FileContentQueue) Add(uid string, filePath, files string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.Queues[uid] == nil {
		q.Queues[uid] = make(map[string]chan string)
	}
	if _, exists := q.Queues[uid][filePath]; !exists {
		q.Queues[uid][filePath] = make(chan string, 1)
	}
	select {
	case <-q.Queues[uid][filePath]: // 清空旧数据
	default: // 若通道为空，继续发送
	}

	// 发送最新的 pids 数据
	q.Queues[uid][filePath] <- files
}

func (q *FileContentQueue) GetOrCreateQueue(uid string, filePath string) chan string {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.Queues[uid] == nil {
		q.Queues[uid] = make(map[string]chan string)
	}
	if _, exists := q.Queues[uid][filePath]; !exists {
		q.Queues[uid][filePath] = make(chan string, 1) // 带缓冲区的通道，防止阻塞
	}
	return q.Queues[uid][filePath]
}
