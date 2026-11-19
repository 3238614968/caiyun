package tasks

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"caiyun/internal/core/api"
)

func TestFunAINineGridRegistersBothModulesBeforeOneSharedAction(t *testing.T) {
	entries := map[string]int{}
	actions := 0
	job := taskStepTestJob(t, func(req *http.Request) string {
		switch {
		case strings.HasPrefix(req.URL.Path, "/deviceprofile/"), strings.HasPrefix(req.URL.Path, "/portal/"):
			return `{"code":1}`
		case req.URL.Path == "/ycloud/funai/api/page/getmodule":
			return `{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"aijiugongge","isComplete":0}]}}`
		case req.URL.Path == "/ycloud/funai/api/timed/getmodule":
			return `{"code":0,"result":[{"moduleId":"timed","taskIds":[{"id":"aijiugongge","isComplete":0}]}]}`
		case req.URL.Path == "/ycloud/funai/api/page/register":
			if actions != 0 {
				t.Fatal("module entry registered after the artwork action")
			}
			var body map[string]string
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["taskId"] != "aijiugongge" {
				t.Fatal("wrong entry")
			}
			entries[body["moduleId"]]++
			return `{"code":0,"result":0}`
		case req.URL.Path == "/api/outer/assistant/accredit/get":
			if entries["main"] != 1 || entries["timed"] != 1 {
				t.Fatal("artwork started before both entries were registered")
			}
			actions++
			return `{"code":0,"data":{"authStatus":0}}`
		case strings.HasSuffix(req.URL.Path, "/lottery"), strings.HasSuffix(req.URL.Path, "/zhangtingLottery"):
			return `{"code":500,"msg":"没有抽奖次数"}`
		default:
			t.Fatalf("unexpected ninegrid action: %s", req.URL.Path)
			return ""
		}
	})
	job.client.SetJWTToken("h." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture"}}`)) + ".s")
	task := NewFunAITask(job.client, job.logger)
	task.wait = func(time.Duration) {}
	if err := task.Run(); err == nil || actions != 1 || entries["main"] != 1 || entries["timed"] != 1 || !strings.Contains(task.Message(), "服务端确认新完成0项") {
		t.Fatalf("actions=%d entries=%v pending=%v err=%v", actions, entries, task.Pending(), err)
	}
}

func TestFunAINineGridNeedsEachModulesCompletionRecord(t *testing.T) {
	attempted := map[string]bool{"main/aijiugongge": true, "timed/aijiugongge": true}
	modules := []api.FunaiModule{{ModuleID: "main", TaskIDs: []api.FunaiTask{{ID: "aijiugongge", IsComplete: 1}}}}
	if !pendingFunAIRecordedActions(attempted, modules) {
		t.Fatal("missing timed record treated as completed")
	}
	modules = append(modules, api.FunaiModule{ModuleID: "timed", TaskIDs: []api.FunaiTask{{ID: "aijiugongge", IsComplete: 0}}})
	if !pendingFunAIRecordedActions(attempted, modules) {
		t.Fatal("unset timed completion treated as completed")
	}
	modules[1].TaskIDs[0].IsComplete = 1
	if pendingFunAIRecordedActions(attempted, modules) {
		t.Fatal("both confirmed records did not settle")
	}
}

func TestFunAIDoesNotInventNineGridEntryWhenServerNoLongerListsIt(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		switch {
		case strings.HasPrefix(req.URL.Path, "/deviceprofile/"), strings.HasPrefix(req.URL.Path, "/portal/"):
			return `{"code":1}`
		case req.URL.Path == "/ycloud/funai/api/page/getmodule":
			return `{"code":0,"result":{"moduleId":"1-250730","taskIds":[{"id":"zhishiku","isComplete":1}]}}`
		case req.URL.Path == "/ycloud/funai/api/timed/getmodule":
			return `{"code":0,"result":[{"moduleId":"2-261104-3","taskIds":[{"id":"zhishiku","isComplete":1}]}]}`
		case strings.HasSuffix(req.URL.Path, "/lottery"), strings.HasSuffix(req.URL.Path, "/zhangtingLottery"):
			return `{"code":500,"msg":"没有抽奖次数"}`
		default:
			t.Fatalf("unlisted ninegrid task caused a registration, file creation or event: %s", req.URL.Path)
			return ""
		}
	})
	task := NewFunAITask(job.client, job.logger)
	if err := task.Run(); err != nil || !strings.Contains(task.Message(), "服务端确认新完成0项") {
		t.Fatalf("message=%s err=%v", task.Message(), err)
	}
}
