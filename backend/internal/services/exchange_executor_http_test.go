package services

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type exchangeRewriteRoundTripper struct {
	base      http.RoundTripper
	targetURL *url.URL
}

func (r *exchangeRewriteRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	urlCopy := *clone.URL
	urlCopy.Scheme = r.targetURL.Scheme
	urlCopy.Host = r.targetURL.Host
	clone.URL = &urlCopy
	return r.base.RoundTrip(clone)
}

func newMockExchangeTLSServer(t *testing.T, exchangeStatus int, exchangeBody string) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sms/solve":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"message":"ok","data":{"offset":18,"confidence":0.99,"method":"mock"}}`)
		case "/ycloud/auth-service/slide/getSlide":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","result":{"puzzle":"cHV6emxl","picture":"cGljdHVyZQ==","picWidth":680,"picHeight":400,"puzzleWidth":96}}`)
		case "/ycloud/signin/page/exchangeV2":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(exchangeStatus)
			_, _ = io.WriteString(w, exchangeBody)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
}

func newMockExchangeSession(t *testing.T, server *httptest.Server) *exchangeHTTPSession {
	t.Helper()
	targetURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	client := server.Client()
	baseTransport := client.Transport
	if baseTransport == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		baseTransport = transport
	}
	client.Transport = &exchangeRewriteRoundTripper{base: baseTransport, targetURL: targetURL}
	return &exchangeHTTPSession{
		client:    client,
		deviceID:  "mock-device-id",
		userAgent: "mock-agent",
		referer:   "https://m.mcloud.139.com/mock",
	}
}

func TestObtainExchangeSlideOffsetWithMockServer(t *testing.T) {
	server := newMockExchangeTLSServer(t, http.StatusOK, `{"msg":"success","result":{"prizeName":"测试商品"}}`)
	defer server.Close()

	t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
	t.Setenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY", "true")

	offset, info, err := obtainExchangeSlideOffset(newMockExchangeSession(t, server), &exchangeAuthContext{jwtToken: "jwt-token"})
	if err != nil {
		t.Fatalf("obtainExchangeSlideOffset() error = %v", err)
	}
	if offset != 18 {
		t.Fatalf("obtainExchangeSlideOffset() = %d, want 18", offset)
	}
	if !strings.Contains(info, "confidence=0.9900") || !strings.Contains(info, "method=mock") {
		t.Fatalf("obtainExchangeSlideOffset() info = %q", info)
	}
}

func TestExecuteExchangeOnceClassifiesMockedHTTPResponses(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		body           string
		wantMessageSub string
		wantLabel      string
	}{
		{
			name:           "auth 401",
			status:         http.StatusUnauthorized,
			body:           `{"msg":"Token 无效"}`,
			wantMessageSub: "http_status=401",
			wantLabel:      "auth",
		},
		{
			name:           "rate limit 429",
			status:         http.StatusTooManyRequests,
			body:           `{"msg":"请求频繁"}`,
			wantMessageSub: "http_status=429",
			wantLabel:      "rate_limited",
		},
		{
			name:           "server 500",
			status:         http.StatusInternalServerError,
			body:           `{"msg":"系统繁忙"}`,
			wantMessageSub: "http_status=500",
			wantLabel:      "invalid_response",
		},
		{
			name:           "malformed json",
			status:         http.StatusOK,
			body:           `<<<not-json>>>`,
			wantMessageSub: "解析响应失败",
			wantLabel:      "invalid_response",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := newMockExchangeTLSServer(t, tc.status, tc.body)
			defer server.Close()

			t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
			t.Setenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY", "true")

			result := executeExchangeOnce("prize-1", &exchangeAuthContext{jwtToken: "jwt-token"}, newMockExchangeSession(t, server))
			if result.success {
				t.Fatalf("executeExchangeOnce() success = true, want false, result=%+v", result)
			}
			if !strings.Contains(result.message, tc.wantMessageSub) {
				t.Fatalf("executeExchangeOnce() message = %q, want substring %q", result.message, tc.wantMessageSub)
			}
			if got := exchangeFailureReasonLabel(result.message); got != tc.wantLabel {
				t.Fatalf("exchangeFailureReasonLabel(%q) = %q, want %q", result.message, got, tc.wantLabel)
			}
			if result.execTime < 0 {
				t.Fatalf("executeExchangeOnce() execTime = %d, want >= 0", result.execTime)
			}
		})
	}
}

func TestExecuteExchangeOnceSuccessWithMockServer(t *testing.T) {
	server := newMockExchangeTLSServer(t, http.StatusOK, `{"msg":"success","result":{"prizeName":"测试商品"}}`)
	defer server.Close()

	t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
	t.Setenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY", "true")

	result := executeExchangeOnce("prize-1", &exchangeAuthContext{jwtToken: "jwt-token"}, newMockExchangeSession(t, server))
	if !result.success {
		t.Fatalf("executeExchangeOnce() success = false, result=%+v", result)
	}
	if !strings.Contains(result.message, "兑换成功") || !strings.Contains(result.message, "测试商品") {
		t.Fatalf("executeExchangeOnce() message = %q", result.message)
	}
	if got := exchangeFailureReasonLabel(result.message); got != "other" {
		t.Fatalf("exchangeFailureReasonLabel(%q) = %q, want %q", result.message, got, "other")
	}
	if result.execTime < 0 {
		t.Fatalf("executeExchangeOnce() execTime = %d, want >= 0", result.execTime)
	}
}

func TestExecuteExchangeOnceRejectsNestedBusinessFailure(t *testing.T) {
	tests := []string{
		`{"msg":"success","result":{"code":401,"message":"账号已失效，请重新登录"}}`,
		`{"msg":"success","result":"账号已失效，请重新登录"}`,
	}
	for _, body := range tests {
		server := newMockExchangeTLSServer(t, http.StatusOK, body)
		t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
		t.Setenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY", "true")

		result := executeExchangeOnce("prize-1", &exchangeAuthContext{jwtToken: "jwt-token"}, newMockExchangeSession(t, server))
		server.Close()
		if result.success {
			t.Fatalf("executeExchangeOnce() success = true, want false, result=%+v", result)
		}
		if !strings.Contains(result.message, "账号已失效") {
			t.Fatalf("executeExchangeOnce() message = %q, want account invalid detail", result.message)
		}
		if !result.stop {
			t.Fatalf("executeExchangeOnce() stop = false, want true for invalid account")
		}
	}
}

func TestExecuteExchangeOncePropagatesSlideErrors(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sms/solve":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":1,"message":"solver unavailable"}`)
		case "/ycloud/auth-service/slide/getSlide":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"code":0,"msg":"ok","result":{"puzzle":"cHV6emxl","picture":"cGljdHVyZQ==","picWidth":680,"picHeight":400,"puzzleWidth":96}}`)
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
	t.Setenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY", "true")

	result := executeExchangeOnce("prize-1", &exchangeAuthContext{jwtToken: "jwt-token"}, newMockExchangeSession(t, server))
	if result.success {
		t.Fatalf("executeExchangeOnce() success = true, want false, result=%+v", result)
	}
	if !strings.Contains(result.message, "滑块验证码识别失败") {
		t.Fatalf("executeExchangeOnce() message = %q", result.message)
	}
	if got := exchangeFailureReasonLabel(result.message); got != "other" {
		t.Fatalf("exchangeFailureReasonLabel(%q) = %q, want %q", result.message, got, "other")
	}
}

func Example_buildExchangeFailureMessage() {
	msg := buildExchangeFailureMessage(http.StatusTooManyRequests, map[string]interface{}{
		"msg":      "请求频繁",
		"result":   "limited",
		"trace_id": "trace-1",
	}, `{"msg":"请求频繁"}`)
	fmt.Println(strings.Contains(msg, "http_status=429"))
	// Output: true
}
