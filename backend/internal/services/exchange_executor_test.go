package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"caiyun/internal/models"
)

type stubExchangeTokenProvider struct {
	info *TokenInfo
	err  error
}

func (s stubExchangeTokenProvider) GetToken(uint) (*TokenInfo, error) { return s.info, s.err }

func TestExchangeAuthDoesNotReuseStaleJWTAfterManagedRefreshFailure(t *testing.T) {
	account := &models.ExchangeAccount{AccountID: 7, JWTToken: "stale-jwt", Auth: "Basic stale-auth"}
	if _, err := prepareExchangeAuthWithProvider(account, stubExchangeTokenProvider{err: errors.New("refresh failed")}); err == nil {
		t.Fatal("managed refresh failure must stop exchange instead of using stale JWT")
	}
	ctx, err := prepareExchangeAuthWithProvider(account, stubExchangeTokenProvider{info: &TokenInfo{
		JWTToken: "fresh-jwt", SSOToken: "fresh-sso", Auth: "Basic fresh-auth",
	}})
	if err != nil || ctx.jwtToken != "fresh-jwt" || ctx.ssoToken != "fresh-sso" {
		t.Fatalf("managed exchange credentials = %+v, err = %v", ctx, err)
	}
}

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

func TestBuildExchangeV3RequestMatchesCapturedContract(t *testing.T) {
	req, err := buildExchangeV3Request(context.Background(), "12345", 327, "fixture-device")
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "POST" || req.URL.Host != "m.mcloud.139.com" || req.URL.Path != "/ycloud/signin/page/exchangeV3" || req.URL.RawQuery != "" {
		t.Fatalf("unexpected exchange request: %s %s", req.Method, req.URL)
	}
	var body map[string]interface{}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["prizeId"] != float64(12345) || body["client"] != "app" || body["puzzleOffset"] != float64(327) || body["clientVersion"] != "13.2.2" || body["smsCode"] != "" || body["deviceId"] != "fixture-device" {
		t.Fatalf("unexpected exchange body: %+v", body)
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
