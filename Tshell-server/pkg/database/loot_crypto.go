package database

import (
	"errors"
	"fmt"
)

// ===== 战果 Secret 的静态加密接线（OPSEC P0-1）=====
//
// 为什么用注入而不是直接 import pkg/security：
// pkg/security 已经依赖 pkg/database（jwt_manager.go），反向 import 会成环。
// 所以这里只声明三个函数位，由 main 在启动时把 security 的实现塞进来。
//
// 加解密**只发生在领域层**（本包）：业务代码拿到的 Loot.Secret 始终是明文，
// 不需要在每个 handler 里记得"先解密"——那种约定迟早会漏掉一处。

var (
	lootSeal        func(string) (string, error)
	lootOpen        func(string) (string, error)
	lootFingerprint func(string) (string, error)
)

// ErrLootCryptoUnavailable 战果静态加密未配置。
//
// 刻意做成**硬错误**：静默退化为明文正是本项目反复踩过的失败模式。
// 宁可写入失败、日志喊出来，也不要让口令悄悄以明文落库。
var ErrLootCryptoUnavailable = errors.New("战果静态加密未配置（请检查 security.at_rest_key_file 或 JWT 私钥初始化）")

// SetLootCrypto 注入加解密与指纹实现（由 main 调用一次）。
func SetLootCrypto(
	seal func(string) (string, error),
	open func(string) (string, error),
	fp func(string) (string, error),
) {
	lootSeal, lootOpen, lootFingerprint = seal, open, fp
}

// LootCryptoReady 是否已接线（启动自检与测试用）
func LootCryptoReady() bool {
	return lootSeal != nil && lootOpen != nil && lootFingerprint != nil
}

// sealLootSecret 写入前加密；空串不加密（保留"没有 secret"的记录形态）
func sealLootSecret(secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	if lootSeal == nil {
		return "", ErrLootCryptoUnavailable
	}
	return lootSeal(secret)
}

// openLootSecret 读出后解密。
//
// 解密失败**必须报错**，不能返回空串：那会让操作员以为"这条战果没有口令"，
// 而不是"密文解不开"（多半是密钥换了）。两者处置完全不同。
func openLootSecret(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if lootOpen == nil {
		return "", ErrLootCryptoUnavailable
	}
	return lootOpen(stored)
}

// lootFp 计算去重指纹（见 security.Fingerprint 的说明：AES-GCM 随机化，
// 不能再用 secret 列本身做去重键）
func lootFp(secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	if lootFingerprint == nil {
		return "", ErrLootCryptoUnavailable
	}
	return lootFingerprint(secret)
}

// decryptLootRows 就地把一批行的 Secret 解成明文；任一行失败即整体报错。
func decryptLootRows(rows []Loot) error {
	if lootOpen == nil {
		if len(rows) == 0 {
			return nil
		}
		return ErrLootCryptoUnavailable
	}
	for i := range rows {
		if rows[i].Secret == "" {
			continue
		}
		plain, err := lootOpen(rows[i].Secret)
		if err != nil {
			return fmt.Errorf("战果 id=%d 解密失败: %w", rows[i].Id, err)
		}
		rows[i].Secret = plain
	}
	return nil
}

// MigrateLootSecrets 把存量明文战果加密回写（升级后一次性动作，幂等）。
//
// 同时补算指纹列：老行没有 secret_fp，去重会退化成"全不重复"。
// 返回实际迁移的行数。
func MigrateLootSecrets() (int, error) {
	if !LootCryptoReady() {
		return 0, ErrLootCryptoUnavailable
	}
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	var rows []Loot
	if err := s.Find(&rows); err != nil {
		return 0, err
	}
	migrated := 0
	for i := range rows {
		row := rows[i]
		needFp := row.SecretFp == ""
		needSeal := row.Secret != "" && !isSealedValue(row.Secret)
		if !needFp && !needSeal {
			continue
		}
		plain := row.Secret
		if !needSeal {
			// 已是密文：解出来才能算指纹
			p, err := openLootSecret(row.Secret)
			if err != nil {
				return migrated, fmt.Errorf("迁移时解密 id=%d 失败: %w", row.Id, err)
			}
			plain = p
		}
		upd := map[string]interface{}{}
		if needSeal {
			sealed, err := sealLootSecret(plain)
			if err != nil {
				return migrated, fmt.Errorf("加密 id=%d 失败: %w", row.Id, err)
			}
			upd["secret"] = sealed
		}
		if needFp && plain != "" {
			fp, err := lootFp(plain)
			if err != nil {
				return migrated, fmt.Errorf("计算 id=%d 指纹失败: %w", row.Id, err)
			}
			upd["secret_fp"] = fp
		}
		if len(upd) == 0 {
			continue
		}
		// 注意：这里不能用 .ID()——xorm 的 ID 条件需要结构体表引用，
		// 传 map 时会报 "ID condition need reference table"。
		if _, err := s.Table("loot").Where("id = ?", row.Id).Update(upd); err != nil {
			return migrated, fmt.Errorf("回写 id=%d 失败: %w", row.Id, err)
		}
		migrated++
	}
	return migrated, nil
}

// isSealedValue 由外部注入的 Open 语义反推"是否密文"。
// 这里用一个保守判定：带 "v1:" 前缀即认为已加密（与 security.Seal 的格式一致）。
// 之所以不 import security：见文件头说明（包依赖成环）。
func isSealedValue(stored string) bool {
	return len(stored) > 3 && stored[0] == 'v' && stored[1] == '1' && stored[2] == ':'
}
