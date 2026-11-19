package api

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigBuildsStructuredAPIConfig(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "false")
	t.Setenv("JWT_ALGORITHM", "HS256")
	t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
	t.Setenv("PORT", "18080")
	t.Setenv("REQUEST_TIMEOUT", "45s")
	t.Setenv("JWT_ACCESS_TTL", "20m")
	t.Setenv("JWT_REFRESH_TTL", "72h")
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8")
	t.Setenv("WS_EVENT_CHANNEL", "test:events")
	t.Setenv("INSTANCE_ID", "api-test")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.Server.Port != "18080" || config.Server.RequestTimeout != 45*time.Second {
		t.Fatalf("server config = %#v", config.Server)
	}
	if config.JWT.Algorithm != "HS256" || config.JWT.AccessTTL != 20*time.Minute || config.JWT.RefreshTTL != 72*time.Hour {
		t.Fatalf("JWT config = %#v", config.JWT)
	}
	if config.Realtime.EventChannel != "test:events" || config.Realtime.InstanceID != "api-test" {
		t.Fatalf("realtime config = %#v", config.Realtime)
	}
}

func TestReverseProxyDeploymentRequiresExplicitTrustedSources(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "false")
	t.Setenv("JWT_ALGORITHM", "HS256")
	t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
	for _, tc := range []struct {
		name, proxies, required string
		wantError               bool
	}{
		{"missing behind proxy", "", "true", true},
		{"trust none behind proxy", "none", "true", true},
		{"trust all rejected", "0.0.0.0/0", "true", true},
		{"invalid source rejected", "not-a-network", "true", true},
		{"explicit sources", "10.42.0.0/16,10.43.0.0/16", "true", false},
		{"direct backend", "none", "false", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TRUSTED_PROXIES", tc.proxies)
			t.Setenv("REVERSE_PROXY_REQUIRED", tc.required)
			_, err := LoadConfig()
			if (err != nil) != tc.wantError {
				t.Fatalf("LoadConfig error=%v, want error=%t", err, tc.wantError)
			}
			if tc.required == "true" && (tc.proxies == "" || tc.proxies == "none") && !strings.Contains(err.Error(), "TRUSTED_PROXIES") {
				t.Fatalf("missing actionable proxy configuration error: %v", err)
			}
		})
	}
}

func TestLoadConfigRejectsInvalidJWTAlgorithm(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "false")
	t.Setenv("JWT_ALGORITHM", "ES256")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want invalid algorithm error")
	}
}
