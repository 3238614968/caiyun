package repository

import (
	"testing"

	"caiyun/internal/models"
	"caiyun/internal/security"
)

func TestBuildExchangeAccountUpdatesEncryptsCredentials(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	security.ResetFieldCryptoForTests()
	defer security.ResetFieldCryptoForTests()

	updates, err := buildExchangeAccountUpdates(&models.ExchangeAccount{
		Phone:         "13900000000",
		Auth:          "plain-auth",
		Token:         "plain-token",
		JWTToken:      "plain-jwt",
		Remark:        "测试规则",
		ExchangeTime1: "10:00:00",
		ExchangeTime2: "16:00:00",
		IsActive:      true,
	})
	if err != nil {
		t.Fatalf("buildExchangeAccountUpdates error: %v", err)
	}
	for _, key := range []string{"auth", "token", "jwt_token"} {
		value, _ := updates[key].(string)
		if !security.IsEncryptedValue(value) {
			t.Fatalf("%s should be encrypted, got %q", key, value)
		}
	}
	if _, exists := updates["user_id"]; exists {
		t.Fatal("updates must not include immutable user_id")
	}
	if _, exists := updates["account_id"]; exists {
		t.Fatal("updates must not include immutable account_id")
	}
}
