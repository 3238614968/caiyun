package tasks

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestTimedLotteryDistinguishesTerminalMessagesFromRealFailures(t *testing.T) {
	for _, fixture := range []struct {
		code            int
		message         string
		failed, pending bool
	}{
		{500, "您已经刮过奖啦！", false, false},
		{500, "抱歉，您暂无刮奖机会，请完成体验上面指定功能来获取刮奖机会哦", false, true},
		{500, "系统繁忙，请稍后重试", true, false},
		{90001, "未登录", true, false},
	} {
		t.Run(fixture.message, func(t *testing.T) {
			calls := 0
			task := autumnFixture(t, "fun_ai_timed_lottery", func(req *http.Request) string {
				if strings.HasSuffix(req.URL.Path, "/getmodule") {
					return `{"code":0,"result":[{"moduleId":"2-261104-1","taskIds":[{"id":"004","isComplete":1},{"id":"004","isComplete":1}]}]}`
				}
				if strings.HasSuffix(req.URL.Path, "/lottery") {
					calls++
					return fmt.Sprintf(`{"code":%d,"msg":%q}`, fixture.code, fixture.message)
				}
				t.Fatalf("unexpected %s", req.URL.Path)
				return ""
			})
			err := task.Run()
			if (err != nil) != fixture.failed || task.Pending() != fixture.pending || calls != 1 {
				t.Fatalf("calls=%d pending=%v err=%v", calls, task.Pending(), err)
			}
		})
	}
}

func TestHiddenPrizeEndedAtClaimIsSkippedButOtherErrorsRemainFailures(t *testing.T) {
	for _, code := range []int{602, 500, 90001} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			job := taskStepTestJob(t, func(req *http.Request) string {
				switch req.URL.Path {
				case "/market/signin/page/signTask":
					return `{"code":0,"result":[{"id":1},{"id":2},{"id":3},{"id":4},{"id":5},{"id":6},{"id":7}]}`
				case "/ycloud/openemailsms-service/openEmailsms/getTaskInfo", "/ycloud/simple/springgift/getTaskList", "/ycloud/fivenewcomer/upgradeGifts/getPrizeRecord":
					return `{"code":602,"msg":"活动已结束"}`
				case "/ycloud/signin/page/infoV3":
					return `{"code":0,"result":{"toReceive":0,"receiveList":[]}}`
				case "/ycloud/api/prize/query":
					if req.URL.Query().Get("marketName") == "National_NewLoginGif" {
						return `{"code":0}`
					}
					return `{"code":410}`
				case "/ycloud/api/prize/accept":
					return fmt.Sprintf(`{"code":%d,"msg":"activity result"}`, code)
				default:
					t.Fatalf("unexpected %s", req.URL.Path)
					return ""
				}
			})
			task := NewHiddenRewardsTask(job.client, nil)
			err := task.Run()
			if (err != nil) != (code != 602) || !strings.Contains(task.Message(), "新领0项") {
				t.Fatalf("message=%s err=%v", task.Message(), err)
			}
			if code == 602 && !strings.Contains(task.Message(), "已结束活动跳过") {
				t.Fatal(task.Message())
			}
		})
	}
}
