package tasks

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func seedTestTask(handler coc3RoundTrip) *Coc3Task {
	task := NewCoc3Task(corehttp.NewClient(corehttp.WithTransport(handler)), logger.NewLogger(logger.LevelError))
	task.tokenValue = "seed-token"
	task.wait = func(time.Duration) {}
	return task
}

func TestCoc3SeedRunsRealTasksAndConfirmsRecordsAndSunlight(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		t.Run(fmt.Sprint(confirmed), func(t *testing.T) {
			signs, visits, shares, claims, listReads, homeReads := 0, 0, 0, 0, 0, 0
			task := seedTestTask(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "dev.coc.10086.cn" || req.Header.Get("Token") != "seed-token" {
					t.Fatal("wrong seed session")
				}
				var payload map[string]interface{}
				_ = json.NewDecoder(req.Body).Decode(&payload)
				body := ""
				switch {
				case strings.HasSuffix(req.URL.Path, "/queryGrowthValue"):
					value := 100
					if claims > 0 && confirmed {
						value = 110
					}
					body = fmt.Sprintf(`{"resultCode":"0","data":{"growthValue":"%d"}}`, value)
				case strings.HasSuffix(req.URL.Path, "/getTaskSignInfo"):
					if payload["taskId"] != float64(3115) || payload["needQyCredits"] != float64(1) {
						t.Fatal(payload)
					}
					body = fmt.Sprintf(`{"resultCode":0,"data":{"signToday":%v,"accumulateCount":"1"}}`, signs > 0 && confirmed)
				case strings.HasSuffix(req.URL.Path, "/accumulateSign"):
					if payload["taskId"] != float64(3115) || payload["channelCode"] != "P00000011939" || payload["wtAc"] != "P00000008211_" || len(fmt.Sprint(payload["wtAcId"])) != 32 {
						t.Fatal(payload)
					}
					signs++
					body = `{"resultCode":0,"data":[{"awardName":"10阳光值"}]}`
				case strings.HasSuffix(req.URL.Path, "/myAwardRecord"):
					body = `{"resultCode":0,"data":{"rows":[{"skuName":"阳光值"}]}}`
				case strings.HasSuffix(req.URL.Path, "/getGrowthTaskList"):
					listReads++
					finish := 0
					if listReads > 1 && confirmed {
						finish = 1
					}
					if payload["taskListId"] != float64(55) {
						t.Fatal(payload)
					}
					body = fmt.Sprintf(`{"resultCode":0,"data":[{"taskId":"9001","taskType":"1","isFinish":"%d","taskMainTitle":"浏览","unfinishRedirectUrl":"https://dev.coc.10086.cn/coc3/canvas/rightsmarket-h5-canvas/online/current-page?x=1"},{"taskId":9002,"taskType":2,"isFinish":%d,"taskMainTitle":"分享"}]}`, finish, finish)
				case strings.HasSuffix(req.URL.Path, "/visit"):
					if payload["taskId"] != float64(9001) || payload["visitUrl"] != "current-page" {
						t.Fatal(payload)
					}
					visits++
					body = `{"resultCode":0,"data":true}`
				case strings.HasSuffix(req.URL.Path, "/share"):
					if payload["taskId"] != float64(9002) || payload["visitUrl"] != "2026dmycj" {
						t.Fatal(payload)
					}
					shares++
					body = `{"resultCode":0,"data":true}`
				case strings.HasSuffix(req.URL.Path, "/getSlgCreditsHomePage"):
					homeReads++
					body = `{"resultCode":0,"data":{"unreceivedCreditsVOList":[{"recordId":1234,"showNum":"10"},{"recordId":1234,"showNum":"10"}]}}`
					if homeReads > 1 && confirmed {
						body = `{"resultCode":0,"data":{"unreceivedCreditsVOList":[]}}`
					}
				case strings.HasSuffix(req.URL.Path, "/getGrowthBubble"):
					if payload["creditsRecordId"] != float64(1234) || payload["referer"] != "2026dmycj" || payload["channelNo"] != "P00000011939" {
						t.Fatal(payload)
					}
					claims++
					body = `{"resultCode":0}`
				default:
					t.Fatalf("unexpected seed endpoint %s", req.URL)
					return nil, nil
				}
				return coc3Reply(req, body), nil
			})
			message := strings.Join(task.runSeed(), "；")
			if len(task.issues) != 0 || signs != 1 || visits != 1 || shares != 1 || claims != 1 || task.Pending() == confirmed {
				t.Fatalf("pending=%v actions=%d/%d/%d/%d issues=%v message=%s", task.Pending(), signs, visits, shares, claims, task.issues, message)
			}
			if confirmed && (!strings.Contains(message, "服务端确认完成2/2项") || !strings.Contains(message, "确认领取阳光值10") || !strings.Contains(message, "签到奖励记录1条")) {
				t.Fatal(message)
			}
			if !confirmed && (strings.Contains(message, "确认领取阳光值10") || !strings.Contains(message, "尚未查到完成记录")) {
				t.Fatal(message)
			}
		})
	}
}

func TestCoc3SeedNeverSubmitsSubscriptionsOrInvitations(t *testing.T) {
	task := seedTestTask(func(req *http.Request) (*http.Response, error) {
		body := ""
		switch {
		case strings.HasSuffix(req.URL.Path, "/queryGrowthValue"):
			body = `{"resultCode":0,"data":{"growthValue":0}}`
		case strings.HasSuffix(req.URL.Path, "/getTaskSignInfo"):
			body = `{"resultCode":0,"data":{"signToday":true}}`
		case strings.HasSuffix(req.URL.Path, "/myAwardRecord"):
			body = `{"resultCode":0,"data":{"rows":[]}}`
		case strings.HasSuffix(req.URL.Path, "/getGrowthTaskList"):
			body = `{"resultCode":0,"data":[{"taskId":1,"taskType":3,"isFinish":0},{"taskId":2,"taskType":6,"isFinish":0}]}`
		case strings.HasSuffix(req.URL.Path, "/getSlgCreditsHomePage"):
			body = `{"resultCode":0,"data":{"unreceivedCreditsVOList":[]}}`
		default:
			t.Fatalf("unsafe seed mutation %s", req.URL)
			return nil, nil
		}
		return coc3Reply(req, body), nil
	})
	message := strings.Join(task.runSeed(), "；")
	if !task.Pending() || len(task.issues) != 0 || !strings.Contains(message, "0/2项") {
		t.Fatalf("%s %v", message, task.issues)
	}
}

func TestCoc3SeedDoesNotRetryAnAmbiguousClaimFromDuplicateRecords(t *testing.T) {
	claims := 0
	task := seedTestTask(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/getGrowthBubble") {
			claims++
			return nil, errors.New("connection lost")
		}
		if strings.HasSuffix(req.URL.Path, "/queryGrowthValue") {
			return coc3Reply(req, `{"resultCode":0,"data":{"growthValue":100}}`), nil
		}
		return coc3Reply(req, `{"resultCode":0,"data":{"unreceivedCreditsVOList":[{"recordId":"same","showNum":10},{"recordId":"same","showNum":10}]}}`), nil
	})
	task.claimSeedBubbles(100)
	if claims != 1 || len(task.issues) != 1 || !task.Pending() {
		t.Fatalf("claims=%d issues=%v pending=%v", claims, task.issues, task.Pending())
	}
}
