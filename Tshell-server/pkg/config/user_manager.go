package config

import (
	"fmt"
	"log"
	"sync"
	"time"

	"BackendTemplate/pkg/database"
	"BackendTemplate/pkg/security"
)

// UserManager 用户管理器，负责处理配置变化时的用户认证更新
type UserManager struct {
	mu              sync.RWMutex
	currentUsername string
	currentPassword string
	listeners       []UserChangeListener
}

// UserChangeListener 用户变化监听器接口
type UserChangeListener interface {
	OnUserChanged(oldUsername, newUsername, newPassword string) error
}

// 全局用户管理器实例
var GlobalUserManager *UserManager

// InitUserManager 初始化用户管理器
func InitUserManager() error {
	GlobalUserManager = &UserManager{
		listeners: make([]UserChangeListener, 0),
	}

	// 获取当前配置中的用户信息
	if GlobalConfig != nil {
		config := GlobalConfig.GetConfig()
		GlobalUserManager.currentUsername = config.Auth.DefaultUsername
		GlobalUserManager.currentPassword = config.Auth.DefaultPassword
	}

	log.Println("✅ 用户管理器初始化成功")
	return nil
}

// AddUserChangeListener 添加用户变化监听器
func (um *UserManager) AddUserChangeListener(listener UserChangeListener) {
	um.mu.Lock()
	defer um.mu.Unlock()
	um.listeners = append(um.listeners, listener)
}

// RemoveUserChangeListener 移除用户变化监听器
func (um *UserManager) RemoveUserChangeListener(listener UserChangeListener) {
	um.mu.Lock()
	defer um.mu.Unlock()
	for i, l := range um.listeners {
		if l == listener {
			um.listeners = append(um.listeners[:i], um.listeners[i+1:]...)
			break
		}
	}
}

// OnConfigChanged 配置变化时调用此方法
func (um *UserManager) OnConfigChanged(newConfig *Config) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	newUsername := newConfig.Auth.DefaultUsername
	newPassword := newConfig.Auth.DefaultPassword

	// 检查用户名和密码是否发生变化
	if newUsername != um.currentUsername || newPassword != um.currentPassword {
		log.Printf("🔄 检测到用户认证配置变化: %s -> %s", um.currentUsername, newUsername)

		oldUsername := um.currentUsername

		// 更新数据库中的用户信息
		err := um.updateDatabaseUser(oldUsername, newUsername, newPassword)
		if err != nil {
			return fmt.Errorf("更新数据库用户失败: %v", err)
		}

		// 撤销旧用户的所有JWT令牌
		err = um.revokeUserTokens(oldUsername)
		if err != nil {
			log.Printf("⚠️ 撤销旧用户令牌失败: %v", err)
		}

		// 通知所有监听器
		for _, listener := range um.listeners {
			if err := listener.OnUserChanged(oldUsername, newUsername, newPassword); err != nil {
				log.Printf("⚠️ 用户变化监听器处理失败: %v", err)
			}
		}

		// 更新当前用户信息
		um.currentUsername = newUsername
		um.currentPassword = newPassword

		log.Printf("✅ 用户认证配置更新完成: %s", newUsername)
	}

	return nil
}

// updateDatabaseUser 更新数据库中的用户信息
func (um *UserManager) updateDatabaseUser(oldUsername, newUsername, newPassword string) error {
	if database.DB() == nil {
		return fmt.Errorf("数据库引擎未初始化")
	}

	// 如果用户名发生变化，需要处理用户名更新
	if oldUsername != newUsername {
		// 检查新用户名是否已存在
		var existingUser database.Users
		exists, err := database.DB().Where("username = ?", newUsername).Get(&existingUser)
		if err != nil {
			return fmt.Errorf("查询新用户名失败: %v", err)
		}

		if exists {
			log.Printf("⚠️ 新用户名 %s 已存在，将更新其密码", newUsername)
		} else {
			// 新用户名不存在，创建新用户
			newUser, err := database.CreateSecureUser(newUsername, newPassword)
			if err != nil {
				return fmt.Errorf("创建新用户失败: %v", err)
			}

			err = database.InsertData(database.DB(), newUser)
			if err != nil {
				return fmt.Errorf("插入新用户失败: %v", err)
			}

			log.Printf("✅ 创建新用户成功: %s", newUsername)
		}

		// 删除旧用户（如果用户名不同）
		if oldUsername != "" && oldUsername != newUsername {
			_, err = database.DB().Where("username = ?", oldUsername).Delete(&database.Users{})
			if err != nil {
				log.Printf("⚠️ 删除旧用户失败: %v", err)
			} else {
				log.Printf("✅ 删除旧用户成功: %s", oldUsername)
			}
		}
	}

	// 更新用户密码（无论用户名是否变化都需要更新密码）
	newUser, err := database.CreateSecureUser(newUsername, newPassword)
	if err != nil {
		return fmt.Errorf("生成新密码哈希失败: %v", err)
	}

	// 保留创建时间
	var existingUser database.Users
	exists, err := database.DB().Where("username = ?", newUsername).Get(&existingUser)
	if err == nil && exists {
		newUser.CreatedAt = existingUser.CreatedAt
	} else {
		newUser.CreatedAt = time.Now()
	}

	_, err = database.DB().Where("username = ?", newUsername).Update(newUser)
	if err != nil {
		return fmt.Errorf("更新用户密码失败: %v", err)
	}

	log.Printf("✅ 更新用户密码成功: %s", newUsername)
	return nil
}

// revokeUserTokens 撤销指定用户的所有JWT令牌
func (um *UserManager) revokeUserTokens(username string) error {
	if security.GlobalJWTManager == nil {
		return fmt.Errorf("JWT管理器未初始化")
	}

	// 获取当前时间，用于黑名单过期时间
	expireTime := time.Now().Add(24 * time.Hour) // 令牌最长有效期

	// 由于我们无法获取所有活跃令牌，我们通过其他方式来撤销
	// 1. 可以在JWT管理器中添加按用户撤销功能
	// 2. 或者记录用户最后密码变更时间，验证时检查令牌签发时间

	// 这里我们实现一个简单的用户级别的令牌撤销
	err := security.GlobalJWTManager.RevokeUserTokens(username, expireTime)
	if err != nil {
		return fmt.Errorf("撤销用户令牌失败: %v", err)
	}

	log.Printf("✅ 撤销用户 %s 的所有令牌", username)
	return nil
}

// GetCurrentUser 获取当前配置的用户信息
func (um *UserManager) GetCurrentUser() (string, string) {
	um.mu.RLock()
	defer um.mu.RUnlock()
	return um.currentUsername, um.currentPassword
}

// JWTManagerListener JWT管理器监听器，实现UserChangeListener接口
type JWTManagerListener struct{}

// OnUserChanged 实现UserChangeListener接口
func (j *JWTManagerListener) OnUserChanged(oldUsername, newUsername, newPassword string) error {
	// 这里可以添加JWT管理器相关的处理逻辑
	log.Printf("🔄 JWT管理器处理用户变化: %s -> %s", oldUsername, newUsername)
	return nil
}