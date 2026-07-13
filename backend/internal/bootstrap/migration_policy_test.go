package bootstrap

import (
	"os"
	"strings"
	"testing"
)

func TestResolveEmbeddedMigrationPolicy(t *testing.T) {
	tests := []struct {
		name        string
		appEnv      string
		autoMigrate *string
		wantEnabled bool
		wantErr     string
	}{
		{name: "production defaults disabled", appEnv: "production", wantEnabled: false},
		{name: "production name is case insensitive", appEnv: " Production ", wantEnabled: false},
		{name: "production blank remains disabled", appEnv: "production", autoMigrate: stringPointer("  "), wantEnabled: false},
		{name: "production explicitly disabled", appEnv: "production", autoMigrate: stringPointer("false"), wantEnabled: false},
		{name: "production zero disables", appEnv: "production", autoMigrate: stringPointer("0"), wantEnabled: false},
		{name: "production cannot explicitly enable", appEnv: "production", autoMigrate: stringPointer("true"), wantErr: "被禁止"},
		{name: "production invalid value fails closed", appEnv: "production", autoMigrate: stringPointer("sometimes"), wantErr: "无效"},
		{name: "development defaults enabled", appEnv: "development", wantEnabled: true},
		{name: "empty environment defaults enabled", appEnv: "", wantEnabled: true},
		{name: "development explicitly enabled", appEnv: "development", autoMigrate: stringPointer("ON"), wantEnabled: true},
		{name: "development explicitly disabled", appEnv: "development", autoMigrate: stringPointer("no"), wantEnabled: false},
		{name: "development invalid value is rejected", appEnv: "development", autoMigrate: stringPointer("enabled"), wantErr: "无效"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(appEnvironmentKey, tt.appEnv)
			setOptionalEnv(t, autoMigrateEnvKey, tt.autoMigrate)

			enabled, err := resolveEmbeddedMigrationPolicy()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveEmbeddedMigrationPolicy() error = %v, want substring %q", err, tt.wantErr)
				}
				if enabled {
					t.Fatal("resolveEmbeddedMigrationPolicy() enabled migrations after a policy error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveEmbeddedMigrationPolicy() unexpected error: %v", err)
			}
			if enabled != tt.wantEnabled {
				t.Fatalf("resolveEmbeddedMigrationPolicy() = %v, want %v", enabled, tt.wantEnabled)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }

func setOptionalEnv(t *testing.T, key string, value *string) {
	t.Helper()
	original, existed := os.LookupEnv(key)
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, original)
			return
		}
		_ = os.Unsetenv(key)
	})
	if value == nil {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		return
	}
	if err := os.Setenv(key, *value); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
}
