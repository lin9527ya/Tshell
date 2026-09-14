//go:build windows

package commands

import (
	"fmt"
	"strings"
)

// ===== A5: 域信息收集（AD 枚举）=====
// cmdBuf: "users" | "groups" | "computers" | "all"
// 通过 net 命令 + ADSI 查询实现；非域环境返回明确提示。

// CmdAdEnum 域信息收集入口
func CmdAdEnum(cmdBuf []byte) ([]byte, error) {
	action := strings.ToLower(strings.TrimSpace(string(cmdBuf)))
	if action == "" {
		action = "all"
	}

	var out strings.Builder
	switch action {
	case "users":
		out.WriteString("===== 域用户 =====\n")
		appendCmdOutput(&out, "net", "user", "/domain")
	case "groups":
		out.WriteString("===== 域组 =====\n")
		appendCmdOutput(&out, "net", "group", "/domain")
	case "computers":
		out.WriteString("===== 域计算机 =====\n")
		appendCmdOutput(&out, "net", "group", "Domain Computers", "/domain")
	case "domain":
		out.WriteString("===== 域信息 =====\n")
		appendCmdOutput(&out, "net", "domains")
	case "dcs":
		out.WriteString("===== 域控制器 =====\n")
		ps := `[System.DirectoryServices.ActiveDirectory.Domain]::GetCurrentDomain().DomainControllers | Select-Object Name, IPAddress | Format-Table -AutoSize`
		o, err := execWithTimeoutLong("powershell", "-NoProfile", "-Command", ps)
		if err != nil {
			out.WriteString("[!] " + err.Error() + "\n")
		} else {
			out.Write(o)
		}
	case "spn":
		out.WriteString("===== SPN 查询 =====\n")
		ps := `setspn -q */* 2>$null`
		o, err := execWithTimeoutLong("cmd", "/c", ps)
		if err != nil {
			out.WriteString("[!] " + err.Error() + "\n")
		} else {
			out.Write(o)
		}
	case "all":
		fallthrough
	default:
		out.WriteString("===== 域信息 =====\n")
		appendCmdOutput(&out, "net", "domains")
		out.WriteString("\n===== 域用户 =====\n")
		appendCmdOutput(&out, "net", "user", "/domain")
		out.WriteString("\n===== 域组 =====\n")
		appendCmdOutput(&out, "net", "group", "/domain")
		out.WriteString("\n===== 域计算机 =====\n")
		appendCmdOutput(&out, "net", "group", "Domain Computers", "/domain")
	}

	result := out.String()
	if strings.Contains(result, "WORKSTATION") && !strings.Contains(result, "NT AUTHORITY") && !strings.Contains(strings.ToLower(result), "domain") {
		// 非域环境提示
		return []byte(result + "\n[!] 当前主机可能不在域环境中（以上为 net 命令原始输出）"), nil
	}
	return []byte(result), nil
}

// appendCmdOutput 执行命令并追加输出
func appendCmdOutput(b *strings.Builder, name string, args ...string) {
	out, err := execWithTimeoutLong(name, args...)
	if err != nil {
		b.WriteString("[!] " + name + " 执行失败: " + err.Error() + "\n")
		return
	}
	b.Write(out)
	if len(out) == 0 {
		b.WriteString("(无输出)\n")
	}
}

var _ = fmt.Sprintf
