package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/pkg/jwt"

	"golang.org/x/crypto/bcrypt"
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

type sessionAuthUserRepo struct {
	user      *models.User
	createErr error
	updateErr error
}

func (r *sessionAuthUserRepo) Create(user *models.User) error {
	if r.createErr != nil {
		return r.createErr
	}
	if user.ID == 0 {
		user.ID = 1
	}
	r.user = user
	return nil
}
func (r *sessionAuthUserRepo) FindByID(id uint) (*models.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, errors.New("not found")
	}
	copy := *r.user
	return &copy, nil
}
func (r *sessionAuthUserRepo) FindByUsername(username string) (*models.User, error) {
	if r.user == nil || r.user.Username != username {
		return nil, errors.New("not found")
	}
	copy := *r.user
	return &copy, nil
}
func (r *sessionAuthUserRepo) Update(user *models.User) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.user = user
	return nil
}
func (r *sessionAuthUserRepo) UpdatePasswordAndRevokeSessions(userID uint, hashedPassword string) error {
	r.user.Password = hashedPassword
	r.user.TokenVersion++
	return nil
}
func (r *sessionAuthUserRepo) ExistsByUsername(username string) (bool, error) {
	return r.user != nil && r.user.Username == username, nil
}
func (r *sessionAuthUserRepo) ExistsByEmail(email string) (bool, error) {
	return r.user != nil && r.user.Email == email, nil
}

type fakeRefreshSessionRepo struct {
	byHash     map[string]*models.RefreshSession
	byID       map[string]*models.RefreshSession
	revokedAll bool
}

func newFakeRefreshSessionRepo() *fakeRefreshSessionRepo {
	return &fakeRefreshSessionRepo{
		byHash: make(map[string]*models.RefreshSession),
		byID:   make(map[string]*models.RefreshSession),
	}
}

func (r *fakeRefreshSessionRepo) Create(_ context.Context, session *models.RefreshSession) error {
	copy := *session
	stored := &copy
	r.byHash[session.RefreshTokenHash] = stored
	r.byID[session.ID] = stored
	return nil
}

func (r *fakeRefreshSessionRepo) Rotate(_ context.Context, oldHash string, replacement *models.RefreshSession, now time.Time) (*models.RefreshSession, error) {
	current := r.byHash[oldHash]
	if current == nil {
		return nil, repository.ErrRefreshSessionNotFound
	}
	if current.RevokedAt != nil {
		if current.ReplacedBySessionID != nil {
			r.revokedAll = true
			for _, session := range r.byID {
				if session.RevokedAt == nil {
					value := now
					session.RevokedAt = &value
				}
			}
			return nil, repository.ErrRefreshSessionReused
		}
		return nil, repository.ErrRefreshSessionRevoked
	}
	if !now.Before(current.ExpiresAt) {
		value := now
		current.RevokedAt = &value
		return nil, repository.ErrRefreshSessionExpired
	}
	replacement.UserID = current.UserID
	replacement.TokenVersion = current.TokenVersion
	copy := *replacement
	stored := &copy
	r.byHash[replacement.RefreshTokenHash] = stored
	r.byID[replacement.ID] = stored
	value := now
	current.RevokedAt = &value
	current.ReplacedBySessionID = &replacement.ID
	result := *current
	return &result, nil
}

func (r *fakeRefreshSessionRepo) IsActive(_ context.Context, sessionID string, userID uint, now time.Time) (bool, error) {
	session := r.byID[sessionID]
	return session != nil && session.UserID == userID && session.RevokedAt == nil && now.Before(session.ExpiresAt), nil
}
func (r *fakeRefreshSessionRepo) Revoke(_ context.Context, sessionID string, userID uint, now time.Time) error {
	if session := r.byID[sessionID]; session != nil && session.UserID == userID && session.RevokedAt == nil {
		value := now
		session.RevokedAt = &value
	}
	return nil
}
func (r *fakeRefreshSessionRepo) RevokeAll(_ context.Context, userID uint, now time.Time) error {
	r.revokedAll = true
	for _, session := range r.byID {
		if session.UserID == userID && session.RevokedAt == nil {
			value := now
			session.RevokedAt = &value
		}
	}
	return nil
}

func newSessionAuthService(t *testing.T) (*AuthService, *jwt.Manager, *fakeRefreshSessionRepo) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("Correct-Horse-9!"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	users := &sessionAuthUserRepo{user: &models.User{
		ID: 1, Username: "alice", Password: string(hash), Email: "alice@example.com", Role: "user", TokenVersion: 3,
	}}
	manager := jwt.NewManager("0123456789abcdef0123456789abcdef")
	sessions := newFakeRefreshSessionRepo()
	service := NewAuthServiceWithPasswordResetCache(
		users,
		manager,
		15*time.Minute,
		PasswordResetConfig{},
		nil,
		WithRefreshSessionRepository(sessions, 30*24*time.Hour),
	)
	return service, manager, sessions
}

func TestAuthServiceLoginCreatesHashedRefreshSession(t *testing.T) {
	service, manager, sessions := newSessionAuthService(t)
	resp, err := service.LoginContext(context.Background(), &LoginRequest{
		Username: "alice", Password: "Correct-Horse-9!",
	}, SessionMetadata{DeviceInfo: "test-browser"})
	if err != nil {
		t.Fatalf("LoginContext() error = %v", err)
	}
	if resp.RefreshToken == "" || resp.RefreshExpiresAt <= resp.ExpiresAt {
		t.Fatalf("unexpected expiry response: %+v", resp)
	}
	claims, err := manager.ValidateToken(resp.Token)
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if claims.SessionID == "" || claims.ID == "" {
		t.Fatalf("access token missing sid/jti: %+v", claims)
	}
	session := sessions.byID[claims.SessionID]
	if session == nil {
		t.Fatal("persisted refresh session not found")
	}
	if session.RefreshTokenHash == resp.RefreshToken || len(session.RefreshTokenHash) != 64 {
		t.Fatalf("raw refresh token was persisted: hash=%q", session.RefreshTokenHash)
	}
	if session.DeviceInfo != "test-browser" || session.TokenVersion != 3 {
		t.Fatalf("unexpected session: %+v", session)
	}
}

func TestAuthServiceRefreshRotatesAndDetectsReuse(t *testing.T) {
	service, manager, sessions := newSessionAuthService(t)
	login, err := service.LoginContext(context.Background(), &LoginRequest{
		Username: "alice", Password: "Correct-Horse-9!",
	}, SessionMetadata{})
	if err != nil {
		t.Fatalf("LoginContext() error = %v", err)
	}
	oldClaims, _ := manager.ValidateToken(login.Token)

	rotated, err := service.RefreshWithToken(context.Background(), login.RefreshToken, SessionMetadata{DeviceInfo: "rotated"})
	if err != nil {
		t.Fatalf("RefreshWithToken() error = %v", err)
	}
	newClaims, err := manager.ValidateToken(rotated.Token)
	if err != nil {
		t.Fatalf("ValidateToken(rotated) error = %v", err)
	}
	if newClaims.SessionID == oldClaims.SessionID || rotated.RefreshToken == login.RefreshToken {
		t.Fatal("refresh did not rotate sid and credential")
	}
	active, _ := sessions.IsActive(context.Background(), oldClaims.SessionID, 1, time.Now())
	if active {
		t.Fatal("old sid remained active after rotation")
	}

	_, err = service.RefreshWithToken(context.Background(), login.RefreshToken, SessionMetadata{})
	if !errors.Is(err, ErrRefreshTokenReuse) {
		t.Fatalf("reused refresh error = %v, want ErrRefreshTokenReuse", err)
	}
	if !sessions.revokedAll {
		t.Fatal("credential reuse did not revoke all sessions")
	}
	active, _ = sessions.IsActive(context.Background(), newClaims.SessionID, 1, time.Now())
	if active {
		t.Fatal("replacement sid remained active after reuse detection")
	}
}

func TestAuthServiceLogoutRevokesAccessSID(t *testing.T) {
	service, manager, sessions := newSessionAuthService(t)
	login, err := service.LoginContext(context.Background(), &LoginRequest{
		Username: "alice", Password: "Correct-Horse-9!",
	}, SessionMetadata{})
	if err != nil {
		t.Fatalf("LoginContext() error = %v", err)
	}
	claims, _ := manager.ValidateToken(login.Token)
	if err := service.RevokeSession(context.Background(), claims.UserID, claims.SessionID); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	active, _ := sessions.IsActive(context.Background(), claims.SessionID, claims.UserID, time.Now())
	if active {
		t.Fatal("sid remained active after logout")
	}
}
func TestAuthServiceMapsRepositoryIdentityConflicts(t *testing.T) {
	manager := jwt.NewManager("0123456789abcdef0123456789abcdef")

	t.Run("register username", func(t *testing.T) {
		repo := &sessionAuthUserRepo{createErr: repository.ErrDuplicateUsername}
		service := NewAuthService(repo, manager, 15*time.Minute)
		_, err := service.Register(&RegisterRequest{
			Username: "Alice",
			Email:    "alice@example.com",
			Password: "Strong-Password-9!",
		})
		if !errors.Is(err, ErrUserExists) {
			t.Fatalf("Register() error = %v, want ErrUserExists", err)
		}
	})

	t.Run("register email", func(t *testing.T) {
		repo := &sessionAuthUserRepo{createErr: repository.ErrDuplicateEmail}
		service := NewAuthService(repo, manager, 15*time.Minute)
		_, err := service.Register(&RegisterRequest{
			Username: "Alice",
			Email:    "alice@example.com",
			Password: "Strong-Password-9!",
		})
		if !errors.Is(err, ErrEmailExists) {
			t.Fatalf("Register() error = %v, want ErrEmailExists", err)
		}
	})

	t.Run("update email", func(t *testing.T) {
		repo := &sessionAuthUserRepo{
			user:      &models.User{ID: 1, Username: "alice", Email: "old@example.com"},
			updateErr: repository.ErrDuplicateEmail,
		}
		service := NewAuthService(repo, manager, 15*time.Minute)
		_, err := service.UpdateProfile(1, "new@example.com")
		if !errors.Is(err, ErrEmailExists) {
			t.Fatalf("UpdateProfile() error = %v, want ErrEmailExists", err)
		}
	})
}
