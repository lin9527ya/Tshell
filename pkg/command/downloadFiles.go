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
	lastActivity time.Time
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

// Start 打开下载目标文件（O_CREATE|O_APPEND），已存在时从现有大小追加（配合客户端断点续传）
func (m *DownloadFileManager) Start(uid string, requestId uint32, filePath string) error {
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
	if m.files[uid] == nil {
		m.files[uid] = make(map[uint32]*downloadEntry)
	}
	m.files[uid][requestId] = &downloadEntry{fp: fp, lastActivity: time.Now()}
	return nil
}

// Append 追加下载分块；返回文件不存在（需重新 Start）的错误
func (m *DownloadFileManager) Append(uid string, requestId uint32, data []byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.files[uid] == nil {
		return false, nil
	}
	entry, ok := m.files[uid][requestId]
	if !ok {
		return false, nil
	}
	// 写错误检查：磁盘满/权限失败时明确报错，不再静默丢数据
	if _, err := entry.fp.Write(data); err != nil {
		return true, err
	}
	entry.lastActivity = time.Now()
	return true, nil
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
	defer m.mu.Unlock()
	if m.files[uid] != nil {
		for _, e := range m.files[uid] {
			e.fp.Close()
		}
		delete(m.files, uid)
	}
}

// CleanupExpired 清理超时未活动的下载会话（由各传输层周期调用或懒清理）
func (m *DownloadFileManager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now().Add(-downloadTTL)
	for uid, reqs := range m.files {
		for rid, e := range reqs {
			if e.lastActivity.Before(cutoff) {
				e.fp.Close()
				delete(reqs, rid)
			}
		}
		if len(reqs) == 0 {
			delete(m.files, uid)
		}
	}
}
