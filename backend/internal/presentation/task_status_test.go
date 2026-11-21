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

func TestGPLoginFailureShowsOnlyBusinessCodeInTaskLogs(t *testing.T) {
	const raw = "获取 gp 域 JWT 失败: gp.mcloud.139.com 登录失败: code=200050409 token=fixture | body=private | build=fixture"
	const want = "GP 渠道登录失败（上游业务码 200050409），本次任务未执行"
	for _, taskType := range []string{"tasklist_gp", "annual_bill", "luckybag"} {
		if got := TaskLogMessage(taskType, "failed", raw); got != want {
			t.Errorf("%s log = %q, want %q", taskType, got, want)
		}
	}
	if got := TaskLogMessage("tasklist", "failed", raw); got != "获取 gp 域 JWT 失败: gp.mcloud.139.com 登录失败:" {
		t.Errorf("unrelated task log retained diagnostics: %q", got)
	}
}
