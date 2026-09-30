package tasks

import (
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

type hiddenRewardRoundTrip func(*http.Request) (*http.Response, error)

func (f hiddenRewardRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func hiddenRewardReply(req *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func TestHiddenRewardsClaimsOnlyEligibleRewards(t *testing.T) {
	var registered, accepted, activated int
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/market/signin/page/signTask":
			return hiddenRewardReply(req, `{"code":0,"result":[{"id":105},{"id":120},{"id":434},{"id":548},{"id":549},{"id":550},{"id":551}]}`), nil
		case "/ycloud/openemailsms-service/openEmailsms/getTaskInfo":
			return hiddenRewardReply(req, `{"code":0,"result":{}}`), nil
		case "/ycloud/openemailsms-service/openEmailsms/reward":
			return hiddenRewardReply(req, `{"code":411}`), nil
		case "/ycloud/simple/springgift/getTaskList":
			return hiddenRewardReply(req, `{"code":0,"result":[{"id":21,"needRegister":true,"complete":false},{"id":22,"needRegister":true,"complete":true}]}`), nil
		case "/ycloud/simple/springgift/registerTask":
			body, _ := io.ReadAll(req.Body)
			if string(body) != "mark=21" || req.Header.Get("Content-Type") != "application/x-www-form-urlencoded;charset=UTF-8" {
				t.Fatalf("springgift registration body=%s contentType=%s", body, req.Header.Get("Content-Type"))
			}
			registered++
			return hiddenRewardReply(req, `{"code":0}`), nil
		case "/ycloud/simple/springgift/getLotteryCount":
			return hiddenRewardReply(req, `{"code":0,"result":0}`), nil
		case "/ycloud/signin/page/infoV3":
			return hiddenRewardReply(req, `{"code":0,"result":{"toReceive":30,"receiveList":[{"cloudType":0,"cloudNum":20},{"cloudType":1,"cloudNum":10}]}}`), nil
		case "/ycloud/fivenewcomer/upgradeGifts/getPrizeRecord":
			return hiddenRewardReply(req, `{"code":0,"result":[]}`), nil
		case "/ycloud/fivenewcomer/upgradeGifts/getPrize":
			activated++
			t.Fatal("upgrade gift activated without record")
		case "/ycloud/api/prize/query":
			if req.URL.Query().Get("marketName") == "National_NewLoginGif" {
				return hiddenRewardReply(req, `{"code":0}`), nil
			}
			return hiddenRewardReply(req, `{"code":410}`), nil
		case "/ycloud/api/prize/accept":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), "National_NewLoginGif") {
				t.Fatalf("wrong prize acceptance: %s", body)
			}
			accepted++
			return hiddenRewardReply(req, `{"code":0}`), nil
		default:
			t.Fatalf("unexpected hidden rewards endpoint: %s", req.URL)
		}
		return nil, nil
	})))
	task := NewHiddenRewardsTask(client, nil)
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if registered != 1 || accepted != 1 || activated != 0 {
		t.Fatalf("registered=%d accepted=%d activated=%d", registered, accepted, activated)
	}
	if !strings.Contains(task.Message(), "待领云朵30") || !strings.Contains(task.Message(), "隐藏548=true") {
		t.Fatalf("hidden rewards message = %s", task.Message())
	}
}
