package services

import "testing"

func TestValidatePasswordStrengthAllowsSixCharacterAlphanumericAndUsername(t *testing.T) {
	for _, password := range []string{"abc123", "Alice1", "user123"} {
		if err := validatePasswordStrength("user", password); err != nil {
			t.Fatalf("validatePasswordStrength(%q) = %v, want nil", password, err)
		}
	}
}

func TestValidatePasswordStrengthRequiresLetterAndDigit(t *testing.T) {
	for _, password := range []string{"abcde", "abcdef", "123456"} {
		if err := validatePasswordStrength("user", password); err == nil {
			t.Fatalf("validatePasswordStrength(%q) = nil, want error", password)
		}
	}
}
