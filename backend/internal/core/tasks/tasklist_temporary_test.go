package tasks

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
)

func temporaryTaskJSON(tasks ...api.Task) string {
	if tasks == nil {
		tasks = []api.Task{}
	}
	body, _ := json.Marshal(map[string]interface{}{"code": 0, "result": map[string]interface{}{"beiyong1": tasks}})
	return string(body)
}

func emptyTemporaryGroupReply(t *testing.T, req *http.Request) string {
	t.Helper()
	var body struct {
		Group  string `json:"group"`
		Market string `json:"marketname"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Group != "beiyong1" || body.Market != "sign_in_3" || req.Method != "POST" {
		t.Fatalf("unexpected temporary query: %+v err=%v", body, err)
	}
	return temporaryTaskJSON()
}

func TestTemporaryTasksMergeWithV3WithoutDuplicateExecution(t *testing.T) {
	for _, v3Available := range []bool{true, false} {
		t.Run(fmt.Sprint(v3Available), func(t *testing.T) {
			groups := map[string]int{}
			job := taskStepTestJob(t, func(req *http.Request) string {
				switch {
				case strings.HasSuffix(req.URL.Path, "taskListV3"):
					var body map[string]interface{}
					_ = json.NewDecoder(req.Body).Decode(&body)
					if body["marketname"] == "newsign_139mail" {
						return taskStepJSON(api.Task{ID: 588, State: "WAIT"})
					}
					if !v3Available {
						return `{"code":500,"result":[]}`
					}
					return `{"code":0,"result":[{"id":106,"state":"WAIT"},{"id":587,"state":"WAIT"}]}`
				case strings.HasSuffix(req.URL.Path, "taskListV2"):
					var body struct {
						Group string `json:"group"`
					}
					_ = json.NewDecoder(req.Body).Decode(&body)
					groups[body.Group]++
					if body.Group == "beiyong1" {
						return temporaryTaskJSON(api.Task{ID: 587, State: "FINISH"}, api.Task{ID: 588, State: "WAIT"})
					}
					return fmt.Sprintf(`{"code":0,"result":{%q:[{"id":106,"state":"WAIT"}]}}`, body.Group)
				}
				return `{"code":0}`
			})
			items, err := job.fetchAllTaskItems()
			if err != nil || len(items) != 4 || groups["beiyong1"] != 1 {
				t.Fatalf("items=%#v groups=%v err=%v", items, groups, err)
			}
			seen := map[string]bool{}
			for _, item := range items {
				key := fmt.Sprintf("%s/%d", item.MarketName, item.Task.ID)
				if seen[key] {
					t.Fatalf("duplicated task: %s", key)
				}
				seen[key] = true
				if item.MarketName == "sign_in_3" && item.Task.ID == 587 && (item.Task.State != "FINISH" || item.GroupKey != "beiyong1") {
					t.Fatalf("temporary group state lost: %#v", item)
				}
			}
			if v3Available && len(groups) != 1 {
				t.Fatalf("queried unrelated V2 groups despite working V3: %v", groups)
			}
			if !v3Available && len(groups) != 5 {
				t.Fatalf("missing V2 fallback group: %v", groups)
			}
		})
	}
}

func TestTemporaryTasksQueryFailureDoesNotProduceCompletedSummary(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		if strings.HasSuffix(req.URL.Path, "taskListV3") {
			return `{"code":0,"result":[]}`
		}
		if strings.HasSuffix(req.URL.Path, "taskListV2") {
			return `{"code":615,"msg":"failed"}`
		}
		return `{"code":0}`
	})
	if err := job.checkIncompleteTasks(); err == nil || !job.Pending() || !strings.Contains(job.Message(), "不能确认完成") {
		t.Fatalf("pending=%v message=%s err=%v", job.Pending(), job.Message(), err)
	}
}

func TestMemoryPhotosUploadOnlyFreshRemainderAndRequireServerFinish(t *testing.T) {
	for _, tc := range []struct {
		name          string
		process       int
		confirm, fail bool
		wantUploads   int
		alreadyDone   bool
	}{
		{name: "fresh remainder", process: 8, confirm: true, wantUploads: 2},
		{name: "lagging callback cannot repeat batch", process: 8, wantUploads: 2},
		{name: "counter ten is not FINISH", process: 10},
		{name: "completion failure stops batch", process: 8, fail: true, wantUploads: 1},
		{name: "server already finished", process: 10, alreadyDone: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			creates, puts, completed, posts, clicks := 0, 0, 0, 0, 0
			state := api.Task{ID: 587, Name: "五一回忆上传", State: "WAIT", Process: tc.process, StepTypeSet: []string{"click", "upload"}}
			if tc.alreadyDone {
				state.State = "FINISH"
			}
			hashes := map[string]bool{}
			currentHash := ""
			job := taskStepTestJob(t, func(req *http.Request) string {
				switch {
				case strings.HasSuffix(req.URL.Path, "taskListV2"):
					return temporaryTaskJSON(state)
				case req.URL.Path == "/hcy/file/create":
					creates++
					if clicks == 0 {
						t.Fatal("photos uploaded before registering activity entry")
					}
					var body map[string]interface{}
					_ = json.NewDecoder(req.Body).Decode(&body)
					currentHash, _ = body["contentHash"].(string)
					if currentHash == "" || hashes[currentHash] || body["contentType"] != "image/jpeg" || !strings.HasSuffix(fmt.Sprint(body["name"]), ".jpg") {
						t.Fatal("photo metadata invalid or reused content hash")
					}
					hashes[currentHash] = true
					return fmt.Sprintf(`{"code":0,"data":{"fileId":"memory-%d","uploadId":"upload-%d","partInfos":[{"uploadUrl":"https://upload.example/photo"}]}}`, creates, creates)
				case req.URL.Host == "upload.example":
					puts++
					content, _ := io.ReadAll(req.Body)
					image, err := jpeg.Decode(bytes.NewReader(content))
					if req.Method != "PUT" || err != nil || image.Bounds().Dx() < 480 || image.Bounds().Dy() < 480 || fmt.Sprintf("%x", sha256.Sum256(content)) != currentHash {
						t.Fatal("uploaded content is not the advertised valid JPEG")
					}
					return ""
				case req.URL.Path == "/hcy/file/complete":
					if tc.fail {
						return `{"success":false,"code":"FAILED"}`
					}
					completed++
					if tc.confirm {
						state.Process++
					}
				case strings.HasSuffix(req.URL.Path, "doTaskPost"):
					posts++
				case strings.HasSuffix(req.URL.Path, "click"):
					clicks++
					if req.URL.Query().Get("id") != "587" || req.URL.Query().Get("key") != "task" {
						t.Fatal("incorrect temporary task registration")
					}
					if tc.confirm && state.Process == 10 && posts > 0 {
						state.State = "FINISH"
					}
				}
				return `{"code":0}`
			})
			handled, err := job.handleSpecialTask(taskListItem{MarketName: "sign_in_3", GroupKey: "beiyong1", Task: api.Task{ID: 587, Process: 0, State: "WAIT"}})
			if !handled || (err != nil) != tc.fail || creates != tc.wantUploads || puts != tc.wantUploads {
				t.Fatalf("handled=%v creates=%d puts=%d completed=%d err=%v", handled, creates, puts, completed, err)
			}
			if !tc.fail && job.Pending() != (!tc.confirm && !tc.alreadyDone) {
				t.Fatalf("pending=%v state=%+v", job.Pending(), state)
			}
			if tc.fail && posts > 0 {
				t.Fatal("failed upload registered as a completed action")
			}
			files, _ := LoadStringList(job.storage, KeyTempFiles)
			if len(files) != creates {
				t.Fatalf("cleanup ownership must include every created file: files=%v creates=%d", files, creates)
			}
			if tc.fail && len(files) != 1 {
				t.Fatalf("failed upload was not registered for cleanup: files=%v", files)
			}
		})
	}
}

func TestTravelTaskAcknowledgementCannotSubstituteForServerRecord(t *testing.T) {
	for _, finish := range []bool{false, true} {
		t.Run(fmt.Sprint(finish), func(t *testing.T) {
			chats, posts, clicks := 0, 0, 0
			state := api.Task{ID: 588, State: "WAIT", StepTypeSet: []string{"click", "ai"}}
			job := taskStepTestJob(t, func(req *http.Request) string {
				switch {
				case strings.HasSuffix(req.URL.Path, "taskListV2"):
					return temporaryTaskJSON(state)
				case strings.HasSuffix(req.URL.Path, "chat/v2/add"):
					chats++
					if clicks == 0 {
						t.Fatal("AI called before activity entry registration")
					}
					return "data: {\"code\":\"0000\"}\n\n"
				case strings.HasSuffix(req.URL.Path, "doTaskPost"):
					posts++
				case strings.HasSuffix(req.URL.Path, "click"):
					clicks++
					if chats > 0 && posts > 0 && finish {
						state.State = "FINISH"
					}
				}
				return `{"code":0}`
			})
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"guide-user"}}`))
			job.client.SetJWTToken("header." + claims + ".signature")
			handled, err := job.handleSpecialTask(taskListItem{MarketName: "sign_in_3", GroupKey: "beiyong1", Task: api.Task{ID: 588}})
			if !handled || err != nil || chats != 1 || job.Pending() == finish {
				t.Fatalf("handled=%v chats=%d pending=%v err=%v", handled, chats, job.Pending(), err)
			}
		})
	}
}

func TestTemporaryTaskConfirmationUsesV2EvenWhenV3HasOtherTasks(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		if strings.HasSuffix(req.URL.Path, "taskListV3") {
			return `{"code":0,"result":[]}`
		}
		if strings.HasSuffix(req.URL.Path, "taskListV2") {
			return `{"code":0,"result":{"time":[{"id":588,"state":"FINISH"}],"beiyong1":[{"id":588,"state":"FINISH"}]}}`
		}
		return `{"code":0}`
	})
	for _, group := range []string{"beiyong1", "time"} {
		state, err := job.queryTaskByMarket("sign_in_3", group, 588)
		if err != nil || state == nil || state.State != "FINISH" {
			t.Fatalf("group=%s state=%+v err=%v", group, state, err)
		}
	}
	if isTaskCompleted(api.Task{ID: 587, Status: 1, Process: 10}) || isTaskCompleted(api.Task{ID: 588, Status: 2}) {
		t.Fatal("legacy status/counter accepted without modern FINISH record")
	}
}

func TestTemporaryTaskDoesNotRunAfterDisableOrRecordDisappearance(t *testing.T) {
	for _, disabled := range []bool{true, false} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			actions, reads := 0, 0
			job := taskStepTestJob(t, func(req *http.Request) string {
				if strings.HasSuffix(req.URL.Path, "taskListV2") {
					reads++
					if !disabled && reads > 1 {
						return temporaryTaskJSON()
					}
					state := api.Task{ID: 588, State: "WAIT"}
					if disabled {
						off := 0
						state.Enable = &off
					}
					return temporaryTaskJSON(state)
				}
				return `{"code":0}`
			})
			err := job.executeTaskSteps(taskListItem{MarketName: "sign_in_3", GroupKey: "beiyong1", Task: api.Task{ID: 588}}, func() error { actions++; return nil })
			if disabled && (err != nil || actions != 0) {
				t.Fatalf("disabled task executed: actions=%d err=%v", actions, err)
			}
			if !disabled && (err == nil || actions != 1) {
				t.Fatalf("disappearing task accepted as complete: actions=%d err=%v", actions, err)
			}
		})
	}
}

func TestTemporaryActionsStayOutOfGPAndEmailChannels(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		if strings.HasSuffix(req.URL.Path, "taskListV2") {
			t.Fatal("GP client queried the regular temporary task group")
		}
		if strings.HasSuffix(req.URL.Path, "taskListV3") {
			return taskStepJSON(api.Task{ID: 588, State: "WAIT"})
		}
		return `{"code":0}`
	})
	job.gp = true
	job.api = api.NewGPSignInAPI(job.client)
	if _, err := job.fetchAllTaskItems(); err != nil {
		t.Fatal(err)
	}
	for _, market := range []string{"sign_in_3", "newsign_139mail"} {
		if handled, err := job.handleSpecialTask(taskListItem{MarketName: market, Task: api.Task{ID: 588}}); handled || err != nil {
			t.Fatal("regular travel activity executed in GP channel")
		}
	}
	job.gp = false
	if handled, err := job.handleSpecialTask(taskListItem{MarketName: "newsign_139mail", Task: api.Task{ID: 587}}); handled || err != nil {
		t.Fatal("regular memory activity executed in email channel")
	}
}

func TestTemporaryTasksDoNotClaimRewardsFromLegacyStatusAlone(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		switch {
		case strings.HasSuffix(req.URL.Path, "taskListV3"):
			var body struct {
				Market string `json:"marketname"`
			}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body.Market == "newsign_139mail" {
				return `{"code":0,"result":[]}`
			}
			return `{"code":0,"result":[{"id":587,"status":1,"process":10},{"id":588,"status":1}]}`
		case strings.HasSuffix(req.URL.Path, "taskListV2"):
			return emptyTemporaryGroupReply(t, req)
		case strings.HasSuffix(req.URL.Path, "infoV3"):
			return `{"code":0,"result":{"total":100,"toReceive":0,"receiveList":[]}}`
		case strings.HasSuffix(req.URL.Path, "receiveTask"), strings.HasSuffix(req.URL.Path, "receiveV3"):
			t.Fatal("temporary task claimed a reward without a server reward record")
		}
		return `{"code":0}`
	})
	if err := job.receiveCompletedTaskRewards(); err != nil || job.ClaimedCloud() != 0 {
		t.Fatalf("claimed=%d err=%v", job.ClaimedCloud(), err)
	}
}
