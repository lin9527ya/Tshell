package command

import (
	"encoding/binary"
	"testing"
)

// TestGetCommandsBatch B5: 批量取命令(保序/上限/不足全取/队列状态)
func TestGetCommandsBatch(t *testing.T) {
	q := &ClientCommandQueue{}
	uid := "batchtest0000000000000000000001"

	// 空队列
	if got := q.GetCommands(uid, 4); got != nil {
		t.Fatalf("空队列应返回 nil, got %d 条", len(got))
	}

	// 入队 5 条(带可识别负载)
	for i := 0; i < 5; i++ {
		payload := []byte{byte('a' + i)}
		if !q.AddCommand(uid, payload) {
			t.Fatalf("AddCommand #%d 失败", i)
		}
	}

	// 取 3 条:保序
	got := q.GetCommands(uid, 3)
	if len(got) != 3 {
		t.Fatalf("应取 3 条, got %d", len(got))
	}
	for i, cmd := range got {
		if string(cmd) != string([]byte{byte('a' + i)}) {
			t.Fatalf("顺序错误: 第%d条 = %q, want %q", i, cmd, string([]byte{byte('a' + i)}))
		}
	}
	// 队列应余 2
	if n := q.QueueLen(uid); n != 2 {
		t.Fatalf("取后队列应余 2, got %d", n)
	}
	// 再取超量(不足全取)
	got = q.GetCommands(uid, 10)
	if len(got) != 2 || string(got[0]) != "d" || string(got[1]) != "e" {
		t.Fatalf("不足全取失败: %v", got)
	}
	// 取空后队列归零
	if n := q.QueueLen(uid); n != 0 {
		t.Fatalf("队列应归零, got %d", n)
	}
	q.DeleteClient(uid)
}

// TestBuildBatchFrame B5: 批量帧构造(帧格式/条数上限/字节上限)
func TestBuildBatchFrame(t *testing.T) {
	// 空输入
	if BuildBatchFrame(nil) != nil {
		t.Fatal("空输入应返回 nil")
	}

	// 正常打包 2 条
	cmdA := append(utils4BE(7), []byte("hello")...)   // cmdType=7 + payload
	cmdB := append(utils4BE(9), []byte("world!!")...) // cmdType=9 + payload
	frame := BuildBatchFrame([][]byte{cmdA, cmdB})
	if frame == nil {
		t.Fatal("打包失败")
	}
	if rt := binary.BigEndian.Uint32(frame[:4]); rt != uint32(CMD_BATCH) {
		t.Fatalf("帧头类型 = %d, want %d", rt, CMD_BATCH)
	}
	if cnt := binary.BigEndian.Uint32(frame[4:8]); cnt != 2 {
		t.Fatalf("帧内条数 = %d, want 2", cnt)
	}
	// 第一条: len(4)|cmd
	off := 8
	l1 := int(binary.BigEndian.Uint32(frame[off : off+4]))
	off += 4
	if l1 != len(cmdA) || string(frame[off:off+l1]) != string(cmdA) {
		t.Fatalf("第一条命令还原失败: len=%d", l1)
	}
	off += l1
	l2 := int(binary.BigEndian.Uint32(frame[off : off+4]))
	off += 4
	if l2 != len(cmdB) || string(frame[off:off+l2]) != string(cmdB) {
		t.Fatalf("第二条命令还原失败: len=%d", l2)
	}

	// 条数上限: 20 条只打包 16
	cmds := make([][]byte, 20)
	for i := range cmds {
		cmds[i] = utils4BE(uint32(i))
	}
	frame = BuildBatchFrame(cmds)
	if cnt := binary.BigEndian.Uint32(frame[4:8]); cnt != uint32(batchMaxCount) {
		t.Fatalf("条数上限失效: %d, want %d", cnt, batchMaxCount)
	}

	// 字节上限: 单条超 1MB 的命令直接放弃(不产生半截帧)
	big := make([]byte, batchMaxBytes)
	frame = BuildBatchFrame([][]byte{big})
	if frame != nil {
		t.Fatalf("超字节上限应返回 nil, got %d 字节帧", len(frame))
	}
}

// TestBatchCapability B5: 能力标记与 osArray 解析
func TestBatchCapability(t *testing.T) {
	uid := "captest000000000000000000000001"
	if IsBatchCapable(uid) {
		t.Fatal("未标记前不应有能力")
	}
	MarkBatchCapable(uid, true)
	if !IsBatchCapable(uid) {
		t.Fatal("标记后应有能力")
	}
	MarkBatchCapable(uid, false)
	if IsBatchCapable(uid) {
		t.Fatal("清除后不应有能力")
	}

	// 第 6 段 "b1" 判定
	if !ParseBatchCapability([]string{"host", "user", "proc", "win", "5", "b1"}) {
		t.Fatal("b1 标记应判定为有能力")
	}
	if ParseBatchCapability([]string{"host", "user", "proc", "win", "5"}) {
		t.Fatal("无第 6 段应判定为无能力")
	}
	if ParseBatchCapability([]string{"host", "user", "proc", "win", "5", "b2"}) {
		t.Fatal("非 b1 标记应判定为无能力")
	}
}

// utils4BE 测试辅助: 4 字节大端
func utils4BE(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}
