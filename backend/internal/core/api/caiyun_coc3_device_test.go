package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/coc3wasm"
	corehttp "caiyun/internal/core/http"
)

func coc3FixtureDeviceRequest() coc3wasm.DeviceRequest {
	return coc3wasm.DeviceRequest{URL: coc3wasm.DeviceTokenURL, Method: "POST", Headers: map[string]string{"coc-aurora": base64.StdEncoding.EncodeToString(make([]byte, 256))}, Body: []byte(`{"benefit_phone":"current-coc-session","encryptedText":"fixture"}`)}
}

func TestCoc3DeviceExchangeScopesSessionAndNeverReplays(t *testing.T) {
	for _, fixture := range []struct {
		name         string
		status       int
		body         string
		networkError bool
	}{
		{"receipt", 200, `{"code":"0","success":true,"data":"fixture-device-token"}`, false},
		{"refused", 403, `{"code":"-1"}`, false},
		{"redirect", 307, `{}`, false},
		{"transient", 503, `{}`, false},
		{"ambiguous", 0, "", true},
		{"oversized", 200, strings.Repeat("x", (64<<10)+1), false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			requests := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.String() != coc3wasm.DeviceTokenURL || req.Method != "POST" || req.Header.Get("Cookie") != "COC3_LOGIN_TOKEN=current-coc-session" {
					t.Fatal("wrong device session/target")
				}
				for _, key := range []string{"Authorization", "jwtToken", "deviceId", "x-yun-client-info", "x-DeviceInfo", "phone", "token", "gr-risk-p3"} {
					if req.Header.Get(key) != "" {
						t.Fatalf("field %s leaked to SDK device exchange", key)
					}
				}
				if fixture.networkError {
					return nil, errors.New("ambiguous network error with private response")
				}
				response := mailTestResponse(req, fixture.status, fixture.body)
				if fixture.status == 307 {
					response.Header.Set("Location", "https://unrelated.example/")
				}
				return response, nil
			})))
			client.SetAuth("private-auth")
			client.SetJWTToken("private-jwt")
			client.SetDeviceID("private-device")
			client.SetCookie("COC3_LOGIN_TOKEN", "other-account", "dev.coc.10086.cn")
			request := coc3FixtureDeviceRequest()
			request.Headers["Authorization"] = "untrusted-extra-header"
			response, err := NewCoc3API(client).ExchangeCoc3DeviceToken(context.Background(), "current-coc-session", request)
			if requests != 1 || (err != nil) != (fixture.networkError || fixture.name == "oversized") {
				t.Fatalf("requests=%d err=%v", requests, err)
			}
			if err == nil && (response.Status != fixture.status || string(response.Body) != fixture.body) {
				t.Fatal("receipt changed before official WASM could validate it")
			}
		})
	}
}

func TestCoc3DeviceExchangeRejectsForeignTargetsAndSessions(t *testing.T) {
	for _, name := range []string{"target", "method", "foreign-session", "header", "oversized", "cookie-injection"} {
		t.Run(name, func(t *testing.T) {
			request := coc3FixtureDeviceRequest()
			token := "current-coc-session"
			switch name {
			case "target":
				request.URL = "https://unrelated.example/"
			case "method":
				request.Method = "GET"
			case "foreign-session":
				request.Body = []byte(`{"benefit_phone":"another-account","encryptedText":"fixture"}`)
			case "header":
				request.Headers["coc-aurora"] = "invalid"
			case "oversized":
				request.Body = make([]byte, (32<<10)+1)
			case "cookie-injection":
				token = "bad; other=account"
			}
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid SDK request reached transport")
				return nil, nil
			})))
			if _, err := NewCoc3API(client).ExchangeCoc3DeviceToken(context.Background(), token, request); err == nil {
				t.Fatal("invalid SDK request accepted")
			}
		})
	}
}
