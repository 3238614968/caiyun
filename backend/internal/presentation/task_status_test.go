package presentation

import "testing"

func TestSavedTaskLogCannotClaimThatAWorkerIsStillRunning(t *testing.T) {
	for _, status := range []string{"pending", "running", "queued", "processing", ""} {
		if TaskLogStatus(status) != "pending" || TaskLogMessage("tasklist", status, "执行中 | build=fixture") != "执行已结束，结果待确认" {
			t.Fatalf("saved %q outcome still looks active", status)
		}
	}
	if TaskLogStatus("failed") != "failed" || TaskLogStatus("success") != "success" || TaskLogStatus("skipped") != "skipped" {
		t.Fatal("terminal outcome changed")
	}
	if got := TaskLogMessage("tasklist", "pending", "已确认领取11云朵，部分任务等待服务端确认"); got != "已确认领取11云朵，部分任务等待服务端确认" {
		t.Fatal("partial reward confirmation was lost")
	}
}
