package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestRafflecodeAssistUsesCurrentMobileHostAndEncodedInvite(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(coc3APIRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "m.mcloud.139.com" || req.URL.Path != "/market/rafflecode/shareInvite" || req.URL.Query().Get("data") != "a+b/c=" || req.Header.Get("Jwttoken") != "mobile-jwt" {
			t.Fatalf("unexpected raffle request %s", req.URL)
		}
		return mailTestResponse(req, 200, `{"code":0}`), nil
	})))
	client.SetJWTToken("mobile-jwt")
	resp, err := NewCaiyunAPI(client).RafflecodeAssist("a+b/c=")
	if err != nil || !resp.IsSuccess() {
		t.Fatalf("response=%+v err=%v", resp, err)
	}
}

func TestRafflecodeConfigurationUsesOfficialFormProtocolAndArrayResult(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(coc3APIRoundTrip(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if req.URL.Path != "/market/rafflecode/rafflecodeConfigList" || req.Method != "POST" || string(body) != "marketName=rafflecode" || !strings.HasPrefix(req.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			t.Fatalf("incorrect config request %s body=%s", req.URL, body)
		}
		return mailTestResponse(req, 200, `{"code":0,"result":[{"id":"fixture"}]}`), nil
	})))
	response, err := NewCaiyunAPI(client).RafflecodeList()
	if err != nil || RafflecodeActiveSessions(response) != 1 {
		t.Fatalf("response=%+v err=%v", response, err)
	}
}
