package auth

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAppRefreshEncryptDecrypt(t *testing.T) {
	plain := `{"clientType":"414"}`
	encrypted, err := appRefreshEncrypt(plain)
	if err != nil {
		t.Fatalf("appRefreshEncrypt failed: %v", err)
	}
	if encrypted == "" {
		t.Fatal("encrypted is empty")
	}

	decrypted, err := appRefreshDecrypt(encrypted)
	if err != nil {
		t.Fatalf("appRefreshDecrypt failed: %v", err)
	}
	if decrypted != plain {
		t.Fatalf("decrypted mismatch: got %q want %q", decrypted, plain)
	}
}

func TestNormalizeJSONString(t *testing.T) {
	if got := normalizeJSONString(`"abc=="`); got != "abc==" {
		t.Fatalf("normalize quoted: got %q", got)
	}
	if got := normalizeJSONString(`abc==`); got != "abc==" {
		t.Fatalf("normalize raw: got %q", got)
	}
}

func TestParseRefreshAuthToken(t *testing.T) {
	body := `{"success":true,"code":"0000","message":"请求成功","data":{"token":"new-token"}}`
	token, err := parseRefreshAuthToken(body)
	if err != nil {
		t.Fatalf("parseRefreshAuthToken failed: %v", err)
	}
	if token != "new-token" {
		t.Fatalf("token mismatch: %s", token)
	}

	if _, err := parseRefreshAuthToken(`{"success":false,"code":"1001","message":"bad"}`); err == nil {
		t.Fatal("expected business error")
	}
}

func TestGenerateMobileAuthorizationFromRefreshToken(t *testing.T) {
	expireAt := time.Now().Add(30 * 24 * time.Hour).UnixMilli()
	token := fmt.Sprintf("VWgoC0wt|1|RCS|%d|tail", expireAt)
	authValue := GenerateAuth(token, "13800138000", "mobile")

	info, err := ParseToken(authValue)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	if info.Platform != "mobile" || info.Phone != "13800138000" || info.Token != token {
		t.Fatalf("unexpected token info: %+v", info)
	}
	if info.AuthFull == "" || !strings.HasPrefix(info.AuthFull, "Basic ") {
		t.Fatalf("unexpected auth full: %q", info.AuthFull)
	}
}
