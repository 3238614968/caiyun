package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"caiyun/internal/core/auth"
	"caiyun/internal/models"
)

type smsGuardRepository struct {
	stubAccountRepository
	active    bool
	lookupErr error
}

func (r *smsGuardRepository) HasActiveLoginForPhone(string, int64) (bool, error) {
	return r.active, r.lookupErr
}

func TestSMSLoginProtectsActiveCredentialsUnlessForced(t *testing.T) {
	for _, force := range []bool{false, true} {
		repo := &smsGuardRepository{active: true}
		s := NewAccountService(repo, &stubAccountUserRepository{}, nil, nil)
		called := false
		_, err := s.WithSMSLoginContext(t.Context(), 1, "13800000001", force, func(context.Context) (*models.Account, error) { called = true; return &models.Account{}, nil })
		if called != force || errors.Is(err, ErrActiveAccountLogin) == force {
			t.Fatalf("force=%t called=%t err=%v", force, called, err)
		}
	}
}

func TestSMSLoginSerializesSamePhoneAcrossUsersAndRechecksSavedCredential(t *testing.T) {
	repo := &smsGuardRepository{}
	s := NewAccountService(repo, &stubAccountUserRepository{}, nil, nil)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := s.WithSMSLoginContext(context.Background(), 1, "13800000002", false, func(context.Context) (*models.Account, error) {
			close(started)
			<-release
			repo.active = true
			return &models.Account{}, nil
		})
		done <- err
	}()
	<-started
	_, err := s.WithSMSLoginContext(t.Context(), 2, "13800000002", true, func(context.Context) (*models.Account, error) { t.Error("concurrent thirdlogin"); return nil, nil })
	if !errors.Is(err, ErrAccountLoginBusy) {
		t.Errorf("err=%v", err)
	}
	// A different phone can proceed while the first phone is logging in.
	if _, err := s.WithSMSLoginContext(t.Context(), 2, "13800000003", false, func(context.Context) (*models.Account, error) { return &models.Account{}, nil }); err != nil {
		t.Error(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_, err = s.WithSMSLoginContext(t.Context(), 2, "13800000002", false, func(context.Context) (*models.Account, error) {
		t.Error("saved credential was rotated")
		return nil, nil
	})
	if !errors.Is(err, ErrActiveAccountLogin) {
		t.Fatalf("err=%v", err)
	}
}

func TestSMSLoginRejectsCancelledAndFailedMetadataChecks(t *testing.T) {
	repo := &smsGuardRepository{lookupErr: errors.New("database unavailable")}
	s := NewAccountService(repo, &stubAccountUserRepository{}, nil, nil)
	if err := s.CheckSMSLoginAllowedContext(t.Context(), 1, "13800000004", false); !errors.Is(err, repo.lookupErr) {
		t.Fatalf("err=%v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.CheckSMSLoginAllowedContext(ctx, 1, "13800000004", true); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestHealthyAuthorizationIsNeverRotatedProactively(t *testing.T) {
	now := time.Now()
	account := &models.Account{ExpireAt: now.Add(-time.Minute).UnixMilli()}
	// No auth client is installed: a valid JWT must short-circuit rotation even
	// when the stored expiry metadata is stale.
	if err := (&TokenManager{}).refreshAuthorizationIfNeeded(account, &tokenRefreshSession{jwtToken: "valid-jwt"}, now); err != nil {
		t.Fatal(err)
	}
}

func TestSMSLeaseAlsoBlocksManualAndWorkerCredentialRefresh(t *testing.T) {
	phone := "13800000005"
	release, err := acquireCredentialRotationLock(t.Context(), nil, phone)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	account := &models.Account{ID: 5, Phone: phone, ExpireAt: time.Now().Add(-time.Hour).UnixMilli()}
	s := NewAccountService(&smsGuardRepository{}, &stubAccountUserRepository{}, nil, nil)
	if err := s.RefreshTokenContext(t.Context(), account); !errors.Is(err, ErrAccountLoginBusy) {
		t.Fatalf("manual refresh err=%v", err)
	}
	if err := (&TokenManager{}).refreshAuthorizationIfNeeded(account, &tokenRefreshSession{authErr: auth.ErrCredentialExpired}, time.Now()); !errors.Is(err, ErrAccountLoginBusy) {
		t.Fatalf("worker refresh err=%v", err)
	}
}

func TestCloudPhoneDisabledEvenForPreviouslyEnabledBatchConfig(t *testing.T) {
	catalog := NewTaskCatalog()
	def, ok := catalog.Get("cloudphone")
	if !ok || def.DefaultEnabled || def.RunInBatch {
		t.Fatal("cloudphone must default to disabled")
	}
	codes := catalog.ResolveBatchCodes([]*models.TaskConfig{{TaskType: "cloud_phone_redpack", IsEnabled: true, RunInBatch: true}})
	if len(codes) != 0 {
		t.Fatalf("batch=%v", codes)
	}
	result := def.execute(&TaskRunner{})
	if result.Status != "skipped" {
		t.Fatalf("result=%+v", result)
	}
}
