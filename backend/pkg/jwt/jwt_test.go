package jwt

import (
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
