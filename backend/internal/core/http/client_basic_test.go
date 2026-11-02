package http

import (
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestBasicOnlyRequestIsolatesCloudCookiesAndDoesNotFollowRedirects(t *testing.T) {
	redirects := 0
	target := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		redirects++
		t.Error("credential request followed redirect")
	}))
	defer target.Close()
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.Header.Get("Authorization") != "Basic fixture-auth" || r.Header.Get("Content-Type") != "application/json" {
			t.Fatal("native headers missing")
		}
		for _, key := range []string{"Cookie", "jwtToken", "x-yun-client-info", "x-yun-app-channel"} {
			if r.Header.Get(key) != "" {
				t.Fatalf("inherited header %s", key)
			}
		}
		w.Header().Set("Location", target.URL)
		w.WriteHeader(302)
	}))
	defer server.Close()
	c := NewClient()
	c.SetAuth("fixture-auth")
	c.SetJWTToken("fixture-jwt")
	u, _ := url.Parse(server.URL)
	c.client.Jar.SetCookies(u, []*stdhttp.Cookie{{Name: "cloud-session", Value: "fixture"}})
	before := c.client.Timeout
	r, err := c.PostWithBasicAuth(server.URL, map[string]string{"test": "value"}, "application/json", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, r.Body)
	r.Body.Close()
	if r.StatusCode != 302 || redirects != 0 || c.client.Timeout != before || c.client.Jar == nil {
		t.Fatal("shared HTTP client was modified")
	}
}
