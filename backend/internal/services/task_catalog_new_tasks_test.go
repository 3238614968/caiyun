package services

import "testing"

func TestNewTaskCatalogDefaultsForRewardAndExchange(t *testing.T) {
	catalog := NewTaskCatalog()
	hidden, ok := catalog.Get("hidden_rewards")
	if !ok || !hidden.DefaultEnabled || !hidden.RunInBatch {
		t.Fatalf("hidden rewards definition = %+v, found=%v", hidden, ok)
	}
	exchange, ok := catalog.Get("makewish_exchange")
	if !ok || exchange.DefaultEnabled || exchange.RunInBatch {
		t.Fatalf("AI bean exchange definition = %+v, found=%v", exchange, ok)
	}
}

func TestRedepositTaskIsRegisteredForBatchExecution(t *testing.T) {
	catalog := NewTaskCatalog()
	def, ok := catalog.Get("redeposit")
	if !ok || def.execute == nil || !def.DefaultEnabled || !def.RunInBatch {
		t.Fatalf("redeposit definition = %+v, found=%v", def, ok)
	}
	batch := map[string]bool{}
	for _, code := range catalog.DefaultBatchCodes() {
		batch[code] = true
	}
	if !batch["redeposit"] {
		t.Fatal("redeposit missing from default batch")
	}
	if catalog.Normalize("save_link") != "redeposit" {
		t.Fatal("save_link alias did not normalize to redeposit")
	}
}
