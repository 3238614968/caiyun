package auth

import (
	"context"
	"errors"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestAuthContextAPIsStopBeforeNetworkWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	authenticator := NewAuth(corehttp.NewClient())
	if _, _, err := authenticator.GetJWTTokenWithSSOTokenContext(ctx, "13800138000"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetJWTTokenWithSSOTokenContext() error = %v, want context.Canceled", err)
	}
	if _, err := authenticator.RefreshAuthorizationContext(ctx, "Basic placeholder", "13800138000", "domain"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RefreshAuthorizationContext() error = %v, want context.Canceled", err)
	}
	if _, err := authenticator.TyrzLoginContext(ctx, "sso"); !errors.Is(err, context.Canceled) {
		t.Fatalf("TyrzLoginContext() error = %v, want context.Canceled", err)
	}
}
