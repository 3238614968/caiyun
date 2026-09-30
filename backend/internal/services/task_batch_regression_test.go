package services

import (
	"caiyun/internal/models"
	"context"
	"testing"
)

func TestEmptyConfiguredBatchDoesNotFallBackToDefaults(t *testing.T) {
	configs := []*models.TaskConfig{nil, {TaskType: "signin", IsEnabled: false, RunInBatch: true}, {TaskType: "makewish_exchange", IsEnabled: true, RunInBatch: false}}
	codes := NewTaskCatalog().ResolveBatchCodes(configs)
	if codes == nil || len(codes) != 0 {
		t.Fatalf("explicitly empty batch=%v", codes)
	}
	if results := (&TaskRunner{}).RunSelectedContext(context.Background(), codes); len(results) != 0 {
		t.Fatalf("empty batch ran tasks: %v", results)
	}
	service := &TaskService{}
	if results, err := service.executeTaskCodesForAccount(context.Background(), nil, codes); err != nil || len(results) != 0 {
		t.Fatalf("empty batch touched an account: results=%v err=%v", results, err)
	}
}

func TestLegacyBatchNeverIncludesManualExchange(t *testing.T) {
	configs := []*models.TaskConfig{{TaskType: "makewish_exchange", IsEnabled: true}, {TaskType: "ai_store", IsEnabled: true}, nil, {TaskType: "signin", IsEnabled: true}}
	codes := resolveLegacyEnabledCodes(configs)
	if len(codes) != 1 || codes[0] != "signin" {
		t.Fatalf("legacy batch included manual task: %v", codes)
	}
}
