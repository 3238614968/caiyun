package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

type protocolTransport func(*http.Request) (*http.Response, error)

func (f protocolTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func protocolResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

func TestJWTGatewayMigrationPreservesSignedContract(t *testing.T) {
	for _, bound := range []bool{false, true} {
		for _, failure := range []string{"404", "business", "missing-code", "network"} {
			t.Run(fmt.Sprintf("bound=%t/%s", bound, failure), func(t *testing.T) {
				var hosts []string
				transport := protocolTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.Path == "/ycloud/api/timestamp" {
						return protocolResponse(r, 200, `{"code":0,"result":1700000000123}`), nil
					}
					if r.URL.Path != "/ycloud/auth-service/auth/tyrzLogin" || r.Method != "POST" {
						t.Fatalf("retired route requested: %s", r.URL)
					}
					hosts = append(hosts, r.URL.Host)
					var body map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body["openAccount"] != float64(0) || body["sourceId"] != "1097" || body["marketName"] != "sign_in_3" || body["token"] != "test-sso" {
						t.Fatalf("payload=%v", body)
					}
					id, ts, nonce := r.Header.Get("x-request-id"), r.Header.Get("x-timestamp"), r.Header.Get("x-nonce")
					if id == "" || nonce == "" || ts != "1700000000123" || r.Header.Get("x-signature") != h5AuthSignature(id, ts, nonce) {
						t.Fatal("gateway fallback lost signature")
					}
					if r.Header.Get("appversion") != "13.2.5.0" {
						t.Fatal("outdated version")
					}
					if r.URL.Host == "m.mcloud.139.com" {
						switch failure {
						case "404":
							return protocolResponse(r, 404, "not found"), nil
						case "business":
							return protocolResponse(r, 200, `{"code":401,"result":{}}`), nil
						case "missing-code":
							return protocolResponse(r, 200, `{"result":{"token":"invalid"}}`), nil
						default:
							return nil, errors.New("offline")
						}
					}
					resp := protocolResponse(r, 200, `{"code":0,"result":{"token":"new-jwt"}}`)
					resp.Header.Add("Set-Cookie", "JSESSIONID=new-session; Path=/; Secure")
					return resp, nil
				})
				var token string
				var err error
				if bound {
					jar, _ := cookiejar.New(nil)
					token, err = LoginH5Session(t.Context(), &http.Client{Jar: jar, Transport: transport}, "test-sso", "test-UA", "https://m.mcloud.139.com/")
				} else {
					token, err = NewAuth(corehttp.NewClient(corehttp.WithTransport(transport))).TyrzLogin("test-sso")
				}
				if err != nil || token != "new-jwt" || strings.Join(hosts, ",") != "m.mcloud.139.com,gp.mcloud.139.com" {
					t.Fatalf("token=%s hosts=%v err=%v", token, hosts, err)
				}
			})
		}
	}
}

func TestSpecTokenUsesOrchesAndStopsOnExpiredCredential(t *testing.T) {
	for _, expired := range []bool{false, true} {
		calls := 0
		client := corehttp.NewClient(corehttp.WithTransport(protocolTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			if r.URL.String() != authQuerySpecTokenURL || r.Method != "POST" || r.Header.Get("Authorization") != "Basic fixture" {
				t.Fatalf("unexpected ticket request: %s", r.URL)
			}
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["account"] != "13800000001" || body["toSourceId"] != "001005" {
				t.Fatalf("body=%v", body)
			}
			if expired {
				return protocolResponse(r, 200, `{"success":false,"code":"05050006","message":"暂无权限"}`), nil
			}
			return protocolResponse(r, 200, `{"success":true,"code":"0000","data":{"token":"ticket"}}`), nil
		})))
		client.SetAuth("Basic fixture")
		result, err := NewAuth(client).QuerySpecToken("13800000001")
		if calls != 1 || errors.Is(err, ErrCredentialExpired) != expired {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
		if !expired && (err != nil || !result.Success || result.Data.Token != "ticket") {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
}

func TestBoundGatewayFallbackHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Transport: protocolTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "gp.mcloud.139.com" {
			t.Fatal("fallback after cancellation")
		}
		if strings.HasSuffix(r.URL.Path, "tyrzLogin") {
			cancel()
		}
		return protocolResponse(r, 404, "not found"), nil
	})}
	if _, err := LoginH5Session(ctx, client, "sso", "ua", "https://m.mcloud.139.com/"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestCredentialExpiryClassification(t *testing.T) {
	if _, err := parseRefreshAuthToken(`{"success":false,"code":"05050006","message":"暂无权限"}`); !errors.Is(err, ErrCredentialExpired) {
		t.Fatalf("refresh error=%v", err)
	}
	for _, code := range []string{"4006", "05050006"} {
		if !errors.Is(&TokenRefreshError{Code: code}, ErrCredentialExpired) {
			t.Fatalf("code=%s was not recognized", code)
		}
	}
}
