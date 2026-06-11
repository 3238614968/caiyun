package tasks

import (
	"reflect"
	"testing"

	"caiyun/internal/core/api"
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
			name: "legacy market keeps normal click fallback",
			in:   api.Task{ID: 1004, MarketName: "newsign_139mail"},
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
