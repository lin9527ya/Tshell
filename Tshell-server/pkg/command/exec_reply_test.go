package command

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"BackendTemplate/pkg/database"
)

func setupExecReplyTest(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	old := database.DatabaseName
	database.DatabaseName = filepath.Join(tmpDir, "exec-reply.db")
	database.ConnectDateBase()
	database.DatabaseName = old
	t.Cleanup(func() { database.Engine.Close() })
}

// TestReplyExecCapabilityAndDone EXEC_CAP 标记能力；EXEC_DONE 解析 taskID/ok/error。
func TestReplyExecCapabilityAndDone(t *testing.T) {
	setupExecReplyTest(t)
	const uid = "uid-exec-reply"
	database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
	database.Engine.Insert(&database.Clients{Uid: uid, Online: "1"})
	ClearExecClientCapability(uid)
	t.Cleanup(func() { ClearExecClientCapability(uid) })

	enableCalled := false
	oldEnable := ExecOutputEnableFunc
	ExecOutputEnableFunc = func(string) { enableCalled = true }
	t.Cleanup(func() { ExecOutputEnableFunc = oldEnable })

	if err := HandleClientReply("tcp", uid, EXEC_CAP, []byte{1}, nil, nil); err != nil {
		t.Fatalf("EXEC_CAP: %v", err)
	}
	if !IsExecClientCapable(uid) {
		t.Fatal("EXEC_CAP 未标记能力")
	}
	if !enableCalled {
		t.Fatal("EXEC_CAP 应触发 EXEC_OUTPUT_ENABLE 下发")
	}

	var gotUID string
	var gotTask int64
	var gotOK bool
	var gotErr string
	oldFn := ExecDoneFunc
	ExecDoneFunc = func(uid string, taskID int64, ok bool, errMsg string) {
		gotUID, gotTask, gotOK, gotErr = uid, taskID, ok, errMsg
	}
	t.Cleanup(func() { ExecDoneFunc = oldFn })

	payload := append([]byte{0, 0, 0, 42, 1}, []byte("")...)
	if err := HandleClientReply("tcp", uid, EXEC_DONE, payload, nil, nil); err != nil {
		t.Fatalf("EXEC_DONE: %v", err)
	}
	if gotUID != uid || gotTask != 42 || !gotOK || gotErr != "" {
		t.Fatalf("EXEC_DONE 解析错误: uid=%s task=%d ok=%v err=%q", gotUID, gotTask, gotOK, gotErr)
	}

	// 失败回执：error 文本进 shell 历史（前端插件页从 shell 读错误）
	failPayload := append([]byte{0, 0, 0, 7, 0}, []byte("boom")...)
	if err := HandleClientReply("tcp", uid, EXEC_DONE, failPayload, nil, nil); err != nil {
		t.Fatalf("EXEC_DONE failure: %v", err)
	}
	if gotTask != 7 || gotOK || gotErr != "boom" {
		t.Fatalf("失败回执解析错误: task=%d ok=%v err=%q", gotTask, gotOK, gotErr)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var shell database.Shell
		database.Engine.Where("uid = ?", uid).Get(&shell)
		if strings.Contains(shell.ShellContent, "[!] 执行失败: boom") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("失败文本未写入 shell: %q", shell.ShellContent)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 过短载荷不应 panic / 不应触发回调
	ExecDoneFunc = func(string, int64, bool, string) { t.Fatal("过短 EXEC_DONE 不应触发回调") }
	if err := HandleClientReply("tcp", uid, EXEC_DONE, []byte{1, 2}, nil, nil); err != nil {
		t.Fatalf("短 EXEC_DONE 不应报错: %v", err)
	}
}

// TestExecOutputDoesNotCompleteTask 执行过程输出写 shell 但不触发任务完成。
func TestExecOutputDoesNotCompleteTask(t *testing.T) {
	setupExecReplyTest(t)
	const uid = "uid-exec-output"
	database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
	database.Engine.Insert(&database.Clients{Uid: uid, Online: "1"})

	called := false
	oldFn := TaskOutputFunc
	TaskOutputFunc = func(string, string) { called = true }
	t.Cleanup(func() { TaskOutputFunc = oldFn })

	if err := HandleClientReply("tcp", uid, EXEC_OUTPUT, []byte("progress line"), nil, nil); err != nil {
		t.Fatalf("EXEC_OUTPUT: %v", err)
	}
	if called {
		t.Fatal("EXEC_OUTPUT 不应触发 TaskOutputFunc")
	}
	if !IsExecOutputReady(uid) {
		t.Fatal("EXEC_OUTPUT 应标记客户端已启用新输出通道")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		var shell database.Shell
		database.Engine.Where("uid = ?", uid).Get(&shell)
		if strings.Contains(shell.ShellContent, "progress line") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("EXEC_OUTPUT 未写入 shell: %q", shell.ShellContent)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestEmptyType0DoesNotCompleteTask 空回包（INJECT_MEM 中间分块）不得触发任务完成。
func TestEmptyType0DoesNotCompleteTask(t *testing.T) {
	setupExecReplyTest(t)
	const uid = "uid-empty-out"
	database.Engine.Insert(&database.Shell{Uid: uid, ShellContent: ""})
	database.Engine.Insert(&database.Clients{Uid: uid, Online: "1"})

	called := false
	oldFn := TaskOutputFunc
	TaskOutputFunc = func(string, string) { called = true }
	t.Cleanup(func() { TaskOutputFunc = oldFn })

	if err := HandleClientReply("tcp", uid, 0, nil, nil, nil); err != nil {
		t.Fatalf("空回包不应报错: %v", err)
	}
	if called {
		t.Fatal("空 type-0 不应触发 TaskOutputFunc")
	}
}
