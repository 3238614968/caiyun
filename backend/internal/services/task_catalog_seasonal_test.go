package services

import (
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
)

func TestSeasonalActivitiesAreRegisteredForBatchExecution(t *testing.T) {
	catalog := NewTaskCatalog()
	batch := map[string]bool{}
	for _, code := range catalog.DefaultBatchCodes() {
		batch[code] = true
	}
	for _, code := range []string{"shake", "lantern", "liveflower"} {
		def, ok := catalog.Get(code)
		if !ok || def.execute == nil || !def.DefaultEnabled || !def.RunInBatch || !batch[code] {
			t.Fatalf("activity %s not ready for batch execution: %+v", code, def)
		}
	}
	for alias, code := range map[string]string{"cloud_phone_redpack": "cloudphone", "shake_shake": "shake", "lantern_riddles": "lantern", "live_flower": "liveflower"} {
		if catalog.Normalize(alias) != code {
			t.Fatalf("alias %s did not normalize to %s", alias, code)
		}
	}
}

type seasonalRunnerTransport func(*http.Request) (*http.Response, error)

func (f seasonalRunnerTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestSeasonalRunnerKeepsUnconfirmedRewardsPending(t *testing.T) {
	for _, code := range []string{"lantern", "liveflower"} {
		client := corehttp.NewClient(corehttp.WithTransport(seasonalRunnerTransport(func(req *http.Request) (*http.Response, error) {
			body := `{"code":0,"result":{}}`
			switch {
			case strings.HasSuffix(req.URL.Path, "/querySpecToken"):
				body = `{"data":{"token":"test-sso"}}`
			case strings.HasSuffix(req.URL.Path, "/tokenValidate"):
				body = `{"header":{"status":200},"data":{"token":"test-session"}}`
			case strings.HasSuffix(req.URL.Path, "/userAccountInfo"):
				body = `{"header":{"status":200},"data":{"info":{"canAmount":0}}}`
			case strings.HasSuffix(req.URL.Path, "/userSign"), strings.HasSuffix(req.URL.Path, "/userCompleteTask"):
				body = `{"header":{"status":200},"data":{"status":1}}`
			case strings.HasSuffix(req.URL.Path, "/configTaskList"):
				body = `{"header":{"status":200},"data":{"configTaskInstallList":[{"id":16,"taskCode":"INSTALL_1","prizeAmount":20}]}}`
			case strings.HasSuffix(req.URL.Path, "/getIndexPuzzleCard"):
				body = `{"code":0,"result":[{"id":1,"puzzleTitleContext":"尚无谜底的新谜题"}]}`
			case strings.HasSuffix(req.URL.Path, "/user/info"):
				body = `{"code":0,"result":{"flowerCnt":0}}`
			case strings.HasSuffix(req.URL.Path, "/firstParticipate/give"):
			default:
				t.Fatalf("unexpected request: %s", req.URL)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		})))
		def, _ := NewTaskCatalog().Get(code)
		result := def.execute(&TaskRunner{httpClient: client, account: &models.Account{Phone: "13800000001"}})
		if result.Status != "pending" || result.CloudGained != 0 || result.Message == "" {
			t.Fatalf("unconfirmed activity reported as completed: %+v", result)
		}
	}
}
