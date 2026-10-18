package database

// ===== 领域存储：用户账号（users.go / mfa.go / key_management.go）=====
//
// 用户查询在这三个文件里都以 `Where("username = ?")` 出现十余次。收口的价值不只是
// 少写字：账号相关的**列更新**必须显式指定列名（否则会把 MFA 密钥、恢复码等
// 字段写成零值），这类"漏列/多列"的错误在散落写法里几乎无法靠 review 发现。

// UserStore 用户账号领域访问
type UserStore struct{}

// Users_ 用户领域方法入口（Users 已是实体类型名）
func Users_() UserStore { return UserStore{} }

// GetByName 按用户名取账号
func (UserStore) GetByName(username string) (Users, bool, error) {
	var u Users
	s := DB()
	if s == nil {
		return u, false, ErrStoreUnavailable
	}
	has, err := s.Where("username = ?", username).Get(&u)
	return u, has, err
}

// Exists 用户名是否已存在
func (UserStore) Exists(username string) (bool, error) {
	s := DB()
	if s == nil {
		return false, ErrStoreUnavailable
	}
	return s.Where("username = ?", username).Exist(new(Users))
}

// Insert 新建账号
func (UserStore) Insert(u *Users) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Insert(u)
}

// UpdateColumns 只更新指定列。
//
// **必须显式列名**：用整结构体 Update 会把未赋值的列写成零值 —— 对 users 表
// 而言就是把 mfa_secret_enc / mfa_recovery_hashes 清空，等于悄悄关掉第二因子。
func (UserStore) UpdateColumns(username string, cols []string, patch *Users) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("username = ?", username).Cols(cols...).Update(patch)
}

// UpdateFields 更新非零字段（登录计数、最后登录时间这类"只加不减"的字段用）
func (UserStore) UpdateFields(username string, patch *Users) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("username = ?", username).Update(patch)
}

// CountByRole 某角色账号数（删除/降级最后一个管理员前的守卫）
func (UserStore) CountByRole(role string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("role = ?", role).Count(new(Users))
}

// List 全部账号（按用户名升序）
func (UserStore) List() ([]Users, error) {
	var out []Users
	s := DB()
	if s == nil {
		return nil, ErrStoreUnavailable
	}
	return out, s.Asc("username").Find(&out)
}

// Delete 删除账号
func (UserStore) Delete(username string) (int64, error) {
	s := DB()
	if s == nil {
		return 0, ErrStoreUnavailable
	}
	return s.Where("username = ?", username).Delete(new(Users))
}

// ===== MFA 相关列（集中列名，避免手写字符串漂移）=====

// mfaColumns MFA 三列的列名。集中成常量：这三列名在 mfa.go 里被手写了多次，
// 任何一处拼错都会表现为"绑定了但登录不要求动态码"（更新到了不存在的列名，
// SQLite 会静默当作新列或报错被忽略）。
var mfaColumns = []string{"mfa_secret_enc", "mfa_enabled", "mfa_recovery_hashes"}

// SetMFA 写入 MFA 三列（绑定/重置时用）
func (UserStore) SetMFA(username, secretEnc string, enabled bool, recoveryHashes string) (int64, error) {
	return UserStore{}.UpdateColumns(username, mfaColumns, &Users{
		MFASecretEnc:      secretEnc,
		MFAEnabled:        enabled,
		MFARecoveryHashes: recoveryHashes,
	})
}

// ClearMFA 清空 MFA 三列（关闭/管理员重置时用）
func (UserStore) ClearMFA(username string) (int64, error) {
	return UserStore{}.UpdateColumns(username, mfaColumns, &Users{
		MFASecretEnc:      "",
		MFAEnabled:        false,
		MFARecoveryHashes: "",
	})
}

// UpdateRecoveryHashes 只更新恢复码哈希（用掉一个恢复码后回写剩余列表）
func (UserStore) UpdateRecoveryHashes(username, hashes string) (int64, error) {
	return UserStore{}.UpdateColumns(username, []string{"mfa_recovery_hashes"},
		&Users{MFARecoveryHashes: hashes})
}
