package repository

import (
	"errors"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestNormalizeIdentity(t *testing.T) {
	if got := NormalizeUsername("  Alice.Admin  "); got != "alice.admin" {
		t.Fatalf("NormalizeUsername() = %q", got)
	}
	if got := NormalizeEmail("  Local.Part@EXAMPLE.COM  "); got == nil || *got != "Local.Part@example.com" {
		t.Fatalf("NormalizeEmail() = %#v", got)
	}
	if got := NormalizeEmail("   "); got != nil {
		t.Fatalf("empty NormalizeEmail() = %#v, want nil", got)
	}
}

func TestMapUserIdentityWriteError(t *testing.T) {
	tests := []struct {
		name   string
		index  string
		target error
	}{
		{name: "username", index: "uk_users_normalized_username", target: ErrDuplicateUsername},
		{name: "email", index: "uk_users_normalized_email", target: ErrDuplicateEmail},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driverErr := &mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry for key '" + tt.index + "'"}
			if err := mapUserIdentityWriteError(driverErr); !errors.Is(err, tt.target) {
				t.Fatalf("mapped error = %v", err)
			}
		})
	}
}
