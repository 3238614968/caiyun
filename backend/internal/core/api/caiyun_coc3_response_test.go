package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func coc3FixtureAPI(body string, status int) *Coc3API {
	return NewCoc3API(corehttp.NewClient(corehttp.WithTransport(coc3APIRoundTrip(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	}))))
}

func TestCoc3NeverTreatsStringErrorsOrMissingCodesAsSuccess(t *testing.T) {
	for _, fixture := range []struct {
		body        string
		wantSuccess bool
		wantError   bool
	}{
		{`{"resultCode":"0"}`, true, false},
		{`{"code":"200"}`, true, false},
		{`{"resultCode":"-516","message":"登录失效"}`, false, false},
		{`{"code":0,"resultCode":"-516"}`, false, false},
		{`{"code":500,"resultCode":0}`, false, false},
		{`{"resultCode":"ERROR"}`, false, false},
		{`{"resultCode":true}`, false, true},
		{`{"data":{}}`, false, true},
	} {
		t.Run(fixture.body, func(t *testing.T) {
			resp, err := coc3FixtureAPI(fixture.body, 200).VisitTask("token", "channel", "page")
			if (err != nil) != fixture.wantError || err == nil && resp.Success != fixture.wantSuccess {
				t.Fatalf("response=%+v err=%v", resp, err)
			}
		})
	}
	if _, err := coc3FixtureAPI(`{"code":0}`, 500).VisitTask("token", "channel", "page"); err == nil {
		t.Fatal("HTTP 500 cannot be accepted because its JSON contains code=0")
	}
}

func TestCoc3CountersAcceptStringsAndRejectMissingOrMalformedData(t *testing.T) {
	for _, data := range []string{`5`, `"5"`, `0`, `"0"`, `null`, `{}`, `"oops"`, `-1`, `1.5`} {
		t.Run(data, func(t *testing.T) {
			wantError := data != `5` && data != `"5"` && data != `0` && data != `"0"`
			api := coc3FixtureAPI(`{"code":200,"data":`+data+`}`, 200)
			for name, read := range map[string]func() (int, error){
				"balance": func() (int, error) { return api.GetBeanBalance("token") },
				"lottery": func() (int, error) { return api.ResidueLotteryCount("token", 17524) },
			} {
				n, err := read()
				if (err != nil) != wantError || !wantError && strings.Contains(data, "5") && n != 5 {
					t.Fatalf("%s=%d err=%v", name, n, err)
				}
			}
		})
	}
	sku, err := coc3FixtureAPI(`{"code":200,"data":{"skuId":"95408"}}`, 200).CurrentLotterySku("token", 17524)
	if err != nil || sku != 95408 {
		t.Fatalf("sku=%d err=%v", sku, err)
	}
}

func TestCoc3SignStatusDoesNotHideAuthenticationFailureAsEndedTask(t *testing.T) {
	_, err := coc3FixtureAPI(`{"resultCode":"-516","msg":"登录失效"}`, 200).GetTaskSignInfo("token", 2999)
	var business *Coc3BusinessError
	if !errors.As(err, &business) || business.Code != "-516" {
		t.Fatalf("authentication failure lost: %v", err)
	}
	info, err := coc3FixtureAPI(`{"resultCode":"-9009","msg":"任务已结束"}`, 200).GetTaskSignInfo("token", 2999)
	if err != nil || !info.Unavailable {
		t.Fatalf("ended task=%+v err=%v", info, err)
	}
	if _, err := coc3FixtureAPI(`{"resultCode":0,"data":{}}`, 200).GetTaskSignInfo("token", 2999); err == nil {
		t.Fatal("missing sign state cannot imply unsigned or signed")
	}
}

func TestCoc3LoginUsesLoginStatusNotActivityStatus(t *testing.T) {
	for _, fixture := range []struct {
		name, body string
		accepted   bool
	}{
		{"login success", `{"code":"0","data":{"token":"issued"}}`, true},
		{"numeric login success", `{"code":0,"data":{"token":"issued"}}`, true},
		{"lottery envelope", `{"code":200,"data":{"token":"unused"}}`, false},
		{"task envelope", `{"resultCode":0,"data":{"token":"unused"}}`, false},
		{"explicit refusal", `{"code":0,"success":false,"data":{"token":"unused"}}`, false},
		{"missing token", `{"code":"0","data":{}}`, false},
		{"invalid body", `<html>login</html>`, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			login, err := coc3FixtureAPI(fixture.body, 200).userLoginV2(t.Context(), "one-use-ticket")
			if (err == nil) != fixture.accepted || (login != nil) != fixture.accepted {
				t.Fatalf("session accepted=%t, err=%v", login != nil, err)
			}
		})
	}
}

func TestCoc3LoginTicketBodyMatchesReference(t *testing.T) {
	const ticket = "offline-single-use"
	client := corehttp.NewClient(corehttp.WithTransport(coc3APIRoundTrip(func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		want := map[string]interface{}{
			"ticket": ticket, "ticketType": float64(11), "businessType": "RIGHTS_MARKET",
			"wxoaType": "", "m1": "", "m2": "", "s1": "", "s2": "", "from": "",
		}
		if !reflect.DeepEqual(body, want) || req.URL.RawQuery != "" || strings.Contains(req.Referer(), ticket) {
			t.Fatal("login ticket must be sent only in the reference JSON body")
		}
		return mailTestResponse(req, 200, `{"code":"0","data":{"token":"issued"}}`), nil
	})))
	if _, err := NewCoc3API(client).userLoginV2(t.Context(), ticket); err != nil {
		t.Fatal(err)
	}
}

func TestCoc3LoginNeverForwardsSingleUseTicketOnRedirect(t *testing.T) {
	calls := 0
	client := corehttp.NewClient(corehttp.WithTransport(coc3APIRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != coc3LoginURL {
			t.Fatal("ticket left the configured login endpoint")
		}
		resp := mailTestResponse(req, http.StatusTemporaryRedirect, "")
		resp.Header.Set("Location", "https://other.invalid/login")
		return resp, nil
	})))
	login, err := NewCoc3API(client).userLoginV2(t.Context(), "one-use-ticket")
	if calls != 1 || login != nil || err == nil || !strings.Contains(err.Error(), "307") {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}
