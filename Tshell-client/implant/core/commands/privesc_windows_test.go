//go:build windows

package commands

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// 提权模块的**可测部分**：参数校验、权限启用语义、注册表还原的幂等性，
// 以及几处"绝不产出假成功"的守护。
//
// 真实提权成功率只能在授权 VM 上验证（见 docs/PRIVILEGE_ESCALATION.md 的验证清单）。
// 与注册表/权限相关的用例会先探测环境能力再断言 —— 受限环境（受管终端、沙箱）
// 会拒 LookupPrivilegeValueW 或 RegDeleteKeyW，那种情况下"跳过并说明"比"红着报失败"更有信息量。

// privLookupSupported 探测本环境能否查询权限 LUID
func privLookupSupported(t *testing.T) bool {
	t.Helper()
	if err := enablePrivilege("SeChangeNotifyPrivilege"); err != nil {
		if strings.Contains(err.Error(), "本机不存在权限") {
			t.Logf("环境不支持权限查询（受限令牌），跳过依赖项: %v", err)
			return false
		}
	}
	return true
}

func TestCmdGetSystemValidatesTech(t *testing.T) {
	// 非法技术必须当场报错，而不是当成 auto 静默跑一遍真实提权
	for _, bad := range []string{"magic", "token extra", "uac2", "cleanup"} {
		if _, err := CmdGetSystem([]byte(bad)); err == nil {
			t.Errorf("CmdGetSystem(%q) 应报错", bad)
		}
	}
}

func TestCmdGetSystemAlreadyPrivilegedIsIdempotent(t *testing.T) {
	// 普通权限下 getsystem 会真的尝试提权（对目标机有副作用），测试不能触发。
	// 因此只在**已经**是 SYSTEM/高完整性时验证幂等分支，否则跳过。
	isSystem, elevated := currentPrivilegeLevel()
	if !isSystem && !elevated {
		t.Skip("当前进程不是 SYSTEM/高完整性，跳过（避免测试触发真实提权动作）")
	}
	out, err := CmdGetSystem(nil)
	if err != nil {
		t.Fatalf("已是高完整性时不应报错: %v", err)
	}
	if !strings.Contains(string(out), "无需提权") {
		t.Fatalf("应报告无需提权，实际: %s", out)
	}
	out2, err := CmdGetSystem([]byte("auto"))
	if err != nil || !strings.Contains(string(out2), "无需提权") {
		t.Fatalf("重复执行应保持一致: %v %s", err, out2)
	}
}

func TestEnablePrivilegeSemantics(t *testing.T) {
	// 1) 不存在的权限名必须报错（走 LookupPrivilegeValueW 失败分支）
	if err := enablePrivilege("SeNoSuchPrivilegeForUnitTest"); err == nil {
		t.Fatal("不存在的权限名应报错")
	}

	if !privLookupSupported(t) {
		t.Skip("环境不支持权限查询，跳过权限启用断言")
	}

	// 2) 普通用户都持有的权限：启用应成功，且可重复调用（幂等）
	for i := 0; i < 2; i++ {
		if err := enablePrivilege("SeShutdownPrivilege"); err != nil {
			t.Fatalf("第 %d 次启用 SeShutdownPrivilege 失败: %v", i+1, err)
		}
	}

	// 3) 语义守护：AdjustTokenPrivileges 返回成功但权限未分配时必须报错。
	//    SeTcbPrivilege 只在 SYSTEM 上分配，普通进程拿不到 —— 这是那条分支的现实样本。
	if isSystem, _ := currentPrivilegeLevel(); !isSystem {
		err := enablePrivilege("SeTcbPrivilege")
		if err == nil {
			t.Fatal("普通进程不应成功启用 SeTcbPrivilege（否则说明没检查 ERROR_NOT_ALL_ASSIGNED）")
		}
		if !strings.Contains(err.Error(), "SeTcbPrivilege") {
			t.Fatalf("错误信息应指明是哪项权限: %v", err)
		}
	}
}

func TestSelfCommandLineShape(t *testing.T) {
	line, err := selfCommandLine()
	if err != nil {
		// 测试进程的 os.Args[1] 不是口令（go test 会带 -test.* 参数）
		t.Skipf("当前进程无启动口令参数（预期内的跳过）: %v", err)
	}
	if !strings.HasPrefix(line, `"`) || !strings.HasSuffix(line, `"`) {
		t.Fatalf("命令行应把 exe 与口令都加引号: %q", line)
	}
	if strings.Count(line, `"`) != 4 {
		t.Fatalf(`命令行应恰好是 "exe" "口令" 两段: %q`, line)
	}
}

// 注册表语义：写 → 还原 → 幂等。用**隔离的临时键**，绝不碰真实的 ms-settings
// （那会影响开发机上输入法/设置页的行为）。
func TestUACRegistryRestoreAndCleanup(t *testing.T) {
	base := `TshellTestPrivesc\ms-settings\shell\open\command`
	delegate := base + `\DelegateExecute`
	t.Cleanup(func() { _, _ = deleteRegKeyTree(settingsRootOf(base)) })

	// 1) 原本有值：还原后必须写回原值
	if err := writeRegString(base, "", "原值-original"); err != nil {
		t.Fatalf("预置原值失败: %v", err)
	}
	restore, err := uacHijackWith(base, delegate, `"C:\x.exe" "pw"`)
	if err != nil {
		t.Fatalf("写劫持键失败: %v", err)
	}
	if v, _ := readRegString(base, ""); v != `"C:\x.exe" "pw"` {
		t.Fatalf("劫持值未写入: %q", v)
	}
	if err := restore(); err != nil {
		t.Fatalf("还原失败: %v", err)
	}
	if v, _ := readRegString(base, ""); v != "原值-original" {
		t.Fatalf("应写回原值，实际 %q", v)
	}

	// 2) 原本不存在：还原后值应为空/不存在
	_, _ = deleteRegKeyTree(settingsRootOf(base))
	restore2, err := uacHijackWith(base, delegate, `"C:\y.exe" "pw"`)
	if err != nil {
		t.Fatalf("第二次写劫持键失败: %v", err)
	}
	if err := restore2(); err != nil {
		t.Fatalf("第二次还原失败: %v", err)
	}
	if v, ok := readRegString(base, ""); ok && strings.TrimSpace(v) != "" {
		t.Fatalf("原本不存在的值还原后应为空，实际 %q", v)
	}

	// 3) uacCleanup 幂等：无残留时返回空且不报错
	removed, err := uacCleanup(base, delegate)
	if err != nil {
		t.Fatalf("无残留时清理不应报错: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("无残留时不应报告清理项: %v", removed)
	}

	// 4) 有残留时必须清掉（删键或清空值，两者都算干净）
	if err := writeRegString(base, "", `"C:\z.exe" "pw"`); err != nil {
		t.Fatal(err)
	}
	removed, err = uacCleanup(base, delegate)
	if err != nil {
		t.Fatalf("有残留时清理失败: %v", err)
	}
	if len(removed) == 0 {
		t.Fatal("有残留时应报告清理项")
	}
	if v, _ := readRegString(base, ""); strings.TrimSpace(v) != "" {
		t.Fatalf("清理后劫持值应为空，实际 %q（清理未生效）", v)
	}
}

// 删不掉键时必须退化为"清空劫持值"，而不是留下可用的劫持 —— 安全兜底
func TestUACCleanupDegradesSafely(t *testing.T) {
	base := `TshellTestPrivescDegrade\ms-settings\shell\open\command`
	delegate := base + `\DelegateExecute`
	t.Cleanup(func() { _, _ = deleteRegKeyTree(settingsRootOf(base)) })

	if err := writeRegString(base, "", `"C:\evil.exe" "pw"`); err != nil {
		t.Fatalf("写劫持值失败: %v", err)
	}
	removed, err := uacCleanup(base, delegate)
	if err != nil {
		t.Fatalf("清理不应报错（要么删键、要么清空值）: %v", err)
	}
	if len(removed) == 0 {
		t.Fatal("应报告清理动作")
	}
	// 无论走了哪条路，劫持值都必须失效
	if v, _ := readRegString(base, ""); strings.TrimSpace(v) != "" {
		t.Fatalf("清理后仍残留可用的劫持命令行: %q", v)
	}
}

func TestSettingsRootOf(t *testing.T) {
	cases := map[string]string{
		`Software\Classes\ms-settings\shell\open\command`: `Software\Classes\ms-settings`,
		`Software\Test\ms-settings\x`:                     `Software\Test\ms-settings`,
		`Software\NoMarker`:                               `Software\NoMarker`,
	}
	for in, want := range cases {
		if got := settingsRootOf(in); got != want {
			t.Errorf("settingsRootOf(%q) = %q，期望 %q", in, got, want)
		}
	}
}

func TestCurrentPrivilegeLevelConsistent(t *testing.T) {
	isSystem, elevated := currentPrivilegeLevel()
	// SYSTEM 必然是高完整性；反过来不成立（管理员高完整性不是 SYSTEM）
	if isSystem && !elevated {
		t.Fatal("令牌是 SYSTEM 时不可能不是高完整性（判定逻辑有矛盾）")
	}
}

// 进程枚举是 token 路径的前提：候选名解析不出来就只能报"没有候选进程"
func TestPidsByNameFiltersRequestedOnly(t *testing.T) {
	self := []string{"explorer.exe", "svchost.exe", "lsass.exe", "services.exe",
		"winlogon.exe", "wininit.exe", "smss.exe"}
	pids, err := pidsByName(self)
	if err != nil {
		t.Fatalf("枚举进程失败: %v", err)
	}
	for _, p := range pids {
		found := false
		for _, n := range self {
			if strings.EqualFold(p.name, n) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("枚举返回了未请求的进程: %s(%d)", p.name, p.pid)
		}
		if p.pid == 0 {
			t.Fatalf("候选进程 PID 不应为 0: %+v", p)
		}
	}
	t.Logf("本机命中候选进程 %d 个", len(pids))
}

// RegDeleteKeyW 的语义记录：**不递归**，带子键的键直接返回 Access is denied。
// 这条 quirk 正是 deleteRegKeyTree 要自己深度优先清子键的原因，
// 留成用例避免以后有人"优化"回单层 DeleteKey。
func TestRegDeleteKeyIsNotRecursive(t *testing.T) {
	parent, err := registry.OpenKey(registry.CURRENT_USER, ``, registry.ALL_ACCESS)
	if err != nil {
		t.Fatalf("打开 HKCU 失败: %v", err)
	}
	defer parent.Close()

	ck, _, err := registry.CreateKey(registry.CURRENT_USER, `TshellQuirkProbe\sub`, registry.ALL_ACCESS)
	if err != nil {
		t.Fatalf("建嵌套键失败: %v", err)
	}
	ck.Close()
	t.Cleanup(func() { _, _ = deleteRegKeyTree(`TshellQuirkProbe`) })

	if err := registry.DeleteKey(parent, `TshellQuirkProbe`); err == nil {
		t.Log("本机 RegDeleteKeyW 竟然递归了（Windows 语义可能已变），请复核 deleteRegKeyTree 的必要性")
	}

	// 我们的封装必须能删掉它（走递归路径）
	hard, err := deleteRegKeyTree(`TshellQuirkProbe`)
	if err != nil {
		t.Fatalf("递归删除失败: %v", err)
	}
	if !hard {
		t.Fatal("应走硬删除路径（本环境允许删键）")
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, `TshellQuirkProbe`, registry.QUERY_VALUE); err == nil {
		t.Fatal("递归删除后键仍存在")
	}
}
