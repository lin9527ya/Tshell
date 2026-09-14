package command

import (
	"BackendTemplate/pkg/database"
	"encoding/binary"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHandleClientReplyShellAppend 验证共享回包处理：命令行输出走 SQL 增量追加
func TestHandleClientReplyShellAppend(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	uid := "testuid00000000000000000000000001"
	database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
	database.Engine.Insert(&database.Clients{Uid: uid, Online: "1"})

	// 回包类型 0（命令行输出）
	payload := []byte("hello world")
	err := HandleClientReply("tcp", uid, 0, payload, nil, nil)
	if err != nil {
		t.Fatalf("HandleClientReply: %v", err)
	}

	// P1-5: shell 追加改为异步批量落库（500ms flush），需轮询等待写入完成
	var shell database.Shell
	deadline := time.Now().Add(3 * time.Second)
	for {
		shell = database.Shell{}
		database.Engine.Where("uid = ?", uid).Get(&shell)
		if shell.ShellContent == "hello world\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("shell 追加超时: %q", shell.ShellContent)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 下载进度节流（回包 26, requestId(4)|chunk）
	reqID := make([]byte, 4)
	binary.BigEndian.PutUint32(reqID, 1)
	chunk := append(reqID, []byte("chunkdata")...)
	database.Engine.Insert(&database.Downloads{Uid: uid, FilePath: "/tmp/x", FileSize: 100, DownloadedSize: 0})
	if err := HandleClientReply("tcp", uid, DOWNLOAD, chunk, nil, nil); err != nil {
		t.Fatalf("download: %v", err)
	}
	// 下载进度为异步节流落库, 不做断言(避免时序), 仅验证不报错

	// 未知类型不应报错
	if err := HandleClientReply("tcp", uid, 9999, []byte("x"), nil, nil); err != nil {
		t.Fatalf("unknown type should not error: %v", err)
	}
}

// TestSplitReplyPayload 验证统一拆包（B4）：合法载荷 / 短载荷报错 / 恰好 4 字节（空 data）
func TestSplitReplyPayload(t *testing.T) {
	rt, data, err := SplitReplyPayload([]byte{0, 0, 0, 36, 'a', 'b'})
	if err != nil || rt != 36 || string(data) != "ab" {
		t.Fatalf("合法载荷拆包失败: rt=%d data=%q err=%v", rt, data, err)
	}
	if _, _, err := SplitReplyPayload([]byte{1, 2, 3}); err == nil {
		t.Fatal("3 字节载荷应报错")
	}
	if rt, data, err := SplitReplyPayload([]byte{0, 0, 0, 49}); err != nil || rt != 49 || len(data) != 0 {
		t.Fatalf("恰好 4 字节应拆出空 data: rt=%d len=%d err=%v", rt, len(data), err)
	}
}

// TestHandleClientReplyBroadcasts 验证 B4 收敛后的广播回调注入路径（OSS/DNS 原缺失）：
// XTerm 输出(36)/错误回包(31) 走 xtermBroadcast；HVNC 帧(58) 走 hvncBroadcast
func TestHandleClientReplyBroadcasts(t *testing.T) {
	uid := "bcasttest0000000000000000000001"

	xtermHits := make(chan []byte, 4)
	xtermBroadcast := func(u string, d []byte) {
		if u != uid {
			t.Errorf("xtermBroadcast uid = %q, want %q", u, uid)
		}
		xtermHits <- d
	}
	hvncHits := make(chan string, 1)
	hvncBroadcast := func(u, sid string, frame []byte) {
		if u != uid {
			t.Errorf("hvncBroadcast uid = %q, want %q", u, uid)
		}
		hvncHits <- sid
	}

	// INTERACTIVE_SHELL_INPUT(36)：终端输出路由到 XTerm 会话
	if err := HandleClientReply("oss", uid, INTERACTIVE_SHELL_INPUT, []byte("term-out"), xtermBroadcast, nil); err != nil {
		t.Fatalf("INTERACTIVE_SHELL_INPUT: %v", err)
	}
	select {
	case d := <-xtermHits:
		if string(d) != "term-out" {
			t.Fatalf("xtermBroadcast data = %q", d)
		}
	case <-time.After(time.Second):
		t.Fatal("INTERACTIVE_SHELL_INPUT 未触发 xtermBroadcast")
	}

	// 错误回包(31)：广播到 XTerm 会话（前端可见提示）
	if err := HandleClientReply("dns", uid, 31, []byte("err-payload"), xtermBroadcast, nil); err != nil {
		t.Fatalf("replyType 31: %v", err)
	}
	select {
	case d := <-xtermHits:
		if string(d) != "err-payload" {
			t.Fatalf("replyType 31 xtermBroadcast data = %q", d)
		}
	case <-time.After(time.Second):
		t.Fatal("replyType 31 未触发 xtermBroadcast")
	}

	// HVNC_FRAME(58)：[4B sidLen][sid][JPEG]
	sid := "sess-1"
	frame := append([]byte{0, 0, 0, byte(len(sid))}, sid...)
	frame = append(frame, 0xFF, 0xD8, 0xFF, 0xE0)
	if err := HandleClientReply("oss", uid, HVNC_FRAME, frame, nil, hvncBroadcast); err != nil {
		t.Fatalf("HVNC_FRAME: %v", err)
	}
	select {
	case got := <-hvncHits:
		if got != sid {
			t.Fatalf("hvncBroadcast sid = %q, want %q", got, sid)
		}
	case <-time.After(time.Second):
		t.Fatal("HVNC_FRAME 未触发 hvncBroadcast")
	}
}

// TestHandleClientReplyScreenshotDataURI 验证 case 0 的 data URI 兼容（B4-4b）：
// 期望截图 + "data:image/..." 文本形态（无二进制魔数）应进截图队列而非被当作 shell 输出
func TestHandleClientReplyScreenshotDataURI(t *testing.T) {
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "test.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	defer database.Engine.Close()

	uid := "shoturi0000000000000000000000001"
	database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
	database.Engine.Insert(&database.Clients{Uid: uid, Online: "1"})

	VarScreenshotQueue.SetScreenshotExpected(uid, true)
	dataURI := "data:image/png;base64," + strings.Repeat("A", 2048)
	if err := HandleClientReply("http", uid, 0, []byte(dataURI), nil, nil); err != nil {
		t.Fatalf("HandleClientReply: %v", err)
	}

	select {
	case got := <-VarScreenshotQueue.GetOrCreateQueue(uid):
		if !strings.HasPrefix(got, "data:image/") {
			t.Fatalf("截图队列内容非 data URI: %.40q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("data URI 截图未进入截图队列")
	}
	if VarScreenshotQueue.IsScreenshotExpected(uid) {
		t.Fatal("处理后截图期望标志未清除")
	}
	VarScreenshotQueue.RemoveQueue(uid)
}
