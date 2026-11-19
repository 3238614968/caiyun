package services

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/models"
)

func TestCloudPhoneRunnerReachesImplementationAndDoesNotConfirmRealInstanceSign(t *testing.T) {
	signs, balances := 0, 0
	client := corehttp.NewClient(corehttp.WithTransport(coc3RunnerRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := ""
		switch req.URL.Path {
		case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
			body = `{"data":{"token":"ticket"}}`
		case "/cloudphone-market/user/tokenValidate":
			body = `{"header":{"status":200},"data":{"token":"session"}}`
		case "/cloudphone-market/redpacket/userSign":
			signs++
			body = `{"header":{"status":200},"data":{"status":1}}`
		case "/cloudphone-market/redpacket/configTaskList":
			body = `{"header":{"status":200},"data":{"configTaskDailyList":[]}}`
		case "/cloudphone-market/redpacket/userAccountInfo":
			balances++
			body = fmt.Sprintf(`{"header":{"status":200},"data":{"info":{"canAmount":%d}}}`, 50)
		default:
			t.Fatalf("unexpected %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})))
	runner := &TaskRunner{httpClient: client, logger: logger.NewLogger(logger.LevelError), account: &models.Account{Phone: "13800000001"}}
	result := runner.runCloudPhoneTask()
	if result.Status != "pending" || signs != 1 || balances != 2 || !strings.Contains(result.Message, "云币50→50") {
		t.Fatalf("signs=%d balances=%d result=%+v", signs, balances, result)
	}
}

func TestCocExtensionDefinitionsAreExecutableAndDiscoveryIsOptIn(t *testing.T) {
	catalog := NewTaskCatalog()
	for code, enabled := range map[string]bool{"coc3": true, "coc_web6": true, "coc3_scan": false, "activity_collect": false} {
		definition, ok := catalog.Get(code)
		if !ok || definition.execute == nil || definition.DefaultEnabled != enabled || definition.RunInBatch != enabled {
			t.Fatalf("code=%s def=%+v", code, definition)
		}
	}
}

func TestTodayCloudRunnerNeverWritesCumulativeCloudAsNewRevenue(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(coc3RunnerRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := `{"code":0,"result":{"records":[]}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})))
	runner := &TaskRunner{httpClient: client, logger: logger.NewLogger(logger.LevelError)}
	result := runner.runTodayCloudTask()
	if result.CloudGained != 0 || result.Status != "success" || !strings.Contains(result.Message, "不重复计入本次收益") {
		t.Fatalf("result=%+v", result)
	}
}
