package security

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestE2ESeedCredentialsMatchFixedEncryptionKey(t *testing.T) {
	seedPath := filepath.Join("..", "..", "..", "scripts", "e2e-seed.sql")
	seed, err := os.ReadFile(seedPath)
	if err != nil {
		t.Fatalf("read E2E seed: %v", err)
	}
	pattern := regexp.MustCompile(`(?s)\(900001,\s*900001,\s*'13900009001',\s*'([^']+)',\s*'([^']+)',\s*'([^']+)'`)
	match := pattern.FindSubmatch(seed)
	if len(match) != 4 {
		t.Fatal("E2E account credential seed row not found")
	}

	t.Setenv("APP_ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEYS", "v1=0123456789abcdef0123456789abcdef")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v1")
	ResetFieldCryptoForTests()
	defer ResetFieldCryptoForTests()

	want := []string{"e2e-auth-placeholder", "e2e-token-placeholder", "e2e-jwt-placeholder"}
	for index, ciphertext := range match[1:] {
		plaintext, err := DecryptString(string(ciphertext))
		if err != nil {
			t.Fatalf("decrypt E2E seed field %d: %v", index, err)
		}
		if plaintext != want[index] {
			t.Fatalf("E2E seed field %d = %q, want %q", index, plaintext, want[index])
		}
	}
}
