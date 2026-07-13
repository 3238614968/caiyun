package api

import (
	"reflect"
	"testing"
)

func TestParseTrustedProxies(t *testing.T) {
	got, err := parseTrustedProxies("127.0.0.1, 10.0.0.0/8;::1,127.0.0.1")
	if err != nil {
		t.Fatalf("parseTrustedProxies() error = %v", err)
	}
	want := []string{"127.0.0.1", "10.0.0.0/8", "::1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseTrustedProxies() = %#v, want %#v", got, want)
	}
}

func TestParseTrustedProxiesDefaultsToTrustNone(t *testing.T) {
	for _, raw := range []string{"", "none", " NONE "} {
		got, err := parseTrustedProxies(raw)
		if err != nil {
			t.Fatalf("parseTrustedProxies(%q) error = %v", raw, err)
		}
		if got != nil {
			t.Fatalf("parseTrustedProxies(%q) = %#v, want nil", raw, got)
		}
	}
}

func TestParseTrustedProxiesRejectsTrustAllAndInvalidValues(t *testing.T) {
	for _, raw := range []string{"*", "0.0.0.0/0", "::/0", "reverse-proxy.local"} {
		if _, err := parseTrustedProxies(raw); err == nil {
			t.Fatalf("parseTrustedProxies(%q) expected error", raw)
		}
	}
}
