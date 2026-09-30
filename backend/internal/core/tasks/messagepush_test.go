package tasks

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestMessagePushClaimsOnlyReadyRewardTypes(t *testing.T) {
	var claimed []int
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/market/msgPushOn/task/status":
			return hiddenRewardReply(req, `{"code":0,"result":{"pushOn":1,"onDuaration":31,"firstTaskStatus":2,"secondTaskStatus":3,"pushTaskStatus":2}}`), nil
		case "/market/msgPushOn/task/obtain":
			body, _ := io.ReadAll(req.Body)
			var payload struct {
				Type int `json:"type"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			claimed = append(claimed, payload.Type)
			return hiddenRewardReply(req, `{"code":0,"result":{"obtainCode":1}}`), nil
		default:
			t.Fatalf("unexpected message push endpoint: %s", req.URL)
		}
		return nil, nil
	})))
	task := NewMessagePushRewardTask(client, logger.NewLogger(logger.LevelError))
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 2 || claimed[0] != 1 || claimed[1] != 3 {
		t.Fatalf("claimed types = %v, want [1 3]", claimed)
	}
}
