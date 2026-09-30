package tasks

import (
	"reflect"
	"testing"

	"caiyun/internal/core/api"
	"caiyun/internal/core/logger"
)

func TestTaskListClickKeys(t *testing.T) {
	task := &TaskListTask{}
	tests := []struct {
		name string
		in   api.Task
		want []string
	}{
		{
			name: "fixed entry task clicks both steps initially",
			in:   api.Task{ID: 409, MarketName: "sign_in_3", CurrStep: 0},
			want: []string{"task", "task2"},
		},
		{
			name: "fixed entry task clicks second step after progress",
			in:   api.Task{ID: 409, MarketName: "sign_in_3", CurrStep: 1},
			want: []string{"task2"},
		},
		{
			name: "random cloud task uses dedicated key initially",
			in:   api.Task{ID: 478, MarketName: "sign_in_3", CurrStep: 0},
			want: []string{"randomCloudTask"},
		},
		{
			name: "random cloud task falls back to normal key after progress",
			in:   api.Task{ID: 478, MarketName: "sign_in_3", CurrStep: 1},
			want: []string{"task"},
		},
		{
			name: "new task center click step uses normal key",
			in:   api.Task{ID: 500, MarketName: "sign_in_3", CurrStep: 0, StepTypeSet: []string{"click"}},
			want: []string{"task"},
		},
		{
			name: "new task center without click step is skipped",
			in:   api.Task{ID: 501, MarketName: "sign_in_3"},
			want: nil,
		},
		{
			name: "mail task without a known click step is not blindly clicked",
			in:   api.Task{ID: 1004, MarketName: "newsign_139mail"},
			want: nil,
		},
		{
			name: "mail click-only task can be registered",
			in:   api.Task{ID: 1005, MarketName: "newsign_139mail", StepTypeSet: []string{"click"}},
			want: []string{"task"},
		},
		{
			name: "click-only task can finish a later step",
			in:   api.Task{ID: 1005, MarketName: "newsign_139mail", CurrStep: 1, StepTypeSet: []string{"click"}},
			want: []string{"task"},
		},
		{
			name: "mail task with an external email step is skipped",
			in:   api.Task{ID: 1004, MarketName: "newsign_139mail", StepTypeSet: []string{"click", "email"}},
			want: nil,
		},
		{
			name: "monthly backup reward waits for availability",
			in: api.Task{ID: 549, MarketName: "sign_in_3", StepTypeSet: []string{"click"},
				Button: map[string]map[string]interface{}{"app": {"canReceive": float64(0)}}},
			want: nil,
		},
		{
			name: "monthly backup reward is clicked when available",
			in: api.Task{ID: 549, MarketName: "sign_in_3", StepTypeSet: []string{"click"},
				Button: map[string]map[string]interface{}{"app": {"canReceive": float64(1)}}},
			want: []string{"task"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := task.getTaskClickKeys(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("getTaskClickKeys() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestTaskActionRegistrationKeysFollowChannelRules(t *testing.T) {
	for _, test := range []struct {
		id       int
		currStep int
		want     []string
	}{
		{id: 1003, want: []string{"openAppPush"}},
		{id: 1005, want: []string{"task2"}},
		{id: 409, want: []string{"task", "task2"}},
		{id: 409, currStep: 1, want: []string{"task2"}},
		{id: 615, want: []string{"task"}},
	} {
		got := taskActionRegistrationKeys(api.Task{ID: test.id, CurrStep: test.currStep})
		if !reflect.DeepEqual(got, test.want) {
			t.Fatalf("task %d step %d keys=%v want %v", test.id, test.currStep, got, test.want)
		}
	}
}

func TestSpecialTaskActionFailureIsNotReportedAsSuccess(t *testing.T) {
	task := &TaskListTask{logger: logger.NewLogger(logger.LevelInfo)}
	handled, err := task.handleSpecialTask(taskListItem{MarketName: "sign_in_3", Task: api.Task{ID: 106, Name: "上传文件"}})
	if !handled || err == nil {
		t.Fatalf("upload without FileAPI = handled %t, err %v; want handled failure", handled, err)
	}
}

func TestV3StateOverridesLegacyStatus(t *testing.T) {
	if isTaskCompleted(api.Task{State: "WAIT", Status: 1}) {
		t.Fatal("V3 WAIT task must not be treated as completed by legacy status")
	}
	if !isTaskCompleted(api.Task{State: "FINISH", Status: 0}) {
		t.Fatal("V3 FINISH task must be treated as completed")
	}
}

func TestTaskEnableMissingIsNotDisabled(t *testing.T) {
	zero, one := 0, 1
	for _, test := range []struct {
		task api.Task
		want bool
	}{
		{task: api.Task{State: "WAIT"}, want: false},
		{task: api.Task{State: "WAIT", Enable: &zero}, want: true},
		{task: api.Task{State: "WAIT", Enable: &one}, want: false},
	} {
		if got := isTaskDisabledByServer(test.task); got != test.want {
			t.Fatalf("isTaskDisabledByServer(%+v) = %t, want %t", test.task, got, test.want)
		}
	}
}

func TestV3RewardRequiresFinishedClaimableButton(t *testing.T) {
	claimable := api.Task{State: "FINISH", Button: map[string]map[string]interface{}{
		"app": {"canReceive": float64(1)},
	}}
	if !taskHasClaimableReward(claimable) {
		t.Fatal("finished V3 task with canReceive=1 should be claimed")
	}
	claimable.State = "WAIT"
	if taskHasClaimableReward(claimable) {
		t.Fatal("unfinished task should not claim a reward")
	}
	claimable.State = "FINISH"
	claimable.Button["app"]["canReceive"] = float64(0)
	if taskHasClaimableReward(claimable) {
		t.Fatal("button with canReceive=0 should not claim a reward")
	}
}
