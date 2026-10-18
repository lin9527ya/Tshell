package command

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 下载终态：协议没有"结束"报文，靠"写满声明大小"与"句柄回收"两种终局判定。
// 这些回调是任务中心判断"文件下完了没有"的唯一依据，必须只触发一次且状态正确。

type endEvent struct {
	uid    string
	path   string
	status string
}

func withDownloadHook(t *testing.T) *[]endEvent {
	t.Helper()
	old := DownloadEndFunc
	events := &[]endEvent{}
	DownloadEndFunc = func(uid, path, status string) {
		*events = append(*events, endEvent{uid, path, status})
	}
	t.Cleanup(func() { DownloadEndFunc = old })
	return events
}

// newManager 直接构造（同包测试，避免为测试在生产代码里开后门）
func newManager() *DownloadFileManager {
	return &DownloadFileManager{files: make(map[string]map[uint32]*downloadEntry)}
}

func startDownload(t *testing.T, m *DownloadFileManager, uid, path string, size int64) uint32 {
	t.Helper()
	const reqID = uint32(7)
	if err := m.Start(uid, reqID, path, size); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	return reqID
}

// TestDownloadCompletesWhenDeclaredSizeReached 写满声明大小 → completed，且只回调一次。
func TestDownloadCompletesWhenDeclaredSizeReached(t *testing.T) {
	t.Chdir(t.TempDir())
	events := withDownloadHook(t)
	m := newManager()

	path := "/tmp/report.docx"
	reqID := startDownload(t, m, "uid-1", path, 10)

	ok, done, err := m.Append("uid-1", reqID, []byte("12345"))
	if !ok || done || err != nil {
		t.Fatalf("首个分块不应判定完成: ok=%v done=%v err=%v", ok, done, err)
	}
	ok, done, err = m.Append("uid-1", reqID, []byte("67890"))
	if !ok || !done || err != nil {
		t.Fatalf("写满应判定完成: ok=%v done=%v err=%v", ok, done, err)
	}
	if len(*events) != 1 || (*events)[0].status != DownloadStatusCompleted || (*events)[0].path != path {
		t.Fatalf("终态回调不符: %+v", *events)
	}

	// 完成后句柄已移除：再来分块不再重复回调（客户端可能多发一次尾部）
	if ok, _, _ := m.Append("uid-1", reqID, []byte("x")); ok {
		t.Fatal("完成后句柄应已移除")
	}
	if len(*events) != 1 {
		t.Fatalf("终态回调应只触发一次: %+v", *events)
	}
}

// TestDownloadResumeCountsExistingBytes 断点续传：从磁盘现有大小起算，不重复计数。
func TestDownloadResumeCountsExistingBytes(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	events := withDownloadHook(t)
	m := newManager()

	path := "/tmp/big.iso"
	reqID := startDownload(t, m, "uid-1", path, 10)

	// 模拟服务端已落盘 6 字节（上一轮连接中断后重连续传）
	dirName := "uid-1"
	target := filepath.Join("./Downloads/", dirName, filepath.Base(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(target, []byte("123456"), 0o644); err != nil {
		t.Fatalf("预置文件失败: %v", err)
	}
	// 重新 Start（模拟客户端重连后重新告知大小）
	if err := m.Start("uid-1", reqID, path, 10); err != nil {
		t.Fatalf("Start 失败: %v", err)
	}
	_, done, err := m.Append("uid-1", reqID, []byte("7890"))
	if err != nil || !done {
		t.Fatalf("续传补满应判定完成: done=%v err=%v", done, err)
	}
	if len(*events) != 1 || (*events)[0].status != DownloadStatusCompleted {
		t.Fatalf("续传终态不符: %+v", *events)
	}
}

// TestDownloadAbortedOnDisconnect 客户端断线 → failed（否则任务永远停在 dispatched）。
func TestDownloadAbortedOnDisconnect(t *testing.T) {
	t.Chdir(t.TempDir())
	events := withDownloadHook(t)
	m := newManager()

	reqID := startDownload(t, m, "uid-1", "/tmp/a.bin", 100)
	if _, _, err := m.Append("uid-1", reqID, []byte("half")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	m.CleanupAll("uid-1")

	if len(*events) != 1 || (*events)[0].status != DownloadStatusFailed || (*events)[0].path != "/tmp/a.bin" {
		t.Fatalf("断线终态不符: %+v", *events)
	}
	// 已完成的下载不会被断线重复标记（句柄早已移除）
	m.CleanupAll("uid-1")
	if len(*events) != 1 {
		t.Fatalf("重复清理不应重复回调: %+v", *events)
	}
}

// TestDownloadAbortedOnIdleTimeout 空闲超时（120s 无分块）→ failed。
func TestDownloadAbortedOnIdleTimeout(t *testing.T) {
	t.Chdir(t.TempDir())
	events := withDownloadHook(t)
	m := newManager()

	reqID := startDownload(t, m, "uid-1", "/tmp/stalled.bin", 100)
	if _, _, err := m.Append("uid-1", reqID, []byte("part")); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	// 把最后活动时间推到 TTL 之外（同包可直接改内部状态）
	m.files["uid-1"][reqID].lastActivity = time.Now().Add(-2 * downloadTTL)
	m.CleanupExpired()

	if len(*events) != 1 || (*events)[0].status != DownloadStatusFailed {
		t.Fatalf("超时终态不符: %+v", *events)
	}
}
