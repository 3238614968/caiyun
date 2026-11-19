package services

import (
	"strings"
	"testing"
)

func TestExchangeSystemLogKeepsDiagnosticsOutOfUserMessage(t *testing.T) {
	raw := buildExchangeFailureMessage(200, map[string]interface{}{"code": float64(1001), "msg": "您的云朵数不足"}, `{"code":1001,"msg":"您的云朵数不足"}`)
	raw += " | 滑块识别offset=376 confidence=0.8511 资格 exchangeCode=0 | build=current commit=dirty"
	if got := sanitizeExchangeMessageForDisplay(raw); got != "云朵不足，兑换失败" {
		t.Fatalf("public system log = %q", got)
	}
	if !strings.Contains(raw, "code=1001") || !strings.Contains(raw, "offset=376") || isExchangeRetryableFailure(raw) {
		t.Fatal("public formatting changed the raw insufficient-cloud result or its terminal classification")
	}
}
