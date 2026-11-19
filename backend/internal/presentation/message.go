// Package presentation prepares execution results for people. Execution and
// retry decisions must continue to use the original diagnostic messages.
package presentation

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

var (
	messageSeparator = regexp.MustCompile(`\s*\|\s*`)
	diagnosticTail   = regexp.MustCompile(`(?i)(?:^|[|;；]\s*)(?:body|raw_body|response_body|build|commit|built_at)\s*=`)
	sliderTail       = regexp.MustCompile(`滑块识别\s*offset\s*=`)
	assignment       = regexp.MustCompile(`(?i)(?:^|[\s,，;；])(?:http_status|code|result_code|result|desc|sub_msg|trace_id|offset|confidence|method|peak_margin|solver|solve_ms|session|exchangeCode|wasmParams|gr-risk-p3|operDfp|deviceId|device_id|jwtToken|token|ssotoken|build|commit|built_at)\s*=`)
	eligibilityCode  = regexp.MustCompile(`(?i)exchangeCode\s*=\s*[a-z0-9_-]+\s*[（(]([^）)]+)[）)]`)
	upstreamErrorID  = regexp.MustCompile(`(?i)\s*Error\s*Code\s*[：:]\s*[a-z0-9_-]+`)
	gatewayRejection = regexp.MustCompile(`(?i)(?:\bcode\s*=\s*610\b|Error\s*Code\s*[：:]\s*GK\b)`)
)

// Message cleans both newly generated results and historical records without
// changing stored diagnostics, execution status, counters or retry policy.
func Message(taskType, raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	if index := diagnosticTail.FindStringIndex(text); index != nil {
		text = text[:index[0]]
	}
	exchange := strings.HasPrefix(strings.ToLower(taskType), "exchange")
	rejected := gatewayRejection.MatchString(text)
	visible := make([]string, 0, 3)
	for _, segment := range messageSeparator.Split(text, -1) {
		segment = strings.TrimSpace(segment)
		if index := sliderTail.FindStringIndex(segment); index != nil {
			segment = segment[:index[0]]
		}
		segment = eligibilityCode.ReplaceAllString(segment, "$1")
		if index := assignment.FindStringIndex(segment); index != nil {
			segment = segment[:index[0]]
		}
		segment = strings.Trim(segment, " \t\r\n，,；;|")
		if segment == "" || segment == "资格" {
			continue
		}
		prefix, value, contextOnly := messageSegment(segment)
		if !contextOnly {
			value = businessMessage(value, exchange, rejected)
		}
		value = strings.TrimSpace(value)
		if value != "" {
			visible = append(visible, prefix+value)
		}
	}
	if len(visible) == 0 {
		if exchange && rejected {
			return "活动验证未通过，请稍后重试"
		}
		return "执行结果待确认"
	}
	return strings.Join(visible, " | ")
}

func messageSegment(segment string) (prefix, value string, contextOnly bool) {
	for _, label := range []string{"商品", "兑换账号", "结果", "原因"} {
		for _, colon := range []string{":", "："} {
			if strings.HasPrefix(segment, label+colon) {
				return label + ": ", strings.TrimSpace(strings.TrimPrefix(segment, label+colon)), label == "商品" || label == "兑换账号"
			}
		}
	}
	// Older notification records already contain their account/product context.
	if index := strings.Index(segment, "："); index >= 0 && strings.Contains(segment[:index], " · ") {
		return segment[:index] + "：", segment[index+len("："):], false
	}
	return "", segment, false
}

func businessMessage(value string, exchange, rejected bool) string {
	value = strings.TrimSpace(value)
	if exchange {
		switch {
		case containsAny(value, "云朵数不足", "云朵数量不足", "云朵不足", "云朵不够", "余额不足"):
			return "云朵不足，兑换失败"
		case containsAny(value, "奖品单日已耗尽", "奖品已兑完", "已兑完", "已耗尽", "库存不足", "无库存"):
			return "库存已抢完"
		case containsAny(value, "未登录", "登录失效", "账号已失效", "账号失效", "重新登录", "认证为空"):
			return "登录状态已失效，请重新登录"
		case strings.Contains(value, "兑换结果待确认"):
			return "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"
		case containsAny(value, "设备认证失败", "设备标识为空"):
			return "账号验证未完成，请稍后重试"
		case containsAny(value, "滑块识别置信度", "滑块验证码识别失败", "滑块匹配存在歧义", "验证失败"):
			return "验证未通过，请重试"
		case strings.Contains(value, "兑换资格校验未通过"):
			return strings.ReplaceAll(value, "兑换资格校验未通过", "兑换条件未满足")
		case containsAny(value, "请求失败", "请求返回异常", "读取响应失败", "timeout", "deadline exceeded", "网络异常"):
			return "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"
		case containsAny(value, "解析响应失败", "响应格式错误"):
			return "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"
		case rejected:
			return "活动验证未通过，请稍后重试"
		}
	}
	return strings.TrimSpace(upstreamErrorID.ReplaceAllString(value, ""))
}

func containsAny(value string, phrases ...string) bool {
	for _, phrase := range phrases {
		if strings.Contains(value, phrase) {
			return true
		}
	}
	return false
}

// MessageJSON preserves JSON number precision and non-message fields. It is
// used at event serialization, including replay of older durable envelopes.
func MessageJSON(taskType string, raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return raw, nil
	}
	switch trimmed[0] {
	case '{':
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return nil, err
		}
		if value, ok := object["task_type"]; ok && !strings.HasPrefix(strings.ToLower(taskType), "exchange") {
			_ = json.Unmarshal(value, &taskType)
		}
		for key, value := range object {
			trimmedValue := bytes.TrimSpace(value)
			if (key == "message" || key == "last_result" || key == "skip_reason" || key == "error_summary" || key == "error") && len(trimmedValue) > 0 && trimmedValue[0] == '"' {
				var text string
				if json.Unmarshal(value, &text) == nil {
					cleaned, _ := json.Marshal(Message(taskType, text))
					object[key] = cleaned
					continue
				}
			}
			if len(trimmedValue) > 0 && (trimmedValue[0] == '{' || trimmedValue[0] == '[') {
				cleaned, err := MessageJSON(taskType, value)
				if err != nil {
					return nil, err
				}
				object[key] = cleaned
			}
		}
		return json.Marshal(object)
	case '[':
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		for index, value := range values {
			cleaned, err := MessageJSON(taskType, value)
			if err != nil {
				return nil, err
			}
			values[index] = cleaned
		}
		return json.Marshal(values)
	}
	return raw, nil
}
