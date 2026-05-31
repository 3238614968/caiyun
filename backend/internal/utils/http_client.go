package utils

import (
	"net/url"
	"strings"
)

// SanitizeAuthValue 清理认证值，移除非法字符
func SanitizeAuthValue(auth string) string {
	var b strings.Builder
	b.Grow(len(auth))
	for _, c := range auth {
		// 只保留可打印的 ASCII 字符
		if c >= 32 && c < 127 {
			b.WriteRune(c)
		}
	}
	return strings.TrimSpace(b.String())
}

// BuildExchangeURL 构建兑换请求URL
// 参数：
//   - prizeID: 奖品ID
//
// 返回：兑换URL
func BuildExchangeURL(prizeID string) string {
	return "https://m.mcloud.139.com/market/signin/page/exchangeV2?prizeId=" + url.QueryEscape(prizeID) + "&client=app&clientVersion=12.5.3&smsCode="
}

// BuildProductListURL 构建商品列表请求URL
// 返回：商品列表URL
func BuildProductListURL() string {
	return "https://m.mcloud.139.com/market/prize/list?client=app&clientVersion=12.4.0"
}

// IsRetryableError 判断错误是否可重试
// 参数：
//   - message: 错误消息
//
// 返回：是否可重试
func IsRetryableError(message string) bool {
	// 以下错误不需要重试
	noRetryPatterns := []string{
		"奖品单日已耗尽",
		"奖品已兑完",
		"今日已兑换",
		"本月已兑换",
		"云朵不足",
		"账号未登录",
		"未登录",
		"Token 无效",
		"账号被封禁",
		"商品ID不是可兑换 prizeId",
		"商品已下架或不存在",
		"请更新商品列表后重新创建抢兑任务",
	}

	for _, pattern := range noRetryPatterns {
		if strings.Contains(message, pattern) {
			return false
		}
	}

	// 其他错误需要重试
	return true
}
