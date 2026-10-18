package command

import (
	"BackendTemplate/pkg/database"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// downloadEntry 单个下载会话（按 requestId 区分，支持并发下载互不污染）
type downloadEntry struct {
	fp           *os.File
	filePath     string
	declaredSize int64 // 客户端声明的文件大小（首包告知；0 = 未知）
	written      int64 // 已落盘字节数
	lastActivity time.Time
}

// 下载终态（通知任务层）
const (
	DownloadStatusCompleted = "completed"
	DownloadStatusFailed    = "failed"
)

// DownloadEndFunc 下载终态回调（completed / failed），由 main.go 注入 api.OnDownloadEnd。
// 协议里下载没有"结束"报文（客户端发完分块就停），此前服务端只能靠字节数/超时猜；
// 这里把"字节数达到声明大小"与"句柄因断线/空闲被回收"两种终局显式暴露出来，
// 让下载成为可归属、可查终态的任务，而不是只有一行进度。
var DownloadEndFunc func(uid, filePath, status string)

// notifyDownloadEnd 触发下载终态回调（调用方须已释放锁）
func notifyDownloadEnd(uid, filePath, status string) {
	if DownloadEndFunc != nil && filePath != "" {
		DownloadEndFunc(uid, filePath, status)
	}
}

// DownloadFileManager 管理下载文件句柄：
// 客户端下载分块按 requestId 路由到独立文件，防止并发下载数据交叉污染；
// 句柄在超时无活动后自动关闭清理（客户端下载完成后不再发分块）。
type DownloadFileManager struct {
	mu    sync.Mutex
	files map[string]map[uint32]*downloadEntry // uid -> requestId -> entry
}

// downloadTTL 下载分块空闲超时（超过则关闭句柄）
const downloadTTL = 120 * time.Second

// VarDownloadFiles 全局下载文件管理器
var VarDownloadFiles = &DownloadFileManager{files: make(map[string]map[uint32]*downloadEntry)}

// Start 打开下载目标文件（O_CREATE|O_APPEND），已存在时从现有大小追加（配合客户端断点续传）。
// declaredSize 来自客户端首包（文件大小）；从现有文件大小起算已写入量，续传时不重复计数。
func (m *DownloadFileManager) Start(uid string, requestId uint32, filePath string, declaredSize int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 清理同 requestId 残留句柄
	if m.files[uid] != nil {
		if old, ok := m.files[uid][requestId]; ok {
			old.fp.Close()
			delete(m.files[uid], requestId)
		}
	}

	// 确保下载目录存在（目录名使用客户端数字 ID，便于识别与归档）
	dirName := uid
	if database.Engine != nil {
		var client database.Clients
		if has, err := database.Engine.Where("uid = ?", uid).Get(&client); err == nil && has && client.ID > 0 {
			dirName = strconv.FormatInt(client.ID, 10)
		}
	}
	baseDir := filepath.Base(filePath)
	dir := "./Downloads/" + dirName
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}
	target := filepath.Join(dir, baseDir)

	fp, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return err
	}
	written := int64(0)
	if st, err := fp.Stat(); err == nil {
		written = st.Size()
	}
	if m.files[uid] == nil {
		m.files[uid] = make(map[uint32]*downloadEntry)
	}
	m.files[uid][requestId] = &downloadEntry{
		fp: fp, filePath: filePath, declaredSize: declaredSize,
		written: written, lastActivity: time.Now(),
	}
	return nil
}

// Append 追加下载分块；返回 (文件存在, 本次是否写满声明大小, 错误)。
// 写满即视为下载完成：关闭句柄、移出管理器并回调终态（只回调一次）。
func (m *DownloadFileManager) Append(uid string, requestId uint32, data []byte) (bool, bool, error) {
	m.mu.Lock()
	if m.files[uid] == nil {
		m.mu.Unlock()
		return false, false, nil
	}
	entry, ok := m.files[uid][requestId]
	if !ok {
		m.mu.Unlock()
		return false, false, nil
	}
	// 写错误检查：磁盘满/权限失败时明确报错，不再静默丢数据
	if _, err := entry.fp.Write(data); err != nil {
		m.mu.Unlock()
		return true, false, err
	}
	entry.lastActivity = time.Now()
	entry.written += int64(len(data))
	done := entry.declaredSize > 0 && entry.written >= entry.declaredSize
	filePath := entry.filePath
	if done {
		entry.fp.Close()
		delete(m.files[uid], requestId)
		if len(m.files[uid]) == 0 {
			delete(m.files, uid)
		}
	}
	m.mu.Unlock()

	// 回调在锁外触发（回调会写库，不能占着管理器锁）
	if done {
		notifyDownloadEnd(uid, filePath, DownloadStatusCompleted)
	}
	return true, done, nil
}

// Cleanup 关闭并移除指定下载会话
func (m *DownloadFileManager) Cleanup(uid string, requestId uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.files[uid] == nil {
		return
	}
	if entry, ok := m.files[uid][requestId]; ok {
		entry.fp.Close()
		delete(m.files[uid], requestId)
	}
	if len(m.files[uid]) == 0 {
		delete(m.files, uid)
	}
}

// CleanupAll 清理指定 uid 全部下载句柄（客户端断开时调用）
func (m *DownloadFileManager) CleanupAll(uid string) {
	m.mu.Lock()
	aborted := make([]string, 0, 2)
	if m.files[uid] != nil {
		for _, e := range m.files[uid] {
			e.fp.Close()
			aborted = append(aborted, e.filePath)
		}
		delete(m.files, uid)
	}
	m.mu.Unlock()

	// 客户端断线时仍在传的下载 → 终态 failed（否则任务永远停在 dispatched）
	for _, path := range aborted {
		notifyDownloadEnd(uid, path, DownloadStatusFailed)
	}
}

// CleanupExpired 清理超时未活动的下载会话（由各传输层周期调用或懒清理）
func (m *DownloadFileManager) CleanupExpired() {
	m.mu.Lock()
	type abortedDownload struct {
		uid  string
		path string
	}
	aborted := make([]abortedDownload, 0, 2)
	cutoff := time.Now().Add(-downloadTTL)
	for uid, reqs := range m.files {
		for rid, e := range reqs {
			if e.lastActivity.Before(cutoff) {
				e.fp.Close()
				aborted = append(aborted, abortedDownload{uid, e.filePath})
				delete(reqs, rid)
			}
		}
		if len(reqs) == 0 {
			delete(m.files, uid)
		}
	}
	m.mu.Unlock()

	// 空闲超时（120s 无分块）→ 终态 failed
	for _, a := range aborted {
		notifyDownloadEnd(a.uid, a.path, DownloadStatusFailed)
	}
}
