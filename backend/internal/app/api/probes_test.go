package api

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestLegacyAPIDeprecationMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	legacy := apiRouteGroup(router, "/api", true)
	legacy.GET("/status", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	v1 := apiRouteGroup(router, "/api/v1", false)
	v1.GET("/status", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	legacyResponse := httptest.NewRecorder()
	router.ServeHTTP(legacyResponse, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if legacyResponse.Code != http.StatusNoContent || legacyResponse.Header().Get("Deprecation") != "true" || legacyResponse.Header().Get("Sunset") != legacyAPISunset {
		t.Fatalf("legacy response headers = %#v", legacyResponse.Header())
	}
	if got := legacyResponse.Header().Get("Link"); got != "</api/v1>; rel=\"successor-version\"" {
		t.Fatalf("legacy Link = %q", got)
	}

	v1Response := httptest.NewRecorder()
	router.ServeHTTP(v1Response, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	if v1Response.Code != http.StatusNoContent || v1Response.Header().Get("Deprecation") != "" || v1Response.Header().Get("Sunset") != "" {
		t.Fatalf("v1 response headers = %#v", v1Response.Header())
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
	for _, raw := range []string{"*", "0.0.0.0/0", "::/0", "0:0:0:0:0:0:0:0/0", "::ffff:0.0.0.0/96", "reverse-proxy.local"} {
		if _, err := parseTrustedProxies(raw); err == nil {
			t.Fatalf("parseTrustedProxies(%q) expected error", raw)
		}
	}
}

func TestTrustedProxyChainSeparatesUsersAndIgnoresUntrustedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	if err := configureTrustedProxies(router, "10.42.0.0/16"); err != nil {
		t.Fatal(err)
	}
	router.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	for _, tc := range []struct{ remote, forwarded, want string }{
		{"10.42.1.9:1234", "192.0.2.66, 198.51.100.1, 10.42.2.20", "198.51.100.1"},
		{"10.42.1.9:1234", "192.0.2.66, 203.0.113.2, 10.42.2.20", "203.0.113.2"},
		{"203.0.113.9:1234", "198.51.100.1", "203.0.113.9"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.RemoteAddr = tc.remote
		req.Header.Set("X-Forwarded-For", tc.forwarded)
		req.Header.Set("X-Real-IP", "192.0.2.66")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		if got := recorder.Body.String(); got != tc.want {
			t.Fatalf("remote=%s XFF=%s ClientIP=%s, want %s", tc.remote, tc.forwarded, got, tc.want)
		}
	}
}
