package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestCocWeb6ChecksServerDatesAndCountsWithSeparateSession(t *testing.T) {
	previous := activityNow
	activityNow = func() time.Time { return time.Date(2026, 11, 11, 16, 30, 0, 0, time.UTC) }
	t.Cleanup(func() { activityNow = previous })
	for _, confirmed := range []bool{true, false} {
		t.Run(fmt.Sprint(confirmed), func(t *testing.T) {
			signs, draws := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(coc3RoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "orches.yun.139.com" {
					return coc3Reply(req, `{"success":true,"code":"0","data":{"token":"ticket"}}`), nil
				}
				if req.URL.Path == "/coc/user/loginCheck" {
					resp := coc3Reply(req, `{"code":0}`)
					resp.Header.Add("Set-Cookie", "User-Token=web6; Path=/")
					return resp, nil
				}
				if req.Header.Get("Cookie") != "User-Token=web6" {
					t.Fatalf("wrong session %s", req.Header.Get("Cookie"))
				}
				var body map[string]interface{}
				_ = json.NewDecoder(req.Body).Decode(&body)
				switch req.URL.Path {
				case "/coc/activities/prize/consecutiveClockInQuery":
					if body["activityId"] != api.CocWeb6SignActivity {
						t.Fatalf("large activity ID was corrupted: %v", body)
					}
					dates := `[]`
					if signs > 0 && confirmed {
						dates = `["20261112"]`
					}
					return coc3Reply(req, `{"code":"0","data":{"clockInDateList":`+dates+`}}`), nil
				case "/coc/activities/prize/templatePrize":
					if body["touchCode"] != "P00000001090" {
						t.Fatal(body)
					}
					if body["activityId"] == api.CocWeb6SignActivity {
						signs++
					} else if body["activityId"] == api.CocWeb6DrawActivity {
						draws++
					} else {
						t.Fatal(body)
					}
					return coc3Reply(req, `{"code":0,"data":{"skuCode":"999999999999"}}`), nil
				case "/coc/activities/prize/drawDayNum":
					n := 1
					if draws > 0 && confirmed {
						n = 0
					}
					return coc3Reply(req, fmt.Sprintf(`{"code":0,"data":{"dayRemainNum":%d}}`, n)), nil
				case "/coc/activities/growth/query":
					return coc3Reply(req, `{"code":0,"data":{"level":"V1"}}`), nil
				case "/coc/activities/growth/query/evaluate":
					return coc3Reply(req, `{"code":0,"data":true}`), nil
				default:
					t.Fatalf("unexpected %s", req.URL)
				}
				return nil, nil
			})))
			task := NewCocWeb6Task(client, logger.NewLogger(logger.LevelError)).SetAccountContext("13800000001", "Basic fixture")
			task.wait = func(time.Duration) {}
			if err := task.Run(); err != nil {
				t.Fatal(err)
			}
			message := task.Message()
			if task.Pending() == confirmed || signs != 1 || draws != 1 || strings.Contains(message, "日期记录已确认") != confirmed || strings.Contains(message, "抽奖确认1次") != confirmed {
				t.Fatalf("pending=%v signs=%d draws=%d message=%s", task.Pending(), signs, draws, message)
			}
			if confirmed && !strings.Contains(message, "未中奖") {
				t.Fatal(message)
			}
		})
	}
}

func TestCocWeb6EvaluationRequiresRealContentAndConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		configured, confirmed bool
		writes                int
		pending               bool
	}{
		{"missing content", false, false, 0, true},
		{"confirmed", true, true, 1, false},
		{"ack without state", true, false, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.configured {
				t.Setenv("CAIYUN_COC_WEB6_EVALUATION_JSON_13800000001", `{"score":4,"label":"16,17","content":"测试账号提供的评价"}`)
			}
			writes := 0
			client := corehttp.NewClient(corehttp.WithTransport(coc3RoundTrip(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/coc/activities/growth/query":
					return coc3Reply(req, `{"code":0,"data":{"level":"V1"}}`), nil
				case "/coc/activities/growth/query/evaluate":
					return coc3Reply(req, fmt.Sprintf(`{"code":0,"data":%v}`, writes > 0 && tc.confirmed)), nil
				case "/coc/activities/growth/submit/evaluate":
					writes++
					var body map[string]interface{}
					_ = json.NewDecoder(req.Body).Decode(&body)
					if body["score"] != float64(4) || body["label"] != "16,17" || body["content"] != "测试账号提供的评价" {
						t.Fatal(body)
					}
					return coc3Reply(req, `{"code":0}`), nil
				default:
					t.Fatalf("unexpected %s", req.URL)
				}
				return nil, nil
			})))
			task := NewCocWeb6Task(client, nil).SetAccountContext("13800000001", "Basic fixture")
			task.wait = func(time.Duration) {}
			message, err := task.growth("token")
			if err != nil || writes != tc.writes || task.Pending() != tc.pending || strings.Contains(message, "完成状态已确认") != tc.confirmed {
				t.Fatalf("writes=%d pending=%v message=%s err=%v", writes, task.Pending(), message, err)
			}
		})
	}
}
