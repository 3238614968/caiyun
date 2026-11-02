package services

import (
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/tasks"
	"caiyun/internal/models"
)

func TestTaskListRunnerPersistsUnfinishedTasksAsPending(t *testing.T) {
	for _, state := range []string{"WAIT", "FINISH"} {
		t.Run(state, func(t *testing.T) {
			client := corehttp.NewClient(corehttp.WithTransport(seasonalRunnerTransport(func(req *http.Request) (*http.Response, error) {
				body := `{"code":0}`
				switch req.URL.Path {
				case "/ycloud/signin/task/taskListV3":
					body = `{"code":0,"result":[{"id":9999,"name":"等待外部回调","state":"` + state + `","currstep":1,"stepTypeSet":["interfacecallback"]}]}`
				case "/ycloud/signin/page/infoV3":
					body = `{"code":0,"result":{"total":100,"toReceive":0,"receiveList":[]}}`
				case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling", "/ycloud/signin/page/doTaskPost":
				default:
					if strings.Contains(req.URL.Path, "/deviceprofile/") {
						body = `{"code":1}`
					} else {
						t.Fatalf("unexpected request %s", req.URL.Path)
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})))
			client.SetDeviceID("Bfixture-device")
			client.SetJWTToken("fixture-jwt")
			runner := &TaskRunner{httpClient: client, logger: logger.NewLogger(logger.LevelError), storage: tasks.NewMemoryStore(), account: &models.Account{Phone: "13800000001"}}
			result := runner.runTaskListTask()
			want := "success"
			if state == "WAIT" {
				want = "pending"
			}
			if result.Status != want {
				t.Fatalf("result=%+v", result)
			}
			if state == "WAIT" && (!strings.Contains(result.Message, "等待外部回调") || !strings.Contains(result.Message, "步骤1")) {
				t.Fatal(result.Message)
			}
		})
	}
}
