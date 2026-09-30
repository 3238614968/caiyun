package services

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"
)

func TestWarmExchangeSessionAvoidsColdStartAndDoesNotMixCookiesOrAccounts(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse("https://m.mcloud.139.com")
	jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "base"}})
	base := &exchangePreparedSession{auth: &exchangeAuthContext{jwtToken: "fixture-jwt"}, http: &exchangeHTTPSession{client: &http.Client{Jar: jar}, deviceID: "device", captchaTID: "old"}, sourceAuth: "Basic original"}
	a, b := cloneExchangeWarmSession(base), cloneExchangeWarmSession(base)
	a.http.client.Jar.SetCookies(u, []*http.Cookie{{Name: "session", Value: "a"}})
	if b.http.client.Jar.Cookies(u)[0].Value != "base" || b.http.captchaTID != "" {
		t.Fatal("captcha/cookies shared between tasks")
	}
	s := &ExchangeScheduler{}
	s.storeWarmSession(1, 7, a)
	if got := s.takeWarmSession(1, 7, "Basic original"); got != a {
		t.Fatal("warmup was discarded on execution path")
	}
	if s.takeWarmSession(1, 7, "Basic original").http != nil {
		t.Fatal("session handed out twice")
	}
	s.storeWarmSession(2, 7, b)
	if s.takeWarmSession(2, 8, "Basic original").auth != nil {
		t.Fatal("another account received credentials")
	}
	s.storeWarmSession(3, 7, b)
	if s.takeWarmSession(3, 7, "Basic changed").auth != nil {
		t.Fatal("credential update ignored")
	}
	s.storeWarmSession(4, 7, b)
	entry := s.warmSessions[4]
	entry.expiresAt = time.Now().Add(-time.Second)
	s.warmSessions[4] = entry
	if s.takeWarmSession(4, 7, "Basic original").http != nil {
		t.Fatal("expired warmup reused")
	}
}
