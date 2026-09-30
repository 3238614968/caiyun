package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestTaskListRunsV3ActionsAndClaimsTheirRewardBubble(t *testing.T) {
	t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "")
	clicked, claimed, posts := false, false, 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/signin/task/taskListV3":
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["marketname"] == "newsign_139mail" {
				return hiddenRewardReply(req, `{"code":0,"result":[]}`), nil
			}
			state := "WAIT"
			if clicked {
				state = "FINISH"
			}
			return hiddenRewardReply(req, fmt.Sprintf(`{"code":0,"result":[{"id":500,"name":"fixture click","enable":1,"state":%q,"stepTypeSet":["click"],"button":{"app":{"canReceive":1}}}]}`, state)), nil
		case "/ycloud/signin/task/click":
			clicked = true
			return hiddenRewardReply(req, `{"code":0}`), nil
		case "/ycloud/signin/page/infoV3":
			if clicked && !claimed {
				return hiddenRewardReply(req, `{"code":0,"result":{"total":1000,"toReceive":5,"receiveList":[{"recordId":3000000001,"cloudType":0,"cloudNum":5}]}}`), nil
			}
			total := 1000
			if claimed {
				total += 5
			}
			return hiddenRewardReply(req, fmt.Sprintf(`{"code":0,"result":{"total":%d,"toReceive":0,"receiveList":[]}}`, total)), nil
		case "/ycloud/signin/page/receiveV3":
			var body struct{ CloudID int64 }
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !clicked || claimed || body.CloudID != 3000000001 {
				t.Fatal("task ID used as reward ID or duplicate claim")
			}
			posts++
			claimed = true
			return hiddenRewardReply(req, `{"code":0,"result":{"receive":5,"total":1005}}`), nil
		case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling", "/ycloud/signin/page/doTaskPost":
			return hiddenRewardReply(req, `{"code":0}`), nil
		default:
			if strings.Contains(req.URL.Path, "/deviceprofile/") {
				return hiddenRewardReply(req, `{"code":1}`), nil
			}
			t.Fatalf("unexpected/legacy reward request: %s", req.URL.Path)
		}
		return nil, nil
	})))
	client.SetDeviceID("Bfixture-device")
	client.SetJWTToken("fixture-jwt")
	task := NewTaskListTask(client, logger.NewLogger(logger.LevelError))
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if !clicked || posts != 1 || task.ClaimedCloud() != 5 {
		t.Fatalf("click=%v posts=%d actual_gain=%d", clicked, posts, task.ClaimedCloud())
	}
	if err := task.receiveCompletedTaskRewards(); err != nil || posts != 1 || task.ClaimedCloud() != 5 {
		t.Fatal("later reward check duplicated the receipt")
	}
}
