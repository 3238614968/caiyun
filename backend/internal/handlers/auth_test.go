package handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/services"
	"caiyun/pkg/jwt"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type handlerUserRepo struct {
	*repository.UserRepository
	user *models.User
}

func (r *handlerUserRepo) FindByID(id uint) (*models.User, error) {
	if r.user == nil || r.user.ID != id {
		return nil, errors.New("not found")
	}
	copy := *r.user
	return &copy, nil
}

func (r *handlerUserRepo) FindByUsername(username string) (*models.User, error) {
	if r.user == nil || !strings.EqualFold(r.user.Username, username) {
		return nil, errors.New("not found")
	}
	copy := *r.user
	return &copy, nil
}

// Override the promoted login-lock methods from the nil embedded repository.
// The handler tests are not exercising brute-force storage, and returning an
// empty state keeps the fixture fully in-memory.
func (r *handlerUserRepo) GetLoginFailure(string) (int, time.Time, error) {
	return 0, time.Time{}, nil
}

func (r *handlerUserRepo) RecordLoginFailure(string, int, time.Duration, time.Duration) error {
	return nil
}

func (r *handlerUserRepo) ClearLoginFailure(string) error { return nil }

type handlerSessionRepo struct {
	byHash     map[string]*models.RefreshSession
	byID       map[string]*models.RefreshSession
	revokedAll bool
}

func newHandlerSessionRepo() *handlerSessionRepo {
	return &handlerSessionRepo{byHash: map[string]*models.RefreshSession{}, byID: map[string]*models.RefreshSession{}}
}

func (r *handlerSessionRepo) Create(_ context.Context, session *models.RefreshSession) error {
	copy := *session
	r.byHash[copy.RefreshTokenHash] = &copy
	r.byID[copy.ID] = &copy
	return nil
}

func (r *handlerSessionRepo) Rotate(_ context.Context, oldHash string, replacement *models.RefreshSession, now time.Time) (*models.RefreshSession, error) {
	current := r.byHash[oldHash]
	if current == nil {
		return nil, repository.ErrRefreshSessionNotFound
	}
	if current.RevokedAt != nil {
		if current.ReplacedBySessionID != nil {
			r.revokedAll = true
			for _, session := range r.byID {
				if session.RevokedAt == nil {
					revokedAt := now
					session.RevokedAt = &revokedAt
				}
			}
			return nil, repository.ErrRefreshSessionReused
		}
		return nil, repository.ErrRefreshSessionRevoked
	}
	if !now.Before(current.ExpiresAt) {
		revokedAt := now
		current.RevokedAt = &revokedAt
		return nil, repository.ErrRefreshSessionExpired
	}
	replacement.UserID = current.UserID
	replacement.TokenVersion = current.TokenVersion
	copy := *replacement
	r.byHash[copy.RefreshTokenHash] = &copy
	r.byID[copy.ID] = &copy
	revokedAt := now
	current.RevokedAt = &revokedAt
	current.ReplacedBySessionID = &copy.ID
	result := *current
	return &result, nil
}

func (r *handlerSessionRepo) IsActive(_ context.Context, sessionID string, userID uint, now time.Time) (bool, error) {
	session := r.byID[sessionID]
	return session != nil && session.UserID == userID && session.RevokedAt == nil && now.Before(session.ExpiresAt), nil
}

func (r *handlerSessionRepo) Revoke(_ context.Context, sessionID string, userID uint, now time.Time) error {
	if session := r.byID[sessionID]; session != nil && session.UserID == userID && session.RevokedAt == nil {
		revokedAt := now
		session.RevokedAt = &revokedAt
	}
	return nil
}

func (r *handlerSessionRepo) RevokeAll(_ context.Context, userID uint, now time.Time) error {
	r.revokedAll = true
	for _, session := range r.byID {
		if session.UserID == userID && session.RevokedAt == nil {
			revokedAt := now
			session.RevokedAt = &revokedAt
		}
	}
	return nil
}

type handlerAuthFixture struct {
	handler  *AuthHandler
	service  *services.AuthService
	manager  *jwt.Manager
	sessions *handlerSessionRepo
}

func newHandlerAuthFixture(t *testing.T) *handlerAuthFixture {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("Correct-Horse-9!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	users := &handlerUserRepo{user: &models.User{
		ID: 1, Username: "alice", Password: string(hash), Email: "alice@example.com", Role: "user", TokenVersion: 4,
	}}
	manager := jwt.NewManager("0123456789abcdef0123456789abcdef")
	sessions := newHandlerSessionRepo()
	service := services.NewAuthServiceWithPasswordResetCache(
		users, manager, 15*time.Minute, services.PasswordResetConfig{}, nil,
		services.WithRefreshSessionRepository(sessions, 30*24*time.Hour),
	)
	return &handlerAuthFixture{NewAuthHandler(service, manager), service, manager, sessions}
}

func (f *handlerAuthFixture) login(t *testing.T) *services.AuthResponse {
	t.Helper()
	response, err := f.service.LoginContext(context.Background(), &services.LoginRequest{
		Username: "alice", Password: "Correct-Horse-9!",
	}, services.SessionMetadata{DeviceInfo: "handler-test"})
	if err != nil {
		t.Fatalf("LoginContext() error = %v", err)
	}
	return response
}

func invokeAuthHandler(t *testing.T, handler gin.HandlerFunc, body string, cookies []*http.Cookie, headers map[string]string, setup func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "https://example.test/api/auth", bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = request
	if setup != nil {
		setup(ginContext)
	}
	handler(ginContext)
	return recorder
}

func cookieMap(t *testing.T, recorder *httptest.ResponseRecorder) map[string]*http.Cookie {
	t.Helper()
	response := recorder.Result()
	t.Cleanup(func() { _ = response.Body.Close() })
	result := map[string]*http.Cookie{}
	for _, cookie := range response.Cookies() {
		result[cookie.Name] = cookie
	}
	return result
}

func requireClearedCookies(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	cookies := cookieMap(t, recorder)
	for _, name := range []string{authCookieName, refreshCookieName, csrfCookieName} {
		cookie := cookies[name]
		if cookie == nil || cookie.Value != "" || cookie.MaxAge >= 0 {
			t.Fatalf("cookie %s was not cleared: %+v; headers=%v", name, cookie, recorder.Header().Values("Set-Cookie"))
		}
	}
}

func TestSetAuthCookiesUsesSecureHttpOnlyAndDoubleSubmitCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "https://example.test/api/auth/login", nil)
	ginContext.Request.TLS = &tls.ConnectionState{}
	if err := setAuthCookies(ginContext, "access", 15*time.Minute, "refresh", 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	cookies := cookieMap(t, recorder)
	access, refresh, csrf := cookies[authCookieName], cookies[refreshCookieName], cookies[csrfCookieName]
	if access == nil || refresh == nil || csrf == nil {
		t.Fatalf("missing cookies: %#v", cookies)
	}
	if !access.HttpOnly || !refresh.HttpOnly || csrf.HttpOnly {
		t.Fatalf("wrong HttpOnly flags: access=%v refresh=%v csrf=%v", access.HttpOnly, refresh.HttpOnly, csrf.HttpOnly)
	}
	for _, cookie := range []*http.Cookie{access, refresh, csrf} {
		if !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.MaxAge <= 0 {
			t.Fatalf("unsafe cookie attributes: %+v", cookie)
		}
	}
	if csrf.Value == "" || csrf.Value == access.Value || csrf.Value == refresh.Value {
		t.Fatalf("invalid CSRF cookie: %q", csrf.Value)
	}
}

func TestRefreshCookieRequiresCSRFAndRotatesSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newHandlerAuthFixture(t)
	login := fixture.login(t)
	claims, _ := fixture.manager.ValidateToken(login.Token)
	cookies := []*http.Cookie{{Name: refreshCookieName, Value: login.RefreshToken}, {Name: csrfCookieName, Value: "csrf"}}

	denied := invokeAuthHandler(t, fixture.handler.RefreshToken, "", cookies, nil, nil)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("missing-CSRF status=%d body=%s", denied.Code, denied.Body.String())
	}
	active, _ := fixture.sessions.IsActive(context.Background(), claims.SessionID, claims.UserID, time.Now())
	if !active {
		t.Fatal("CSRF rejection consumed refresh session")
	}

	rotated := invokeAuthHandler(t, fixture.handler.RefreshToken, "", cookies, map[string]string{"X-CSRF-Token": "csrf"}, nil)
	if rotated.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", rotated.Code, rotated.Body.String())
	}
	set := cookieMap(t, rotated)
	if set[refreshCookieName] == nil || set[refreshCookieName].Value == login.RefreshToken || set[authCookieName] == nil || set[authCookieName].Value == login.Token || set[csrfCookieName] == nil || set[csrfCookieName].Value == "csrf" {
		t.Fatalf("credentials were not all rotated: %#v", set)
	}
	active, _ = fixture.sessions.IsActive(context.Background(), claims.SessionID, claims.UserID, time.Now())
	if active {
		t.Fatal("old sid remained active after refresh")
	}
}

func TestRefreshReuseRevokesReplacementAndClearsCookies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newHandlerAuthFixture(t)
	login := fixture.login(t)
	cookies := []*http.Cookie{{Name: refreshCookieName, Value: login.RefreshToken}, {Name: csrfCookieName, Value: "csrf"}}
	headers := map[string]string{"X-CSRF-Token": "csrf"}
	first := invokeAuthHandler(t, fixture.handler.RefreshToken, "", cookies, headers, nil)
	if first.Code != http.StatusOK {
		t.Fatalf("first refresh status=%d body=%s", first.Code, first.Body.String())
	}
	replacement := cookieMap(t, first)[authCookieName]
	replacementClaims, err := fixture.manager.ValidateToken(replacement.Value)
	if err != nil {
		t.Fatal(err)
	}

	reused := invokeAuthHandler(t, fixture.handler.RefreshToken, "", cookies, headers, nil)
	if reused.Code != http.StatusUnauthorized {
		t.Fatalf("reuse status=%d body=%s", reused.Code, reused.Body.String())
	}
	active, _ := fixture.sessions.IsActive(context.Background(), replacementClaims.SessionID, replacementClaims.UserID, time.Now())
	if !fixture.sessions.revokedAll || active {
		t.Fatalf("reuse revocation failed: revokedAll=%v active=%v", fixture.sessions.revokedAll, active)
	}
	requireClearedCookies(t, reused)
}

func TestLogoutRevokesOnlyCurrentSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newHandlerAuthFixture(t)
	current, other := fixture.login(t), fixture.login(t)
	currentClaims, _ := fixture.manager.ValidateToken(current.Token)
	otherClaims, _ := fixture.manager.ValidateToken(other.Token)
	recorder := invokeAuthHandler(t, fixture.handler.Logout, "", nil, nil, func(c *gin.Context) {
		c.Set("user_id", currentClaims.UserID)
		c.Set("session_id", currentClaims.SessionID)
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	currentActive, _ := fixture.sessions.IsActive(context.Background(), currentClaims.SessionID, currentClaims.UserID, time.Now())
	otherActive, _ := fixture.sessions.IsActive(context.Background(), otherClaims.SessionID, otherClaims.UserID, time.Now())
	if currentActive || !otherActive {
		t.Fatalf("wrong logout scope: current=%v other=%v", currentActive, otherActive)
	}
	requireClearedCookies(t, recorder)
}

func TestLogoutAllRevokesEverySession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newHandlerAuthFixture(t)
	first, second := fixture.login(t), fixture.login(t)
	firstClaims, _ := fixture.manager.ValidateToken(first.Token)
	secondClaims, _ := fixture.manager.ValidateToken(second.Token)
	recorder := invokeAuthHandler(t, fixture.handler.LogoutAll, "", nil, nil, func(c *gin.Context) {
		c.Set("user_id", firstClaims.UserID)
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("logout-all status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	for _, claims := range []*jwt.Claims{firstClaims, secondClaims} {
		active, _ := fixture.sessions.IsActive(context.Background(), claims.SessionID, claims.UserID, time.Now())
		if active {
			t.Fatalf("session %s remained active", claims.SessionID)
		}
	}
	requireClearedCookies(t, recorder)
}
