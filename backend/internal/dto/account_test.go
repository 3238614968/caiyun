package dto

import (
	"caiyun/internal/models"
	"encoding/json"
	"strings"
	"testing"
)

func TestAccountResponseNeverSerializesCredentialOrInternalUserState(t *testing.T) {
	response := ToAccountResponse(&models.Account{
		ID: 7, UserID: 9, Phone: "13800138000", Auth: "Basic secret", Token: "token-secret", JWTToken: "jwt-secret",
		User: models.User{ID: 9, Username: "owner", Password: "password-secret", TokenVersion: 5},
	})
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	text := string(raw)
	for _, forbidden := range []string{"secret", "auth", "token", "password", "token_version", "accounts"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("public account DTO leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"phone":"13800138000"`) || !strings.Contains(text, `"username":"owner"`) {
		t.Fatalf("public fields missing: %s", text)
	}
}
