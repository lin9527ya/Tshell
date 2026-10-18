package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// 空闲休眠必须走 sleep mask。原实现只对轮询型传输遮蔽，tcp/kcp/websocket/forward
// 这类**长连接**走的是无遮蔽的 SleepWithJitter —— 而长连接的会话密钥驻留时间最长
// （数小时），恰恰是最该保护的一类。这条守卫防止回退。
func TestIdleSleepPathsUseMask(t *testing.T) {
	src, err := os.ReadFile("runloop.go")
	if err != nil {
		t.Fatalf("读取 runloop.go 失败: %v", err)
	}
	text := string(src)

	// 空闲休眠分支里不得直接调用无遮蔽的 SleepWithJitter
	if regexp.MustCompile(`(?m)^\s*app\.SleepWithJitter\(`).MatchString(text) {
		t.Error("runloop 空闲分支不应调用无遮蔽的 app.SleepWithJitter（应走 SleepMask / SleepMaskedWithJitter）")
	}
	for _, want := range []string{"app.SleepMask(", "app.SleepMaskedWithJitter("} {
		if !strings.Contains(text, want) {
			t.Errorf("runloop.go 缺少 %s（轮询型与持久型都需遮蔽）", want)
		}
	}
}
