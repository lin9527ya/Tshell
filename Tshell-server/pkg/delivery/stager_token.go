package delivery

import (
	"BackendTemplate/pkg/database"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"BackendTemplate/pkg/utils/logx"
)

// StagerToken 门禁卡服务：为每个生成的 Stager 创建配额令牌，
// 下载时校验并扣减；用完即废（返回伪装 404）。令牌以 64 位随机整型生成，
// 以 hex 存储（编译进 Stager 时作为二进制数据，不产生明文字符串痕迹）。

// GenerateStagerToken 为指定投递生成一个带配额的令牌。
// quota<=0 时默认 3 次。返回令牌 hex 字符串。
func GenerateStagerToken(publicKey string, quota int, note string) (string, error) {
	if quota <= 0 {
		quota = 3
	}
	// 64 位随机令牌（8 字节），hex 编码 16 字符
	tok := make([]byte, 8)
	if _, err := rand.Read(tok); err != nil {
		return "", fmt.Errorf("生成令牌失败: %v", err)
	}
	tokenHex := hex.EncodeToString(tok)

	now := time.Now().Format("2006-01-02 15:04:05")
	rec := &database.StagerToken{
		PublicKey: publicKey,
		Token:     tokenHex,
		Quota:     quota,
		Used:      0,
		Note:      note,
		CreatedAt: now,
	}
	if _, err := database.DB().Insert(rec); err != nil {
		return "", fmt.Errorf("存储令牌失败: %v", err)
	}
	logx.Infof("🔑 [STAGER-TOKEN] 已生成门禁令牌: key=%s quota=%d token=%s\n", publicKey, quota, tokenHex)
	return tokenHex, nil
}

// ConsumeStagerToken 校验并扣减令牌配额（原子：同 token 并发只有一个能通过）。
// 返回 true=允许下载（配额已扣减）；false=令牌不存在或已用完（应返回伪装 404）。
// clientIP 用于审计日志。
func ConsumeStagerToken(publicKey, token, clientIP string) bool {
	if token == "" {
		return false
	}
	// 先取记录
	var rec database.StagerToken
	has, err := database.DB().Where("token = ?", token).Get(&rec)
	if err != nil || !has {
		return false
	}
	// 校验属于该投递（防跨投递使用）
	if rec.PublicKey != publicKey {
		return false
	}
	if rec.Used >= rec.Quota {
		logx.Warnf("🚫 [STAGER-TOKEN] 令牌配额已用完: token=%s used=%d quota=%d ip=%s\n", token, rec.Used, rec.Quota, clientIP)
		return false
	}
	// 原子扣减（CAS：where used=当前值，防并发双消费）
	now := time.Now().Format("2006-01-02 15:04:05")
	updated, err := database.DB().Where("id = ? AND used = ?", rec.ID, rec.Used).
		Update(&database.StagerToken{Used: rec.Used + 1, LastUsedAt: now, LastUsedIP: clientIP})
	if err != nil || updated == 0 {
		// 并发下另一请求已扣减 → 拒绝本次（配额语义：一次一密）
		return false
	}
	logx.Infof("🔑 [STAGER-TOKEN] 令牌消费成功: token=%s used=%d/%d ip=%s\n", token, rec.Used+1, rec.Quota, clientIP)
	return true
}

// GetStagerTokens 查询投递的令牌列表（用于管理展示）
func GetStagerTokens(publicKey string) ([]database.StagerToken, error) {
	var list []database.StagerToken
	err := database.DB().Where("public_key = ?", publicKey).Asc("id").Find(&list)
	if err != nil {
		return nil, err
	}
	return list, nil
}
