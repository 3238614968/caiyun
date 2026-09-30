package tasks

import (
	"testing"

	"caiyun/internal/core/api"
)

func activityTaskForTest(id int, state, platformExt string) api.ActivityTask {
	task := api.ActivityTask{ID: id, State: state}
	if platformExt != "" {
		task.Button = map[string]interface{}{
			"android": map[string]interface{}{"ext": platformExt},
		}
	}
	return task
}

func TestPlanTokenPKActions(t *testing.T) {
	tasks := []api.ActivityTask{
		activityTaskForTest(1, "FINISH", "backup"),
		activityTaskForTest(2, "SUCCESS", "uploadPhoto"),
		activityTaskForTest(3, "WAIT", "aiCamera"),
		activityTaskForTest(4, "WAIT", "aiAssistant"),
		activityTaskForTest(5, "WAIT", "loginPc"),
		activityTaskForTest(8, "WAIT", "reserveLogin"),
		activityTaskForTest(14, "ING", "someFutureKey"),
	}

	actions := planTokenPKActions(tasks)
	if len(actions) != 6 {
		t.Fatalf("planTokenPKActions() len = %d, want 6 (only the FINISH task is skipped)", len(actions))
	}

	byTask := map[int]tokenPKAction{}
	for _, action := range actions {
		byTask[action.Task.ID] = action
	}

	if by, ok := byTask[2]; !ok || by.Kind != tokenPKActionReceive {
		t.Fatalf("task 2 kind = %+v, want receive", by)
	}
	if by, ok := byTask[3]; !ok || by.Kind != tokenPKActionRun || by.Key != "aiCamera" {
		t.Fatalf("task 3 kind = %+v, want run/aiCamera", by)
	}
	if by, ok := byTask[5]; !ok || by.Kind != tokenPKActionRun || by.Key != "loginPc" {
		t.Fatalf("task 5 kind = %+v, want run/loginPc", by)
	}
	if by, ok := byTask[8]; !ok || by.Kind != tokenPKActionReserve {
		t.Fatalf("task 8 kind = %+v, want reserve", by)
	}
	if by, ok := byTask[14]; !ok || by.Kind != tokenPKActionManual {
		t.Fatalf("task 14 kind = %+v, want manual for unknown key", by)
	}
	if _, ok := byTask[1]; ok {
		t.Fatal("FINISH task must not appear in planned actions")
	}
}

func TestFormatTokenCount(t *testing.T) {
	cases := map[int64]string{
		999:        "999",
		500000:     "50万",
		2200648000: "22.0亿",
	}
	for input, want := range cases {
		if got := formatTokenCount(input); got != want {
			t.Fatalf("formatTokenCount(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestActivityTaskPrizeSummary(t *testing.T) {
	task := api.ActivityTask{Prizes: []api.ActivityTaskPrize{
		{PrizeName: "Token+50万"},
		{PrizeName: "Token+100万"},
		{PrizeName: "  "},
	}}
	if got := task.PrizeSummary(); got != "Token+50万、Token+100万" {
		t.Fatalf("PrizeSummary() = %q", got)
	}
}

func TestTokenPKMonthlyCapSkipsActionsButKeepsClaimableRewards(t *testing.T) {
	blocked := activityTaskForTest(3, "WAIT", "aiCamera")
	blocked.MonthlyCompleted = 3
	blocked.MonthlyLimit = 3
	claimable := blocked
	claimable.ID = 9
	claimable.State = "SUCCESS"
	actions := planTokenPKActions([]api.ActivityTask{blocked, claimable})
	if len(actions) != 1 || actions[0].Task.ID != 9 || actions[0].Kind != tokenPKActionReceive {
		t.Fatalf("monthly cap actions=%+v", actions)
	}
}
