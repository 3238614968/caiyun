package tasks

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
)

func TestCountNewFunAICompletionsUsesModuleAndTaskID(t *testing.T) {
	before := []api.FunaiModule{
		{ModuleID: "main", TaskIDs: []api.FunaiTask{{ID: "003"}, {ID: "aizhushou", IsComplete: 1}}},
		{ModuleID: "timed", TaskIDs: []api.FunaiTask{{ID: "003"}}},
	}
	after := []api.FunaiModule{
		{ModuleID: "main", TaskIDs: []api.FunaiTask{{ID: "003", IsComplete: 1}, {ID: "aizhushou", IsComplete: 1}}},
		{ModuleID: "timed", TaskIDs: []api.FunaiTask{{ID: "003"}}},
	}
	if got := countNewFunAICompletions(before, after); got != 1 {
		t.Fatalf("newly completed count = %d, want 1", got)
	}
}

func TestFunAIImageIsSubmittedOnceAcrossModulesAndReconcilesServerCompletion(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		t.Run(fmt.Sprint(confirmed), func(t *testing.T) {
			imageCalls := 0
			job := taskStepTestJob(t, func(req *http.Request) string {
				complete := 0
				if confirmed && imageCalls > 0 {
					complete = 1
				}
				switch {
				case req.URL.Path == "/deviceprofile/v4":
					return `{"code":1}`
				case strings.HasPrefix(req.URL.Path, "/portal/"):
					return "<html></html>"
				case req.URL.Path == "/ycloud/funai/api/page/getmodule":
					return fmt.Sprintf(`{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"3Drenou","isComplete":%d}]}}`, complete)
				case req.URL.Path == "/ycloud/funai/api/timed/getmodule":
					return fmt.Sprintf(`{"code":0,"result":[{"moduleId":"timed","taskIds":[{"id":"3Drenou","isComplete":%d}]}]}`, complete)
				case req.URL.Path == "/api/image/edit/faceAnime":
					imageCalls++
					return `{"code":10000112,"message":"rendering failed"}`
				case strings.HasSuffix(req.URL.Path, "/lottery"), strings.HasSuffix(req.URL.Path, "/zhangtingLottery"):
					return `{"code":500,"msg":"次数已用完"}`
				default:
					t.Fatalf("unexpected request %s", req.URL)
					return ""
				}
			})
			job.client.SetJWTToken("x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture-user"}}`)) + ".x")
			task := NewFunAITask(job.client, job.logger)
			err := task.Run()
			if imageCalls != 1 || (err == nil) != confirmed {
				t.Fatalf("imageCalls=%d confirmed=%v err=%v", imageCalls, confirmed, err)
			}
			if confirmed && !strings.Contains(task.Message(), "服务端确认新完成2项") {
				t.Fatal(task.Message())
			}
			if !confirmed && !strings.Contains(err.Error(), "10000112 rendering failed") {
				t.Fatal(err)
			}
		})
	}
}
