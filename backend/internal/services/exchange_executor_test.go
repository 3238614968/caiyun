package services

import (
	"encoding/base64"
	"net/url"
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

func TestBuildExchangeURLWithPuzzleUsesNewYCloudEndpoint(t *testing.T) {
	rawURL := buildExchangeURLWithPuzzle("12345", 327)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse exchange url: %v", err)
	}

	if parsed.Scheme != "https" || parsed.Host != "m.mcloud.139.com" || parsed.Path != "/ycloud/signin/page/exchangeV2" {
		t.Fatalf("unexpected exchange URL: %s", rawURL)
	}

	query := parsed.Query()
	assertQuery := func(key, want string) {
		t.Helper()
		if got := query.Get(key); got != want {
			t.Fatalf("query[%s] = %q, want %q", key, got, want)
		}
	}
	assertQuery("prizeId", "12345")
	assertQuery("client", "app")
	assertQuery("clientVersion", exchangeClientVersion)
	assertQuery("puzzleOffset", "327")
	if _, ok := query["smsCode"]; !ok {
		t.Fatal("expected smsCode query key to be present")
	}
}

func TestExchangeUserDomainIDParsesJWTSubString(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"{\"userDomainId\":\"domain-123\"}"}`))
	token := "e30." + payload + ".sig"

	if got := exchangeUserDomainID(token); got != "domain-123" {
		t.Fatalf("exchangeUserDomainID() = %q, want domain-123", got)
	}
}

func TestDecodeExchangeSlideResponseAcceptsStringDimensions(t *testing.T) {
	payload, err := decodeExchangeSlideResponse([]byte(`{
		"code": 0,
		"msg": "success",
		"result": {
			"puzzle": "puzzle-base64",
			"picture": "picture-base64",
			"picWidth": "680",
			"picHeight": "400",
			"puzzleWidth": "96"
		}
	}`))
	if err != nil {
		t.Fatalf("decodeExchangeSlideResponse returned error: %v", err)
	}
	if payload.picWidth != 680 || payload.picHeight != 400 || payload.puzzleWidth != 96 {
		t.Fatalf("unexpected dimensions: picWidth=%d picHeight=%d puzzleWidth=%d", payload.picWidth, payload.picHeight, payload.puzzleWidth)
	}
}
