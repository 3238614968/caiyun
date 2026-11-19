package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

// 2026-11-03 网关分域回归：gp 票只打 gp 网关，签名契约与 m 完全一致。
func TestTyrzLoginGPUsesGPHostWithSignature(t *testing.T) {
	var hosts []string
	transport := protocolTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/ycloud/api/timestamp" {
			return protocolResponse(r, 200, `{"code":0,"result":1700000000123}`), nil
		}
		if r.URL.Path != "/ycloud/auth-service/auth/tyrzLogin" || r.Method != "POST" {
			t.Fatalf("unexpected route: %s", r.URL)
		}
		hosts = append(hosts, r.URL.Host)
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["openAccount"] != float64(0) || body["sourceId"] != "1097" ||
			body["marketName"] != "sign_in_3" || body["token"] != "gp-sso" {
			t.Fatalf("payload=%v", body)
		}
		id, ts, nonce := r.Header.Get("x-request-id"), r.Header.Get("x-timestamp"), r.Header.Get("x-nonce")
		if id == "" || nonce == "" || ts != "1700000000123" || r.Header.Get("x-signature") != h5AuthSignature(id, ts, nonce) {
			t.Fatal("gp login lost signature")
		}
		return protocolResponse(r, 200, `{"code":0,"result":{"token":"gp-jwt"}}`), nil
	})
	token, err := NewAuth(corehttp.NewClient(corehttp.WithTransport(transport))).
		TyrzLoginGPContext(context.Background(), "gp-sso")
	if err != nil || token != "gp-jwt" || strings.Join(hosts, ",") != "gp.mcloud.139.com" {
		t.Fatalf("token=%s hosts=%v err=%v", token, hosts, err)
	}
}

func TestGPLoginRetryUsesFreshTicketAndPreservesActivationError(t *testing.T) {
	for _, activation := range []bool{true, false} {
		t.Run(fmt.Sprint(activation), func(t *testing.T) {
			ssoCalls, loginCalls := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(protocolTransport(func(req *http.Request) (*http.Response, error) {
				switch {
				case req.URL.String() == authQuerySpecTokenURL:
					ssoCalls++
					return protocolResponse(req, 200, fmt.Sprintf(`{"success":true,"code":"0","data":{"token":"ticket-%d"}}`, ssoCalls)), nil
				case req.URL.Path == "/ycloud/api/timestamp":
					return protocolResponse(req, 200, `{"code":0,"result":1700000000123}`), nil
				case strings.HasSuffix(req.URL.Path, "/tyrzLogin"):
					loginCalls++
					var payload map[string]interface{}
					_ = json.NewDecoder(req.Body).Decode(&payload)
					if payload["token"] != fmt.Sprintf("ticket-%d", loginCalls) {
						t.Fatal("single-use ticket was replayed")
					}
					if activation {
						return protocolResponse(req, 200, `{"code":9430,"msg":"账号不存在，请注册后再登录！"}`), nil
					}
					if loginCalls == 1 {
						return protocolResponse(req, 200, `{"code":200050409,"msg":"token 已失效"}`), nil
					}
					return protocolResponse(req, 200, `{"code":0,"result":{"token":"gp-jwt"}}`), nil
				default:
					t.Fatalf("unexpected %s", req.URL)
					return nil, nil
				}
			})))
			client.SetAuth("fixture")
			token, err := NewAuth(client).GetGPJWTToken("13800000000")
			if activation {
				if !GPAccountRequiresActivation(err) || !strings.Contains(err.Error(), "9430") || ssoCalls != 1 || loginCalls != 1 {
					t.Fatalf("calls=%d/%d err=%v", ssoCalls, loginCalls, err)
				}
			} else if err != nil || token != "gp-jwt" || ssoCalls != 2 || loginCalls != 2 {
				t.Fatalf("calls=%d/%d token=%s err=%v", ssoCalls, loginCalls, token, err)
			}
		})
	}
}

// sso 单次消费：GetGPJWTToken 必须单独取新票，不能复用 m 流程的 sso。
func TestGetGPJWTTokenQueriesFreshSSO(t *testing.T) {
	var ssoCalls, loginCalls int
	transport := protocolTransport(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.URL.String() == authQuerySpecTokenURL:
			ssoCalls++
			if r.Header.Get("Authorization") != "Basic fixture" {
				t.Fatalf("ticket request missing credential: %v", r.Header)
			}
			return protocolResponse(r, 200, `{"success":true,"code":"0000","data":{"token":"fresh-gp-sso"}}`), nil
		case r.URL.Path == "/ycloud/api/timestamp":
			return protocolResponse(r, 200, `{"code":0,"result":1700000000123}`), nil
		case r.URL.Host == "gp.mcloud.139.com" && strings.HasSuffix(r.URL.Path, "tyrzLogin"):
			loginCalls++
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["token"] != "fresh-gp-sso" {
				t.Fatalf("gp login reused stale sso: %v", body)
			}
			return protocolResponse(r, 200, `{"code":0,"result":{"token":"gp-jwt"}}`), nil
		}
		return nil, errors.New("unexpected request: " + r.URL.String())
	})
	client := corehttp.NewClient(corehttp.WithTransport(transport))
	client.SetAuth("fixture")
	token, err := NewAuth(client).GetGPJWTTokenContext(context.Background(), "13800000000")
	if err != nil || token != "gp-jwt" || ssoCalls != 1 || loginCalls != 1 {
		t.Fatalf("token=%s ssoCalls=%d loginCalls=%d err=%v", token, ssoCalls, loginCalls, err)
	}
}

// gp 换票失败要返回错误（调用方据此降级），不能静默返回空票当成功。
func TestTyrzLoginGPRejectsBusinessFailure(t *testing.T) {
	transport := protocolTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/ycloud/api/timestamp" {
			return protocolResponse(r, 200, `{"code":0,"result":1700000000123}`), nil
		}
		return protocolResponse(r, 200, `{"code":200050409,"msg":"token 已失效"}`), nil
	})
	if _, err := NewAuth(corehttp.NewClient(corehttp.WithTransport(transport))).
		TyrzLoginGPContext(context.Background(), "consumed-sso"); err == nil {
		t.Fatal("consumed sso should fail gp login")
	}
}
