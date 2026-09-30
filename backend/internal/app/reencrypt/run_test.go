package reencrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"strings"
	"testing"

	"caiyun/internal/security"
)

func TestResolveTableTargetsSupportsAliases(t *testing.T) {
	targets, err := resolveTableTargets("exchange_accounts, accounts, rules")
	if err != nil {
		t.Fatalf("resolveTableTargets() error = %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("resolveTableTargets() len = %d, want 2", len(targets))
	}
	if got := joinTargetNames(targets); got != "accounts,exchange_rules" {
		t.Fatalf("joinTargetNames() = %q", got)
	}
}

func TestResolveTableTargetsRejectsUnknownTable(t *testing.T) {
	if _, err := resolveTableTargets("foo"); err == nil {
		t.Fatal("resolveTableTargets() expected error for unknown table")
	}
}

func TestRotateCredentialValueEncryptsPlaintextIntoCurrentVersion(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEYS", "v1=0123456789abcdef0123456789abcdef,v2=abcdef0123456789abcdef0123456789")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v2")
	security.ResetFieldCryptoForTests()
	defer security.ResetFieldCryptoForTests()

	rotated, changed, state, err := rotateCredentialValue("plain-secret", "v2")
	if err != nil {
		t.Fatalf("rotateCredentialValue() error = %v", err)
	}
	if !changed {
		t.Fatal("rotateCredentialValue() changed = false, want true")
	}
	if state != rotationStatePlaintext {
		t.Fatalf("rotateCredentialValue() state = %q, want plaintext", state)
	}
	if !strings.HasPrefix(rotated, "enc:v2:") {
		t.Fatalf("rotateCredentialValue() = %q, want enc:v2 prefix", rotated)
	}
}

func TestRotateCredentialValueKeepsCurrentVersionCiphertext(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEYS", "v2=abcdef0123456789abcdef0123456789")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v2")
	security.ResetFieldCryptoForTests()
	defer security.ResetFieldCryptoForTests()

	ciphertext, err := security.EncryptString("same-secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	rotated, changed, state, err := rotateCredentialValue(ciphertext, "v2")
	if err != nil {
		t.Fatalf("rotateCredentialValue() error = %v", err)
	}
	if changed {
		t.Fatal("rotateCredentialValue() changed = true, want false")
	}
	if state != rotationStateCurrent {
		t.Fatalf("rotateCredentialValue() state = %q, want current", state)
	}
	if rotated != ciphertext {
		t.Fatal("rotateCredentialValue() unexpectedly changed ciphertext")
	}
}

func TestRotateCredentialValueUpgradesLegacyCiphertext(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	security.ResetFieldCryptoForTests()
	legacyCiphertext, err := security.EncryptString("legacy-secret")
	if err != nil {
		t.Fatalf("EncryptString() legacy error = %v", err)
	}

	t.Setenv("DATA_ENCRYPTION_KEYS", "v1=0123456789abcdef0123456789abcdef,v2=abcdef0123456789abcdef0123456789")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v2")
	security.ResetFieldCryptoForTests()
	defer security.ResetFieldCryptoForTests()

	rotated, changed, state, err := rotateCredentialValue(legacyCiphertext, "v2")
	if err != nil {
		t.Fatalf("rotateCredentialValue() error = %v", err)
	}
	if !changed {
		t.Fatal("rotateCredentialValue() changed = false, want true")
	}
	if state != rotationStateLegacy {
		t.Fatalf("rotateCredentialValue() state = %q, want legacy", state)
	}
	if !strings.HasPrefix(rotated, "enc:v2:") {
		t.Fatalf("rotateCredentialValue() = %q, want enc:v2 prefix", rotated)
	}
}

func TestRotateCredentialValueUpgradesLegacyNoAADWithSameKeyVersion(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef"
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEYS", "v1="+key)
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v1")
	// Even during compatibility mode, same-version old-format ciphertext must
	// be rewritten so removing the flag does not break the next deployment.
	t.Setenv("FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD", "true")
	security.ResetFieldCryptoForTests()
	defer security.ResetFieldCryptoForTests()
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	payload := append(nonce, gcm.Seal(nil, nonce, []byte("legacy-secret"), nil)...)
	old := "enc:v1:" + base64.RawStdEncoding.EncodeToString(payload)
	rotated, changed, state, err := rotateCredentialValue(old, "v1")
	if err != nil || !changed || state != rotationStateLegacy || rotated == old {
		t.Fatalf("same-version legacy rotation: changed=%t state=%s err=%v", changed, state, err)
	}
	t.Setenv("FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD", "false")
	security.ResetFieldCryptoForTests()
	plain, err := security.DecryptString(rotated)
	if err != nil || plain != "legacy-secret" {
		t.Fatalf("rotated credential unavailable after compatibility disabled: %v", err)
	}
	_, changed, state, err = rotateCredentialValue(rotated, "v1")
	if err != nil || changed || state != rotationStateCurrent {
		t.Fatalf("second migration must be idempotent: changed=%t state=%s err=%v", changed, state, err)
	}
}
