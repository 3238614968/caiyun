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

// Step counts and daily-share buttons match the sanitized reference task table.
// The fake upstream advances only after a real action, doTaskPost and the right
// click key, with one deliberately stale read to exercise callback lag.
func TestTaskCenterAdvancesScreenshotTasksUntilServerConfirmation(t *testing.T) {
	for _, tc := range []struct {
		id, steps int
		name      string
	}{
		{106, 3, "手动上传一个文件"}, {113, 3, "使用PC客户端"},
		{585, 4, "去体验AI相机"}, {609, 6, "和AI助手对话"},
		{604, 2, "去云盘图书馆阅读书籍"}, {409, 3, "从固定入口访问云朵中心"},
		{434, 4, "分享文件领好礼"}, {547, 2, "加入一个知识库"},
		{431, 3, "去使用移动云手机"}, {1075, 5, "和AI助手对话"},
	} {
		t.Run(fmt.Sprint(tc.id), func(t *testing.T) {
			state := api.Task{ID: tc.id, Name: tc.name, State: "WAIT", StepTypeSet: []string{"sendcloud", "click"}}
			actions, posts, reads, clicks := 0, 0, 0, 0
			job := taskStepTestJob(t, func(req *http.Request) string {
				switch req.URL.Path {
				case "/ycloud/signin/page/doTaskPost":
					if actions == 0 {
						t.Fatal("task registered without a real action")
					}
					posts++
				case "/ycloud/signin/task/click":
					if posts == 0 {
						t.Fatal("click before doTaskPost")
					}
					key := req.URL.Query().Get("key")
					if tc.id == 409 && state.CurrStep > 0 && key != "task2" {
						// A stale read may resend the first key; the upstream ignores
						// it, and only task2 can advance the remaining steps.
						return `{"code":0}`
					}
					clicks++
					state.CurrStep++
					if state.CurrStep >= tc.steps {
						if tc.id == 434 {
							state.Button = map[string]map[string]interface{}{"app": {"text": "已完成"}}
						} else {
							state.State = "FINISH"
						}
					}
				case "/ycloud/signin/task/taskListV3":
					reads++
					visible := state
					if reads == 2 {
						visible.CurrStep = 0
					}
					return taskStepJSON(visible)
				}
				return `{"code":0}`
			})
			initial := state
			if err := job.executeTaskSteps(taskListItem{MarketName: "sign_in_3", Task: initial}, func() error { actions++; return nil }); err != nil {
				t.Fatal(err)
			}
			if !isTaskCompleted(state) && !taskCompletedToday(state) {
				t.Fatalf("not confirmed: %+v", state)
			}
			if job.Pending() || clicks < tc.steps || actions != 1 {
				t.Fatalf("pending=%v clicks=%d actions=%d", job.Pending(), clicks, actions)
			}
		})
	}
}

func TestTaskCenterDoesNotTreatAcceptedClicksAsCompletion(t *testing.T) {
	state := api.Task{ID: 547, Name: "加入一个知识库", State: "WAIT"}
	actions, reads := 0, 0
	job := taskStepTestJob(t, func(req *http.Request) string {
		if strings.HasSuffix(req.URL.Path, "taskListV3") {
			reads++
			return taskStepJSON(state)
		}
		return `{"code":0}`
	})
	if err := job.executeTaskSteps(taskListItem{MarketName: "sign_in_3", Task: state}, func() error { actions++; return nil }); err != nil {
		t.Fatal(err)
	}
	if !job.Pending() || reads < 4 || reads > 9 || actions != 2 {
		t.Fatalf("pending=%v reads=%d actions=%d", job.Pending(), reads, actions)
	}
}

func TestTaskCenterPropagatesMissingAndFailedConfirmation(t *testing.T) {
	for _, reply := range []string{`{"code":0,"result":[]}`, `{"code":505,"msg":"权限不足"}`} {
		reads := 0
		job := taskStepTestJob(t, func(req *http.Request) string {
			if strings.HasSuffix(req.URL.Path, "taskListV3") {
				reads++
				if reads > 1 {
					return reply
				}
				return taskStepJSON(api.Task{ID: 1075, State: "WAIT"})
			}
			return `{"code":0}`
		})
		if err := job.executeTaskSteps(taskListItem{MarketName: "newsign_139mail", Task: api.Task{ID: 1075}}, func() error { return nil }); err == nil {
			t.Fatal("unverifiable task reported as success")
		}
	}
}

func TestTaskCenterSkipsTasksCompletedSinceBatchSnapshot(t *testing.T) {
	for _, state := range []api.Task{
		{ID: 106, State: "FINISH"},
		{ID: 434, State: "WAIT", Button: map[string]map[string]interface{}{"app": {"text": "已完成"}, "appcard": {"text": "去完成"}}},
	} {
		job := taskStepTestJob(t, func(req *http.Request) string {
			if strings.HasSuffix(req.URL.Path, "taskListV3") {
				return taskStepJSON(state)
			}
			if strings.HasSuffix(req.URL.Path, "click") || strings.HasSuffix(req.URL.Path, "doTaskPost") {
				t.Fatal("completed task was registered again")
			}
			return `{"code":0}`
		})
		if err := job.executeTaskSteps(taskListItem{MarketName: "sign_in_3", Task: api.Task{ID: state.ID, State: "WAIT"}}, func() error { t.Fatal("completed task executed again"); return nil }); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTaskCenterSummaryDistinguishesDailyAndUnfinished(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		if strings.HasSuffix(req.URL.Path, "taskListV3") {
			var payload struct {
				Market string `json:"marketname"`
			}
			_ = json.NewDecoder(req.Body).Decode(&payload)
			if payload.Market == "newsign_139mail" {
				return `{"code":0,"result":[]}`
			}
			return `{"code":0,"result":[{"id":106,"name":"上传文件","state":"FINISH"},{"id":434,"name":"分享<span>1/7</span>","state":"WAIT","button":{"app":{"text":"已完成"}}},{"id":609,"name":"AI对话","state":"WAIT","currstep":2}]}`
		}
		return `{"code":0}`
	})
	if err := job.checkIncompleteTasks(); err != nil {
		t.Fatal(err)
	}
	if !job.Pending() || !strings.Contains(job.Message(), "已完成1项") || !strings.Contains(job.Message(), "待完成1项") || !strings.Contains(job.Message(), "分享1/7") || strings.Contains(job.Message(), "<span>") {
		t.Fatal(job.Message())
	}
}

func TestTaskCenterRealUploadsKeepAppAndPCChannels(t *testing.T) {
	for _, tc := range []struct {
		id      int
		channel string
	}{{106, "10000023"}, {113, "10200153"}} {
		uploads, steps := 0, 0
		job := taskStepTestJob(t, func(req *http.Request) string {
			switch req.URL.Path {
			case "/hcy/file/create":
				if req.Header.Get("x-yun-app-channel") != tc.channel {
					t.Fatalf("channel=%s want %s", req.Header.Get("x-yun-app-channel"), tc.channel)
				}
				uploads++
				return `{"success":true,"data":{"fileId":"fixture-file","rapidUpload":true}}`
			case "/ycloud/signin/task/click":
				steps++
			case "/ycloud/signin/task/taskListV3":
				state := "WAIT"
				if steps >= 3 {
					state = "FINISH"
				}
				return taskStepJSON(api.Task{ID: tc.id, State: state, CurrStep: steps})
			}
			return `{"code":0}`
		})
		if handled, err := job.handleSpecialTask(taskListItem{MarketName: "sign_in_3", Task: api.Task{ID: tc.id, State: "WAIT"}}); !handled || err != nil {
			t.Fatalf("handled=%v err=%v", handled, err)
		}
		if uploads != 1 || steps != 3 || job.Pending() {
			t.Fatalf("uploads=%d steps=%d pending=%v", uploads, steps, job.Pending())
		}
	}
}

func taskStepJSON(state api.Task) string {
	body, _ := json.Marshal(map[string]interface{}{"code": 0, "result": []api.Task{state}})
	return string(body)
}

func TestMonthlyUploadsReconcileFreshCountersAndWaitForCallbacks(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		process, wantUploads int
		callback             bool
	}{
		{"fresh counter prevents redundant uploads", 99, 1, true},
		{"unconfirmed batch is not blindly uploaded again", 0, 20, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uploads, polls := 0, 0
			job := taskStepTestJob(t, func(req *http.Request) string {
				switch req.URL.Path {
				case "/hcy/file/create":
					uploads++
					return fmt.Sprintf(`{"success":true,"data":{"fileId":"monthly-%d","rapidUpload":true}}`, uploads)
				case "/ycloud/signin/task/taskListV3":
					state := api.Task{ID: 522, State: "WAIT", Process: tc.process}
					if uploads > 0 {
						polls++
					}
					if tc.callback && polls >= 2 {
						state.Process += uploads
						state.State = "FINISH"
					}
					return taskStepJSON(state)
				}
				return `{"code":0}`
			})
			// The original batch snapshot says zero even though the latest is 99.
			if err := job.handleMonthlyUploadTask(taskListItem{MarketName: "sign_in_3", Task: api.Task{ID: 522, State: "WAIT"}}); err != nil {
				t.Fatal(err)
			}
			if uploads != tc.wantUploads || polls < 2 || job.Pending() == tc.callback {
				t.Fatalf("uploads=%d polls=%d pending=%v", uploads, polls, job.Pending())
			}
		})
	}
}

func taskStepTestJob(t *testing.T, reply func(*http.Request) string) *TaskListTask {
	t.Helper()
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) { return hiddenRewardReply(req, reply(req)), nil })))
	client.SetJWTToken("fixture-jwt")
	client.SetDeviceID("Bfixture-device")
	job := NewTaskListTask(client, logger.NewLogger(logger.LevelError))
	job.wait = func(time.Duration) {}
	return job
}
