package app

// 命令防抖（P5）的接线棘轮。
//
// 防抖单独成一个值（config.CommandDebounce）而不是继续用 config.WaitTime，
// 是为了把"信标多久出现一次"和"操作员连点多快"分开。这两件事一旦再次合并，
// 症状是"每条命令都要等一个 sleep"—— 编译通过、单测全绿、只有实机手感不对，
// 所以只能靠源码级棘轮钉住。

import (
	"os"
	"strings"
	"testing"
)

func TestDispatchUsesCommandDebounceNotSleep(t *testing.T) {
	src, err := os.ReadFile("dispatch.go")
	if err != nil {
		t.Fatalf("读取 dispatch.go 失败: %v", err)
	}
	s := strings.ReplaceAll(string(src), "\r\n", "\n")

	if !strings.Contains(s, "debounce := config.CommandDebounce") {
		t.Error("dispatch.go 的命令节流没有使用 config.CommandDebounce —— 交互命令会被信标节奏拖慢")
	}
	// 节流块里不能再出现 "elapsedTime < config.WaitTime"：那正是"每条命令等一个 sleep"的写法
	if strings.Contains(s, "elapsedTime < config.WaitTime") {
		t.Error("dispatch.go 仍用 config.WaitTime 当命令间隔 —— sleep 5 会让每次点击等 5 秒")
	}
	// 防抖不得超过信标节奏（sleep 设得比防抖还小时以 sleep 为准）
	if !strings.Contains(s, "config.WaitTime < debounce") {
		t.Error("dispatch.go 缺少 `config.WaitTime < debounce` 的钳制")
	}
}

func TestCommandDebounceIsShortAndDefined(t *testing.T) {
	cfg := strings.ReplaceAll(mustRead(t, "../config/config.go"), "\r\n", "\n")
	if !strings.Contains(cfg, "CommandDebounce = 200 * time.Millisecond") {
		t.Error("config.CommandDebounce 未定义或数值被改动（预期 200ms）")
	}
}
