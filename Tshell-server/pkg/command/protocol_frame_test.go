package command

import (
	"bytes"
	"testing"
)

// TestParseHVNCFrameBounds 协议层边界: 短载荷/越界 sidLen/合法帧
func TestParseHVNCFrameBounds(t *testing.T) {
	if _, _, ok := ParseHVNCFrame(nil); ok {
		t.Fatal("nil 载荷应返回 false")
	}
	if _, _, ok := ParseHVNCFrame([]byte{1, 2, 3}); ok {
		t.Fatal("3 字节载荷应返回 false")
	}
	// sidLen 声称 8 但实际只有 4 字节 sid → 越界拒绝
	bad := []byte{0, 0, 0, 8, 'a', 'b', 'c', 'd'}
	if _, _, ok := ParseHVNCFrame(bad); ok {
		t.Fatal("sidLen 越界应返回 false")
	}
	// sidLen = 0 拒绝
	if _, _, ok := ParseHVNCFrame([]byte{0, 0, 0, 0, 'x'}); ok {
		t.Fatal("sidLen=0 应返回 false")
	}
	// 合法帧: sidLen=4, sid="sess", jpeg=FFD8FF
	good := append([]byte{0, 0, 0, 4}, []byte("sess")...)
	good = append(good, 0xFF, 0xD8, 0xFF)
	sid, jpeg, ok := ParseHVNCFrame(good)
	if !ok || sid != "sess" || !bytes.Equal(jpeg, []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("合法帧解析失败: sid=%q jpeg=%x ok=%v", sid, jpeg, ok)
	}
}

// TestSplitReplyAndBatchRoundtrip B5 帧路径往返一致性:
// BuildBatchFrame 构造的帧在客户端 dispatch 语义下能无损还原每条命令
func TestSplitReplyAndBatchRoundtrip(t *testing.T) {
	// 构造 3 条模拟命令(cmdType(4)|body)
	cmds := make([][]byte, 3)
	for i := range cmds {
		cmds[i] = append(utils4BE(uint32(100+i)), []byte("payload-")...)
		cmds[i] = append(cmds[i], byte('0'+i))
	}
	frame := BuildBatchFrame(cmds)
	if frame == nil {
		t.Fatal("帧构造失败")
	}

	// 按 dispatch.go 的 CMD_BATCH 解析语义还原
	if rt := utilsBE(frame[:4]); rt != uint32(CMD_BATCH) {
		t.Fatalf("帧头 = %d", rt)
	}
	count := int(utilsBE(frame[4:8]))
	if count != 3 {
		t.Fatalf("条数 = %d, want 3", count)
	}
	off := 8
	for i := 0; i < count; i++ {
		l := int(utilsBE(frame[off : off+4]))
		off += 4
		if off+l > len(frame) {
			t.Fatalf("第%d条越界", i)
		}
		if !bytes.Equal(frame[off:off+l], cmds[i]) {
			t.Fatalf("第%d条还原不一致:\n got  %x\n want %x", i, frame[off:off+l], cmds[i])
		}
		off += l
	}
	if off != len(frame) {
		t.Fatalf("帧尾有残留: 解析至 %d, 帧长 %d", off, len(frame))
	}
}

// TestHeartbeatAckFrame 心跳 ack 帧构造(len(4)|base64 载荷, TCP/KCP 帧格式)
func TestHeartbeatAckFrame(t *testing.T) {
	payload := []byte("dGVzdA==")
	f := FrameHeartbeatAck(payload)
	if f == nil {
		t.Fatal("ack 帧构造失败")
	}
	if len(f) != 4+len(payload) {
		t.Fatalf("帧长 = %d, want %d", len(f), 4+len(payload))
	}
	if l := utilsBE(f[:4]); l != uint32(len(payload)) {
		t.Fatalf("长度头 = %d, want %d", l, len(payload))
	}
	if !bytes.Equal(f[4:], payload) {
		t.Fatalf("载荷不一致: %q", f[4:])
	}
}

// utilsBE 测试辅助
func utilsBE(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
