package command

import (
	"log"
	"sync"
)

// MonitorFrame 监控帧队列，用于存储监控帧数据
type MonitorFrameQueue struct {
	queues map[string]chan []byte
	mutex  sync.RWMutex
	// 【关键修复】添加请求进行状态跟踪，防止并发请求
	processing   map[string]bool
	processMutex sync.RWMutex
}

// VarMonitorFrameQueue 全局监控帧队列实例
var VarMonitorFrameQueue = &MonitorFrameQueue{
	queues: make(map[string]chan []byte),
	// 【关键修复】初始化请求进行状态跟踪
	processing: make(map[string]bool),
}

// GetOrCreateQueue 获取或创建指定UID的监控帧队列
func (mq *MonitorFrameQueue) GetOrCreateQueue(uid string) chan []byte {
	mq.mutex.Lock()
	defer mq.mutex.Unlock()

	if queue, exists := mq.queues[uid]; exists {
		return queue
	}

	// 创建新的缓冲队列，容量为3帧，避免阻塞和内存滥用
	// 【关键修复】减少缓冲区大小，避免大数据包堆积
	queue := make(chan []byte, 3)
	mq.queues[uid] = queue

	log.Printf("创建监控帧队列: UID=%s", uid)
	return queue
}

// Add 向指定UID的队列添加监控帧
// I2: 锁内非阻塞发送（原锁外 send 与 Remove 锁内 close 竞态 → "send on closed channel" panic，
// recover 后直接掐断客户端 C2 连接）。写锁与 Remove 互斥，channel 生命周期安全。
func (mq *MonitorFrameQueue) Add(uid string, frameData []byte) {
	mq.mutex.Lock()
	defer mq.mutex.Unlock()

	queue, exists := mq.queues[uid]
	if !exists {
		return
	}

	// 非阻塞发送，如果队列满了就丢弃所有旧帧，保持最新帧
	select {
	case queue <- frameData:
	default:
		// 队列满了，清空队列并放入新帧
		for {
			select {
			case <-queue:
			default:
				goto clearDone
			}
		}
	clearDone:
		select {
		case queue <- frameData:
		default:
		}
	}
}

// Remove 移除指定UID的监控帧队列
func (mq *MonitorFrameQueue) Remove(uid string) {
	mq.mutex.Lock()
	defer mq.mutex.Unlock()

	if queue, exists := mq.queues[uid]; exists {
		close(queue)
		delete(mq.queues, uid)
		log.Printf("移除监控帧队列: UID=%s", uid)
	}
}

// GetQueue 获取指定UID的队列（用于接收）
func (mq *MonitorFrameQueue) GetQueue(uid string) (chan []byte, bool) {
	mq.mutex.RLock()
	defer mq.mutex.RUnlock()

	queue, exists := mq.queues[uid]
	return queue, exists
}

// 【关键修复】添加请求去重功能，防止并发请求导致的问题

// IsProcessing 检查指定UID是否正在处理请求
func (mq *MonitorFrameQueue) IsProcessing(uid string) bool {
	mq.processMutex.RLock()
	defer mq.processMutex.RUnlock()
	return mq.processing[uid]
}

// SetProcessing 设置指定UID的请求处理状态
func (mq *MonitorFrameQueue) SetProcessing(uid string, processing bool) {
	mq.processMutex.Lock()
	defer mq.processMutex.Unlock()
	if processing {
		mq.processing[uid] = true
	} else {
		delete(mq.processing, uid)
	}
}
// RemoveEntries 删除指定客户端的监控帧通道与处理状态（断线/删除客户端时调用）
func (mq *MonitorFrameQueue) RemoveEntries(uid string) {
	mq.mutex.Lock()
	defer mq.mutex.Unlock()
	delete(mq.queues, uid)
	delete(mq.processing, uid)
}
