package command

import (
	"encoding/binary"
	"strconv"
	"sync"
)

// FileOpQueue 文件操作结果/上传 ACK 按 (uid, requestId) 匹配，
// 替代 UID 单通道模型，支持并发文件操作互不干扰。
type FileOpQueue struct {
	mu sync.Mutex
	m  map[string]map[uint32]chan []byte // uid -> requestId -> chan
}

// VarFileOpQueue 全局文件操作队列
var VarFileOpQueue = &FileOpQueue{m: make(map[string]map[uint32]chan []byte)}

// Create 为指定 uid+requestId 创建等待通道（已存在则复用）
func (q *FileOpQueue) Create(uid string, requestId uint32) chan []byte {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.m[uid] == nil {
		q.m[uid] = make(map[uint32]chan []byte)
	}
	if ch, ok := q.m[uid][requestId]; ok {
		return ch
	}
	ch := make(chan []byte, 2)
	q.m[uid][requestId] = ch
	return ch
}

// Get 获取等待通道
func (q *FileOpQueue) Get(uid string, requestId uint32) (chan []byte, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.m[uid] == nil {
		return nil, false
	}
	ch, ok := q.m[uid][requestId]
	return ch, ok
}

// Add 投递结果（非阻塞；无等待者返回 false）
func (q *FileOpQueue) Add(uid string, requestId uint32, data []byte) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.m[uid] == nil {
		return false
	}
	ch, ok := q.m[uid][requestId]
	if !ok {
		return false
	}
	select {
	case ch <- data:
		return true
	default:
		return false
	}
}

// Remove 移除等待项
func (q *FileOpQueue) Remove(uid string, requestId uint32) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.m[uid] == nil {
		return
	}
	if ch, ok := q.m[uid][requestId]; ok {
		close(ch)
		delete(q.m[uid], requestId)
	}
	if len(q.m[uid]) == 0 {
		delete(q.m, uid)
	}
}

// RemoveAll 清理指定 uid 的所有等待项（客户端断开时调用）
func (q *FileOpQueue) RemoveAll(uid string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if chans, ok := q.m[uid]; ok {
		for _, ch := range chans {
			close(ch)
		}
		delete(q.m, uid)
	}
}

// ParseFileOpResult 解析 FILE_OP_RESULT：requestId(4)|ok(1)|msg
func ParseFileOpResult(data []byte) (requestId uint32, ok bool, msg string, valid bool) {
	if len(data) < 5 {
		return 0, false, "", false
	}
	requestId = binary.BigEndian.Uint32(data[:4])
	return requestId, data[4] == 1, string(data[5:]), true
}

// ParseUploadAck 解析 UPLOAD_ACK：requestId(4)|seq(4)|total(4)
func ParseUploadAck(data []byte) (requestId, seq, total uint32, valid bool) {
	if len(data) < 12 {
		return 0, 0, 0, false
	}
	requestId = binary.BigEndian.Uint32(data[:4])
	seq = binary.BigEndian.Uint32(data[4:8])
	total = binary.BigEndian.Uint32(data[8:12])
	return requestId, seq, total, true
}

// HandleFileEditRead 各传输层共用的 FILE_EDIT_READ 响应处理：
// requestId(4)|flag(1)|len(4)|content 或 requestId(4)|flag(1)|msg
func HandleFileEditRead(uid string, data []byte) {
	if len(data) < 5 {
		return
	}
	reqKey := strconv.Itoa(int(binary.BigEndian.Uint32(data[:4])))
	body := data[4:]
	success := body[0] == 1
	if success && len(body) >= 5 {
		contentLen := binary.BigEndian.Uint32(body[1:5])
		if len(body) >= 5+int(contentLen) {
			result := &FileEditResult{Success: true, Content: string(body[5 : 5+contentLen]), Message: "文件读取成功"}
			VarFileEditQueue.AddReadResult(uid, reqKey, result)
			return
		}
	}
	errorMsg := ""
	if len(body) > 1 {
		errorMsg = string(body[1:])
	}
	VarFileEditQueue.AddReadResult(uid, reqKey, &FileEditResult{Success: false, ErrorMsg: errorMsg, Message: "文件读取失败"})
}

// HandleFileEditWrite 各传输层共用的 FILE_EDIT_WRITE 响应处理：
// requestId(4)|flag(1)|msg
func HandleFileEditWrite(uid string, data []byte) {
	if len(data) < 5 {
		return
	}
	reqKey := strconv.Itoa(int(binary.BigEndian.Uint32(data[:4])))
	body := data[4:]
	success := body[0] == 1
	message := ""
	if len(body) > 1 {
		message = string(body[1:])
	}
	result := &FileEditResult{Success: success, Message: message}
	if !success {
		result.ErrorMsg = message
	}
	VarFileEditQueue.AddWriteResult(uid, reqKey, result)
}

// HandleUploadAck 各传输层共用的 UPLOAD_ACK 处理
func HandleUploadAck(uid string, data []byte) {
	if reqID, _, _, ok := ParseUploadAck(data); ok {
		VarFileOpQueue.Add(uid, reqID, data)
	}
}

// HandleFileOpResult 各传输层共用的 FILE_OP_RESULT 处理
func HandleFileOpResult(uid string, data []byte) {
	if reqID, _, _, ok := ParseFileOpResult(data); ok {
		VarFileOpQueue.Add(uid, reqID, data)
	}
}
// RemoveAllEntries 删除指定客户端的所有 requestId 等待条目（断线/删除客户端时调用）
func (q *FileOpQueue) RemoveAllEntries(uid string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.m, uid)
}
