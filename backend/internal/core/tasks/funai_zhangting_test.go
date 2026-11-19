package tasks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestFunaiZhangtingLotterySendsVerifiedPayload(t *testing.T) {
	calls := 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/ycloud/funai/api/page/zhangtingLottery" {
			t.Fatalf("unexpected zhangting endpoint: %s", req.URL)
		}
		if req.Method != http.MethodPost {
			t.Fatalf("zhangting method = %s", req.Method)
		}
		if req.Header.Get("activityId") != api.FunAIMarketName {
			t.Fatalf("zhangting activityId = %s", req.Header.Get("activityId"))
		}
		var body map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["isZhangting"] != true || body["marketName"] != "National_playAI" {
			t.Fatalf("zhangting body = %v", body)
		}
		calls++
		return hiddenRewardReply(req, `{"code":0,"result":{"isWin":1,"prizeName":"100M流量"}}`), nil
	})))
	prize, err := api.NewCaiyunAPI(client).FunaiZhangtingLottery()
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || prize.IsWin != 1 || prize.PrizeName != "100M流量" {
		t.Fatalf("calls=%d prize=%+v", calls, prize)
	}
}

func TestFunAIRunAppendsDailyZhangtingLottery(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		reply       string
		wantMessage string
	}{
		{name: "win", reply: `{"code":0,"result":{"isWin":1,"prizeName":"100M流量"}}`, wantMessage: "掌厅抽奖中奖: 100M流量"},
		{name: "already-drawn", reply: `{"code":601,"msg":"今日已抽奖"}`, wantMessage: "掌厅抽奖未执行"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			zhangtingCalls := 0
			client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/portal/cloudCircle/index.html":
					return hiddenRewardReply(req, `{"code":0}`), nil
				case "/ycloud/funai/api/page/getmodule":
					return hiddenRewardReply(req, `{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"003","isComplete":1}]}}`), nil
				case "/ycloud/funai/api/timed/getmodule":
					return hiddenRewardReply(req, `{"code":0,"result":[]}`), nil
				case "/ycloud/funai/api/page/lottery":
					return hiddenRewardReply(req, `{"code":500,"msg":"没有抽奖次数"}`), nil
				case "/ycloud/funai/api/page/zhangtingLottery":
					zhangtingCalls++
					return hiddenRewardReply(req, scenario.reply), nil
				default:
					if strings.Contains(req.URL.Path, "/deviceprofile/") {
						return hiddenRewardReply(req, `{"code":1}`), nil
					}
					t.Fatalf("unexpected funai zhangting request: %s", req.URL)
				}
				return nil, nil
			})))
			client.SetDeviceID("test-device")
			client.SetJWTToken("fixture-jwt")
			task := NewFunAITask(client, logger.NewLogger(logger.LevelFatal))
			if err := task.Run(); err != nil {
				t.Fatal(err)
			}
			if zhangtingCalls != 1 {
				t.Fatalf("zhangting calls = %d, want exactly one daily draw", zhangtingCalls)
			}
			if !strings.Contains(task.Message(), scenario.wantMessage) {
				t.Fatalf("funai message = %s, want it to contain %q", task.Message(), scenario.wantMessage)
			}
		})
	}
}
