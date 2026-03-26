package services

import (
	"strings"
	"testing"
)

func TestBuildExchangeFailureMessageIncludesCoreFields(t *testing.T) {
	response := map[string]interface{}{
		"msg":        "奖品已兑完",
		"code":       float64(412),
		"resultCode": "SOLD_OUT",
		"traceId":    "trace-123",
	}

	message := buildExchangeFailureMessage(200, response, `{"msg":"奖品已兑完","code":412}`)

	expected := []string{"奖品已兑完", "http_status=200", "code=412", "result_code=SOLD_OUT", "trace_id=trace-123"}
	for _, fragment := range expected {
		if !strings.Contains(message, fragment) {
			t.Fatalf("expected %q in message %q", fragment, message)
		}
	}
}

func TestSummarizeExchangeBodyCompactsWhitespaceAndTruncates(t *testing.T) {
	body := strings.Repeat("a ", 120)
	summary := summarizeExchangeBody("\n  " + body + "  \n")

	if strings.Contains(summary, "\n") {
		t.Fatalf("expected compact summary, got %q", summary)
	}
	if len(summary) > 183 {
		t.Fatalf("expected truncated summary, got len=%d", len(summary))
	}
	if !strings.HasSuffix(summary, "...") {
		t.Fatalf("expected truncated summary to end with ellipsis, got %q", summary)
	}
}
