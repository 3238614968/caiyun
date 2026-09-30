package tasks

import (
	"testing"

	"caiyun/internal/core/api"
)

func TestPosterTaskFinishedUsesServerState(t *testing.T) {
	items := []api.ActivityTask{{ID: 33, State: "WAIT"}, {ID: 34, State: "FINISH"}}
	if posterTaskFinished(items, 33) || !posterTaskFinished(items, 34) || posterTaskFinished(items, 35) {
		t.Fatalf("poster completion check misread activity states: %+v", items)
	}
}
