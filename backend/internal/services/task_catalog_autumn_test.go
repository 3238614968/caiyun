package services

import "testing"

func TestAutumnTasksHaveBatchAndExecutionEntries(t *testing.T) {
	c := NewTaskCatalog()
	batch := map[string]bool{}
	for _, code := range c.DefaultBatchCodes() {
		batch[code] = true
	}
	for _, code := range []string{"annual_bill", "luckybag", "redcavalry", "fun_ai_timed_lottery", "tasklist_gp"} {
		d, ok := c.Get(code)
		if !ok || d.execute == nil || !d.DefaultEnabled || !batch[code] {
			t.Fatalf("unwired task %s", code)
		}
	}
	if batch["makewish_exchange"] {
		t.Fatal("optional spending enabled by activity rollout")
	}
	if def, ok := c.Get("redeem_code"); !ok || def.execute == nil || def.DefaultEnabled || batch["redeem_code"] {
		t.Fatal("redeem codes must be explicitly configured")
	}
}
