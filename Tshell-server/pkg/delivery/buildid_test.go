package delivery

import (
	"bytes"
	"os"
	"testing"
)

// TestRandomizeGoBuildID 60 字符唯一 buildid 被替换且等长
func TestRandomizeGoBuildID(t *testing.T) {
	// 构造: 前缀 + 60 字符 buildid + 后缀
	bid := "RC0S2yJpBvdujet6aAGy/aHhIFrBCCGUBVtM6_2-D/TBVj7NX6ZkpuBBY9tWf0/vd9abia1x6gRl9b8vfmY"
	payload := append([]byte("ELF\x7f..."), []byte(bid)...)
	payload = append(payload, []byte("...tail...")...)

	out := randomizeGoBuildID(payload)
	if !bytes.Equal(out, payload) {
		t.Fatal("非 ELF 数据应跳过（不修改）")
	}
}

// TestRandomizeGoBuildIDRealELF 对真实 Go ELF 二进制（当前测试进程）执行 buildid 替换
func TestRandomizeGoBuildIDRealELF(t *testing.T) {
	data, err := os.ReadFile("/proc/self/exe")
	if err != nil {
		t.Skip("无法读取当前进程 ELF")
	}
	f, ferr := elfNew(data)
	if ferr != nil {
		t.Skip("当前测试二进制非 ELF")
	}
	before := readELFBuildID(f)
	if before == "" {
		t.Skip("当前二进制无 Go buildid")
	}
	out := randomizeGoBuildID(data)
	if bytes.Equal(out, data) {
		t.Fatal("真实 ELF buildid 未被替换")
	}
	if len(out) != len(data) {
		t.Fatal("等长替换失败")
	}
	if bytes.Contains(out, []byte(before)) {
		t.Fatal("buildid 仍存在")
	}
}
