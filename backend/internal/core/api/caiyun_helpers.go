package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// marketJSONRequest 发送活动层请求并把响应解析为通用的 {code,msg,result} 信封。
// headers 为 nil 时使用活动层默认鉴权头（jwttoken/jwtToken + Referer）。
func (api *CaiyunAPI) marketJSONRequest(method, endpoint string, payload interface{}, headers map[string]string) (*CaiyunResponse, error) {
	return api.marketJSONRequestContext(context.Background(), method, endpoint, payload, headers)
}

func (api *CaiyunAPI) marketJSONRequestContext(ctx context.Context, method, endpoint string, payload interface{}, headers map[string]string) (*CaiyunResponse, error) {
	body, err := api.marketRawRequestContext(ctx, method, endpoint, payload, headers)
	if err != nil {
		return nil, err
	}
	var result CaiyunResponse
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("解析活动响应失败: %s", summarizeActivityBody(body))
	}
	if result.Code == nil && result.ReturnCode != nil {
		result.Code = result.ReturnCode
		if result.Msg == "" {
			result.Msg = result.ReturnMsg
		}
		if result.Result == nil {
			result.Result = result.Body
		}
	}
	if result.Result == nil && result.Data != nil {
		result.Result = result.Data
	}
	return &result, nil
}

// marketRawRequest 返回活动层接口的原始响应正文，供信封结构不同的平台接口使用。
func (api *CaiyunAPI) marketRawRequest(method, endpoint string, payload interface{}, headers map[string]string) (string, error) {
	return api.marketRawRequestContext(context.Background(), method, endpoint, payload, headers)
}

func (api *CaiyunAPI) marketRawRequestContext(ctx context.Context, method, endpoint string, payload interface{}, headers map[string]string) (string, error) {
	if headers == nil {
		headers = api.buildMarketHeaders(nil, "")
	}
	resp, err := api.client.RequestWithContext(ctx, method, endpoint, headers, payload)
	if err != nil {
		return "", err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("活动接口 HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func summarizeActivityBody(body string) string {
	summary := strings.TrimSpace(body)
	if len(summary) > 200 {
		summary = summary[:200]
	}
	return summary
}

// resultMap 在 result 字段为对象时返回该对象。
func resultMap(resp *CaiyunResponse) (map[string]interface{}, bool) {
	if resp == nil {
		return nil, false
	}
	value, ok := resp.Result.(map[string]interface{})
	return value, ok
}

// resultArray 在 result 字段为数组时返回该数组。
func resultArray(resp *CaiyunResponse) []interface{} {
	if resp == nil {
		return nil
	}
	items, _ := resp.Result.([]interface{})
	return items
}

// resultString 在 result 字段为字符串时返回该字符串。
func resultString(resp *CaiyunResponse) string {
	if resp == nil {
		return ""
	}
	value, _ := resp.Result.(string)
	return strings.TrimSpace(value)
}

// resultStringList 把 result 数组里的元素转成字符串列表（服务端有时返回纯字符串数组）。
func resultStringList(resp *CaiyunResponse) []string {
	items := resultArray(resp)
	if len(items) == 0 {
		if value := resultString(resp); value != "" {
			return []string{value}
		}
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(fmt.Sprint(item))
		if text != "" && text != "<nil>" {
			out = append(out, text)
		}
	}
	return out
}

// mapString 读取 map 中的字符串字段。
func mapString(source map[string]interface{}, key string) string {
	if source == nil {
		return ""
	}
	value, ok := source[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// mapInt 读取 map 中的整数字段，兼容数字/字符串两种形态。
func mapInt(source map[string]interface{}, key string) int {
	if source == nil {
		return 0
	}
	switch value := source[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	case json.Number:
		parsed, _ := value.Int64()
		return int(parsed)
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed); err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

// mapBool 读取 map 中的布尔字段，兼容 bool/数字/字符串。
func mapBool(source map[string]interface{}, key string) bool {
	if source == nil {
		return false
	}
	return boolFromAny(source[key])
}
