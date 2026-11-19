package services

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/models"
	"caiyun/internal/presentation"
)

type coc3RunnerRoundTrip func(*http.Request) (*http.Response, error)

func (f coc3RunnerRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCocTicketOutageRemainsPendingAndExpiredCredentialsRemainFailed(t *testing.T) {
	for _, code := range []string{"coc3", "coc_web6", "coc3_scan"} {
		for _, fixture := range []struct {
			name, body, status string
			visible            []string
		}{
			{"not issued", `{"success":false,"code":"1010199999","data":{"result":{"resultCode":"05010048"}}}`, "pending", []string{"未签发", "056002", "1010199999/05010048", "任务未执行", "中国移动 App"}},
			{"outage", `{"success":false,"code":"1010000500"}`, "pending", []string{"暂不可用", "1010000500", "任务未执行"}},
			{"expired", `{"success":false,"code":"1010010015"}`, "failed", []string{"重新登录"}},
		} {
			t.Run(code+"/"+fixture.name, func(t *testing.T) {
				calls := 0
				client := corehttp.NewClient(corehttp.WithTransport(coc3RunnerRoundTrip(func(req *http.Request) (*http.Response, error) {
					calls++
					if req.URL.Host != "orches.yun.139.com" {
						t.Fatalf("activity executed without a ticket: %s", req.URL.Path)
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fixture.body)), Request: req}, nil
				})))
				runner := &TaskRunner{httpClient: client, logger: logger.NewLogger(logger.LevelError), account: &models.Account{Phone: "13800000001", Auth: "Basic fixture"}}
				var result *TaskResult
				switch code {
				case "coc3":
					result = runner.runCoc3Task()
				case "coc_web6":
					result = runner.runCocWeb6Task()
				default:
					result = runner.runCoc3ScanTask()
				}
				if calls != 1 || result.Status != fixture.status || result.CloudGained != 0 || result.MiguNotesGained != 0 {
					t.Fatalf("calls=%d result=%+v", calls, result)
				}
				visible := presentation.Message(code, result.Message)
				for _, want := range fixture.visible {
					if !strings.Contains(visible, want) {
						t.Fatalf("visible=%q, missing %q", visible, want)
					}
				}
			})
		}
	}
}

func TestCoc3RunnerExchangesCompleteAccountAuthorization(t *testing.T) {
	encodedAuth := base64.StdEncoding.EncodeToString([]byte("mobile:13800000001:fresh-token"))
	for _, accountAuth := range []string{encodedAuth, "Basic " + encodedAuth} {
		t.Run(accountAuth, func(t *testing.T) {
			var paths []string
			client := corehttp.NewClient(corehttp.WithTransport(coc3RunnerRoundTrip(func(req *http.Request) (*http.Response, error) {
				paths = append(paths, req.URL.Path)
				body := ""
				switch req.URL.Host + req.URL.Path {
				case "orches.yun.139.com/orchestration/auth-rebuild/token/v1.0/querySpecToken":
					if req.Header.Get("Authorization") != "Basic "+encodedAuth {
						t.Fatalf("Authorization = %q, want complete encoded account credentials", req.Header.Get("Authorization"))
					}
					var payload map[string]string
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						t.Fatal(err)
					}
					if payload["account"] != "13800000001" || payload["toSourceId"] != "056002" {
						t.Fatalf("ticket payload = %v", payload)
					}
					body = `{"success":true,"code":"0","data":{"token":"fixture-ticket"}}`
				case "dev.coc.10086.cn/coc3/gr/login/ssoRights/userLoginV2":
					// Stop after checking the real runner-to-login protocol; activity
					// requests are covered separately without task pacing sleeps.
					body = `{"code":"FIXTURE_STOP","message":"fixture login stop"}`
				default:
					t.Fatalf("unexpected request: %s", req.URL)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})))
			runner := &TaskRunner{
				httpClient: client,
				logger:     logger.NewLogger(logger.LevelError),
				account:    &models.Account{Phone: "13800000001", Auth: accountAuth, Token: "stale-token"},
			}
			result := runner.runCoc3Task()
			if result.Status != "failed" || !strings.Contains(result.Message, "FIXTURE_STOP") || len(paths) != 2 {
				t.Fatalf("runner did not reach coc3 login: result=%+v paths=%v", result, paths)
			}
		})
	}
}

func TestCoc3RunnerDoesNotReportLoginAsTaskCompletion(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(coc3RunnerRoundTrip(func(req *http.Request) (*http.Response, error) {
		body := ""
		switch {
		case req.URL.Host == "orches.yun.139.com":
			body = `{"success":true,"code":"0","data":{"token":"ticket"}}`
		case strings.HasSuffix(req.URL.Path, "/userLoginV2"):
			body = `{"code":"0","data":{"token":"token"}}`
		case strings.HasSuffix(req.URL.Path, "/getTaskSignInfo"):
			body = `{"resultCode":0,"data":{"signToday":true}}`
		case strings.HasSuffix(req.URL.Path, "/visit"):
			body = `{"resultCode":0}`
		case strings.HasSuffix(req.URL.Path, "/slgCredits/queryGrowthValue"):
			body = `{"resultCode":-9009,"msg":"活动已结束"}`
		case strings.HasSuffix(req.URL.Path, "/getTaskList"):
			body = `{"resultCode":0,"data":{}}`
		case strings.HasSuffix(req.URL.Path, "/arrange/checkQualificByActivityId"):
			body = `{"resultCode":-403,"msg":"活动不在进行中"}`
		case strings.HasSuffix(req.URL.Path, "/getBalance"), strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			body = `{"code":200,"data":"0"}`
		default:
			t.Fatalf("unexpected %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})))
	runner := &TaskRunner{httpClient: client, logger: logger.NewLogger(logger.LevelError), account: &models.Account{Phone: "13800000001", Auth: "Basic fixture"}}
	result := runner.runCoc3Task()
	if result.Status != "pending" || result.CloudGained != 0 || !strings.Contains(result.Message, "不以历史任务ID确认完成") {
		t.Fatalf("%+v", result)
	}
}
