package command

import (
	"sync"
	"time"
)

// FileEditResult 文件编辑操作结果
type FileEditResult struct {
	Success   bool   `json:"success"`
	Content   string `json:"content,omitempty"` // 文件内容（读取时使用）
	Message   string `json:"message,omitempty"` // 操作结果消息
	ErrorMsg  string `json:"error,omitempty"`   // 错误消息
	Timestamp int64  `json:"timestamp"`         // 操作时间戳
}

// FileEditQueue 文件编辑队列管理器（按 requestId 严格匹配，支持并发多编辑框）
type FileEditQueue struct {
	mutex       sync.RWMutex
	readQueues  map[string]map[string]chan *FileEditResult // UID -> requestId -> Channel (读取)
	writeQueues map[string]map[string]chan *FileEditResult // UID -> requestId -> Channel (写入)
}

var VarFileEditQueue = &FileEditQueue{
	readQueues:  make(map[string]map[string]chan *FileEditResult),
	writeQueues: make(map[string]map[string]chan *FileEditResult),
}

// AddReadResult 添加文件读取结果（key 为 requestId 字符串，严格匹配）
func (q *FileEditQueue) AddReadResult(uid string, requestId string, result *FileEditResult) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.readQueues[uid] == nil {
		q.readQueues[uid] = make(map[string]chan *FileEditResult)
	}

	if _, exists := q.readQueues[uid][requestId]; !exists {
		q.readQueues[uid][requestId] = make(chan *FileEditResult, 1)
	}

	// 清空旧数据并发送新结果
	select {
	case <-q.readQueues[uid][requestId]:
	default:
	}

	result.Timestamp = time.Now().Unix()
	q.readQueues[uid][requestId] <- result
}

// AddWriteResult 添加文件写入结果（key 为 requestId 字符串，严格匹配）
func (q *FileEditQueue) AddWriteResult(uid string, requestId string, result *FileEditResult) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.writeQueues[uid] == nil {
		q.writeQueues[uid] = make(map[string]chan *FileEditResult)
	}

	if _, exists := q.writeQueues[uid][requestId]; !exists {
		q.writeQueues[uid][requestId] = make(chan *FileEditResult, 1)
	}

	// 清空旧数据并发送新结果
	select {
	case <-q.writeQueues[uid][requestId]:
	default:
	}

	result.Timestamp = time.Now().Unix()
	q.writeQueues[uid][requestId] <- result
}

// GetReadQueue 获取或创建文件读取队列（key 为 requestId 字符串）
func (q *FileEditQueue) GetReadQueue(uid string, requestId string) chan *FileEditResult {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.readQueues[uid] == nil {
		q.readQueues[uid] = make(map[string]chan *FileEditResult)
	}

	if _, exists := q.readQueues[uid][requestId]; !exists {
		q.readQueues[uid][requestId] = make(chan *FileEditResult, 1)
	}

	return q.readQueues[uid][requestId]
}

// GetWriteQueue 获取或创建文件写入队列（key 为 requestId 字符串）
func (q *FileEditQueue) GetWriteQueue(uid string, requestId string) chan *FileEditResult {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if q.writeQueues[uid] == nil {
		q.writeQueues[uid] = make(map[string]chan *FileEditResult)
	}

	if _, exists := q.writeQueues[uid][requestId]; !exists {
		q.writeQueues[uid][requestId] = make(chan *FileEditResult, 1)
	}

	return q.writeQueues[uid][requestId]
}

// CleanupQueues 清理指定UID的所有队列
func (q *FileEditQueue) CleanupQueues(uid string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	delete(q.readQueues, uid)
	delete(q.writeQueues, uid)
}

// RemoveReadEntry 移除指定 uid+requestId 的读取等待条目（handler 完成后主动回收，防止 map 无界增长）
func (q *FileEditQueue) RemoveReadEntry(uid string, requestId string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if m, ok := q.readQueues[uid]; ok {
		delete(m, requestId)
		if len(m) == 0 {
			delete(q.readQueues, uid)
		}
	}
}

// RemoveWriteEntry 移除指定 uid+requestId 的写入等待条目（handler 完成后主动回收，防止 map 无界增长）
func (q *FileEditQueue) RemoveWriteEntry(uid string, requestId string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if m, ok := q.writeQueues[uid]; ok {
		delete(m, requestId)
		if len(m) == 0 {
			delete(q.writeQueues, uid)
		}
	}
}
