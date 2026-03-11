package utils

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// String 字符串工具

// Substr 截取字符串（支持中文）
func Substr(s string, start, length int) string {
	if start < 0 {
		start = 0
	}
	
	runes := []rune(s)
	if start >= len(runes) {
		return ""
	}
	
	end := start + length
	if end > len(runes) {
		end = len(runes)
	}
	
	return string(runes[start:end])
}

// Truncate 截断字符串，超出部分用...表示
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}
	
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	
	return string(runes[:maxLen-3]) + "..."
}

// Reverse 反转字符串
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// CamelCase 转驼峰命名
func CamelCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	
	words := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	
	if len(words) == 0 {
		return s
	}
	
	var result strings.Builder
	result.WriteString(strings.ToLower(words[0]))
	for _, word := range words[1:] {
		if len(word) > 0 {
			result.WriteString(strings.ToUpper(string(word[0])))
			if len(word) > 1 {
				result.WriteString(strings.ToLower(word[1:]))
			}
		}
	}
	
	return result.String()
}

// SnakeCase 转蛇形命名
func SnakeCase(s string) string {
	var result strings.Builder
	
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteRune('_')
			}
			result.WriteRune(r + 32) // 转小写
		} else {
			result.WriteRune(r)
		}
	}
	
	return result.String()
}

// Number 数字工具

// IntToBytes int 转字节数组
func IntToBytes(n int) []byte {
	return []byte(fmt.Sprintf("%d", n))
}

// BytesToInt 字节数组转 int
func BytesToInt(b []byte) (int, error) {
	var n int
	_, err := fmt.Sscanf(string(b), "%d", &n)
	return n, err
}

// FormatNumber 格式化数字（千分位）
func FormatNumber(n int64) string {
	numStr := fmt.Sprintf("%d", n)
	
	sign := ""
	if numStr[0] == '-' {
		sign = "-"
		numStr = numStr[1:]
	}
	
	var result strings.Builder
	digits := len(numStr)
	
	for i, digit := range numStr {
		if i > 0 && (digits-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(digit)
	}
	
	return sign + result.String()
}

// Random 随机工具

// RandomString 生成随机字符串
func RandomString(length int, charset string) (string, error) {
	if length <= 0 {
		return "", nil
	}
	
	if charset == "" {
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	}
	
	result := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))
	
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	
	return string(result), nil
}

// RandomNumber 生成随机数
func RandomNumber(min, max int64) (int64, error) {
	if min > max {
		min, max = max, min
	}
	
	n, err := rand.Int(rand.Reader, big.NewInt(max-min+1))
	if err != nil {
		return 0, err
	}
	
	return n.Int64() + min, nil
}

// UUID 生成 UUID（简化版）
func UUID() (string, error) {
	uuid := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, uuid)
	if err != nil {
		return "", err
	}
	
	// Set version (4) and variant bits
	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80
	
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

// Crypto 加密工具

// MD5 MD5 哈希
func MD5(data []byte) string {
	hash := md5.Sum(data)
	return hex.EncodeToString(hash[:])
}

// MD5String MD5 哈希（字符串）
func MD5String(s string) string {
	return MD5([]byte(s))
}

// SHA256 SHA256 哈希
func SHA256(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// SHA256String SHA256 哈希（字符串）
func SHA256String(s string) string {
	return SHA256([]byte(s))
}

// Base64Encode Base64 编码
func Base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// Base64Decode Base64 解码
func Base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// File 文件工具

// FileExists 检查文件是否存在
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DirExists 检查目录是否存在
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CreateDirIfNotExists 创建目录（如果不存在）
func CreateDirIfNotExists(path string) error {
	if !DirExists(path) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}

// GetFileSize 获取文件大小
func GetFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// GetFileModTime 获取文件修改时间
func GetFileModTime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// EnsureDir 确保目录存在
func EnsureDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return CreateDirIfNotExists(dir)
}

// CopyFile 复制文件
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	if err := EnsureDir(dst); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// Time 时间工具

// FormatTime 格式化时间
func FormatTime(t time.Time, layout string) string {
	if layout == "" {
		layout = "2006-01-02 15:04:05"
	}
	return t.Format(layout)
}

// ParseTime 解析时间
func ParseTime(s, layout string) (time.Time, error) {
	if layout == "" {
		layout = "2006-01-02 15:04:05"
	}
	return time.Parse(layout, s)
}

// IsToday 判断是否是今天
func IsToday(t time.Time) bool {
	now := time.Now()
	return t.Year() == now.Year() && t.YearDay() == now.YearDay()
}

// IsYesterday 判断是否是昨天
func IsYesterday(t time.Time) bool {
	yesterday := time.Now().AddDate(0, 0, -1)
	return t.Year() == yesterday.Year() && t.YearDay() == yesterday.YearDay()
}

// BeginOfDay 获取当天开始时间
func BeginOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// EndOfDay 获取当天结束时间
func EndOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
}

// BeginOfWeek 获取本周开始时间（周一）
func BeginOfWeek(t time.Time) time.Time {
	offset := int(time.Monday - t.Weekday())
	if offset > 0 {
		offset -= 7
	}
	return time.Date(t.Year(), t.Month(), t.Day()+offset, 0, 0, 0, 0, t.Location())
}

// BeginOfMonth 获取本月开始时间
func BeginOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// JSON JSON 工具

// ToJSON 转 JSON 字符串
func ToJSON(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ToJSONPretty 转格式化的 JSON 字符串
func ToJSONPretty(v interface{}) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FromJSON 从 JSON 字符串解析
func FromJSON(s string, v interface{}) error {
	return json.Unmarshal([]byte(s), v)
}

// MustJSON 转 JSON 字符串（失败则 panic）
func MustJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// Map 地图工具

// MergeMaps 合并多个 map
func MergeMaps(maps ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

// FilterMap 过滤 map
func FilterMap(m map[string]interface{}, filter func(string, interface{}) bool) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		if filter(k, v) {
			result[k] = v
		}
	}
	return result
}

// MapKeys 获取所有键
func MapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// MapValues 获取所有值
func MapValues(m map[string]interface{}) []interface{} {
	values := make([]interface{}, 0, len(m))
	for _, v := range m {
		values = append(values, v)
	}
	return values
}

// Slice 切片工具

// UniqueSlice 去重
func UniqueSlice[T comparable](slice []T) []T {
	result := make([]T, 0)
	seen := make(map[T]bool)
	
	for _, v := range slice {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	
	return result
}

// ContainsSlice 检查是否包含元素
func ContainsSlice[T comparable](slice []T, item T) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}

// IntersectSlice 交集
func IntersectSlice[T comparable](a, b []T) []T {
	result := make([]T, 0)
	set := make(map[T]bool)
	
	for _, v := range a {
		set[v] = true
	}
	
	for _, v := range b {
		if set[v] {
			result = append(result, v)
		}
	}
	
	return result
}

// DiffSlice 差集
func DiffSlice[T comparable](a, b []T) []T {
	result := make([]T, 0)
	setB := make(map[T]bool)
	
	for _, v := range b {
		setB[v] = true
	}
	
	for _, v := range a {
		if !setB[v] {
			result = append(result, v)
		}
	}
	
	return result
}

// ChunkSlice 分组
func ChunkSlice[T any](slice []T, size int) [][]T {
	if size <= 0 {
		return nil
	}
	
	result := make([][]T, 0, (len(slice)+size-1)/size)
	
	for i := 0; i < len(slice); i += size {
		end := i + size
		if end > len(slice) {
			end = len(slice)
		}
		result = append(result, slice[i:end])
	}
	
	return result
}
