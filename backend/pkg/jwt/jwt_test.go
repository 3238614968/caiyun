package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestManagerGenerateAndValidateTokenVersion(t *testing.T) {
	manager := NewManager("test-secret-at-least-16-bytes")

	token, err := manager.GenerateToken(42, "alice", "admin", 7, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := manager.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if claims.UserID != 42 || claims.Username != "alice" || claims.Role != "admin" || claims.TokenVersion != 7 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestManagerRejectsExpiredToken(t *testing.T) {
	manager := NewManager("test-secret-at-least-16-bytes")

	token, err := manager.GenerateToken(1, "bob", "user", 0, -time.Minute)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	_, err = manager.ValidateToken(token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("ValidateToken() error = %v, want ErrExpiredToken", err)
	}
}

func TestManagerRejectsTamperedToken(t *testing.T) {
	manager := NewManager("test-secret-at-least-16-bytes")

	token, err := manager.GenerateToken(1, "bob", "user", 0, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	tampered := token[:len(token)-1]
	if strings.HasSuffix(token, "a") {
		tampered += "b"
	} else {
		tampered += "a"
	}

	_, err = manager.ValidateToken(tampered)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ValidateToken() error = %v, want ErrInvalidToken", err)
	}
}

func TestRS256ManagerGenerateAndValidate(t *testing.T) {
	privatePEM, publicPEM := generateTestRSAKeyPair(t)
	manager, err := NewRS256Manager(privatePEM, publicPEM)
	if err != nil {
		t.Fatalf("NewRS256Manager() error = %v", err)
	}

	token, err := manager.GenerateToken(9, "charlie", "user", 3, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	claims, err := manager.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if claims.UserID != 9 || claims.Username != "charlie" || claims.Role != "user" || claims.TokenVersion != 3 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestRS256ManagerRejectsHS256Token(t *testing.T) {
	privatePEM, publicPEM := generateTestRSAKeyPair(t)
	rsManager, err := NewRS256Manager(privatePEM, publicPEM)
	if err != nil {
		t.Fatalf("NewRS256Manager() error = %v", err)
	}
	hsManager := NewManager("test-secret-at-least-16-bytes")

	token, err := hsManager.GenerateToken(1, "bob", "user", 0, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	_, err = rsManager.ValidateToken(token)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("ValidateToken() error = %v, want ErrInvalidToken", err)
	}
}

func generateTestRSAKeyPair(t *testing.T) (string, string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}

	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	publicPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	})

	return string(privatePEM), string(publicPEM)
}
