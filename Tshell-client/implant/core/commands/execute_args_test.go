package commands

import (
	"testing"
)

func TestSplitCommandLine(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`"C:\Windows\System32\cmd.exe" /c whoami`, []string{`C:\Windows\System32\cmd.exe`, "/c", "whoami"}},
		{`C:\Tools\a.exe -x "b c"`, []string{`C:\Tools\a.exe`, "-x", "b c"}},
		{`/usr/bin/env bash -lc "echo hi"`, []string{"/usr/bin/env", "bash", "-lc", "echo hi"}},
		{`'/opt/my app/run.sh' --flag`, []string{"/opt/my app/run.sh", "--flag"}},
		{`  spaced   out  `, []string{"spaced", "out"}},
		{`""`, []string{""}},
		{``, nil},
	}
	for _, c := range cases {
		got := SplitCommandLine(c.in)
		if len(got) != len(c.want) {
			t.Fatalf("SplitCommandLine(%q) = %#v, want %#v", c.in, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("SplitCommandLine(%q) = %#v, want %#v", c.in, got, c.want)
			}
		}
	}
}

// 反斜杠必须原样保留（Windows 路径），不能被当成转义符吃掉
func TestSplitCommandLineKeepsBackslash(t *testing.T) {
	got := SplitCommandLine(`"C:\Program Files\x\a.exe" -q`)
	if len(got) != 2 || got[0] != `C:\Program Files\x\a.exe` || got[1] != "-q" {
		t.Fatalf("反斜杠被改写: %#v", got)
	}
}
