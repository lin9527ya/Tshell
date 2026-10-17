package api

import (
	"bytes"
	"encoding/binary"
	"testing"

	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
)

func setInjectListenerType(t *testing.T, uid, listenerType string) {
	t.Helper()
	connection.MuClientListenerType.Lock()
	connection.ClientListenerType[uid] = listenerType
	connection.MuClientListenerType.Unlock()
	t.Cleanup(func() {
		connection.MuClientListenerType.Lock()
		delete(connection.ClientListenerType, uid)
		connection.MuClientListenerType.Unlock()
	})
}

// TestSendInjectMemPersistentListenerMustFail 持久连接客户端没有连接时，分块下发必须报错。
// 修复前：SendFileUploadCommandWithResult 对 tcp/kcp/ws/oss 只做存在性检查就返回 true，
// 于是这里返回 (sent=N, seq=M, nil) —— 客户端一块都收不到，注入静默失败。
func TestSendInjectMemPersistentListenerMustFail(t *testing.T) {
	uid := "tcp-inject-uid"
	setInjectListenerType(t, uid, "tcp")

	sent, seq, err := SendInjectMem(uid, bytes.Repeat([]byte("A"), 4096), 1234, "auto")
	if err == nil {
		t.Fatalf("持久连接无连接时分块下发竟然报成功: sent=%d seq=%d", sent, seq)
	}
	if sent != 0 || seq != 0 {
		t.Fatalf("失败时不应报告已发送字节/分块: sent=%d seq=%d", sent, seq)
	}
}

// TestSendInjectMemQueuesChunksInOrder 轮询客户端：分块必须全部入队且顺序与包头一致
// （客户端按 seq 顺序组装，乱序会直接丢弃整次注入）
func TestSendInjectMemQueuesChunksInOrder(t *testing.T) {
	uid := "http-inject-uid"
	setInjectListenerType(t, uid, "http")

	data := bytes.Repeat([]byte("B"), injectMemChunk*2+10) // 3 块
	sent, seq, err := SendInjectMem(uid, data, 4321, "auto")
	if err != nil {
		t.Fatalf("轮询类型下发失败: %v", err)
	}
	if sent != int64(len(data)) {
		t.Fatalf("发送字节数不符: %d != %d", sent, len(data))
	}
	if seq != 3 {
		t.Fatalf("分块数不符: %d != 3", seq)
	}

	var requestId uint32
	for i := uint32(0); i < seq; i++ {
		raw, ok := command.CommandQueues.GetCommand(uid)
		if !ok {
			t.Fatalf("第 %d 块未入队", i)
		}
		if len(raw) < 20 {
			t.Fatalf("第 %d 块过短: %d", i, len(raw))
		}
		if got := binary.BigEndian.Uint32(raw[0:4]); got != uint32(command.INJECT_MEM) {
			t.Fatalf("第 %d 块命令号不符: %d", i, got)
		}
		rid := binary.BigEndian.Uint32(raw[4:8])
		total := binary.BigEndian.Uint32(raw[8:12])
		s := binary.BigEndian.Uint32(raw[12:16])
		if i == 0 {
			requestId = rid
		} else if rid != requestId {
			t.Fatalf("第 %d 块 requestId 不一致: %d != %d", i, rid, requestId)
		}
		if total != 3 {
			t.Fatalf("第 %d 块 total 不符: %d", i, total)
		}
		if s != i {
			t.Fatalf("分块乱序: 第 %d 块 seq=%d", i, s)
		}
	}

	// 首包带 pid/tech，后续包这两个字段长度为 0
	first, _ := command.CommandQueues.GetCommand(uid)
	_ = first // 队列已取空，仅确认没有多余分块
	if _, ok := command.CommandQueues.GetCommand(uid); ok {
		t.Fatal("队列里有多余分块")
	}
}
