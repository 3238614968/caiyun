package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestMakeWishExchangeRequiresAvailabilityAndCorrectPayload(t *testing.T) {
	var postCount int
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/makewish/task/cloud-exchange/info":
			if req.URL.Query().Get("platform") != "android" {
				t.Fatalf("exchange info query = %s", req.URL.RawQuery)
			}
			return mailTestResponse(req, 200, `{"code":0,"result":{"canExchange":true}}`), nil
		case "/ycloud/makewish/task/cloud-exchange":
			postCount++
			body, _ := io.ReadAll(req.Body)
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil || payload["platform"] != "android" || payload["channel"] != "app" || payload["appVersion"] != "13.2.2" {
				t.Fatalf("exchange request = %s, err=%v", body, err)
			}
			return mailTestResponse(req, 200, `{"code":0}`), nil
		default:
			t.Fatalf("unexpected exchange endpoint: %s", req.URL)
		}
		return nil, nil
	})))
	api := NewCaiyunAPI(client)
	canExchange, err := api.MakeWishCloudExchangeEligibility()
	if err != nil || !canExchange {
		t.Fatalf("canExchange=%v err=%v", canExchange, err)
	}
	if err := api.ExchangeMakeWishCloud(); err != nil {
		t.Fatal(err)
	}
	if postCount != 1 {
		t.Fatalf("exchange POST count=%d", postCount)
	}
}
