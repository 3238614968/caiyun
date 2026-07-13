package validator

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// 常见错误
var (
	ErrRequired        = errors.New("此字段为必填项")
	ErrMinLength       = errors.New("长度过短")
	ErrMaxLength       = errors.New("长度过长")
	ErrInvalidEmail    = errors.New("邮箱格式不正确")
	ErrInvalidPhone    = errors.New("手机号格式不正确")
	ErrInvalidPassword = errors.New("密码强度不足")
	ErrInvalidRange    = errors.New("数值超出范围")
)

// Validator 验证器
type Validator struct {
	errors map[string]string
}

// New 创建验证器
func New() *Validator {
	return &Validator{
		errors: make(map[string]string),
	}
}

// ValidateString 验证字符串
func (v *Validator) ValidateString(field, value string, required bool, minLength, maxLength int) error {
	if required && strings.TrimSpace(value) == "" {
		v.errors[field] = ErrRequired.Error()
		return ErrRequired
	}

	length := len(value)
	if minLength > 0 && length < minLength {
		v.errors[field] = fmt.Sprintf("%s，最小长度为%d", ErrMinLength.Error(), minLength)
		return ErrMinLength
	}

	if maxLength > 0 && length > maxLength {
		v.errors[field] = fmt.Sprintf("%s，最大长度为%d", ErrMaxLength.Error(), maxLength)
		return ErrMaxLength
	}

	return nil
}

// ValidateEmail 验证邮箱
func (v *Validator) ValidateEmail(field, email string, required bool) error {
	if required && strings.TrimSpace(email) == "" {
		v.errors[field] = ErrRequired.Error()
		return ErrRequired
	}

	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			v.errors[field] = ErrInvalidEmail.Error()
			return ErrInvalidEmail
		}
	}

	return nil
}

// ValidatePhone 验证手机号（中国大陆）
func (v *Validator) ValidatePhone(field, phone string, required bool) error {
	if required && strings.TrimSpace(phone) == "" {
		v.errors[field] = ErrRequired.Error()
		return ErrRequired
	}

	if phone != "" {
		// 中国大陆手机号：1 开头，第二位 3-9，后面 9 位数字
		matched, _ := regexp.MatchString(`^1[3-9]\d{9}$`, phone)
		if !matched {
			v.errors[field] = ErrInvalidPhone.Error()
			return ErrInvalidPhone
		}
	}

	return nil
}

// ValidatePassword 验证密码强度
func (v *Validator) ValidatePassword(field, password string, required bool, minLength int) error {
	if required && password == "" {
		v.errors[field] = ErrRequired.Error()
		return ErrRequired
	}

	if password != "" {
		if len(password) < minLength {
			v.errors[field] = fmt.Sprintf("%s，最小长度为%d", ErrInvalidPassword.Error(), minLength)
			return ErrInvalidPassword
		}

		// 至少包含字母或数字中的一种
		hasLetter := false
		hasDigit := false
		for _, r := range password {
			if unicode.IsLetter(r) {
				hasLetter = true
			}
			if unicode.IsDigit(r) {
				hasDigit = true
			}
		}

		if !hasLetter && !hasDigit {
			v.errors[field] = "密码必须包含字母或数字"
			return ErrInvalidPassword
		}
	}

	return nil
}

// ValidateInt 验证整数
func (v *Validator) ValidateInt(field string, value, min, max int) error {
	if min > 0 && value < min {
		v.errors[field] = fmt.Sprintf("%s，最小值为%d", ErrInvalidRange.Error(), min)
		return ErrInvalidRange
	}

	if max > 0 && value > max {
		v.errors[field] = fmt.Sprintf("%s，最大值为%d", ErrInvalidRange.Error(), max)
		return ErrInvalidRange
	}

	return nil
}

// ValidateUint 验证无符号整数
func (v *Validator) ValidateUint(field string, value, min, max uint) error {
	if min > 0 && value < min {
		v.errors[field] = fmt.Sprintf("%s，最小值为%d", ErrInvalidRange.Error(), min)
		return ErrInvalidRange
	}

	if max > 0 && value > max {
		v.errors[field] = fmt.Sprintf("%s，最大值为%d", ErrInvalidRange.Error(), max)
		return ErrInvalidRange
	}

	return nil
}

// ValidateBool 验证布尔值
func (v *Validator) ValidateBool(field string, value bool, required bool) error {
	// 布尔值通常不需要特殊验证
	return nil
}

// HasErrors 是否有错误
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// GetErrors 获取所有错误
func (v *Validator) GetErrors() map[string]string {
	return v.errors
}

// GetError 获取单个错误
func (v *Validator) GetError(field string) string {
	return v.errors[field]
}

// AddError 添加自定义错误
func (v *Validator) AddError(field, message string) {
	v.errors[field] = message
}

// Reset 重置验证器
func (v *Validator) Reset() {
	v.errors = make(map[string]string)
}

// ValidateFunc 验证函数类型
type ValidateFunc func() error

// ValidateAll 执行多个验证，返回所有错误
func ValidateAll(validators ...ValidateFunc) []error {
	var errors []error
	for _, fn := range validators {
		if err := fn(); err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// IsEmpty 检查是否为空
func IsEmpty(value interface{}) bool {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	case nil:
		return true
	default:
		return false
	}
}

// SanitizeString 清理字符串（去除首尾空格和不可见字符）
func SanitizeString(s string) string {
	s = strings.TrimSpace(s)
	// 移除不可见字符
	result := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return result
}

// SanitizeHTML 简单的 HTML 转义（防止 XSS）
func SanitizeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// 常用验证辅助函数

// IsValidEmail 验证邮箱格式
func IsValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// IsValidPhone 验证手机号格式
func IsValidPhone(phone string) bool {
	matched, _ := regexp.MatchString(`^1[3-9]\d{9}$`, phone)
	return matched
}

// IsValidURL 验证 URL 格式
func IsValidURL(url string) bool {
	matched, _ := regexp.MatchString(`^(https?|ftp)://[^\s/$.?#].[^\s]*$`, url)
	return matched
}

// IsValidIPv4 验证 IPv4 地址
func IsValidIPv4(ip string) bool {
	matched, _ := regexp.MatchString(`^(\d{1,3}\.){3}\d{1,3}$`, ip)
	if !matched {
		return false
	}
	// 验证每段是否在 0-255 之间
	parts := strings.Split(ip, ".")
	for _, part := range parts {
		if num, err := strconv.Atoi(part); err != nil || num < 0 || num > 255 {
			return false
		}
	}
	return true
}

// IsValidIPv6 验证 IPv6 地址
func IsValidIPv6(ip string) bool {
	matched, _ := regexp.MatchString(`^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$`, ip)
	return matched
}

// MinLength 检查最小长度
func MinLength(s string, min int) bool {
	return len(s) >= min
}

// MaxLength 检查最大长度
func MaxLength(s string, max int) bool {
	return len(s) <= max
}

// InRange 检查是否在范围内
func InRange(value, min, max int) bool {
	return value >= min && value <= max
}

// ContainsOnly 检查是否只包含指定字符
func ContainsOnly(s string, allowed string) bool {
	for _, r := range s {
		if !strings.ContainsRune(allowed, r) {
			return false
		}
	}
	return true
}

// MatchesPattern 检查是否匹配正则表达式
func MatchesPattern(s, pattern string) bool {
	matched, _ := regexp.MatchString(pattern, s)
	return matched
}
