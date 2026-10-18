package api

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"strings"
	"testing"

	"BackendTemplate/pkg/delivery"
)

// TestUAPatchOnHTTPTemplate 对真实 http 模板做 UA patch,验证 Firefox UA 落位
func TestUAPatchOnHTTPTemplate(t *testing.T) {
	// 路径相对包目录（go test 的 cwd 是 pkg/api）：原写成 "pkg/api/server/..." 永远读不到，
	// 该用例此前恒被 skip；模板缺失时（CI 未构建模板）仍然跳过。
	data, err := os.ReadFile("server/http/http_linux_amd64")
	if err != nil {
		t.Skip("模板缺失")
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(gz)
	out, err := delivery.PatchClientBinary(raw, delivery.PatchOptions{
		ListenerConfig: "http://1.2.3.4:8080",
		OS:             "linux",
		Arch:           "amd64",
		Pass:           "",
		VKey:           "0123456789abcdef0123456789abcdef",
		EncryptSalt:    "0123456789abcdef",
		UserAgent:      "Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
		MasqueradeName: "svc",
		QuietMode:      "1",
	})
	if err != nil {
		t.Fatalf("patch err: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "Firefox/121.0") {
		t.Fatal("Firefox UA 未落位")
	}
	if strings.Contains(s, "HTTP_UA_AAA") {
		t.Fatal("UA 占位符残留")
	}
}
