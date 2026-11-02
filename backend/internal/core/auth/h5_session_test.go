package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestBoundH5LoginPreservesSDKAndServerCookiesWithoutCloudHeaders(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse("https://m.mcloud.139.com/")
	jar.SetCookies(u, []*http.Cookie{{Name: "smidV2", Value: "fixture-local"}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ycloud/api/timestamp" {
			io.WriteString(w, `{"code":0,"result":1700000000123}`)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("x-yun-app-channel") != "" {
			t.Error("native cloud headers leaked into H5 login")
		}
		if c, e := r.Cookie("smidV2"); e != nil || c.Value != "fixture-local" {
			t.Error("SDK context lost")
		}
		id, ts, nonce := r.Header.Get("x-request-id"), r.Header.Get("x-timestamp"), r.Header.Get("x-nonce")
		if ts != "1700000000123" || r.Header.Get("x-signature") != h5AuthSignature(id, ts, nonce) {
			t.Error("signature mismatch")
		}
		var b map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b["token"] != "fresh-sso" || len(b) != 4 {
			t.Error("login payload mismatch")
		}
		http.SetCookie(w, &http.Cookie{Name: "jwtToken", Value: "fresh-jwt", Path: "/", Secure: true})
		http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "server-session", Path: "/", HttpOnly: true})
		io.WriteString(w, `{"code":0,"result":{"token":"fresh-jwt"}}`)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client := &http.Client{Jar: jar, Transport: h5LoginTransport{target, http.DefaultTransport}}
	token, err := LoginH5Session(context.Background(), client, "fresh-sso", "fixture-UA", "https://m.mcloud.139.com/portal/mobilecloud/index.html")
	if err != nil || token != "fresh-jwt" {
		t.Fatalf("err=%v", err)
	}
	found := map[string]string{}
	for _, c := range jar.Cookies(u) {
		found[c.Name] = c.Value
	}
	if found["jwtToken"] != "fresh-jwt" || found["JSESSIONID"] != "server-session" || found["smidV2"] != "fixture-local" {
		t.Fatal("bound cookie jar lost state")
	}
}
