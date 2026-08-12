package dto

import (
	"caiyun/internal/models"
	"encoding/json"
	"strings"
	"testing"
)

func TestNestedDTOsDoNotLeakCredentialOrExecutionOwnership(t *testing.T) {
	rule := models.ExchangeRule{ID: 2, Auth: "Basic credential", Token: "secret-token", JWTToken: "secret-jwt", Account: models.Account{Auth: "nested-auth", Token: "nested-token"}}
	task := &models.ExchangeTask{ID: 3, ExecutionToken: "lease-token", SourceOperationID: stringPtr("operation"), ExchangeAccount: rule}
	record := &models.ExchangeRecord{ID: 4, ExchangeAccount: &rule}
	for name, value := range map[string]any{"rule": ToExchangeRuleResponse(&rule), "task": ToExchangeTaskResponse(task), "record": ToExchangeRecordResponse(record)} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s marshal: %v", name, err)
		}
		lower := strings.ToLower(string(raw))
		for _, forbidden := range []string{"credential", "secret", "lease-token", "source_operation", "execution_token", "auth", "jwt_token"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s leaked %q: %s", name, forbidden, raw)
			}
		}
	}
}
func stringPtr(value string) *string { return &value }
