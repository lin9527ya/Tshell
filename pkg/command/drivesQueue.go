package command

import "sync"

type DrivesQueue struct {
	mutex  sync.Mutex
	Queues map[string]chan []string
}

var VarDrivesQueue = &DrivesQueue{Queues: make(map[string]chan []string)}

func (q *DrivesQueue) Add(uid string, files []string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if _, exists := q.Queues[uid]; !exists {
		q.Queues[uid] = make(chan []string, 1)
	}
	select {
	case <-q.Queues[uid]: // 清空旧数据
	default: // 若通道为空，继续发送
	}

	// 发送最新的 pids 数据
	q.Queues[uid] <- files
}

func (q *DrivesQueue) GetOrCreateQueue(uid string) chan []string {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if _, exists := q.Queues[uid]; !exists {
		q.Queues[uid] = make(chan []string, 1) // 带缓冲区的通道，防止阻塞
	}
	return q.Queues[uid]
}

func ParseDrives(uid string, drives []string) []*FileNode {
	for _, drive := range drives {
		if !exsitPan(UidFileBrowser[uid], drive) {
			UidFileBrowser[uid] = append(UidFileBrowser[uid], &FileNode{Name: drive, Type: "D", Path: drive})
		}
	}
	return UidFileBrowser[uid]
}
