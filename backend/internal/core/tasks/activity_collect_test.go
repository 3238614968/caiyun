package tasks

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestActivityCollectionUsesCurrentIDsAndNeverCountsRejectedRequests(t *testing.T) {
	collected := map[string]int{}
	job := taskStepTestJob(t, func(req *http.Request) string {
		if req.URL.Path == "/ycloud/advert/getAdvertInfo" {
			return `{"code":0,"result":[{"id":1},{"id":"1"},{"id":2}]}`
		}
		if req.URL.Path != "/ycloud/api/activity/collect" || req.Method != "GET" {
			t.Fatalf("unexpected %s", req.URL)
		}
		id := req.URL.Query().Get("advertId")
		collected[id]++
		if id == "2" {
			return `{"code":90001,"msg":"未登录"}`
		}
		return `{"code":0}`
	})
	task := NewActivityCollectTask(job.client)
	task.wait = func(time.Duration) {}
	err := task.Run()
	if err == nil || !strings.Contains(err.Error(), "90001") || collected["1"] != 1 || collected["2"] != 1 || !strings.Contains(task.Message(), "收藏1/2项") {
		t.Fatalf("collected=%v message=%s err=%v", collected, task.Message(), err)
	}
}
