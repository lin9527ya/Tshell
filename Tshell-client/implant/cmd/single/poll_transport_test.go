package main

import (
	"os"
	"regexp"
	"testing"
)

// TestPollTransportsImplementPollInterval 守卫：轮询型传输必须实现 pollTransport。
// runloop 对实现了该接口的传输走 app.SleepMask（睡眠期加密会话密钥），
// 未实现的会被当持久传输走 SleepWithJitter —— 空闲期密钥明文驻留。
// HTTP/HTTPS 曾因此成为唯一没有 sleep mask 的主传输。
//
// 用源码级断言而不是编译期断言：各 transport 由 build tag 单选，
// 无 tag 的测试文件引用不到具体传输类型。
func TestPollTransportsImplementPollInterval(t *testing.T) {
	files := []string{
		"transport_http.go",
		"transport_https.go",
		"transport_dns.go",
		"transport_oss.go",
	}
	// 匹配 "func (x *XxxClient) PollInterval() time.Duration"
	re := regexp.MustCompile(`func \([^)]*\) PollInterval\(\) time\.Duration`)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", f, err)
		}
		if !re.Match(data) {
			t.Errorf("%s 未实现 PollInterval —— 该传输空闲期不会启用 sleep mask", f)
		}
	}
}
