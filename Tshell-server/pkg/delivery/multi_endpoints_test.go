package delivery

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// 多传输端点的 patch 有两个硬约束：
//  1. 等长替换（超长会撑大文件、PE 偏移错乱，曾导致 OOM）；
//  2. 占位符两侧必须逐字节一致（客户端 core/config/multi.go vs 服务端）。

// TestMultiEndpointsPlaceholderMatchesClient 跨模块一致性：
// 服务端按客户端源码里的字面量生成占位符，任何一边改动都会在 CI 里立刻失败。
func TestMultiEndpointsPlaceholderMatchesClient(t *testing.T) {
	src, err := os.ReadFile("../../../Tshell-client/implant/core/config/multi.go")
	if err != nil {
		t.Skip("客户端源码不在预期位置（独立检出？）:", err)
	}
	m := regexp.MustCompile(`var MultiEndpoints = "([^"]+)"`).FindStringSubmatch(string(src))
	if m == nil {
		t.Fatal("客户端 core/config/multi.go 中找不到 MultiEndpoints 定义")
	}
	clientPh := m[1]
	if clientPh != MultiEndpointsPlaceholder {
		t.Fatalf("占位符不一致（客户端与服务端必须逐字节相同）:\n client=%q(%d)\n server=%q(%d)",
			clientPh, len(clientPh), MultiEndpointsPlaceholder, len(MultiEndpointsPlaceholder))
	}
	if len(clientPh) != 512 {
		t.Fatalf("占位符长度 = %d，期望 512（patch 为等长替换）", len(clientPh))
	}
}

func TestPatchMultiEndpoints(t *testing.T) {
	tmpl := append([]byte("PREFIX"), []byte(MultiEndpointsPlaceholder)...)
	tmpl = append(tmpl, []byte("SUFFIX")...)

	out, err := patchMultiEndpoints(tmpl, "tcp://10.0.0.5:443, https://cdn.example.com/api")
	if err != nil {
		t.Fatalf("替换失败: %v", err)
	}
	if len(out) != len(tmpl) {
		t.Fatalf("替换后长度变化: %d → %d（必须等长）", len(tmpl), len(out))
	}
	if !bytes.HasPrefix(out, []byte("PREFIX")) || !bytes.HasSuffix(out, []byte("SUFFIX")) {
		t.Fatal("占位符之外的字节被改动")
	}
	body := string(out[len("PREFIX") : len(out)-len("SUFFIX")])
	if !strings.HasPrefix(body, "tcp://10.0.0.5:443,https://cdn.example.com/api") {
		t.Fatalf("端点内容不符: %q", body[:64])
	}
	if strings.TrimRight(body, " ") != "tcp://10.0.0.5:443,https://cdn.example.com/api" {
		t.Fatalf("填充不符: %q", body[len(body)-20:])
	}

	// 空值不改动
	same, err := patchMultiEndpoints(tmpl, "   ")
	if err != nil || !bytes.Equal(same, tmpl) {
		t.Fatalf("空端点不应改动模板: %v", err)
	}
}

func TestPatchMultiEndpointsErrors(t *testing.T) {
	// 单传输模板不含占位符 → 明确报错（而不是静默生成一个连不上的载荷）
	plain := []byte("PREFIX-ONLY")
	if _, err := patchMultiEndpoints(plain, "tcp://a:1"); err == nil {
		t.Fatal("模板缺占位符时应报错")
	} else if !strings.Contains(err.Error(), "multi 模板") {
		t.Fatalf("错误信息应提示使用 multi 模板: %v", err)
	}

	// 超长端点列表 → 拒绝
	tmpl := []byte(MultiEndpointsPlaceholder)
	long := strings.Repeat("tcp://a:1,", 100)
	if _, err := patchMultiEndpoints(tmpl, long); err == nil {
		t.Fatal("超长端点列表应被拒绝")
	}
}
