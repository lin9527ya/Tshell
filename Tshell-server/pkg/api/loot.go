package api

import (
	"BackendTemplate/pkg/database"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ===== Loot 凭据/制品库(功能批) =====
// 集中管理行动中获取的凭据与制品: 手动录入、命令产出自动识别(如 WiFi 密码)、
// 截图等制品引用。V1 提供库表 + CRUD + 按 uid/类型过滤;提取命令族(LSASS/DPAPI/
// 浏览器)后续批次接入。

// lootType 合法值校验(V1 枚举,前端过滤 pill 对齐)
var lootTypes = map[string]bool{
	"pwd": true,    // 明文密码
	"hash": true,   // NTLM/哈希
	"domain": true, // 域账号
	"note": true,   // 备忘(密钥/token/连接串等自由文本)
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

	session := database.Engine.Where("1=1")
	if uid != "" {
		session = session.And("uid = ?", uid)
	}
	if lootType != "" && lootType != "all" {
		if !lootTypes[lootType] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "type 非法"})
			return
		}
		session = session.And("type = ?", lootType)
	}

	var total int64
	session.Table("loot").Count(&total)
	var items []database.Loot
	session.Table("loot").Desc("id").Limit(size, (page-1)*size).Find(&items)

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
	if _, err := database.Engine.Insert(&body); err != nil {
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
	if _, err := database.Engine.ID(id).Delete(&database.Loot{}); err != nil {
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
	item := database.Loot{
		Uid: uid, Type: lootType, User: user, Realm: realm, Host: host,
		Secret: secret, Note: note, Source: source,
		CreatedAt: time.Now().Format("2006-01-02 15:04:05"),
	}
	if _, err := database.Engine.Insert(&item); err != nil {
		// 静默: 自动入库失败不阻断命令回包主链路
		return
	}
}
