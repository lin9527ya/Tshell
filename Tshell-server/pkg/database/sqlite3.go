package database

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	mathrand "math/rand"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
	"xorm.io/xorm"
)

// DefaultClientSleep 默认客户端心跳间隔（秒），由 main 启动时从配置 client.sleep 设置
var DefaultClientSleep = "5"

var Engine *xorm.Engine
var DatabaseName string = "./database.db"

// 密码安全处理函数
// HashPassword 使用bcrypt生成密码哈希
func HashPassword(password string) (string, error) {
	// 使用bcrypt生成哈希，成本因子设为14（推荐安全级别）
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// CheckPasswordHash 验证密码是否匹配哈希
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateSalt 生成随机盐值
func GenerateSalt() (string, error) {
	bytes := make([]byte, 32) // 256位盐值
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateRandomPassword 生成随机密码（crypto/rand，字母数字混合）
func GenerateRandomPassword(length int) string {
	if length < 8 {
		length = 8
	}
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		// 极端情况下退化到时间种子（仅作兜底）
		r := mathrand.New(mathrand.NewSource(time.Now().UnixNano()))
		for i := range b {
			b[i] = charset[r.Intn(len(charset))]
		}
	} else {
		for i := range b {
			b[i] = charset[int(b[i])%len(charset)]
		}
	}
	return string(b)
}

// isWeakPassword 常见弱口令检测（避免默认口令落地）
func isWeakPassword(pwd string) bool {
	weak := []string{
		"admin", "admin123", "123456", "12345678", "password",
		"passw0rd", "root", "toor", "1234", "000000", "qwerty",
		"admin888", "123123", "666666", "888888",
	}
	for _, w := range weak {
		if strings.EqualFold(pwd, w) {
			return true
		}
	}
	if len(pwd) < 8 {
		return true
	}
	return false
}

// CreateSecureUser 创建安全用户
func CreateSecureUser(username, password string) (*Users, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("密码哈希失败: %v", err)
	}

	salt, err := GenerateSalt()
	if err != nil {
		return nil, fmt.Errorf("盐值生成失败: %v", err)
	}

	now := time.Now()
	return &Users{
		Username:      username,
		PasswordHash:  hash,
		Salt:          salt,
		CreatedAt:     now,
		LastLogin:     time.Time{}, // 零值表示从未登录
		LoginAttempts: 0,
		Locked:        false,
		Role:          "admin", // C13 RBAC：默认 admin（首个用户为管理员）
	}, nil
}

// InitializeDefaultUserFromConfig 从配置文件初始化默认管理员用户
func InitializeDefaultUserFromConfig(username, password string) error {
	if Engine == nil {
		return fmt.Errorf("数据库引擎未初始化")
	}

	// 随机密码仅用于"创建新用户"场景；迁移恢复（哈希缺失）必须用配置密码，
	// 否则每次启动都会更换密码导致无法登录
	configPwd := password
	if configPwd == "" {
		return fmt.Errorf("配置中默认密码为空，无法初始化用户")
	}

	// 检查是否已存在用户
	var user Users
	exists, err := Engine.Where("username = ?", username).Get(&user)
	if err != nil {
		return fmt.Errorf("查询用户失败: %v", err)
	}

	if !exists {
		// 不存在则创建新用户（弱口令不落地：生成随机密码并显著告警）
		if isWeakPassword(password) {
			password = GenerateRandomPassword(16)
			log.Printf("⚠️⚠️ 配置的默认密码为空或过于简单，已生成随机密码：username=%s, password=%s（请立即登录并修改）", username, password)
		}
		defaultUser, err := CreateSecureUser(username, password)
		if err != nil {
			return fmt.Errorf("创建默认管理员用户失败: %v", err)
		}

		err = InsertData(Engine, defaultUser)
		if err != nil {
			return fmt.Errorf("插入默认管理员用户失败: %v", err)
		}

		log.Printf("默认管理员账户已创建：username=%s", username)
		return nil
	}

	// 如果存在但是旧版本的明文密码，则进行迁移（使用配置密码，不覆盖为随机密码）
	if user.PasswordHash == "" {
		log.Printf("⚠️ 检测到用户 %s 密码哈希缺失，使用配置文件密码重新设置（若此前修改过密码，请重新设置）", username)

		// 使用配置中的密码进行迁移
		updatedUser, err := CreateSecureUser(username, configPwd)
		if err != nil {
			return fmt.Errorf("密码迁移失败: %v", err)
		}

		// 保留原有的时间戳
		updatedUser.CreatedAt = user.CreatedAt
		if user.CreatedAt.IsZero() {
			updatedUser.CreatedAt = time.Now()
		}
		updatedUser.LastLogin = user.LastLogin

		_, err = Engine.Where("username = ?", username).Update(updatedUser)
		if err != nil {
			return fmt.Errorf("更新管理员密码失败: %v", err)
		}

		log.Printf("用户 %s 密码迁移完成，现在使用安全哈希存储", username)
	}

	return nil
}

type Users struct {
	Username      string    `json:"username" xorm:"index"` // 登录/刷新/用户管理均按 username 查询
	PasswordHash  string    `json:"password_hash"`  // bcrypt哈希密码
	Salt          string    `json:"salt"`           // 密码盐值
	CreatedAt     time.Time `json:"created_at"`     // 创建时间
	LastLogin     time.Time `json:"last_login"`     // 最后登录时间
	LoginAttempts int       `json:"login_attempts"` // 登录尝试次数
	Locked        bool      `json:"locked"`         // 账户锁定状态
	//Email       string
	//Phone       string
	Role          string    `json:"role"` // admin | operator | auditor（C13 RBAC）
	//Permissions int
}
type Clients struct {
	ID                int64 `xorm:"'id' pk autoincr" json:"id"`
	Uid               string `xorm:"index"`
	FirstStart        string
	ExternalIP        string
	InternalIP        string
	Username          string
	Computer          string
	Process           string
	Pid               string
	Address           string
	Arch              string
	Note              string
	Sleep             string
	Online            string `xorm:"index"`
	Color             string
	Group             string `json:"group" xorm:"index"` // C14: 客户端分组
	Listener          string `xorm:"index"` // 客户端对应的监听器协议类型（websocket/tcp/kcp等）
	Os                string `xorm:"default('')"` // 客户端操作系统类型（linux/windows/darwin，元数据上报）
	HeartbeatTimeout  string `xorm:"default('30')"` // 心跳超时次数，保持30次
	HeartbeatInterval string `xorm:"default('10')"` // 心跳检测间隔（秒），保持10秒
	LastActive        string `json:"last_active"`   // 最后活跃时间（用于统计离线时长）
}
type Notes struct {
	Uid  string `xorm:"index"`
	Note string
}
type Shell struct {
	Uid          string `xorm:"index"`
	ShellContent string
}
type Downloads struct {
	Uid            string `xorm:"index"`
	FileName       string
	FilePath       string `xorm:"index"`
	FileSize       int
	DownloadedSize int
}
type Listener struct {
	Id                int64 `xorm:"pk autoincr"`
	Type              string `xorm:"index"`
	ListenAddress     string `xorm:"index"`
	ConnectAddress    string `xorm:"index"`
	Status            int    `xorm:"index"`
	HeartbeatTimeout  string `xorm:"default('30')"`                 // 心跳超时次数，保持30次
	HeartbeatInterval string `xorm:"default('10')"`                 // 心跳检测间隔（秒），保持10秒
	Note              string `xorm:"default('')"`                   // 监听器备注
	VKey              string `xorm:"'v_key' default('')"`           // VKey
	EncryptionSalt    string `xorm:"'encryption_salt' default('')"` // 流量加密盐
}
// WebDelivery 载荷投递配置（Delivery Hub）
// 兼容旧字段 ListeningPort/FileName；新字段支持统一路由、伪装、防护与统计
type WebDelivery struct {
	ID             int64  `xorm:"'id' pk autoincr" json:"id"`
	ListenerConfig string `json:"listener_config"`
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	ListeningPort  string `json:"listening_port"` // 独立端口模式使用；shared 模式可空或标记 shared
	Status         int    `json:"status"`         // 1=开启 2=关闭
	BuildCount     int    `json:"build_count"`    // stager 构建次数（独立字段，避免污染 Status）
	LastBuild      string `json:"last_build"`     // 最近一次 stager 构建时间(P2-10;原 stager-info 返回假数据)
	ServerAddress  string `json:"server_address"` // 完整客户端公开 URL
	FileName       string `json:"file_name"`
	Pass           string `json:"pass"`
	DownloadPath   string `json:"download_path"`
	SaveFilename   string `json:"save_filename"`

	// === Delivery Hub 扩展 ===
	// bind_mode: shared=挂主服务 /d/{key}/...；dedicated=独立端口（兼容旧行为）
	BindMode string `xorm:"varchar(20) default('shared')" json:"bind_mode"`
	// 公开路由 key（短随机串），用于 /d/{public_key}/full|stager|sc
	PublicKey string `xorm:"varchar(64) index" json:"public_key"`
	// 对外 Host（如 1.2.3.4:8089 或 cdn.example.com），空则自动推断
	PublicHost string `xorm:"varchar(255) default('')" json:"public_host"`
	// 公开 scheme：http | https | 空=跟随全局 delivery.default_scheme
	PublicScheme string `xorm:"varchar(16) default('')" json:"public_scheme"`
	// CDN/反代路径前缀，如 /static；最终 {scheme}://{host}{base_path}/d/{key}/...
	BasePath string `xorm:"varchar(255) default('')" json:"base_path"`
	// 自定义 URI 路径片段（可选伪装），默认用 public_key
	URIPath string `xorm:"varchar(255) default('')" json:"uri_path"`
	// 完整客户端 Content-Type 伪装
	ContentType string `xorm:"varchar(128) default('application/octet-stream')" json:"content_type"`
	// shellcode/stager 的 Content-Type（如 font/woff）
	StageContentType string `xorm:"varchar(128) default('font/woff')" json:"stage_content_type"`
	// 访问 token（空=不校验）；请求需 ?t=token 或 Header X-Delivery-Token
	AccessToken string `xorm:"varchar(128) default('')" json:"access_token"`
	// UA 子串白名单，逗号分隔；空=不限制
	UAAllow string `xorm:"varchar(512) default('')" json:"ua_allow"`
	// 最大下载次数，0=不限制；达到后自动关闭
	MaxDownloads int `xorm:"default(0)" json:"max_downloads"`
	// 命中/下载统计
	HitCount    int64  `xorm:"default(0)" json:"hit_count"`
	LastHitAt   string `xorm:"varchar(64) default('')" json:"last_hit_at"`
	LastHitIP   string `xorm:"varchar(64) default('')" json:"last_hit_ip"`
	// 一次性链接：1=启用（首次成功下载后作废对应资源）
	OneTime int `xorm:"default(0)" json:"one_time"`
	// stager / shellcode 公开 URL（创建时填好，便于前端展示）
	StagerURL    string `xorm:"varchar(512) default('')" json:"stager_url"`
	ShellcodeURL string `xorm:"varchar(512) default('')" json:"shellcode_url"`
	// Stage XOR 混淆密钥（0=不加密；生成投递时随机设置，Stager 模板内置同 key 解密）
	XorKey int `xorm:"default(0)" json:"xor_key"`
	// 备注
	Note string `xorm:"varchar(500) default('')" json:"note"`
}

// StagerToken 每个 Stager 独立的"门禁卡"令牌（防盗门）：
// 生成 Stager 时随机生成 64 位令牌并记录配额（如 3 次），下载时校验并扣减，
// 用完返回伪装 404。区别于 WebDelivery.AccessToken（全局静态 token）。
type StagerToken struct {
	ID        int64  `xorm:"'id' pk autoincr" json:"id"`
	PublicKey string `xorm:"varchar(64) index" json:"public_key"` // 关联投递 public_key
	Token     string `xorm:"varchar(64) index unique" json:"token"` // 64 位随机令牌（hex）
	Quota     int    `json:"quota"`          // 允许下载次数
	Used      int    `json:"used"`           // 已使用次数
	Note      string `xorm:"varchar(200) default('')" json:"note"` // 备注（如 Stager 标识）
	CreatedAt string `xorm:"varchar(32) default('')" json:"created_at"`
	LastUsedAt string `xorm:"varchar(32) default('')" json:"last_used_at"`
	LastUsedIP string `xorm:"varchar(64) default('')" json:"last_used_ip"`
}

// DeliveryAccessLog 投递访问日志
type DeliveryAccessLog struct {
	ID         int64     `xorm:"'id' pk autoincr" json:"id"`
	DeliveryID int64     `xorm:"'delivery_id' index" json:"delivery_id"`
	PublicKey  string    `xorm:"varchar(64) index" json:"public_key"`
	Resource   string    `xorm:"varchar(32)" json:"resource"` // full/stager/sc/woff
	ClientIP   string    `xorm:"varchar(64)" json:"client_ip"`
	UserAgent  string    `xorm:"varchar(512)" json:"user_agent"`
	StatusCode int       `json:"status_code"`
	BytesSent  int64     `json:"bytes_sent"`
	CreatedAt  time.Time `xorm:"created" json:"created_at"`
}

// SOCKS5代理数据库模型
type Socks5 struct {
	ID             int64     `xorm:"'id' pk autoincr" json:"id"`
	UID            string    `xorm:"varchar(64) notnull index" json:"uid"`
	Type           string    `xorm:"varchar(20) notnull" json:"type"`
	ConnectAddress string    `xorm:"varchar(255) notnull" json:"connect_address"`
	Socks5port     string    `xorm:"varchar(10) notnull" json:"socks5port"`
	UserName       string    `xorm:"varchar(100)" json:"username"`
	Password       string    `xorm:"varchar(100)" json:"password"`
	Note           string    `xorm:"varchar(500)" json:"note"`
	Status         int       `xorm:"int default(0) index" json:"status"`
	CreatedAt      time.Time `xorm:"datetime notnull" json:"created_at"`
	UpdatedAt      time.Time `xorm:"updated datetime notnull" json:"updated_at"`
}

// TunnelProxy 隧道代理数据库模型
type TunnelProxy struct {
	ID            int64     `json:"id" xorm:"'id' pk autoincr"`
	ClientUID     string    `json:"client_uid" xorm:"'client_uid' varchar(64) notnull index"`
	Protocol      string    `json:"protocol" xorm:"'protocol' varchar(20) notnull index"`
	ServerPort    int       `json:"server_port" xorm:"'server_port' int notnull unique"`
	TargetAddress string    `json:"target_address" xorm:"'target_address' varchar(255)"` // TCP/UDP代理的目标地址(IP:端口)
	Note          string    `json:"note" xorm:"'note' varchar(500)"`
	Username      string    `json:"username" xorm:"'username' varchar(100)"`
	Password      string    `json:"password" xorm:"'password' varchar(100)"`
	Status        int       `json:"status" xorm:"'status' int default(0) index"`
	CreatedAt     time.Time `json:"created_at" xorm:"'created_at' datetime notnull index"`
	UpdatedAt     time.Time `json:"updated_at" xorm:"'updated_at' updated datetime notnull"`
}

// AuditLogDB 审计日志数据库模型
type AuditLogDB struct {
	ID         int64     `json:"id" xorm:"'id' pk autoincr"`
	LogID      string    `json:"log_id" xorm:"varchar(64) notnull unique index"`
	Timestamp  time.Time `json:"timestamp" xorm:"datetime notnull index"`
	Level      string    `json:"level" xorm:"varchar(20) notnull index"`
	Category   string    `json:"category" xorm:"varchar(20) notnull index"`
	Action     string    `json:"action" xorm:"varchar(100) notnull index"`
	UserID     string    `json:"user_id" xorm:"varchar(64) index"`
	Username   string    `json:"username" xorm:"varchar(100) index"`
	ClientIP   string    `json:"client_ip" xorm:"varchar(45) index"`
	UserAgent  string    `json:"user_agent" xorm:"text"`
	RequestURI string    `json:"request_uri" xorm:"varchar(500)"`
	Method     string    `json:"method" xorm:"varchar(10)"`
	StatusCode int       `json:"status_code" xorm:"int"`
	Duration   int64     `json:"duration" xorm:"bigint"` // 毫秒
	Result     string    `json:"result" xorm:"varchar(20) notnull index"`
	Message    string    `json:"message" xorm:"text"`
	ErrorMsg   string    `json:"error_msg" xorm:"text"`
	Details    string    `json:"details" xorm:"text"` // JSON字符串
	CreatedAt  time.Time `json:"created_at" xorm:"datetime notnull index"`
}

// Plugin 插件元数据（对齐 tshell 的 Plugin 表，P1+P5）
type Plugin struct {
	Id         int64     `json:"id" xorm:"pk autoincr"`
	Name       string    `json:"name" xorm:"varchar(255) notnull unique index"` // 显示名
	Os         string    `json:"os" xorm:"varchar(20)"`                         // windows / linux
	Type       string    `json:"type" xorm:"varchar(50)"`                       // execute-assembly / inline-bin / shellcode-inject / script / binary
	FileName   string    `json:"file_name" xorm:"varchar(255) notnull"`         // 存储文件名
	FileHash   string    `json:"file_hash" xorm:"varchar(64)"`                  // SHA256 完整性校验
	FileSize   int64     `json:"file_size" xorm:"bigint"`
	UploadTime time.Time `json:"upload_time" xorm:"datetime notnull"`
}

// Loot 凭据/制品库(功能批): 行动中获取的凭据与制品集中管理。
// 来源: 手动录入 / 命令产出自动识别(WiFi 密码等) / 后续提取命令族(LSASS/DPAPI/浏览器)。
type Loot struct {
	Id        int64  `json:"id" xorm:"pk autoincr"`
	Uid       string `json:"uid" xorm:"index"`         // 来源会话
	Type      string `json:"type" xorm:"varchar(20) index"` // pwd 明文 / hash NTLM哈希 / domain 域账号 / note 备忘
	User      string `json:"user" xorm:"varchar(255)"`      // 用户名
	Realm     string `json:"realm" xorm:"varchar(255)"`     // 域/REALM
	Host      string `json:"host" xorm:"varchar(255)"`      // 主机名
	Secret    string `json:"secret" xorm:"text"`            // 密码/哈希/密钥
	Note      string `json:"note" xorm:"text"`              // 备注
	Source    string `json:"source" xorm:"varchar(50)"`     // 来源: manual/wifi/keylog/lsass/...
	CreatedAt string `json:"created_at" xorm:"varchar(32) index"`
}

// MigrateDatabase 数据库迁移函数，为现有表添加新字段
func MigrateDatabase() error {
	if Engine == nil {
		return fmt.Errorf("数据库引擎未初始化")
	}

	// 迁移 Clients 表
	err := migrateClientsTable()
	if err != nil {
		return fmt.Errorf("Clients表迁移失败: %v", err)
	}

	// 迁移 Listener 表
	err = migrateListenerTable()
	if err != nil {
		return fmt.Errorf("Listener表迁移失败: %v", err)
	}

	// 索引由 xorm Sync2 依据结构 tag 统一管理（Clients.Online / Notes.Uid 已加 index tag）

	return nil
}

// migrateClientsTable 迁移 Clients 表
func migrateClientsTable() error {

	// 检查Clients表是否已经有Listener字段和心跳相关字段
	var err error
	tableName := "clients"
	listenerColumnName := "listener"
	heartbeatTimeoutColumnName := "heartbeat_timeout"
	heartbeatIntervalColumnName := "heartbeat_interval"
	lastActiveColumnName := "last_active"
	idColumnName := "id"

	// 查询表结构信息
	rows, err := Engine.QueryString("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return fmt.Errorf("查询表结构失败: %v", err)
	}

	// 检查是否已经存在所有字段
	hasListenerColumn := false
	hasHeartbeatTimeoutColumn := false
	hasHeartbeatIntervalColumn := false
	hasLastActiveColumn := false
	hasIdColumn := false
	hasOsColumn := false
	for _, row := range rows {
		if row["name"] == "os" {
			hasOsColumn = true
		}
		if row["name"] == listenerColumnName {
			hasListenerColumn = true
		}
		if row["name"] == heartbeatTimeoutColumnName {
			hasHeartbeatTimeoutColumn = true
		}
		if row["name"] == heartbeatIntervalColumnName {
			hasHeartbeatIntervalColumn = true
		}
		if row["name"] == lastActiveColumnName {
			hasLastActiveColumn = true
		}
		if row["name"] == idColumnName {
			hasIdColumn = true
		}
	}

	// 如果不存在os字段，则添加
	if !hasOsColumn {
		log.Printf("正在为Clients表添加Os字段...")
		err = ExecuteSQL(Engine, "ALTER TABLE "+tableName+" ADD COLUMN os TEXT DEFAULT ''")
		if err != nil {
			return fmt.Errorf("添加Os字段失败: %v", err)
		}
		log.Printf("✅ Clients表Os字段添加成功")
	} else {
		log.Printf("✅ Clients表已经存在Os字段，跳过迁移")
	}

	// 如果不存在listener字段，则添加
	if !hasListenerColumn {
		log.Printf("正在为Clients表添加Listener字段...")
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT DEFAULT ''", tableName, listenerColumnName)
		err = ExecuteSQL(Engine, alterSQL)
		if err != nil {
			return fmt.Errorf("添加Listener字段失败: %v", err)
		}
		log.Printf("✅ Clients表Listener字段添加成功")
	} else {
		log.Printf("✅ Clients表已经存在Listener字段，跳过迁移")
	}

	// 如果不存在heartbeat_timeout字段，则添加
	if !hasHeartbeatTimeoutColumn {
		log.Printf("正在为Clients表添加HeartbeatTimeout字段...")
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT DEFAULT '30'", tableName, heartbeatTimeoutColumnName)
		err = ExecuteSQL(Engine, alterSQL)
		if err != nil {
			return fmt.Errorf("添加HeartbeatTimeout字段失败: %v", err)
		}
		log.Printf("✅ Clients表HeartbeatTimeout字段添加成功")
	} else {
		log.Printf("✅ Clients表已经存在HeartbeatTimeout字段，跳过迁移")
	}

	// 如果不存在heartbeat_interval字段，则添加
	if !hasHeartbeatIntervalColumn {
		log.Printf("正在为Clients表添加HeartbeatInterval字段...")
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT DEFAULT '10'", tableName, heartbeatIntervalColumnName)
		err = ExecuteSQL(Engine, alterSQL)
		if err != nil {
			return fmt.Errorf("添加HeartbeatInterval字段失败: %v", err)
		}
		log.Printf("✅ Clients表HeartbeatInterval字段添加成功")
	} else {
		log.Printf("✅ Clients表已经存在HeartbeatInterval字段，跳过迁移")
	}

	// 如果不存在last_active字段，则添加
	if !hasLastActiveColumn {
		log.Printf("正在为Clients表添加LastActive字段...")
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT DEFAULT ''", tableName, lastActiveColumnName)
		err = ExecuteSQL(Engine, alterSQL)
		if err != nil {
			return fmt.Errorf("添加LastActive字段失败: %v", err)
		}
		log.Printf("✅ Clients表LastActive字段添加成功")
	} else {
		log.Printf("✅ Clients表已经存在LastActive字段，跳过迁移")
	}

	// 如果不存在id字段，则添加（需要重建表）
	if !hasIdColumn {
		log.Printf("正在为Clients表添加ID字段...")
		// SQLite需要重建表来添加主键
		err = migrateClientsTableWithId()
		if err != nil {
			return fmt.Errorf("添加ID字段失败: %v", err)
		}
		log.Printf("✅ Clients表ID字段添加成功")
	} else {
		log.Printf("✅ Clients表已经存在ID字段，跳过迁移")
	}

	return nil
}

// migrateListenerTable 迁移Listener表，添加心跳配置字段和新增字段
func migrateListenerTable() error {
	tableName := "listener"
	heartbeatTimeoutColumnName := "heartbeat_timeout"
	heartbeatIntervalColumnName := "heartbeat_interval"
	noteColumnName := "note"
	vkeyColumnName := "vkey"
	encryptionSaltColumnName := "encryption_salt"

	// 查询表结构信息
	rows, err := Engine.QueryString("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return fmt.Errorf("查询Listener表结构失败: %v", err)
	}

	// 检查是否已经存在所有字段
	hasHeartbeatTimeoutColumn := false
	hasHeartbeatIntervalColumn := false
	hasNoteColumn := false
	hasVKeyColumn := false
	hasEncryptionSaltColumn := false
	for _, row := range rows {
		if row["name"] == heartbeatTimeoutColumnName {
			hasHeartbeatTimeoutColumn = true
		}
		if row["name"] == heartbeatIntervalColumnName {
			hasHeartbeatIntervalColumn = true
		}
		if row["name"] == noteColumnName {
			hasNoteColumn = true
		}
		if row["name"] == vkeyColumnName {
			hasVKeyColumn = true
		}
		if row["name"] == encryptionSaltColumnName {
			hasEncryptionSaltColumn = true
		}
	}

	// 如果不存在heartbeat_timeout字段，则添加
	if !hasHeartbeatTimeoutColumn {
		log.Printf("正在为Listener表添加HeartbeatTimeout字段...")
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT DEFAULT '30'", tableName, heartbeatTimeoutColumnName)
		err = ExecuteSQL(Engine, alterSQL)
		if err != nil {
			return fmt.Errorf("添加Listener表HeartbeatTimeout字段失败: %v", err)
		}
		log.Printf("✅ Listener表HeartbeatTimeout字段添加成功")
	} else {
		log.Printf("✅ Listener表已经存在HeartbeatTimeout字段，跳过迁移")
	}

	// 如果不存在heartbeat_interval字段，则添加
	if !hasHeartbeatIntervalColumn {
		log.Printf("正在为Listener表添加HeartbeatInterval字段...")
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT DEFAULT '10'", tableName, heartbeatIntervalColumnName)
		err = ExecuteSQL(Engine, alterSQL)
		if err != nil {
			return fmt.Errorf("添加Listener表HeartbeatInterval字段失败: %v", err)
		}
		log.Printf("✅ Listener表HeartbeatInterval字段添加成功")
	} else {
		log.Printf("✅ Listener表已经存在HeartbeatInterval字段，跳过迁移")
	}

	// 如果不存在note字段，则添加
	if !hasNoteColumn {
		log.Printf("正在为Listener表添加Note字段...")
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT DEFAULT ''", tableName, noteColumnName)
		err = ExecuteSQL(Engine, alterSQL)
		if err != nil {
			return fmt.Errorf("添加Listener表Note字段失败: %v", err)
		}
		log.Printf("✅ Listener表Note字段添加成功")
	} else {
		log.Printf("✅ Listener表已经存在Note字段，跳过迁移")
	}

	// 如果不存在vkey字段，则添加
	if !hasVKeyColumn {
		log.Printf("正在为Listener表添加VKey字段...")
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT DEFAULT ''", tableName, vkeyColumnName)
		err = ExecuteSQL(Engine, alterSQL)
		if err != nil {
			return fmt.Errorf("添加Listener表VKey字段失败: %v", err)
		}
		log.Printf("✅ Listener表VKey字段添加成功")
	} else {
		log.Printf("✅ Listener表已经存在VKey字段，跳过迁移")
	}

	// 如果不存在encryption_salt字段，则添加
	if !hasEncryptionSaltColumn {
		log.Printf("正在为Listener表添加EncryptionSalt字段...")
		alterSQL := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT DEFAULT ''", tableName, encryptionSaltColumnName)
		err = ExecuteSQL(Engine, alterSQL)
		if err != nil {
			return fmt.Errorf("添加Listener表EncryptionSalt字段失败: %v", err)
		}
		log.Printf("✅ Listener表EncryptionSalt字段添加成功")
	} else {
		log.Printf("✅ Listener表已经存在EncryptionSalt字段，跳过迁移")
	}

	return nil
}

// migrateClientsTableWithId 重建Clients表以添加ID主键
func migrateClientsTableWithId() error {
	// 1. 创建新表
	createNewTableSQL := `
		CREATE TABLE IF NOT EXISTS clients_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uid TEXT,
			first_start TEXT,
			external_ip TEXT,
			internal_ip TEXT,
			username TEXT,
			computer TEXT,
			process TEXT,
			pid TEXT,
			address TEXT,
			arch TEXT,
			note TEXT,
			sleep TEXT,
			online TEXT,
			color TEXT,
			listener TEXT,
			heartbeat_timeout TEXT DEFAULT '30',
			heartbeat_interval TEXT DEFAULT '10'
		)`

	err := ExecuteSQL(Engine, createNewTableSQL)
	if err != nil {
		return fmt.Errorf("创建新表失败: %v", err)
	}

	// 2. 查询原表结构
	rows, err := Engine.QueryString("PRAGMA table_info(clients)")
	if err != nil {
		return fmt.Errorf("查询原表结构失败: %v", err)
	}

	// 收集存在的字段名
	existingColumns := make(map[string]bool)
	for _, row := range rows {
		columnName := row["name"]
		existingColumns[columnName] = true
	}

	// 构建动态的复制SQL
	selectFields := []string{}
	insertFields := []string{}

	// 定义所有可能的字段映射
	fieldMappings := map[string]string{
		"uid":                "uid",
		"first_start":        "first_start",
		"external_ip":        "external_ip",
		"internal_ip":        "internal_ip",
		"username":           "username",
		"computer":           "computer",
		"process":            "process",
		"pid":                "pid",
		"address":            "address",
		"arch":               "arch",
		"note":               "note",
		"sleep":              "sleep",
		"online":             "online",
		"color":              "color",
		"listener":           "listener",
		"heartbeat_timeout":  "heartbeat_timeout",
		"heartbeat_interval": "heartbeat_interval",
	}

	for newField, oldField := range fieldMappings {
		insertFields = append(insertFields, newField)
		if existingColumns[oldField] {
			if oldField == "listener" {
				selectFields = append(selectFields, "COALESCE(listener, '') as listener")
			} else if oldField == "heartbeat_timeout" {
				selectFields = append(selectFields, "COALESCE(heartbeat_timeout, '30') as heartbeat_timeout")
			} else if oldField == "heartbeat_interval" {
				selectFields = append(selectFields, "COALESCE(heartbeat_interval, '10') as heartbeat_interval")
			} else {
				selectFields = append(selectFields, oldField)
			}
		} else {
			// 字段不存在，使用默认值
			switch oldField {
			case "listener":
				selectFields = append(selectFields, "'' as listener")
			case "heartbeat_timeout":
				selectFields = append(selectFields, "'30' as heartbeat_timeout")
			case "heartbeat_interval":
				selectFields = append(selectFields, "'10' as heartbeat_interval")
			default:
				selectFields = append(selectFields, "'' as "+oldField)
			}
		}
	}

	// 构建复制数据SQL
	copyDataSQL := fmt.Sprintf(`
		INSERT INTO clients_new (%s)
		SELECT %s
		FROM clients
		ORDER BY datetime(COALESCE(first_start, '1970-01-01 00:00:00')) ASC`,
		strings.Join(insertFields, ", "),
		strings.Join(selectFields, ", "))

	err = ExecuteSQL(Engine, copyDataSQL)
	if err != nil {
		return fmt.Errorf("复制数据失败: %v", err)
	}

	// 3. 删除旧表
	err = ExecuteSQL(Engine, "DROP TABLE clients")
	if err != nil {
		return fmt.Errorf("删除旧表失败: %v", err)
	}

	// 4. 重命名新表
	err = ExecuteSQL(Engine, "ALTER TABLE clients_new RENAME TO clients")
	if err != nil {
		return fmt.Errorf("重命名表失败: %v", err)
	}

	return nil
}

// GetListenerHeartbeatConfig 根据监听器地址获取心跳配置
func GetListenerHeartbeatConfig(listenAddress string) (heartbeatTimeout string, heartbeatInterval string) {
	var listener Listener
	has, err := Engine.Where("listen_address = ?", listenAddress).Get(&listener)
	if err != nil || !has {
		// 如果查找失败或未找到，使用原始默认值
		return "30", "10"
	}

	heartbeatTimeout = listener.HeartbeatTimeout
	heartbeatInterval = listener.HeartbeatInterval

	// 确保返回有效值
	if heartbeatTimeout == "" {
		heartbeatTimeout = "30"
	}
	if heartbeatInterval == "" {
		heartbeatInterval = "10"
	}

	return heartbeatTimeout, heartbeatInterval
}

// GetListenerHeartbeatConfigByAddr 按地址定位监听器后取心跳配置，供多实例场景使用。
// 同类型存在多个监听器时，GetListenerHeartbeatConfigByType 只能取到任意一行，
// 入站处理与客户端生成都可能用到另一个监听器的配置。
// 依次尝试：connect_address 精确 → listen_address 精确 → 同类型下按端口匹配
// （监听地址常绑通配地址，而连接的 LocalAddr 是具体 IP，只能按端口对）。
// 定位不到或 addr 为空时回退按类型，保持既有行为。
func GetListenerHeartbeatConfigByAddr(addr, listenerType string) (heartbeatTimeout string, heartbeatInterval string) {
	if addr == "" || Engine == nil {
		return GetListenerHeartbeatConfigByType(listenerType)
	}
	var listener Listener
	has, err := Engine.Where("connect_address = ? OR listen_address = ?", addr, addr).Get(&listener)
	if err == nil && !has {
		// 端口兜底：取最后一个 ':' 之后的部分（IPv6 字面量也适用）
		if i := strings.LastIndex(addr, ":"); i >= 0 && i < len(addr)-1 {
			port := addr[i+1:]
			has, _ = Engine.Where("type = ? AND listen_address LIKE ?", listenerType, "%:"+port).Get(&listener)
		}
	}
	if err != nil || !has {
		return GetListenerHeartbeatConfigByType(listenerType)
	}
	heartbeatTimeout = listener.HeartbeatTimeout
	heartbeatInterval = listener.HeartbeatInterval
	if heartbeatTimeout == "" {
		heartbeatTimeout = "30"
	}
	if heartbeatInterval == "" {
		heartbeatInterval = defaultHeartbeatInterval()
	}
	return heartbeatTimeout, heartbeatInterval
}

// GetListenerHeartbeatConfigByType 根据监听器类型获取活跃监听器的心跳配置
func GetListenerHeartbeatConfigByType(listenerType string) (heartbeatTimeout string, heartbeatInterval string) {
	var listener Listener
	has, err := Engine.Where("type = ? AND status = 1", listenerType).Get(&listener)
	if err != nil || !has {
		// 如果查找失败或未找到：回退配置文件默认值（client.sleep）
		return "30", defaultHeartbeatInterval()
	}

	heartbeatTimeout = listener.HeartbeatTimeout
	heartbeatInterval = listener.HeartbeatInterval

	// 确保返回有效值
	if heartbeatTimeout == "" {
		heartbeatTimeout = "30"
	}
	if heartbeatInterval == "" {
		heartbeatInterval = defaultHeartbeatInterval()
	}

	return heartbeatTimeout, heartbeatInterval
}

// defaultHeartbeatInterval 返回默认心跳间隔（配置文件 client.sleep，由 main 注入；未设置时 5s）
func defaultHeartbeatInterval() string {
	if DefaultClientSleep != "" && DefaultClientSleep != "0" {
		if v, err := strconv.Atoi(DefaultClientSleep); err == nil && v > 0 {
			return strconv.Itoa(v)
		}
	}
	return "5"
}

// AppendShellContent 增量追加 shell 内容（P1-5: 异步批量落库，见 shell_batch_writer.go）。
// 原同步实现每次输出都执行一条 UPDATE，高频会话 + 多客户端时 SQLite 写锁竞争严重。


// ensureWebDeliverySchema 迁移旧 web_delivery 表（无 id 主键）到新结构。
// SQLite 无法 ALTER TABLE 添加 PRIMARY KEY，必须重建表。
func ensureWebDeliverySchema(engine *xorm.Engine) error {
	if engine == nil {
		return fmt.Errorf("engine is nil")
	}

	// 表不存在则交给后续 Sync2 创建
	has, err := engine.IsTableExist(new(WebDelivery))
	if err != nil {
		return err
	}
	if !has {
		return nil
	}

	// 检查是否已有 id 列
	type colInfo struct {
		Cid        int    `xorm:"cid"`
		Name       string `xorm:"name"`
		Type       string `xorm:"type"`
		Notnull    int    `xorm:"notnull"`
		DfltValue  string `xorm:"dflt_value"`
		Pk         int    `xorm:"pk"`
	}
	var cols []colInfo
	if err := engine.SQL("PRAGMA table_info(`web_delivery`)").Find(&cols); err != nil {
		return fmt.Errorf("PRAGMA table_info web_delivery: %v", err)
	}
	hasID := false
	hasIDPk := false
	for _, c := range cols {
		name := strings.ToLower(c.Name)
		if name == "id" {
			hasID = true
			if c.Pk > 0 {
				hasIDPk = true
			}
		}
		// 旧错误列名 i_d（若曾部分迁移失败）
		if name == "i_d" {
			// 强制重建
			hasID = false
			hasIDPk = false
			break
		}
	}
	if hasID && hasIDPk {
		return nil // 已是新结构
	}

	log.Printf("🔄 检测到旧版 web_delivery 表（无 id 主键），开始迁移...")

	// 读取旧数据为通用 map
	results, err := engine.QueryString("SELECT * FROM `web_delivery`")
	if err != nil {
		return fmt.Errorf("读取旧 web_delivery 失败: %v", err)
	}

	// 备份并删除旧表
	_, _ = engine.Exec("DROP TABLE IF EXISTS `web_delivery_backup_pre_id`")
	if _, err := engine.Exec("ALTER TABLE `web_delivery` RENAME TO `web_delivery_backup_pre_id`"); err != nil {
		return fmt.Errorf("重命名旧 web_delivery 失败: %v", err)
	}

	// 用新结构创建空表
	if err := engine.Sync2(new(WebDelivery)); err != nil {
		// 尝试回滚表名
		_, _ = engine.Exec("ALTER TABLE `web_delivery_backup_pre_id` RENAME TO `web_delivery`")
		return fmt.Errorf("创建新 web_delivery 失败: %v", err)
	}

	// 回填数据
	copied := 0
	for _, row := range results {
		get := func(keys ...string) string {
			for _, k := range keys {
				if v, ok := row[k]; ok && v != "" {
					return v
				}
				// sqlite 可能返回不同大小写
				for rk, rv := range row {
					if strings.EqualFold(rk, k) && rv != "" {
						return rv
					}
				}
			}
			return ""
		}
		status := 2
		if s := get("status", "Status"); s != "" {
			if s == "1" {
				status = 1
			}
		}
		maxDL := 0
		if s := get("max_downloads", "MaxDownloads"); s != "" {
			fmt.Sscanf(s, "%d", &maxDL)
		}
		oneTime := 0
		if s := get("one_time", "OneTime"); s == "1" {
			oneTime = 1
		}
		var hit int64
		if s := get("hit_count", "HitCount"); s != "" {
			fmt.Sscanf(s, "%d", &hit)
		}

		wd := &WebDelivery{
			ListenerConfig:   get("listener_config", "ListenerConfig"),
			OS:               get("os", "OS"),
			Arch:             get("arch", "Arch"),
			ListeningPort:    get("listening_port", "ListeningPort"),
			Status:           status,
			ServerAddress:    get("server_address", "ServerAddress"),
			FileName:         get("file_name", "FileName"),
			Pass:             get("pass", "Pass"),
			DownloadPath:     get("download_path", "DownloadPath"),
			SaveFilename:     get("save_filename", "SaveFilename"),
			BindMode:         get("bind_mode", "BindMode"),
			PublicKey:        get("public_key", "PublicKey"),
			PublicHost:       get("public_host", "PublicHost"),
			PublicScheme:     get("public_scheme", "PublicScheme"),
			BasePath:         get("base_path", "BasePath"),
			URIPath:          get("uri_path", "URIPath"),
			ContentType:      get("content_type", "ContentType"),
			StageContentType: get("stage_content_type", "StageContentType"),
			AccessToken:      get("access_token", "AccessToken"),
			UAAllow:          get("ua_allow", "UAAllow"),
			MaxDownloads:     maxDL,
			HitCount:         hit,
			LastHitAt:        get("last_hit_at", "LastHitAt"),
			LastHitIP:        get("last_hit_ip", "LastHitIP"),
			OneTime:          oneTime,
			StagerURL:        get("stager_url", "StagerURL"),
			ShellcodeURL:     get("shellcode_url", "ShellcodeURL"),
			Note:             get("note", "Note"),
		}
		if wd.BindMode == "" {
			// 旧数据：有真实端口则 dedicated，否则 shared
			if wd.ListeningPort != "" && wd.ListeningPort != "0" && !strings.HasPrefix(wd.ListeningPort, "shared") {
				wd.BindMode = "dedicated"
			} else {
				wd.BindMode = "shared"
			}
		}
		if _, err := engine.Insert(wd); err != nil {
			log.Printf("⚠️ 迁移 web_delivery 行失败 port=%s: %v", wd.ListeningPort, err)
			continue
		}
		copied++
	}

	// 可选：保留备份表 web_delivery_backup_pre_id 以便回滚；成功后可删除
	_, _ = engine.Exec("DROP TABLE IF EXISTS `web_delivery_backup_pre_id`")
	log.Printf("✅ web_delivery 表迁移完成，恢复 %d 条记录", copied)
	return nil
}

// renameColumnIfExists 若表中存在旧列且不存在新列，则重命名（SQLite ≥3.25）。
// 用于修正 xorm 驼峰拆分产生的错误列名。
func renameColumnIfExists(engine *xorm.Engine, table, oldName, newName, label string) error {
	rows, err := engine.QueryString("PRAGMA table_info(`" + table + "`)")
	if err != nil {
		return fmt.Errorf("查询 %s 表结构失败: %v", table, err)
	}
	hasOld, hasNew := false, false
	for _, row := range rows {
		switch row["name"] {
		case oldName:
			hasOld = true
		case newName:
			hasNew = true
		}
	}
	if hasNew || !hasOld {
		return nil
	}
	// SQLite 重命名列会同步更新其索引定义
	if _, err := engine.Exec(fmt.Sprintf("ALTER TABLE `%s` RENAME COLUMN `%s` TO `%s`", table, oldName, newName)); err != nil {
		return fmt.Errorf("重命名 %s.%s -> %s 失败: %v", table, oldName, newName, err)
	}
	log.Printf("✅ %s 列 %s → %s 重命名成功", label, oldName, newName)
	return nil
}

// ensureColumnNameMigrations 修正早期 xorm 驼峰拆分产生的错误列名
// （ID → i_d / client_uid → client_u_i_d / delivery_id → delivery_i_d）。
// 必须在 Sync2 之前执行：否则 Sync2 依新 tag 尝试 ADD COLUMN 主键会失败。
func ensureColumnNameMigrations(engine *xorm.Engine) error {
	// tunnel_proxy: i_d → id, client_u_i_d → client_uid
	if err := renameColumnIfExists(engine, "tunnel_proxy", "i_d", "id", "TunnelProxy"); err != nil {
		return err
	}
	if err := renameColumnIfExists(engine, "tunnel_proxy", "client_u_i_d", "client_uid", "TunnelProxy"); err != nil {
		return err
	}
	// delivery_access_log: i_d → id, delivery_i_d → delivery_id
	if err := renameColumnIfExists(engine, "delivery_access_log", "i_d", "id", "DeliveryAccessLog"); err != nil {
		return err
	}
	if err := renameColumnIfExists(engine, "delivery_access_log", "delivery_i_d", "delivery_id", "DeliveryAccessLog"); err != nil {
		return err
	}
	// audit_log: 无 tag 的 ID 字段被 xorm 命名为 i_d
	if err := renameColumnIfExists(engine, "audit_log", "i_d", "id", "AuditLogDB"); err != nil {
		return err
	}
	return nil
}

// engineMu 保护全局 Engine 指针的读写。
// 后台 flusher goroutine 会长期运行并读取 Engine，而 ConnectDateBase 在启动/测试里
// 会替换该指针——不加锁时 -race 会报数据竞争（本轮 CI 加 -race 后实测 13 处）。
var engineMu sync.RWMutex

// ConfigurePool 应用配置里的连接池参数（config.database.max_open_conns 等此前是死配置：
// ConnectDateBase 里硬编码 10/5/5m）。传 0/负值表示沿用默认。
func ConfigurePool(maxOpen, maxIdle int, lifetime time.Duration) {
	engineMu.RLock()
	e := Engine
	engineMu.RUnlock()
	if e == nil {
		return
	}
	if maxOpen > 0 {
		e.SetMaxOpenConns(maxOpen)
	}
	if maxIdle > 0 {
		e.SetMaxIdleConns(maxIdle)
	}
	if lifetime > 0 {
		e.SetConnMaxLifetime(lifetime)
	}
}

// currentEngine 后台 goroutine 安全取当前引擎（未初始化时返回 nil）。
func currentEngine() *xorm.Engine {
	engineMu.RLock()
	defer engineMu.RUnlock()
	return Engine
}

func ConnectDateBase() {
	var err error
	dsn := DatabaseName
	if !strings.Contains(dsn, "?") {
		dsn += "?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL"
	}
	engineMu.Lock()
	Engine, err = xorm.NewEngine("sqlite3", dsn)
	engineMu.Unlock()
	if err != nil {
		log.Fatalf("连接sqlite3数据库失败: %v", err)
	}

	// 优化 SQLite 连接池配置，提升并发吞吐
	// P0-1B: 单写者模型收敛并发写（WAL 下写仍串行），10 即可饱和，过大仅增加锁竞争
	Engine.SetMaxOpenConns(10)
	Engine.SetMaxIdleConns(5)
	Engine.SetConnMaxLifetime(5 * time.Minute)
_, _ = Engine.Exec("PRAGMA foreign_keys=ON")  
_, _ = Engine.Exec("PRAGMA temp_store=MEMORY")  
_, _ = Engine.Exec("PRAGMA cache_size=-64000")  
_, _ = Engine.Exec("PRAGMA mmap_size=268435456")  
	// P0-1B: WAL 调优——限制 WAL 文件上限 64M，autocheckpoint 1000 页
	_, _ = Engine.Exec("PRAGMA journal_size_limit=67108864")
	_, _ = Engine.Exec("PRAGMA wal_autocheckpoint=1000")
	// 后台 5m 增量 checkpoint(TRUNCATE)，失败跳过（仅 idle 时有效）
	go func() {
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for range t.C {
			_, _ = Engine.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
		}
	}()

	// 先迁移旧 web_delivery（无主键），再 Sync2 / 连接池管理器
	if err = ensureWebDeliverySchema(Engine); err != nil {
		log.Fatalf("迁移 web_delivery 表失败: %v", err)
	}

	// 【列名修复】早期 xorm 驼峰拆分产生错误列名（i_d / client_u_i_d / delivery_i_d），
	// 必须在 Sync2 之前重命名——否则 Sync2 依新 tag 尝试 ADD COLUMN 主键会失败
	if err = ensureColumnNameMigrations(Engine); err != nil {
		log.Fatalf("列名迁移失败: %v", err)
	}


	// C35: 不再初始化第二个数据库引擎（DatabaseManager）——
	// 原实现用第二个连接池操作同一 SQLite 文件（双池竞争）、可能开启 ShowSQL 刷屏，
	// 且夜间 VACUUM 会锁库阻塞；API 层统一使用全局 Engine。

	// 【关键修复】原手写复合索引 idx_clients_online_listener 引发启动死循环:
	// 某次启动创建该索引 → 下次启动 xorm Sync2 无法将其匹配到结构体声明的索引,
	// 生成删除目标 IDX_clients_idx_clients_online_listener(不存在)→
	// "no such index" → log.Fatalf 启动失败,即"第二次重启必炸"。
	// 修复: 在 Sync2 之前自愈清理该索引(必须早于 Sync2——它是崩溃点),
	// 新库不再创建; 复合索引价值有限(online/listener 各有单列索引)。
	_, _ = Engine.Exec("DROP INDEX IF EXISTS idx_clients_online_listener")

	err = Engine.Sync2(new(Users), new(Clients), new(Notes), new(Shell), new(Downloads), new(Listener), new(WebDelivery), new(DeliveryAccessLog), new(Socks5), new(TunnelProxy), new(AuditLogDB), new(Plugin), new(StagerToken), new(Loot))
_, _ = Engine.Exec("CREATE INDEX IF NOT EXISTS idx_clients_uid ON clients(uid)")  
_, _ = Engine.Exec("CREATE INDEX IF NOT EXISTS idx_clients_online ON clients(online)")  
_, _ = Engine.Exec("CREATE INDEX IF NOT EXISTS idx_clients_listener ON clients(listener)")  
_, _ = Engine.Exec("CREATE INDEX IF NOT EXISTS idx_shell_uid ON shell(uid)")  
_, _ = Engine.Exec("CREATE INDEX IF NOT EXISTS idx_downloads_uid ON downloads(uid)")  
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}

	// 执行数据库迁移
	err = MigrateDatabase()
	if err != nil {
		log.Printf("警告：数据库迁移失败: %v", err)
	}
	var user Users
	exists, err := Engine.Where("username = ?", "admin").Get(&user)
	if !exists {
		// 【安全修复】不再硬编码创建 admin/admin123。
		// 默认管理员由 main.go 在 ConnectDateBase 之后调用
		// InitializeDefaultUserFromConfig（使用配置 Auth.DefaultPassword）创建，
		// 密码为空/弱口令时会在那里生成随机密码并打印日志。
		log.Println("未检测到 admin 用户，将由配置初始化流程创建默认管理员")
	} else {
		// C13 RBAC：旧用户补齐默认角色
		if user.Role == "" {
			_, _ = Engine.Where("username = ?", user.Username).Update(&Users{Role: "admin"})
			log.Printf("已为用户 %s 补齐默认角色: admin", user.Username)
		}
		// 检查是否为旧版本明文密码，如果是则进行迁移
		if user.PasswordHash == "" && user.Username == "admin" {
			log.Println("检测到旧版本明文密码，正在迁移到安全哈希存储...")

			// 【安全修复】迁移时不再回填弱口令 admin123，改为生成随机密码
			randomPass := GenerateRandomPassword(16)
			updatedUser, err := CreateSecureUser("admin", randomPass)
			if err != nil {
				log.Fatalf("密码迁移失败: %v", err)
			}

			// 保留原有的ID和时间戳
			updatedUser.CreatedAt = user.CreatedAt
			if user.CreatedAt.IsZero() {
				updatedUser.CreatedAt = time.Now()
			}

			_, err = Engine.Where("username = ?", "admin").Update(updatedUser)
			if err != nil {
				log.Fatalf("更新管理员密码失败: %v", err)
			}

			log.Printf("⚠️ 旧版明文密码已迁移为随机密码：username=admin, password=%s（请立即登录并修改）", randomPass)
			log.Println("密码迁移完成，现在使用安全哈希存储")
		}
	}
}

// InsertData 函数用于插入任意表的数据（D45: 移除每次插入的 Sync2——schema 同步在启动时统一完成）
func InsertData(engine *xorm.Engine, table interface{}) error {
	// 使用反射获取表的信息
	valueOfTable := reflect.ValueOf(table)
	if valueOfTable.Kind() != reflect.Ptr || valueOfTable.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("table parameter must be a pointer to a struct")
	}

	// 插入数据
	_, err := engine.Insert(table)
	if err != nil {
		return fmt.Errorf("failed to insert data: %v", err)
	}

	return nil
}
func ExecuteSQL(engine *xorm.Engine, sql string, args ...interface{}) error {
	_, err := engine.Exec(append([]interface{}{sql}, args...)...)
	if err != nil {
		return fmt.Errorf("failed to execute SQL: %v", err)
	}
	return nil
}
func QuerySQL(engine *xorm.Engine, sql string, args ...interface{}) ([]map[string]string, error) {
	results, err := engine.QueryString(append([]interface{}{sql}, args...)...)
	if err != nil {
		return nil, fmt.Errorf("failed to query SQL: %v", err)
	}
	return results, nil
}

// GetOnlineClients 获取所有在线客户端
func GetOnlineClients() ([]Clients, error) {
	var clients []Clients
	err := Engine.Where("online = ?", "1").Find(&clients)
	if err != nil {
		return nil, fmt.Errorf("failed to get online clients: %v", err)
	}
	return clients, nil
}

// GetClientByUid 根据UID获取客户端信息
func GetClientByUid(uid string) (*Clients, error) {
	var client Clients
	has, err := Engine.Where("uid = ?", uid).Get(&client)
	if err != nil {
		return nil, fmt.Errorf("failed to get client by uid: %v", err)
	}
	if !has {
		return nil, fmt.Errorf("client not found with uid: %s", uid)
	}
	return &client, nil
}
// database_engine_role_migrate 为旧用户补齐默认角色（C13 RBAC 迁移）
func database_engine_role_migrate(engine *xorm.Engine, username string) {
	if engine == nil || username == "" {
		return
	}
	_, err := engine.Where("username = ?", username).Update(&Users{Role: "admin"})
	if err != nil {
		log.Printf("警告：用户 %s 角色迁移失败: %v", username, err)
	}
}
