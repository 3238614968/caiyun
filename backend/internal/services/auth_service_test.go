package services

import (
	"errors"
	"testing"
	"time"

	"caiyun/internal/models"
	"caiyun/pkg/jwt"
)

type fakeAuthUserRepo struct {
	failCount   map[string]int
	lockedUntil map[string]time.Time
}

func newFakeAuthUserRepo() *fakeAuthUserRepo {
	return &fakeAuthUserRepo{
		failCount:   make(map[string]int),
		lockedUntil: make(map[string]time.Time),
	}
}

func (r *fakeAuthUserRepo) Create(user *models.User) error { return nil }

func (r *fakeAuthUserRepo) FindByID(id uint) (*models.User, error) {
	return nil, errors.New("not found")
}

func (r *fakeAuthUserRepo) FindByUsername(username string) (*models.User, error) {
	return nil, errors.New("not found")
}

func (r *fakeAuthUserRepo) Update(user *models.User) error { return nil }

func (r *fakeAuthUserRepo) UpdatePasswordAndRevokeSessions(userID uint, hashedPassword string) error {
	return nil
}

func (r *fakeAuthUserRepo) ExistsByUsername(username string) (bool, error) { return false, nil }

func (r *fakeAuthUserRepo) ExistsByEmail(email string) (bool, error) { return false, nil }

func (r *fakeAuthUserRepo) GetLoginFailure(keyHash string) (int, time.Time, error) {
	return r.failCount[keyHash], r.lockedUntil[keyHash], nil
}

func (r *fakeAuthUserRepo) RecordLoginFailure(keyHash string, maxAttempts int, window, lockTTL time.Duration) error {
	r.failCount[keyHash]++
	if r.failCount[keyHash] >= maxAttempts {
		r.lockedUntil[keyHash] = time.Now().Add(lockTTL)
	}
	return nil
}

func (r *fakeAuthUserRepo) ClearLoginFailure(keyHash string) error {
	delete(r.failCount, keyHash)
	delete(r.lockedUntil, keyHash)
	return nil
}

func TestAuthServiceLoginLockFallsBackToStore(t *testing.T) {
	repo := newFakeAuthUserRepo()
	service := NewAuthServiceWithPasswordResetCache(
		repo,
		jwt.NewManager("0123456789abcdef0123456789abcdef"),
		time.Hour,
		PasswordResetConfig{},
		nil,
	)

	for i := 0; i < loginLockMaxAttempts; i++ {
		_, err := service.Login(&LoginRequest{Username: "ghost", Password: "bad-password"})
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Login() attempt %d error = %v, want ErrInvalidCredentials", i+1, err)
		}
	}

	_, err := service.Login(&LoginRequest{Username: "ghost", Password: "bad-password"})
	if !errors.Is(err, ErrAccountLocked) {
		t.Fatalf("Login() after lock error = %v, want ErrAccountLocked", err)
	}
}
