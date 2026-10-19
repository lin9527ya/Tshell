package api

import (
	"fmt"
	"time"

	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/utils/logx"
)

// ===== 战果保留期与自动清理（OPSEC P0-5）=====
//
// 现状差异：任务日志有 7 天保留（taskRetention），审计日志有按天清理，
// 而**战果永久留存** —— 一条 2019 年的域管理员口令会和今天刚拿到的躺在一起。
// 这不是功能缺陷，是数据管理缺陷：留存越久，泄露面越大，而实际用得上的往往只是近期战果。
//
// 默认值刻意是 0（不清理），保持既有语义 —— 清理战果是**不可逆的破坏性操作**，
// 不该由升级动作替运维做决定。要开就显式配 security.loot_retention_days。

// LootRetentionInterval 清理周期。一天一次足够：战果不是高频数据，
// 而且删除是不可逆的，跑太勤只会增加误删的机会。
const LootRetentionInterval = 24 * time.Hour

// PurgeExpiredLoot 删除超过保留期的战果，返回删除条数。
//
// days <= 0 表示不清理（返回 0，不报错）—— 让调用方不必到处判 0。
func PurgeExpiredLoot(days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	n, err := database.LootStore_().PurgeOlderThan(cutoff)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		// 删除是不可逆的，必须留痕（删了多少、按什么策略、截止到哪天）
		lootAuditEvent("LOOT_PURGE", "SUCCESS",
			fmt.Sprintf("按保留期清理战果：删除 %d 条（保留 %d 天，截止 %s）",
				n, days, cutoff.Format("2006-01-02")),
			map[string]interface{}{
				"deleted": n, "retention_days": days,
				"cutoff": cutoff.Format("2006-01-02 15:04:05"),
			})
	}
	return n, nil
}

// StartLootRetention 启动保留期清理循环，返回停止函数。
//
// 启动时先跑一次：否则"配了 30 天"要等 24 小时才生效，
// 而这期间库里的陈旧战果一直躺着。
func StartLootRetention(days int) (stop func()) {
	if days <= 0 {
		return func() {} // 未启用：返回空停止函数，调用方不必判空
	}
	done := make(chan struct{})

	runOnce := func() {
		if n, err := PurgeExpiredLoot(days); err != nil {
			logx.Warnf("⚠️ [安全] 战果保留期清理失败: %v\n", err)
		} else if n > 0 {
			logx.Infof("🧹 [安全] 已清理 %d 条超过 %d 天的战果\n", n, days)
		}
	}

	go func() {
		runOnce()
		ticker := time.NewTicker(LootRetentionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runOnce()
			case <-done:
				return
			}
		}
	}()

	return func() { close(done) }
}
