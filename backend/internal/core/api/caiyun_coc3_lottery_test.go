package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
)

func coc3LotteryAPI(t *testing.T, handler func(*http.Request) string) *Coc3API {
	t.Helper()
	return NewCoc3API(corehttp.NewClient(corehttp.WithTransport(coc3APIRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "dev.coc.10086.cn" || req.Header.Get("Cookie") != "COC3_LOGIN_TOKEN=token" || !strings.Contains(req.Header.Get("Accept"), "application/json") {
			t.Fatalf("incorrect COC3 host/session: %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(handler(req))), Request: req}, nil
	}))))
}

func TestCoc3PicLotteryHasEndpointSpecificCodesAndHeaders(t *testing.T) {
	for _, code := range []string{`60000`, `"60010"`, `0`, `200`, `-1`} {
		t.Run(code, func(t *testing.T) {
			api := coc3LotteryAPI(t, func(req *http.Request) string {
				if req.Method != "POST" || req.URL.Path != "/coc3/coc3-market-activity/cmpp-manage/lottery/lottery" || req.Header.Get("wasmParams") != "params" || req.Header.Get("gr-risk-p3") != "risk" {
					t.Fatalf("bad submit %s", req.URL)
				}
				var body map[string]interface{}
				_ = json.NewDecoder(req.Body).Decode(&body)
				if body["activityId"] != float64(17766) || body["channelNo"] != "P00000123844" || body["operDfp"] != "device" || body["referer"] != coc3Referer {
					t.Fatalf("body=%v", body)
				}
				return `{"code":` + code + `,"data":{"prizeName":"fixture"}}`
			})
			resp, err := api.SubmitCoc3Lottery("token", "params", "risk", "device", 17766)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Success != (code == `60000` || code == `"60010"`) {
				t.Fatalf("incorrect accepted code %s", code)
			}
			// The same numeric code never makes an order/login/task successful.
			ordinary, err := api.decodeResponse(&http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"code":` + code + `}`))}, "task")
			if err != nil || ordinary.Success != (code == `0` || code == `200`) {
				t.Fatalf("code leaked into generic decoder: %+v err=%v", ordinary, err)
			}
		})
	}
}

func TestCoc3EligibilityUsesGETAndNeverSendsSubmitFingerprint(t *testing.T) {
	for _, tc := range []struct {
		body            string
		eligible, valid bool
	}{
		{`{"resultCode":0,"data":null}`, true, true},
		{`{"resultCode":0,"data":{"noQualificationList":[1]}}`, false, true},
		{`{"resultCode":-1,"data":{"outOfStockList":[1]}}`, false, true},
		{`{"resultCode":-403,"msg":"活动不在进行中"}`, false, true},
		{`{"resultCode":-516,"msg":"登录失效"}`, false, false},
		{`{"data":{}}`, false, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			api := coc3LotteryAPI(t, func(req *http.Request) string {
				if req.Method != "GET" || req.URL.Query().Get("activityId") != "17456" || req.Header.Get("wasmParams") != "" || req.Header.Get("gr-risk-p3") != "" {
					t.Fatalf("bad preflight %s", req.URL)
				}
				return tc.body
			})
			result, err := api.Coc3LotteryEligibility("token", 17456)
			if (err == nil) != tc.valid || (tc.valid && result.Eligible != tc.eligible) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestCoc3ActivityConfigDoesNotTreatUnknownOrPaidPriceAsFree(t *testing.T) {
	api := coc3LotteryAPI(t, func(req *http.Request) string {
		for key, value := range map[string]string{"activityId": "17761", "batchId": "0", "showLimit": "3", "seckillType": "1", "from": "page", "selfChannelCode": "P00000067264"} {
			if req.Method != "GET" || req.URL.Query().Get(key) != value {
				t.Fatalf("incomplete config query: %s", req.URL)
			}
		}
		return `{"code":0,"data":{"startTime":"1793318400000","endTime":"1795996800000","subActivityList":[{"goodsList":[{"skuid":1,"price":0},{"skuId":"2","price":"0.00"},{"skuid":3,"price":null},{"skuid":4,"price":""},{"skuid":5,"price":1},{"skuid":6}]}]}}`
	})
	config, err := api.Coc3ActivityConfig("token", 17761)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.ZeroPriceSKUs) != 2 || !config.ZeroPriceSKUs[1] || !config.ZeroPriceSKUs[2] || !config.ActiveAt(time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)) || config.ActiveAt(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("config=%+v", config)
	}
}
