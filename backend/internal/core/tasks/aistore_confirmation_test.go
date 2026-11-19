package tasks

import (
	"net/http"
	"strings"
	"testing"
)

func TestAIStoreDoesNotConfirmWorkFromSaveTaskAcknowledgement(t *testing.T) {
	for _, response := range []string{`{"code":0}`, `{"code":0,"body":{"code":"30101","msg":"云盘目录创建失败"}}`} {
		t.Run(response, func(t *testing.T) {
			t.Setenv("CAIYUN_AISTORE_SAVE_MODULE_13800000001", "14")
			job := taskStepTestJob(t, func(req *http.Request) string {
				if strings.HasSuffix(req.URL.Path, "/accredit/get") {
					return `{"code":0,"result":{"data":{"path":"/task-tool"}}}`
				}
				if strings.HasSuffix(req.URL.Path, "/createUploadTask") {
					return response
				}
				t.Fatalf("unexpected %s", req.URL)
				return ""
			})
			task := NewAIStoreTask(job.client, job.logger)
			task.SetAccountPhone("13800000001")
			if err := task.Run(); err != nil || !task.Pending() || strings.Contains(task.Message(), "作品已保存") {
				t.Fatalf("pending=%v message=%s err=%v", task.Pending(), task.Message(), err)
			}
		})
	}
}
