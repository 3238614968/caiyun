package services

import "testing"

func TestValidatePasswordStrengthRequiresEightCharactersAndRejectsUsername(t *testing.T) {
	for _, password := range []string{"abc12345", "Alice123", "user1234"} {
		if err := validatePasswordStrength("user", password); err != nil {
			t.Fatalf("validatePasswordStrength(%q) = %v, want nil", password, err)
		}
	}
	if err := validatePasswordStrength("user1234", "user1234"); err == nil {
		t.Fatal("username-equivalent password accepted")
	}
}

func TestValidatePasswordStrengthRequiresLetterAndDigit(t *testing.T) {
	for _, password := range []string{"abcde", "abcdef", "123456"} {
		if err := validatePasswordStrength("user", password); err == nil {
			t.Fatalf("validatePasswordStrength(%q) = nil, want error", password)
		}
	}
}
