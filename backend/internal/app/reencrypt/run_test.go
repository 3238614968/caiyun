package reencrypt

import (
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
