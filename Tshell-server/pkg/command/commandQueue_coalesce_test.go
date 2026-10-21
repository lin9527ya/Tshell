package command

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// frameOf 构造一条「4 字节类型头 + payload」的待下发命令。
func frameOf(typ uint32, payload string) []byte {
	head := make([]byte, 4)
	binary.BigEndian.PutUint32(head, typ)
	return append(head, []byte(payload)...)
}

func queueOf(t *testing.T, clientID string, cmds ...[]byte) *ClientCommandQueue {
	t.Helper()
	q := &ClientCommandQueue{}
	for _, c := range cmds {
		if !q.AddCommand(clientID, c) {
			t.Fatalf("入队失败: %q", c)
		}
	}
	return q
}

// 交互式输入连续入队时应合并成一条，payload 字节按序拼接。
func TestGetCommandCoalescingInputMergesConsecutive(t *testing.T) {
	q := queueOf(t, "u1",
		frameOf(INTERACTIVE_SHELL_INPUT, "l"),
		frameOf(INTERACTIVE_SHELL_INPUT, "s"),
		frameOf(INTERACTIVE_SHELL_INPUT, " "),
		frameOf(INTERACTIVE_SHELL_INPUT, "-"),
		frameOf(INTERACTIVE_SHELL_INPUT, "l"),
	)

	got, ok := q.GetCommandCoalescingInput("u1", INTERACTIVE_SHELL_INPUT)
	if !ok {
		t.Fatal("应取到合并后的命令")
	}
	if typ := binary.BigEndian.Uint32(got[:4]); typ != INTERACTIVE_SHELL_INPUT {
		t.Fatalf("类型头被破坏: %d", typ)
	}
	if payload := string(got[4:]); payload != "ls -l" {
		t.Fatalf("payload = %q, 期望 %q", payload, "ls -l")
	}
	if rest, ok := q.GetCommandCoalescingInput("u1", INTERACTIVE_SHELL_INPUT); ok {
		t.Fatalf("队列应已空，却取到 %q", rest)
	}
}

// 中间夹了其它命令时必须停下：不能把后面的输入并到前面去（会打乱相对顺序）。
func TestGetCommandCoalescingInputStopsAtOtherCommand(t *testing.T) {
	q := queueOf(t, "u2",
		frameOf(INTERACTIVE_SHELL_INPUT, "a"),
		frameOf(INTERACTIVE_SHELL_INPUT, "b"),
		frameOf(SHELL, "whoami"),
		frameOf(INTERACTIVE_SHELL_INPUT, "c"),
	)

	first, ok := q.GetCommandCoalescingInput("u2", INTERACTIVE_SHELL_INPUT)
	if !ok || string(first[4:]) != "ab" {
		t.Fatalf("第一批 = %q, 期望 %q", first[4:], "ab")
	}
	second, ok := q.GetCommandCoalescingInput("u2", INTERACTIVE_SHELL_INPUT)
	if !ok || binary.BigEndian.Uint32(second[:4]) != SHELL || string(second[4:]) != "whoami" {
		t.Fatalf("第二条应原样返回 SHELL/whoami, got %q", second)
	}
	third, ok := q.GetCommandCoalescingInput("u2", INTERACTIVE_SHELL_INPUT)
	if !ok || string(third[4:]) != "c" {
		t.Fatalf("第三条 = %q, 期望 %q", third[4:], "c")
	}
}

// 队首不是交互式输入时行为必须与 GetCommand 一致（逐条取出，不合并）。
func TestGetCommandCoalescingInputPassesThroughOtherTypes(t *testing.T) {
	q := queueOf(t, "u3",
		frameOf(SHELL, "dir"),
		frameOf(INTERACTIVE_SHELL_INPUT, "x"),
		frameOf(INTERACTIVE_SHELL_INPUT, "y"),
	)
	first, ok := q.GetCommandCoalescingInput("u3", INTERACTIVE_SHELL_INPUT)
	if !ok || binary.BigEndian.Uint32(first[:4]) != SHELL || string(first[4:]) != "dir" {
		t.Fatalf("首条应原样返回 SHELL/dir, got %q", first)
	}
	second, ok := q.GetCommandCoalescingInput("u3", INTERACTIVE_SHELL_INPUT)
	if !ok || string(second[4:]) != "xy" {
		t.Fatalf("第二条应合并为 xy, got %q", second[4:])
	}
}

// 单条交互式输入原样返回（不能丢掉 4 字节头，也不能平白改动内容）。
func TestGetCommandCoalescingInputSingle(t *testing.T) {
	q := queueOf(t, "u4", frameOf(INTERACTIVE_SHELL_INPUT, "\r"))
	got, ok := q.GetCommandCoalescingInput("u4", INTERACTIVE_SHELL_INPUT)
	if !ok {
		t.Fatal("应取到单条输入")
	}
	if !bytes.Equal(got, frameOf(INTERACTIVE_SHELL_INPUT, "\r")) {
		t.Fatalf("单条输入被改动: %v", got)
	}
}

// 空队列/未知客户端不能 panic，且必须返回 ok=false（GetCommand 的既有契约）。
func TestGetCommandCoalescingInputEmpty(t *testing.T) {
	q := &ClientCommandQueue{}
	if _, ok := q.GetCommandCoalescingInput("nobody", INTERACTIVE_SHELL_INPUT); ok {
		t.Fatal("未知客户端不应取到命令")
	}
	q.AddCommand("u5", frameOf(INTERACTIVE_SHELL_INPUT, "a"))
	if _, ok := q.GetCommandCoalescingInput("u5", INTERACTIVE_SHELL_INPUT); !ok {
		t.Fatal("应取到命令")
	}
	if _, ok := q.GetCommandCoalescingInput("u5", INTERACTIVE_SHELL_INPUT); ok {
		t.Fatal("取空的队列应返回 ok=false")
	}
	if n := q.QueueLen("u5"); n != 0 {
		t.Fatalf("队列长度 = %d, 期望 0", n)
	}
}

// 合并后队列里不能残留已弹出的条目（底层数组引用释放，防长连接内存泄漏）。
func TestGetCommandCoalescingInputLeavesNoResidue(t *testing.T) {
	q := queueOf(t, "u6",
		frameOf(INTERACTIVE_SHELL_INPUT, "a"),
		frameOf(INTERACTIVE_SHELL_INPUT, "b"),
		frameOf(INTERACTIVE_SHELL_INPUT, "c"),
	)
	if _, ok := q.GetCommandCoalescingInput("u6", INTERACTIVE_SHELL_INPUT); !ok {
		t.Fatal("应取到命令")
	}
	v, _ := q.queues.Load("u6")
	cq := v.(*clientQueue)
	cq.mu.Lock()
	defer cq.mu.Unlock()
	for i, got := range cq.data {
		if got != nil {
			t.Fatalf("残留条目 data[%d] = %v", i, got)
		}
	}
}
