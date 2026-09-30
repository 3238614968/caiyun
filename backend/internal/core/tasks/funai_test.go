package tasks

import (
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
