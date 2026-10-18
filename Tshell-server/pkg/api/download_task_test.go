package api

import (
	"strings"
	"testing"

	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/database"
)

// 下载终态 → 任务状态：下载没有结束报文，任务中心的"下完没有"完全依赖这条链路。
func TestOnDownloadEnd(t *testing.T) {
	setupTaskTest(t)

	_, rows := createTaskRows([]string{"uid-1"}, "alice", "download", "download /tmp/a.bin")
	if len(rows) != 1 {
		t.Fatalf("建下载任务失败: %d", len(rows))
	}
	// 同 uid 的另一类任务不应被下载终态影响
	_, other := createTaskRows([]string{"uid-1"}, "alice", "console", "whoami")

	OnDownloadEnd("uid-1", "/tmp/a.bin", command.DownloadStatusCompleted)

	var got database.Task
	if has, _ := database.Engine.ID(rows[0].Id).Get(&got); !has || got.Status != taskStatusCompleted {
		t.Fatalf("下载任务未置 completed: %+v", got)
	}
	if !strings.Contains(got.Result, "/tmp/a.bin") || got.FinishedAt.IsZero() {
		t.Fatalf("完成态字段不符: %+v", got)
	}

	// 幂等：重复回调不覆盖终态
	OnDownloadEnd("uid-1", "/tmp/a.bin", command.DownloadStatusFailed)
	database.Engine.ID(rows[0].Id).Get(&got)
	if got.Status != taskStatusCompleted {
		t.Fatalf("终态被重复回调覆盖: %+v", got)
	}

	var untouched database.Task
	database.Engine.ID(other[0].Id).Get(&untouched)
	if untouched.Status != taskStatusDispatched {
		t.Fatalf("其他类型任务被误改: %+v", untouched)
	}
}

// 下载中断（断线/空闲超时）→ failed，且错误信息可读。
func TestOnDownloadEndFailed(t *testing.T) {
	setupTaskTest(t)

	_, rows := createTaskRows([]string{"uid-2"}, "bob", "download", "download /tmp/b.bin")
	OnDownloadEnd("uid-2", "/tmp/b.bin", command.DownloadStatusFailed)

	var got database.Task
	if has, _ := database.Engine.ID(rows[0].Id).Get(&got); !has || got.Status != taskStatusFailed {
		t.Fatalf("下载中断未置 failed: %+v", got)
	}
	if !strings.Contains(got.Error, "/tmp/b.bin") {
		t.Fatalf("失败原因未带文件路径: %q", got.Error)
	}
}

// 找不到对应任务（例如历史下载或 DB 未就绪）时静默返回，不 panic。
func TestOnDownloadEndNoMatch(t *testing.T) {
	setupTaskTest(t)

	OnDownloadEnd("uid-none", "/tmp/ghost.bin", command.DownloadStatusCompleted)
	OnDownloadEnd("", "", command.DownloadStatusCompleted)
}
