package api

import (
	"strings"
	"testing"

	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/database"
)

// TestExecTaskOnlyDoneByExecDone 能力客户端：exec 任务不再被普通输出提前完成。
func TestExecTaskOnlyDoneByExecDone(t *testing.T) {
	setupTaskTest(t)
	const uid = "uid-exec-1"
	command.MarkExecClientCapable(uid)
	t.Cleanup(func() { command.ClearExecClientCapability(uid) })

	_, execRows := createTaskRows([]string{uid}, "alice", "exec", "inline-bin p.exe")
	if len(execRows) != 1 {
		t.Fatal("exec 任务未创建")
	}
	execID := execRows[0].Id

	// 进度输出到达：任务必须保持 dispatched（不能被提前完成）
	OnTaskOutput(uid, "[*] Starting InlineBin execution...")
	var afterOutput database.Task
	database.Engine.ID(execID).Get(&afterOutput)
	if afterOutput.Status != taskStatusDispatched {
		t.Fatalf("exec 任务被普通输出提前完成: %+v", afterOutput)
	}

	// 显式完成回执：按 taskID 精确收敛
	OnExecDone(uid, execID, true, "")
	var afterDone database.Task
	database.Engine.ID(execID).Get(&afterDone)
	if afterDone.Status != taskStatusCompleted {
		t.Fatalf("EXEC_DONE 未完成任务: %+v", afterDone)
	}

	// 幂等：终态不被覆盖
	OnExecDone(uid, execID, false, "late")
	var afterLate database.Task
	database.Engine.ID(execID).Get(&afterLate)
	if afterLate.Status != taskStatusCompleted || afterLate.Error != "" {
		t.Fatalf("终态被覆盖: %+v", afterLate)
	}
}

// TestExecTaskLegacyOutputCompletes 旧客户端（无能力）保持原有语义。
func TestExecTaskLegacyOutputCompletes(t *testing.T) {
	setupTaskTest(t)
	const uid = "uid-legacy-1"
	command.ClearExecClientCapability(uid)

	_, rows := createTaskRows([]string{uid}, "alice", "exec", "bof x.o")
	if len(rows) != 1 {
		t.Fatal("exec 任务未创建")
	}
	OnTaskOutput(uid, "legacy output")
	var got database.Task
	database.Engine.ID(rows[0].Id).Get(&got)
	if got.Status != taskStatusCompleted || got.Result != "legacy output" {
		t.Fatalf("旧客户端应仍按输出完成: %+v", got)
	}
}

// TestOnTaskOutputSkipsExecHead 能力客户端：exec 输出走 EXEC_OUTPUT(98)，
// 普通输出可以跳过 exec 队首，正确归属后面的 console 任务。
func TestOnTaskOutputSkipsExecHead(t *testing.T) {
	setupTaskTest(t)
	const uid = "uid-exec-head"
	command.MarkExecClientCapable(uid)
	command.MarkExecOutputReady(uid)
	t.Cleanup(func() { command.ClearExecClientCapability(uid) })

	_, execRows := createTaskRows([]string{uid}, "alice", "exec", "exec")
	_, consoleRows := createTaskRows([]string{uid}, "alice", "console", "whoami")

	OnTaskOutput(uid, "whoami output")
	var e, c database.Task
	database.Engine.ID(execRows[0].Id).Get(&e)
	database.Engine.ID(consoleRows[0].Id).Get(&c)
	if e.Status != taskStatusDispatched {
		t.Fatalf("exec 任务不应被 console 输出完成: %+v", e)
	}
	if c.Status != taskStatusCompleted || c.Result != "whoami output" {
		t.Fatalf("console 任务应被正确完成: %+v", c)
	}

	OnExecDone(uid, execRows[0].Id, true, "")
	var eDone database.Task
	database.Engine.ID(execRows[0].Id).Get(&eDone)
	if eDone.Status != taskStatusCompleted {
		t.Fatalf("EXEC_DONE 未完成 exec 任务: %+v", eDone)
	}
}

// TestOnExecDoneFailureAndFallback 失败写 error；taskID=0 回退最早 exec；错误截断。
func TestOnExecDoneFailureAndFallback(t *testing.T) {
	setupTaskTest(t)
	const uid = "uid-exec-fail"
	command.MarkExecClientCapable(uid)
	t.Cleanup(func() { command.ClearExecClientCapability(uid) })

	_, rows := createTaskRows([]string{uid}, "alice", "exec", "assembly a.exe")
	if len(rows) != 1 {
		t.Fatal("exec 任务未创建")
	}
	longErr := strings.Repeat("x", 800)
	OnExecDone(uid, 0, false, longErr) // taskID=0 → 回退最早 exec
	var got database.Task
	database.Engine.ID(rows[0].Id).Get(&got)
	if got.Status != taskStatusFailed {
		t.Fatalf("失败回执未落库: %+v", got)
	}
	if len(got.Error) > 500 || !strings.HasPrefix(got.Error, "xxx") {
		t.Fatalf("error 未按 varchar(512) 截断: len=%d", len(got.Error))
	}

	// 任务不匹配：console 任务的 ID 传给 EXEC_DONE 不得被改动
	_, consoleRows := createTaskRows([]string{uid}, "alice", "console", "whoami")
	OnExecDone(uid, consoleRows[0].Id, true, "")
	var c database.Task
	database.Engine.ID(consoleRows[0].Id).Get(&c)
	if c.Status != taskStatusDispatched {
		t.Fatalf("EXEC_DONE 不应完成 console 任务: %+v", c)
	}
}
