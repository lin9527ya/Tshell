package errors

import (
	"fmt"
	"reflect"
	"regexp"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// ValidationRule 验证规则接口
type ValidationRule interface {
	Validate(value interface{}) error
}

// RequiredRule 必填验证规则
type RequiredRule struct{}

func (r RequiredRule) Validate(value interface{}) error {
	if value == nil {
		return NewValidationError("字段不能为空")
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.String:
		if v.String() == "" {
			return NewValidationError("字段不能为空")
		}
	case reflect.Slice, reflect.Array, reflect.Map:
		if v.Len() == 0 {
			return NewValidationError("字段不能为空")
		}
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return NewValidationError("字段不能为空")
		}
	}

	return nil
}

// MinLengthRule 最小长度验证规则
type MinLengthRule struct {
	Min int
}

func (r MinLengthRule) Validate(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return NewValidationError("字段必须是字符串类型")
	}

	if utf8.RuneCountInString(str) < r.Min {
		return NewValidationError(fmt.Sprintf("字段长度不能少于%d个字符", r.Min))
	}

	return nil
}

// MaxLengthRule 最大长度验证规则
type MaxLengthRule struct {
	Max int
}

func (r MaxLengthRule) Validate(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return NewValidationError("字段必须是字符串类型")
	}

	if utf8.RuneCountInString(str) > r.Max {
		return NewValidationError(fmt.Sprintf("字段长度不能超过%d个字符", r.Max))
	}

	return nil
}

// RangeRule 数值范围验证规则
type RangeRule struct {
	Min, Max int64
}

func (r RangeRule) Validate(value interface{}) error {
	v := reflect.ValueOf(value)

	var intValue int64
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		intValue = v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		intValue = int64(v.Uint())
	case reflect.Float32, reflect.Float64:
		intValue = int64(v.Float())
	default:
		return NewValidationError("字段必须是数值类型")
	}

	if intValue < r.Min || intValue > r.Max {
		return NewValidationError(fmt.Sprintf("字段值必须在%d到%d之间", r.Min, r.Max))
	}

	return nil
}

// RegexRule 正则表达式验证规则
type RegexRule struct {
	Pattern string
	Message string
}

func (r RegexRule) Validate(value interface{}) error {
	str, ok := value.(string)
	if !ok {
		return NewValidationError("字段必须是字符串类型")
	}

	matched, err := regexp.MatchString(r.Pattern, str)
	if err != nil {
		return NewInternalError("正则表达式验证失败")
	}

	if !matched {
		if r.Message != "" {
			return NewValidationError(r.Message)
		}
		return NewValidationError("字段格式不正确")
	}

	return nil
}

// EmailRule 邮箱验证规则
type EmailRule struct{}

func (r EmailRule) Validate(value interface{}) error {
	emailPattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	regexRule := RegexRule{
		Pattern: emailPattern,
		Message: "邮箱格式不正确",
	}
	return regexRule.Validate(value)
}

// PhoneRule 手机号验证规则
type PhoneRule struct{}

func (r PhoneRule) Validate(value interface{}) error {
	phonePattern := `^1[3-9]\d{9}$`
	regexRule := RegexRule{
		Pattern: phonePattern,
		Message: "手机号格式不正确",
	}
	return regexRule.Validate(value)
}

// IPRule IP地址验证规则
type IPRule struct{}

func (r IPRule) Validate(value interface{}) error {
	ipPattern := `^((25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(25[0-5]|2[0-4]\d|[01]?\d\d?)$`
	regexRule := RegexRule{
		Pattern: ipPattern,
		Message: "IP地址格式不正确",
	}
	return regexRule.Validate(value)
}

// Validator 验证器
type Validator struct {
	rules map[string][]ValidationRule
}

// NewValidator 创建验证器
func NewValidator() *Validator {
	return &Validator{
		rules: make(map[string][]ValidationRule),
	}
}

// AddRule 添加验证规则
func (v *Validator) AddRule(field string, rule ValidationRule) *Validator {
	v.rules[field] = append(v.rules[field], rule)
	return v
}

// Required 添加必填验证
func (v *Validator) Required(field string) *Validator {
	return v.AddRule(field, RequiredRule{})
}

// MinLength 添加最小长度验证
func (v *Validator) MinLength(field string, min int) *Validator {
	return v.AddRule(field, MinLengthRule{Min: min})
}

// MaxLength 添加最大长度验证
func (v *Validator) MaxLength(field string, max int) *Validator {
	return v.AddRule(field, MaxLengthRule{Max: max})
}

// Range 添加数值范围验证
func (v *Validator) Range(field string, min, max int64) *Validator {
	return v.AddRule(field, RangeRule{Min: min, Max: max})
}

// Email 添加邮箱验证
func (v *Validator) Email(field string) *Validator {
	return v.AddRule(field, EmailRule{})
}

// Phone 添加手机号验证
func (v *Validator) Phone(field string) *Validator {
	return v.AddRule(field, PhoneRule{})
}

// IP 添加IP地址验证
func (v *Validator) IP(field string) *Validator {
	return v.AddRule(field, IPRule{})
}

// Regex 添加正则验证
func (v *Validator) Regex(field, pattern, message string) *Validator {
	return v.AddRule(field, RegexRule{Pattern: pattern, Message: message})
}

// Validate 执行验证
func (v *Validator) Validate(data map[string]interface{}) error {
	var errors []string

	for field, rules := range v.rules {
		value := data[field]

		for _, rule := range rules {
			if err := rule.Validate(value); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %s", field, err.Error()))
			}
		}
	}

	if len(errors) > 0 {
		return NewValidationError(strings.Join(errors, "; "))
	}

	return nil
}

// ValidateStruct 验证结构体
func (v *Validator) ValidateStruct(obj interface{}) error {
	data := make(map[string]interface{})

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	typ := val.Type()
	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldValue := val.Field(i)

		// 获取字段名（优先使用json标签）
		fieldName := field.Name
		if jsonTag := field.Tag.Get("json"); jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" && parts[0] != "-" {
				fieldName = parts[0]
			}
		}

		data[fieldName] = fieldValue.Interface()
	}

	return v.Validate(data)
}

// 预定义验证器

// ValidateUsername 验证用户名
func ValidateUsername(username string) error {
	validator := NewValidator()
	validator.Required("username").MinLength("username", 3).MaxLength("username", 20)
	validator.Regex("username", "^[a-zA-Z0-9_]+$", "用户名只能包含字母、数字和下划线")

	return validator.Validate(map[string]interface{}{
		"username": username,
	})
}

// ValidatePassword 验证密码（8-128位，必须包含大小写字母和数字）
// 【修复】原正则 ^(?=.*[a-z])(?=.*[A-Z])(?=.*\d)... 使用 lookahead，
// Go 标准库 regexp(RE2) 不支持 (?=) → 编译失败 → 任何密码都被拒 →
// 修改密码/创建用户/重置密码全部失效。
func ValidatePassword(password string) error {
	validator := NewValidator()
	validator.Required("password").MinLength("password", 8).MaxLength("password", 128)
	// 允许字符集（不含 lookahead）
	validator.Regex("password", "^[a-zA-Z0-9@$!%*?&]+$",
		"密码只能包含字母、数字和 @$!%*?& 特殊字符")
	if err := validator.Validate(map[string]interface{}{"password": password}); err != nil {
		return err
	}

	// 大小写 + 数字三类组合检查（RE2 兼容，逐项判断）
	hasLower := regexp.MustCompile(`[a-z]`)
	hasUpper := regexp.MustCompile(`[A-Z]`)
	hasDigit := regexp.MustCompile(`[0-9]`)
	if !hasLower.MatchString(password) || !hasUpper.MatchString(password) || !hasDigit.MatchString(password) {
		return NewValidationError("密码必须包含大小写字母和数字")
	}
	return nil
}

// ValidateEmail 验证邮箱
func ValidateEmail(email string) error {
	validator := NewValidator()
	validator.Required("email").Email("email")

	return validator.Validate(map[string]interface{}{
		"email": email,
	})
}

// ValidateIP 验证IP地址
func ValidateIP(ip string) error {
	validator := NewValidator()
	validator.Required("ip").IP("ip")

	return validator.Validate(map[string]interface{}{
		"ip": ip,
	})
}

// ValidatePort 验证端口号
func ValidatePort(port int) error {
	validator := NewValidator()
	validator.Range("port", 1, 65535)

	return validator.Validate(map[string]interface{}{
		"port": port,
	})
}

// ValidateFilePath 验证文件路径
func ValidateFilePath(path string) error {
	if path == "" {
		return NewValidationError("文件路径不能为空")
	}

	// 检查危险路径
	dangerousPatterns := []string{
		"..",
		"//",
		"\\\\",
		":",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(path, pattern) {
			return NewValidationError("文件路径包含非法字符")
		}
	}

	return nil
}

// ValidateUID 验证客户端UID
func ValidateUID(uid string) error {
	if uid == "" {
		return NewValidationError("客户端UID不能为空")
	}

	// MD5哈希值应该是32位十六进制字符
	validator := NewValidator()
	validator.Regex("uid", "^[a-f0-9]{32}$", "客户端UID格式不正确")

	return validator.Validate(map[string]interface{}{
		"uid": uid,
	})
}

// ValidateTimeRange 验证时间范围
func ValidateTimeRange(startTime, endTime time.Time) error {
	if startTime.After(endTime) {
		return NewValidationError("开始时间不能晚于结束时间")
	}

	// 检查时间范围是否过大（比如超过1年）
	if endTime.Sub(startTime) > 365*24*time.Hour {
		return NewValidationError("时间范围不能超过1年")
	}

	return nil
}

// SanitizeInput 清理输入数据，防止XSS等攻击
func SanitizeInput(input string) string {
	// 移除HTML标签
	htmlTagPattern := regexp.MustCompile(`<[^>]*>`)
	input = htmlTagPattern.ReplaceAllString(input, "")

	// 移除JavaScript
	jsPattern := regexp.MustCompile(`(?i)javascript:`)
	input = jsPattern.ReplaceAllString(input, "")

	// 移除SQL注入风险字符
	sqlPattern := regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter|exec|execute)`)
	input = sqlPattern.ReplaceAllString(input, "")

	// 转义特殊字符
	input = strings.ReplaceAll(input, "<", "&lt;")
	input = strings.ReplaceAll(input, ">", "&gt;")
	input = strings.ReplaceAll(input, "\"", "&quot;")
	input = strings.ReplaceAll(input, "'", "&#x27;")
	input = strings.ReplaceAll(input, "&", "&amp;")

	return strings.TrimSpace(input)
}

// ValidatePluginName 校验插件文件名：仅允许普通文件名（禁止路径分隔符、..、空字节与绝对路径）
func ValidatePluginName(name string) error {
	if name == "" {
		return NewValidationError("文件名不能为空")
	}
	if len(name) > 255 {
		return NewValidationError("文件名过长")
	}
	if name != filepath.Base(name) {
		return NewValidationError("文件名不能包含路径分隔符")
	}
	if strings.Contains(name, "..") || strings.ContainsRune(name, '\x00') {
		return NewValidationError("文件名包含非法字符")
	}
	// 仅允许常规文件名字符集
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			r == '.' || r == '_' || r == '-' || r == ' ' || r == '(' || r == ')') {
			return NewValidationError("文件名包含非法字符")
		}
	}
	return nil
}
