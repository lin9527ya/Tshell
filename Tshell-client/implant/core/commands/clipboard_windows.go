//go:build windows

package commands

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Clipboard 读取 Windows 剪贴板文本内容。
func Clipboard() ([]byte, error) {
	user32 := syscall.NewLazyDLL("user32.dll")
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	openClipboard := user32.NewProc("OpenClipboard")
	closeClipboard := user32.NewProc("CloseClipboard")
	getClipboardData := user32.NewProc("GetClipboardData")
	globalLock := kernel32.NewProc("GlobalLock")
	globalUnlock := kernel32.NewProc("GlobalUnlock")

	const CF_UNICODETEXT = 13

	ret, _, _ := openClipboard.Call(0)
	if ret == 0 {
		return nil, fmt.Errorf("打开剪贴板失败")
	}
	defer closeClipboard.Call()

	// 优先尝试 Unicode 文本
	h, _, _ := getClipboardData.Call(CF_UNICODETEXT)
	if h != 0 {
		ptr, _, _ := globalLock.Call(h)
		if ptr != 0 {
			defer globalUnlock.Call(h)
			text := syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(ptr))[:])
			return []byte(text), nil
		}
	}

	// 回落到 ANSI 文本
	h, _, _ = getClipboardData.Call(1) // CF_TEXT
	if h != 0 {
		ptr, _, _ := globalLock.Call(h)
		if ptr != 0 {
			defer globalUnlock.Call(h)
			p := (*[1 << 20]byte)(unsafe.Pointer(ptr))
			n := 0
			for n < len(p) && p[n] != 0 {
				n++
			}
			return p[:n], nil
		}
	}

	return []byte("(剪贴板为空或非文本格式)"), nil
}
