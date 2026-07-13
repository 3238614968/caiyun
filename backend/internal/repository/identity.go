package repository

import (
	"errors"
	"fmt"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

var (
	// ErrDuplicateUsername and ErrDuplicateEmail are stable repository-domain
	// errors. They intentionally hide driver messages while remaining usable
	// with errors.Is at the service boundary.
	ErrDuplicateUsername = errors.New("normalized username already exists")
	ErrDuplicateEmail    = errors.New("normalized email already exists")
	ErrDuplicateIdentity = errors.New("normalized identity already exists")
)

// NormalizeUsername makes username identity comparison independent from the
// database collation while preserving the display value on models.User.Username.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// NormalizeEmail trims surrounding whitespace and lower-cases only the domain
// part. The local part is deliberately preserved. Empty email addresses are
// represented by nil so a unique index permits multiple users without email.
func NormalizeEmail(email string) *string {
	trimmed := strings.TrimSpace(email)
	if trimmed == "" {
		return nil
	}
	if at := strings.LastIndexByte(trimmed, '@'); at >= 0 {
		trimmed = trimmed[:at+1] + strings.ToLower(trimmed[at+1:])
	}
	return &trimmed
}

func mapUserIdentityWriteError(err error) error {
	if err == nil {
		return nil
	}

	message := strings.ToLower(err.Error())
	var mysqlErr *mysqlDriver.MySQLError
	isDuplicate := errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
	isDuplicate = isDuplicate || errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(message, "unique constraint failed")
	if !isDuplicate {
		return err
	}

	switch {
	case strings.Contains(message, "normalized_email"), strings.Contains(message, "uk_users_normalized_email"):
		return fmt.Errorf("%w: %v", ErrDuplicateEmail, err)
	case strings.Contains(message, "normalized_username"), strings.Contains(message, "uk_users_normalized_username"), strings.Contains(message, "users.username"):
		return fmt.Errorf("%w: %v", ErrDuplicateUsername, err)
	default:
		return fmt.Errorf("%w: %v", ErrDuplicateIdentity, err)
	}
}
