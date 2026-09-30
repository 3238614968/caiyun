package api

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAppActivityPublicKeyParses(t *testing.T) {
	key, err := AppActivityPublicKey()
	if err != nil {
		t.Fatalf("AppActivityPublicKey() error = %v", err)
	}
	if key == nil {
		t.Fatal("AppActivityPublicKey() returned nil key")
	}
	if bits := key.N.BitLen(); bits != 1024 {
		t.Fatalf("public key size = %d bits, want 1024", bits)
	}
}

func TestEncryptActivityPayloadProducesPKCS1v15Ciphertext(t *testing.T) {
	encrypted, err := EncryptActivityPayload(map[string]interface{}{
		"marketName":  RedInviteMarketName,
		"encryptTime": 1758960000000,
	})
	if err != nil {
		t.Fatalf("EncryptActivityPayload() error = %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("ciphertext is not valid base64: %v", err)
	}
	// 1024-bit RSA 的 PKCS#1 v1.5 密文固定 128 字节。
	if len(raw) != 128 {
		t.Fatalf("ciphertext length = %d, want 128", len(raw))
	}
}

func TestEncryptActivityStringSucceeds(t *testing.T) {
	encrypted, err := EncryptActivityString("hello-activity")
	if err != nil {
		t.Fatalf("EncryptActivityString() unexpected error = %v", err)
	}
	if encrypted == "" {
		t.Fatal("EncryptActivityString() returned empty ciphertext")
	}
}

func TestParseOpRequestTime(t *testing.T) {
	cases := []struct {
		body string
		want int64
	}{
		{"1758960000000", 1758960000000},
		{`{"code":0,"result":1758960000000}`, 1758960000000},
		{`{"code":0,"result":"1758960000000"}`, 1758960000000},
	}
	for _, tc := range cases {
		got, ok := parseOpRequestTime(tc.body)
		if !ok || got != tc.want {
			t.Fatalf("parseOpRequestTime(%q) = (%d, %v), want (%d, true)", tc.body, got, ok, tc.want)
		}
	}
	if _, ok := parseOpRequestTime("not-a-timestamp"); ok {
		t.Fatal("parseOpRequestTime should reject non-numeric bodies")
	}
}

func TestEncryptPhoneWithIVRoundTrip(t *testing.T) {
	const phone = "13391221213"
	encrypted, err := EncryptPhoneWithIV(caixunClientKeyRelease, phone)
	if err != nil {
		t.Fatalf("EncryptPhoneWithIV() error = %v", err)
	}
	decrypted, err := DecryptWithIV(caixunClientKeyRelease, encrypted)
	if err != nil {
		t.Fatalf("DecryptWithIV() error = %v", err)
	}
	if decrypted != phone {
		t.Fatalf("round trip = %q, want %q", decrypted, phone)
	}

	raw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("ciphertext is not valid base64: %v", err)
	}
	// base64(IV‖ciphertext)，且整体按 16 字节对齐。
	if len(raw)%16 != 0 || len(raw) < 32 {
		t.Fatalf("ciphertext length = %d, want multiple of 16 and >= 32", len(raw))
	}
}

func TestNewActivityUUIDFormat(t *testing.T) {
	uuid, err := newActivityUUID()
	if err != nil {
		t.Fatalf("newActivityUUID() error = %v", err)
	}
	parts := strings.Split(uuid, "-")
	if len(parts) != 5 {
		t.Fatalf("uuid = %q, want 5 dash-separated groups", uuid)
	}
	if len(parts[0]) != 8 || len(parts[4]) != 12 {
		t.Fatalf("uuid = %q has unexpected group lengths", uuid)
	}
	if parts[2][0] != '4' {
		t.Fatalf("uuid = %q is not version 4", uuid)
	}
	other, err := newActivityUUID()
	if err != nil {
		t.Fatalf("newActivityUUID() second call error = %v", err)
	}
	if uuid == other {
		t.Fatalf("newActivityUUID() returned duplicate %q", uuid)
	}
}
