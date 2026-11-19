package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

type coc3APIRoundTrip func(*http.Request) (*http.Response, error)

func (f coc3APIRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCoc3BusinessEndpointsUseCanonicalHostAndPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
		call func(*Coc3API) error
	}{
		{"sign status", "/coc3/coc3-market-activity/cmpp-manage/task/getTaskSignInfo", `{"resultCode":0,"data":{"signToday":true}}`, func(api *Coc3API) error {
			_, err := api.GetTaskSignInfo("fixture-token", 2999)
			return err
		}},
		{"sign", "/coc3/coc3-market-activity/cmpp-manage/task/accumulateSign", `{"resultCode":0}`, func(api *Coc3API) error {
			_, err := api.AccumulateSign("fixture-token", 2999, "fixture-channel")
			return err
		}},
		{"visit", "/coc3/coc3-market-activity/cmpp-manage/task/visit", `{"resultCode":0}`, func(api *Coc3API) error {
			_, err := api.VisitTask("fixture-token", "fixture-channel", "fixture-page")
			return err
		}},
		{"progress", "/coc3/coc3-market-activity/cmpp-manage/task/getTaskList", `{"resultCode":0,"data":{}}`, func(api *Coc3API) error {
			_, err := api.GetTaskProgress("fixture-token", []int{2999})
			return err
		}},
		{"balance", "/coc3/gr-integral-ability/api/external/score/digIntel/getBalance", `{"code":0,"data":5}`, func(api *Coc3API) error {
			_, err := api.GetBeanBalance("fixture-token")
			return err
		}},
		{"lottery count", "/coc3/coc3-market-activity/cmpp-manage/lottery/residueLotteryCount", `{"code":200,"data":1}`, func(api *Coc3API) error {
			_, err := api.ResidueLotteryCount("fixture-token", 17524)
			return err
		}},
		{"lottery sku", "/coc3/coc3-market-activity/cmpp-manage/lottery/currentLottery", `{"code":200,"data":{"skuId":123}}`, func(api *Coc3API) error {
			_, err := api.CurrentLotterySku("fixture-token", 17524)
			return err
		}},
		{"zero order", "/coc3/coc3-market-order/api/external/mixedOrder", `{"code":0}`, func(api *Coc3API) error {
			_, err := api.SubmitCoc3ZeroOrder("fixture-token", "fixture-wasm", 17524, 123)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			client := corehttp.NewClient(corehttp.WithTransport(coc3APIRoundTrip(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.Method != http.MethodPost || req.URL.Scheme != "https" || req.URL.Host != "dev.coc.10086.cn" || req.URL.Path != tt.path {
					t.Fatalf("request = %s %s, want POST https://dev.coc.10086.cn%s", req.Method, req.URL, tt.path)
				}
				if req.Header.Get("Token") != "fixture-token" || req.Header.Get("Cookie") != "COC3_LOGIN_TOKEN=fixture-token" {
					t.Fatal("request is missing coc3 session credentials")
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tt.body)), Request: req}, nil
			})))
			if err := tt.call(NewCoc3API(client)); err != nil {
				t.Fatal(err)
			}
			if requests != 1 {
				t.Fatalf("requests = %d, want 1", requests)
			}
		})
	}
}
