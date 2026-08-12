package repository

import (
	"errors"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestMapExchangeRuleWriteErrorMapsActiveRuleUniqueConstraint(t *testing.T) {
	err := mapExchangeRuleWriteError(&mysqlDriver.MySQLError{
		Number:  1062,
		Message: "Duplicate entry '7' for key 'uk_exchange_rules_active_account'",
	})
	if !errors.Is(err, ErrDuplicateExchangeRule) {
		t.Fatalf("mapExchangeRuleWriteError() = %v, want ErrDuplicateExchangeRule", err)
	}
}
