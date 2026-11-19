package tasks

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFunAIKnowledgeRegistersBothEntriesBeforeOneActionAndUsesEachServerRecord(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		main          int
		timed         int
		confirmedRead int
		missingTimed  bool
		wantNew       int
		wantPending   bool
		wantWaits     int
	}{
		{"both confirmed", 1, 1, 2, false, 2, false, 1},
		{"only main confirmed", 1, 0, 2, false, 1, true, 6},
		{"acknowledgements alone", 0, 0, 2, false, 0, true, 6},
		{"delayed credit", 1, 1, 4, false, 2, false, 3},
		{"missing record", 1, 0, 2, true, 1, true, 6},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			entries := map[string]int{}
			creates, events, moduleReads, waits := 0, 0, 0, 0
			job := taskStepTestJob(t, func(req *http.Request) string {
				switch {
				case strings.HasPrefix(req.URL.Path, "/deviceprofile/"), strings.HasPrefix(req.URL.Path, "/portal/"):
					return `{"code":1}`
				case req.URL.Path == "/ycloud/funai/api/page/getmodule":
					moduleReads++
					complete := 0
					if creates > 0 && moduleReads >= scenario.confirmedRead {
						complete = scenario.main
					}
					return fmt.Sprintf(`{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"zhishiku","taskSign":0,"isComplete":%d}]}}`, complete)
				case req.URL.Path == "/ycloud/funai/api/timed/getmodule":
					if creates > 0 && scenario.missingTimed {
						return `{"code":0,"result":[]}`
					}
					complete := 0
					if creates > 0 && moduleReads >= scenario.confirmedRead {
						complete = scenario.timed
					}
					return fmt.Sprintf(`{"code":0,"result":[{"moduleId":"timed","taskIds":[{"id":"zhishiku","taskSign":0,"isComplete":%d}]}]}`, complete)
				case req.URL.Path == "/ycloud/funai/api/page/register":
					if creates > 0 || events > 0 {
						t.Fatal("entry registered after the knowledge action")
					}
					var body map[string]string
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body["taskId"] != "zhishiku" {
						t.Fatal("invalid knowledge entry")
					}
					entries[body["moduleId"]]++
					return `{"code":0,"result":0}`
				case req.URL.Hostname() == "dc.cmicapm.com":
					if entries["main"] != 1 || entries["timed"] != 1 || creates != 0 {
						t.Fatal("event outside the real creation flow")
					}
					events++
					return `{"e":0,"web_id":"fixture-web-id"}`
				case strings.HasPrefix(req.URL.Path, "/api/outer/assistant/"):
					if entries["main"] != 1 || entries["timed"] != 1 {
						t.Fatal("knowledge action before both module entries")
					}
					switch req.URL.Path {
					case "/api/outer/assistant/knowledge/personal/v2/base/list":
						return `{"code":"0000","data":{"baseList":[]}}`
					case "/api/outer/assistant/knowledge/personal/v2/base/profilePhoto/list":
						return `{"code":"0000","data":[{"photoId":"official-default","type":1}]}`
					case "/api/outer/assistant/knowledge/personal/v2/base/create":
						if events != 2 {
							t.Fatal("missing official attribution")
						}
						creates++
						return `{"code":"0000","data":{"base":{"baseId":"owned"}}}`
					}
				case strings.HasSuffix(req.URL.Path, "/lottery"), strings.HasSuffix(req.URL.Path, "/zhangtingLottery"):
					return `{"code":500,"msg":"没有抽奖次数"}`
				}
				t.Fatalf("unexpected knowledge request %s", req.URL.Path)
				return ""
			})
			job.client.SetMarketAccount("13800000000")
			job.client.SetJWTToken("h." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture"}}`)) + ".s")
			task := NewFunAITask(job.client, job.logger)
			task.wait = func(time.Duration) { waits++ }
			if err := task.Run(); err != nil {
				t.Fatal(err)
			}
			if creates != 1 || events != 2 || waits != scenario.wantWaits || task.Pending() != scenario.wantPending || !strings.Contains(task.Message(), fmt.Sprintf("服务端确认新完成%d项", scenario.wantNew)) {
				t.Fatalf("creates=%d events=%d waits=%d pending=%v message=%s", creates, events, waits, task.Pending(), task.Message())
			}
		})
	}
}

func TestFunAIKnowledgeEntryFailureDoesNotCreateLibrary(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		switch {
		case strings.HasPrefix(req.URL.Path, "/deviceprofile/"), strings.HasPrefix(req.URL.Path, "/portal/"):
			return `{"code":1}`
		case req.URL.Path == "/ycloud/funai/api/page/getmodule":
			return `{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"zhishiku","isComplete":0}]}}`
		case req.URL.Path == "/ycloud/funai/api/timed/getmodule":
			return `{"code":0,"result":[]}`
		case req.URL.Path == "/ycloud/funai/api/page/register":
			return `{"code":403,"msg":"入口登记失败"}`
		case strings.HasSuffix(req.URL.Path, "/lottery"), strings.HasSuffix(req.URL.Path, "/zhangtingLottery"):
			return `{"code":500,"msg":"没有抽奖次数"}`
		default:
			t.Fatalf("failed entry must not create or use a library: %s", req.URL.Path)
			return ""
		}
	})
	task := NewFunAITask(job.client, job.logger)
	task.wait = func(time.Duration) {}
	if err := task.Run(); err == nil || !strings.Contains(err.Error(), "入口登记失败") {
		t.Fatalf("entry error = %v", err)
	}
}

func TestFunAIKnowledgeAlreadyCompleteDoesNotRegisterReportOrCreate(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		switch {
		case strings.HasPrefix(req.URL.Path, "/deviceprofile/"), strings.HasPrefix(req.URL.Path, "/portal/"):
			return `{"code":1}`
		case req.URL.Path == "/ycloud/funai/api/page/getmodule":
			return `{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"zhishiku","isComplete":1}]}}`
		case req.URL.Path == "/ycloud/funai/api/timed/getmodule":
			return `{"code":0,"result":[{"moduleId":"timed","taskIds":[{"id":"zhishiku","isComplete":1}]}]}`
		case strings.HasSuffix(req.URL.Path, "/lottery"), strings.HasSuffix(req.URL.Path, "/zhangtingLottery"):
			return `{"code":500,"msg":"没有抽奖次数"}`
		default:
			t.Fatalf("completed knowledge must have no side effects: %s", req.URL)
			return ""
		}
	})
	task := NewFunAITask(job.client, job.logger)
	task.wait = func(time.Duration) {}
	if err := task.Run(); err != nil || task.Pending() {
		t.Fatalf("completed knowledge: pending=%v err=%v", task.Pending(), err)
	}
}
