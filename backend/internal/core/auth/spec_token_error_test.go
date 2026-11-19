package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestSpecTokenTargetUnavailablePreservesIssuerReasonWithoutExpiringAccount(t *testing.T) {
	for _, body := range []string{
		`{"success":false,"code":"1010199999","message":"未知异常","data":{"result":{"resultCode":"05010048","resultDesc":"获取移动统一认证临时票据失败"}}}`,
		`{"success":false,"code":"1010000500","message":"统一认证上游不可用"}`,
	} {
		calls := 0
		client := corehttp.NewClient(corehttp.WithTransport(protocolTransport(func(req *http.Request) (*http.Response, error) {
			calls++
			return protocolResponse(req, 200, body), nil
		})))
		_, err := NewAuth(client).QuerySpecTokenForSourceContext(context.Background(), "Basic fixture", "13800000001", "056002")
		var issuer *SpecTokenError
		if calls != 1 || !errors.As(err, &issuer) || !issuer.TargetUnavailable() || issuer.SourceID != "056002" || errors.Is(err, ErrCredentialExpired) || !strings.Contains(err.Error(), issuer.Code) {
			t.Fatalf("issuer error lost its cause or invalidated the account: calls=%d err=%v", calls, err)
		}
		if strings.Contains(body, "05010048") && (!strings.Contains(err.Error(), "05010048") || !strings.Contains(err.Error(), "获取移动统一认证临时票据失败")) {
			t.Fatalf("nested reason disappeared: %v", err)
		}
	}
}

func TestSpecTokenRequiresAnIssuedTicket(t *testing.T) {
	for _, fixture := range []struct {
		name, body, ticket, reason string
	}{
		{"issued", `{"success":true,"code":"0000","data":{"result":{"resultCode":"0"},"token":"issued"}}`, "issued", ""},
		{"numeric codes", `{"success":true,"code":0,"data":{"result":{"resultCode":0},"token":"issued"}}`, "issued", ""},
		{"issuer refused despite token field", `{"success":true,"code":"1010199999","data":{"result":{"resultCode":"05010048"},"token":"unused"}}`, "", "05010048"},
		{"inner refusal despite outer success", `{"success":true,"code":"0","data":{"result":{"resultCode":"05010048"},"token":"unused"}}`, "", "05010048"},
		{"expired despite token field", `{"success":true,"code":"1010010015","data":{"token":"unused"}}`, "", "重新登录"},
		{"blank ticket", `{"success":true,"code":"0","data":{"token":"  "}}`, "", "票据为空"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			client := corehttp.NewClient(corehttp.WithTransport(protocolTransport(func(req *http.Request) (*http.Response, error) {
				return protocolResponse(req, 200, fixture.body), nil
			})))
			ticket, err := NewAuth(client).QuerySpecTokenForSourceContext(t.Context(), "Basic fixture", "13800000001", "056002")
			if ticket != fixture.ticket || (err == nil) != (fixture.reason == "") || err != nil && !strings.Contains(err.Error(), fixture.reason) {
				t.Fatalf("ticket accepted=%t, err=%v", ticket != "", err)
			}
		})
	}
}

func TestSpecTokenRedirectStopsAtIssuer(t *testing.T) {
	calls := 0
	client := corehttp.NewClient(corehttp.WithTransport(protocolTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != authQuerySpecTokenURL {
			t.Fatal("ticket request left the configured issuer")
		}
		resp := protocolResponse(req, http.StatusTemporaryRedirect, "")
		resp.Header.Set("Location", "https://other.invalid/login")
		return resp, nil
	})))
	ticket, err := NewAuth(client).QuerySpecTokenForSourceContext(t.Context(), "Basic fixture", "13800000001", "056002")
	if calls != 1 || ticket != "" || err == nil || !strings.Contains(err.Error(), "307") {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
