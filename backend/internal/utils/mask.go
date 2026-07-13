package utils

import (
	"strings"
	"unicode/utf8"
)

// MaskPhone masks a mainland China style mobile number for logs and persisted
// operational messages. Short or malformed values are never returned verbatim.
func MaskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) < 7 {
		if phone == "" {
			return ""
		}
		return "***"
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

// MaskAccountName masks an account display name before writing it to logs.
// Phone-like names keep the common 3-4-4 mask; remarks keep only their edges.
func MaskAccountName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if isLikelyPhone(name) {
		return MaskPhone(name)
	}

	runes := []rune(name)
	switch len(runes) {
	case 0:
		return ""
	case 1, 2:
		return "***"
	default:
		return string(runes[0]) + "***" + string(runes[len(runes)-1])
	}
}

func isLikelyPhone(value string) bool {
	if len(value) != 11 || !utf8.ValidString(value) {
		return false
	}
	for i, r := range value {
		if r < '0' || r > '9' {
			return false
		}
		if i == 0 && r != '1' {
			return false
		}
	}
	return true
}
