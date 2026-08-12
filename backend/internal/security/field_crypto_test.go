package security

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func legacyNoAADCiphertext(t *testing.T, plaintext string) string {
	t.Helper()
	block, err := aes.NewCipher([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("aes.NewCipher() error = %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("cipher.NewGCM() error = %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	payload := append(nonce, gcm.Seal(nil, nonce, []byte(plaintext), nil)...)
	return "enc:v1:" + base64.RawStdEncoding.EncodeToString(payload)
}

func TestEncryptDecryptString(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	ResetFieldCryptoForTests()

	encrypted, err := EncryptString("secret-token")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	if encrypted == "secret-token" {
		t.Fatalf("expected ciphertext, got plaintext")
	}
	if !IsEncryptedValue(encrypted) {
		t.Fatalf("expected encrypted prefix, got %q", encrypted)
	}
	if !strings.HasPrefix(encrypted, "enc:v1:") {
		t.Fatalf("expected v1 prefix, got %q", encrypted)
	}

	decrypted, err := DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}
	if decrypted != "secret-token" {
		t.Fatalf("expected plaintext roundtrip, got %q", decrypted)
	}
}

func TestEncryptStringFallbackWithoutKey(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	_ = os.Unsetenv("DATA_ENCRYPTION_KEY")
	t.Setenv("DATA_ENCRYPTION_KEYS", "")
	ResetFieldCryptoForTests()

	encrypted, err := EncryptString("legacy-value")
	if err != nil {
		t.Fatalf("EncryptString() unexpected error = %v", err)
	}
	if encrypted != "legacy-value" {
		t.Fatalf("expected plaintext passthrough without key, got %q", encrypted)
	}
}

func TestDecryptStringFailsWhenKeyMissingForCiphertext(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	ResetFieldCryptoForTests()

	encrypted, err := EncryptString("secret-token")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	_ = os.Unsetenv("DATA_ENCRYPTION_KEY")
	t.Setenv("DATA_ENCRYPTION_KEYS", "")
	ResetFieldCryptoForTests()

	if _, err := DecryptString(encrypted); err == nil || !strings.Contains(err.Error(), "DATA_ENCRYPTION_KEY") {
		t.Fatalf("expected missing-key error, got %v", err)
	}
}

func TestEncryptStringUsesConfiguredCurrentVersion(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATA_ENCRYPTION_KEYS", "v1=0123456789abcdef0123456789abcdef,v2=abcdef0123456789abcdef0123456789")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v2")
	ResetFieldCryptoForTests()

	encrypted, err := EncryptString("rotated-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	if !strings.HasPrefix(encrypted, "enc:v2:") {
		t.Fatalf("expected v2 prefix, got %q", encrypted)
	}
	decrypted, err := DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}
	if decrypted != "rotated-secret" {
		t.Fatalf("DecryptString() = %q, want rotated-secret", decrypted)
	}
}

func TestDecryptStringSupportsLegacyVersionAfterRotation(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	ResetFieldCryptoForTests()
	legacyEncrypted, err := EncryptString("legacy-secret")
	if err != nil {
		t.Fatalf("EncryptString() legacy error = %v", err)
	}

	t.Setenv("DATA_ENCRYPTION_KEYS", "v1=0123456789abcdef0123456789abcdef;v2=abcdef0123456789abcdef0123456789")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v2")
	ResetFieldCryptoForTests()

	decrypted, err := DecryptString(legacyEncrypted)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}
	if decrypted != "legacy-secret" {
		t.Fatalf("DecryptString() = %q, want legacy-secret", decrypted)
	}
}

func TestDecryptStringFailsWhenVersionKeyMissing(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATA_ENCRYPTION_KEYS", "v2=abcdef0123456789abcdef0123456789")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v2")
	ResetFieldCryptoForTests()

	if _, err := DecryptString("enc:v1:ZmFrZQ"); err == nil || !strings.Contains(err.Error(), "版本 v1") {
		t.Fatalf("expected missing version-key error, got %v", err)
	}
}

func TestValidateFieldCryptoConfigProductionRequirements(t *testing.T) {
	const validKey = "0123456789abcdef0123456789abcdef"

	tests := []struct {
		name           string
		singleKey      string
		versionedKeys  string
		currentVersion string
		wantError      string
	}{
		{
			name:           "legacy single key is not sufficient",
			singleKey:      validKey,
			currentVersion: "v1",
			wantError:      "DATA_ENCRYPTION_KEYS",
		},
		{
			name:          "current version is required",
			versionedKeys: "v1=" + validKey,
			wantError:     "DATA_ENCRYPTION_CURRENT_VERSION",
		},
		{
			name:           "current version must exist in keyring",
			versionedKeys:  "v1=" + validKey,
			currentVersion: "v2",
			wantError:      "未在 DATA_ENCRYPTION_KEYS",
		},
		{
			name:           "legacy key cannot supply current version",
			singleKey:      validKey,
			versionedKeys:  "v2=" + validKey,
			currentVersion: "v1",
			wantError:      "未在 DATA_ENCRYPTION_KEYS",
		},
		{
			name:           "key must be valid",
			versionedKeys:  "v1=too-short",
			currentVersion: "v1",
			wantError:      "密钥无效",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "production")
			t.Setenv("DATA_ENCRYPTION_KEY", tt.singleKey)
			t.Setenv("DATA_ENCRYPTION_KEYS", tt.versionedKeys)
			t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", tt.currentVersion)
			ResetFieldCryptoForTests()

			err := ValidateFieldCryptoConfig()
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("ValidateFieldCryptoConfig() error = %v, want error containing %q", err, tt.wantError)
			}
		})
	}
}

func TestValidateFieldCryptoConfigProductionAcceptsVersionedKeyring(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEYS", "v1=0123456789abcdef0123456789abcdef,v2=abcdef0123456789abcdef0123456789")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v2")
	ResetFieldCryptoForTests()

	if err := ValidateFieldCryptoConfig(); err != nil {
		t.Fatalf("ValidateFieldCryptoConfig() unexpected error = %v", err)
	}
	version, err := CurrentEncryptionVersion()
	if err != nil {
		t.Fatalf("CurrentEncryptionVersion() unexpected error = %v", err)
	}
	if version != "v2" {
		t.Fatalf("CurrentEncryptionVersion() = %q, want v2", version)
	}

	encrypted, err := EncryptString("production-secret")
	if err != nil {
		t.Fatalf("EncryptString() unexpected error = %v", err)
	}
	if !strings.HasPrefix(encrypted, "enc:v2:") {
		t.Fatalf("EncryptString() = %q, want enc:v2 prefix", encrypted)
	}
}

func TestEncryptStringFailsClosedWithInvalidProductionConfig(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATA_ENCRYPTION_KEYS", "")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v1")
	ResetFieldCryptoForTests()

	if encrypted, err := EncryptString("must-not-remain-plaintext"); err == nil {
		t.Fatalf("EncryptString() = %q without error; production must fail closed", encrypted)
	}
}

func TestProductionDecryptRejectsPlaintextButMigrationCanReadIt(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEYS", "v1=0123456789abcdef0123456789abcdef")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v1")
	ResetFieldCryptoForTests()
	defer ResetFieldCryptoForTests()

	if _, err := DecryptString("legacy-plaintext"); err == nil || !strings.Contains(err.Error(), "拒绝读取未加密") {
		t.Fatalf("DecryptString() error = %v, want production plaintext rejection", err)
	}
	plain, err := DecryptStringAllowPlaintext("legacy-plaintext")
	if err != nil || plain != "legacy-plaintext" {
		t.Fatalf("DecryptStringAllowPlaintext() = %q, %v", plain, err)
	}
}

func TestDecryptStringRequiresExplicitCutoverFlagForLegacyNoAAD(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEYS", "v1=0123456789abcdef0123456789abcdef")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v1")
	t.Setenv("FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD", "false")
	legacy := legacyNoAADCiphertext(t, "legacy-secret")
	ResetFieldCryptoForTests()

	if _, err := DecryptString(legacy); err == nil || !strings.Contains(err.Error(), "FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD") {
		t.Fatalf("DecryptString() error = %v, want explicit legacy cutover guidance", err)
	}
	plaintext, err := DecryptStringAllowPlaintext(legacy)
	if err != nil || plaintext != "legacy-secret" {
		t.Fatalf("DecryptStringAllowPlaintext() = %q, %v", plaintext, err)
	}

	t.Setenv("FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD", "true")
	ResetFieldCryptoForTests()
	plaintext, err = DecryptString(legacy)
	if err != nil || plaintext != "legacy-secret" {
		t.Fatalf("DecryptString() with cutover flag = %q, %v", plaintext, err)
	}
}

func TestEncryptStringRejectsMalformedCiphertext(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATA_ENCRYPTION_KEYS", "")
	ResetFieldCryptoForTests()
	defer ResetFieldCryptoForTests()

	if value, err := EncryptString("enc:v1:not-valid-base64!"); err == nil {
		t.Fatalf("EncryptString() accepted malformed ciphertext %q", value)
	}
	if IsEncryptedValue("enc:v../../escape:payload") {
		t.Fatal("invalid encryption version was accepted")
	}
}
