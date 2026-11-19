package tasks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestClosedCoc3ActivityIsPreflightedBeforeStaleChanceOrSKU(t *testing.T) {
	requests := 0
	task := campaignCoc3Task(t, func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != "GET" || !strings.HasSuffix(req.URL.Path, "/checkQualificByActivityId") {
			t.Fatalf("closed activity progressed to %s", req.URL)
		}
		return coc3Reply(req, `{"resultCode":-403,"msg":"活动不在进行中"}`), nil
	})
	message := strings.Join(task.runLottery(), "；")
	if requests != 4 || len(task.issues) != 0 || !strings.Contains(message, "共0次") {
		t.Fatalf("requests=%d issues=%v message=%s", requests, task.issues, message)
	}
}

func TestCoc3ProgressUsesCurrentIDsAndDoesNotInterpretNullAsZero(t *testing.T) {
	task := campaignCoc3Task(t, func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/getBalance") {
			return coc3Reply(req, `{"code":0,"data":0}`), nil
		}
		var body struct {
			IDs   []int `json:"taskIdList"`
			Scene int   `json:"scene"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.IDs) != 2 || body.IDs[0] != 3097 || body.IDs[1] != 3098 || body.Scene != 1 {
			t.Fatalf("stale or incorrect progress request: %+v", body)
		}
		return coc3Reply(req, `{"resultCode":0,"data":{"taskQueryTimeVOS":[{"taskId":3097,"totalNum":null,"completedNum":null},{"taskId":3098,"totalNum":null,"completedNum":null}]}}`), nil
	})
	message := strings.Join(task.runInventory([]int{3097, 3098}), "；")
	if !task.Pending() || len(task.issues) != 0 || !strings.Contains(message, "进度未下发") || strings.Contains(message, "0/0") {
		t.Fatal(message)
	}
}

func TestCoc3AlreadySignedCodeStillRequiresTodayRecord(t *testing.T) {
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/getTaskSignInfo") {
			return coc3Reply(req, `{"resultCode":0,"data":{"signToday":false}}`), nil
		}
		return coc3Reply(req, `{"resultCode":-9014,"msg":"本日已签到"}`), nil
	})
	message := strings.Join(task.runSignIn(), "；")
	if !task.Pending() || !strings.Contains(message, "今日记录未确认") || strings.Contains(message, "复查确认") {
		t.Fatal(message)
	}
}

func TestSpringGiftDuplicateRegistrationIsNotNewCompletion(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		switch req.URL.Path {
		case "/market/signin/page/signTask":
			return `{"code":0,"result":[{"id":1},{"id":2},{"id":3},{"id":4},{"id":5},{"id":6},{"id":7}]}`
		case "/ycloud/openemailsms-service/openEmailsms/getTaskInfo", "/ycloud/fivenewcomer/upgradeGifts/getPrizeRecord":
			return `{"code":602,"msg":"活动已结束"}`
		case "/ycloud/simple/springgift/getTaskList":
			return `{"code":0,"result":[{"id":21,"needRegister":true,"complete":false}]}`
		case "/ycloud/simple/springgift/registerTask":
			return `{"code":"5006","msg":"已存在该任务记录"}`
		case "/ycloud/simple/springgift/getLotteryCount":
			return `{"code":0,"result":0}`
		case "/ycloud/signin/page/infoV3":
			return `{"code":0,"result":{"toReceive":0,"receiveList":[]}}`
		case "/ycloud/api/prize/query":
			return `{"code":410}`
		default:
			t.Fatalf("unexpected %s", req.URL)
		}
		return ""
	})
	task := NewHiddenRewardsTask(job.client, nil)
	if err := task.Run(); err != nil || !task.Pending() || !strings.Contains(task.Message(), "新登记0项、此前已登记1项") {
		t.Fatalf("message=%s err=%v", task.Message(), err)
	}
}

func TestFamilyCircleRequiresSelectedAccountsRealGroup(t *testing.T) {
	t.Setenv("CAIYUN_FAMILY_GROUP_ID", "another-account-group")
	job := taskStepTestJob(t, func(req *http.Request) string {
		t.Fatalf("missing group must not query guessed route: %s", req.URL)
		return ""
	})
	task := NewFamilyCircleTask(job.client, job.logger)
	task.SetAccountPhone("13800000001")
	if err := task.Run(); err != nil || !task.Pending() || !strings.Contains(task.Message(), "群组ID") {
		t.Fatalf("pending=%v err=%v message=%s", task.Pending(), err, task.Message())
	}
}
