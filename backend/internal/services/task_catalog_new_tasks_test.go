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
