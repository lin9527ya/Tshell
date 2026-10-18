package api

import (
	"strings"
	"testing"

	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
)

// 多传输载荷生成的两个校验：端点格式/类型，以及"端点监听器密钥必须一致"。
// 后者是真正会让人排查半天的坑：密钥不一致的 beacon 切到备用通道后握手必失败，
// 现场只表现为"切过去就掉线"。

func TestValidateMultiEndpoints(t *testing.T) {
	ok, msg := validateMultiEndpoints("tcp://10.0.0.5:443, https://cdn.example.com/api")
	if msg != "" {
		t.Fatalf("合法列表被拒: %s", msg)
	}
	if len(ok) != 2 || ok[0] != "tcp://10.0.0.5:443" || ok[1] != "https://cdn.example.com/api" {
		t.Fatalf("归一化结果不符: %v", ok)
	}
	// ws 别名为 websocket
	ok, msg = validateMultiEndpoints("ws://h/ws")
	if msg != "" || ok[0] != "websocket://h/ws" {
		t.Fatalf("ws 别名归一化失败: %v %s", ok, msg)
	}
	// 去重
	ok, _ = validateMultiEndpoints("tcp://a:1,tcp://a:1")
	if len(ok) != 1 {
		t.Fatalf("重复端点应去重: %v", ok)
	}

	bad := []string{
		"tcp://",       // 缺地址
		"10.0.0.5:443", // 缺 scheme
		"smtp://h:25",  // 不支持的传输
		"",             // 空
		"tcp://a:1,tcp://a:2,tcp://a:3,tcp://a:4,tcp://a:5,tcp://a:6,tcp://a:7,tcp://a:8,tcp://a:9", // 超过 8 个
	}
	for _, in := range bad {
		if _, msg := validateMultiEndpoints(in); msg == "" {
			t.Errorf("validateMultiEndpoints(%q) 应报错", in)
		}
	}
	// 总长度超过占位符（512 字节）：多个长端点累加
	long := "tcp://" + strings.Repeat("a", 200) + ":1,https://" + strings.Repeat("b", 200) + "/x,http://" + strings.Repeat("c", 200) + "/y"
	if _, msg := validateMultiEndpoints(long); msg == "" {
		t.Error("总长度超过占位符应被拒绝")
	}
}

func TestEndpointKeyConsistency(t *testing.T) {
	setupTaskTest(t) // 临时库 + 全部表

	master := []byte("MASTERKEY_0123456789ABCDEFGHIJKLM") // 32 字节
	salt := []byte("SALT_0123456789")                     // 16 字节
	if _, err := database.Engine.Insert(&database.Listener{
		Type: "tcp", ListenAddress: "0.0.0.0:9001", ConnectAddress: "10.0.0.5:9001",
		VKey: strings.TrimRight(string(master), "\x00"), EncryptionSalt: string(salt),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Engine.Insert(&database.Listener{
		Type: "https", ListenAddress: "0.0.0.0:9002", ConnectAddress: "cdn.example.com",
		VKey: "DIFFERENT_KEY", EncryptionSalt: "DIFFERENT_SALT",
	}); err != nil {
		t.Fatal(err)
	}

	// 同密钥端点 → 通过
	if msg := checkEndpointKeyConsistency([]string{"tcp://10.0.0.5:9001"}, master, salt); msg != "" {
		t.Fatalf("同密钥端点应通过: %s", msg)
	}
	// 未注册地址（重定向器/域前置）→ 放行
	if msg := checkEndpointKeyConsistency([]string{"https://redirector.example.net/api"}, master, salt); msg != "" {
		t.Fatalf("未注册地址应放行: %s", msg)
	}
	// 密钥不同的已注册监听器 → 拒绝并说明原因
	msg := checkEndpointKeyConsistency([]string{"tcp://10.0.0.5:9001", "https://cdn.example.com"}, master, salt)
	if msg == "" {
		t.Fatal("密钥不一致的端点应被拒绝")
	}
	if !strings.Contains(msg, "密钥") || !strings.Contains(msg, "cdn.example.com") {
		t.Fatalf("错误信息应指明端点与原因: %s", msg)
	}
	// 占位符长度必须能容纳 8 个典型端点
	if len(delivery.MultiEndpointsPlaceholder) != 512 {
		t.Fatalf("端点占位符长度 = %d", len(delivery.MultiEndpointsPlaceholder))
	}
}
