package repository

import (
	"errors"
	"fmt"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var (
	ErrDuplicateActiveExchangeTask = errors.New("duplicate active exchange task")
	ErrDuplicateExchangeRule       = errors.New("duplicate active exchange rule")
)

func mapExchangeTaskWriteError(err error) error {
	if !isDatabaseDuplicate(err) {
		return err
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "active_dedupe_key") || strings.Contains(message, "uk_exchange_tasks_active_dedupe") || errors.Is(err, gorm.ErrDuplicatedKey) {
		return fmt.Errorf("%w: %v", ErrDuplicateActiveExchangeTask, err)
	}
	return err
}

func mapExchangeRuleWriteError(err error) error {
	if !isDatabaseDuplicate(err) {
		return err
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "active_account_id") || strings.Contains(message, "uk_exchange_rules_active_account") || errors.Is(err, gorm.ErrDuplicatedKey) {
		return fmt.Errorf("%w: %v", ErrDuplicateExchangeRule, err)
	}
	return err
}

func isDatabaseDuplicate(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return true
	}
	message := strings.ToLower(err.Error())
	return errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(message, "unique constraint failed")
}
