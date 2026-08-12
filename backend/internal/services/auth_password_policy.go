package services

import (
	"fmt"
	"strings"
	"unicode"
)

// validatePasswordStrength applies the common password policy used by register,
// self-service change, reset, and administrator reset flows.
func validatePasswordStrength(username string, password string) error {
	if len([]rune(password)) < 8 {
		return fmt.Errorf("%w：长度至少 8 个字符", ErrWeakPassword)
	}
	if normalizedUsername := strings.TrimSpace(username); normalizedUsername != "" && strings.EqualFold(password, normalizedUsername) {
		return fmt.Errorf("%w：密码不能与用户名相同", ErrWeakPassword)
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("%w：需同时包含字母和数字", ErrWeakPassword)
	}
	return nil
}
