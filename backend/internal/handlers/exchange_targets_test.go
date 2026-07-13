package handlers

import (
	"reflect"
	"testing"
)

func TestCreateExchangeTaskRequestNormalizedExchangeRuleIDs(t *testing.T) {
	req := CreateExchangeTaskRequest{
		ExchangeRuleID:     11,
		ExchangeRuleIDs:    []uint{3, 5},
		ExchangeAccountID:  5,
		ExchangeAccountIDs: []uint{7, 11, 0},
		AccountID:          9,
		AccountIDs:         []uint{9, 12, 12},
	}

	if got, want := req.NormalizedExchangeRuleIDs(), []uint{3, 5, 7, 11}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizedExchangeRuleIDs() = %v, want %v", got, want)
	}

	if got, want := req.NormalizedAccountIDs(), []uint{9, 12}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizedAccountIDs() = %v, want %v", got, want)
	}
}

func TestImmediateExchangeRequestNormalizedExchangeRuleID(t *testing.T) {
	t.Run("prefer new exchange_rule_id", func(t *testing.T) {
		req := ImmediateExchangeRequest{ExchangeRuleID: 8, ExchangeAccountID: 2}
		if got := req.NormalizedExchangeRuleID(); got != 8 {
			t.Fatalf("NormalizedExchangeRuleID() = %d, want 8", got)
		}
	})

	t.Run("fallback to legacy exchange_account_id", func(t *testing.T) {
		req := ImmediateExchangeRequest{ExchangeAccountID: 6}
		if got := req.NormalizedExchangeRuleID(); got != 6 {
			t.Fatalf("NormalizedExchangeRuleID() = %d, want 6", got)
		}
	})
}
