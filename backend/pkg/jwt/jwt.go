package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("无效的token")
	ErrExpiredToken = errors.New("token已过期")
)

type Claims struct {
	UserID       uint   `json:"user_id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	TokenVersion int    `json:"token_version"`
	jwt.RegisteredClaims
}

type Manager struct {
	secretKey     []byte
	privateKey    *rsa.PrivateKey
	publicKey     *rsa.PublicKey
	signingMethod jwt.SigningMethod
}

func NewManager(secretKey string) *Manager {
	return &Manager{
		secretKey:     []byte(secretKey),
		signingMethod: jwt.SigningMethodHS256,
	}
}

func NewRS256Manager(privateKeyPEM, publicKeyPEM string) (*Manager, error) {
	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	publicKey, err := parseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}
	return &Manager{
		privateKey:    privateKey,
		publicKey:     publicKey,
		signingMethod: jwt.SigningMethodRS256,
	}, nil
}

func (m *Manager) GenerateToken(userID uint, username, role string, tokenVersion int, expiration time.Duration) (string, error) {
	if m.signingMethod == nil {
		m.signingMethod = jwt.SigningMethodHS256
	}

	now := time.Now()
	claims := &Claims{
		UserID:       userID,
		Username:     username,
		Role:         role,
		TokenVersion: tokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(expiration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(m.signingMethod, claims)
	if m.signingMethod == jwt.SigningMethodRS256 {
		return token.SignedString(m.privateKey)
	}
	return token.SignedString(m.secretKey)
}

func (m *Manager) ValidateToken(tokenString string) (*Claims, error) {
	if !isCanonicalJWT(tokenString) {
		return nil, ErrInvalidToken
	}
	if m.signingMethod == nil {
		m.signingMethod = jwt.SigningMethodHS256
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != m.signingMethod.Alg() {
			return nil, ErrInvalidToken
		}
		if m.signingMethod == jwt.SigningMethodRS256 {
			return m.publicKey, nil
		}
		return m.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func parseRSAPrivateKey(keyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(normalizePEM(keyPEM)))
	if block == nil {
		return nil, fmt.Errorf("解析 RSA 私钥失败：PEM 为空")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析 RSA 私钥失败: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("解析 RSA 私钥失败：不是 RSA 私钥")
	}
	return key, nil
}

func parseRSAPublicKey(keyPEM string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(normalizePEM(keyPEM)))
	if block == nil {
		return nil, fmt.Errorf("解析 RSA 公钥失败：PEM 为空")
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析 RSA 公钥失败: %w", err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("解析 RSA 公钥失败：不是 RSA 公钥")
	}
	return key, nil
}

func normalizePEM(keyPEM string) string {
	return strings.ReplaceAll(strings.TrimSpace(keyPEM), `\n`, "\n")
}

func isCanonicalJWT(tokenString string) bool {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		raw, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil {
			return false
		}
		if base64.RawURLEncoding.EncodeToString(raw) != part {
			return false
		}
	}
	return true
}
