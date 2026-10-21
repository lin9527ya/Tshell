package sendcommand

import "testing"

// 只认 "sleep " 开头的命令：别的 shell 命令不能被误判成休眠调整。
func TestParseSleepCommandRecognisesOnlySleepPrefix(t *testing.T) {
	cases := []struct {
		command string
		want    int
		ok      bool
	}{
		{"sleep 5", 5, true},
		{"sleep 1", 1, true},
		{"sleep 10 20", 10, true}, // 第二段是 jitter，取第一个数
		{"sleep  7", 7, true},
		{"sleep", 0, false},
		{"sleep ", 0, false},
		{"sleepy 5", 0, false},
		{"sleepy.exe", 0, false},
		{"interactive_shell_input sleep 5", 0, false},
		{"sleep abc", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseSleepCommand(c.command)
		if ok != c.ok || got != c.want {
			t.Errorf("parseSleepCommand(%q) = (%d, %v), 期望 (%d, %v)", c.command, got, ok, c.want, c.ok)
		}
	}
}

// 钳制规则必须与客户端 CmdSleep/EditSleep 一致，否则 UI 显示值与真实节奏分裂。
func TestParseSleepCommandClamps(t *testing.T) {
	cases := []struct {
		command string
		want    int
	}{
		{"sleep 0", 1},
		{"sleep -1", 1},
		{"sleep -100", 1},
		{"sleep 300", 300},
		{"sleep 301", 300},
		{"sleep 99999", 300},
	}
	for _, c := range cases {
		got, ok := parseSleepCommand(c.command)
		if !ok {
			t.Errorf("parseSleepCommand(%q) 应被识别", c.command)
			continue
		}
		if got != c.want {
			t.Errorf("parseSleepCommand(%q) = %d, 期望钳制到 %d", c.command, got, c.want)
		}
	}
}
