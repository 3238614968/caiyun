package http

import (
	"encoding/base64"
	"net/url"
	"testing"
)

func TestSetAuthAcceptsCaseInsensitiveBasicScheme(t *testing.T) {
	for _, value := range []string{"Basic abc", "basic abc", "BASIC abc", "abc"} {
		client := NewClient()
		client.SetAuth(value)
		if got := client.buildHeaders("https://mail.10086.cn", nil)["Authorization"]; got != "Basic abc" {
			t.Fatalf("SetAuth(%q) produced %q", value, got)
		}
	}
}

func TestCurrentAuthCookieIsScopedAndClearedOnCredentialChange(t *testing.T) {
	client := NewClient()
	first := base64.StdEncoding.EncodeToString([]byte("mobile:13800000001:first"))
	second := base64.StdEncoding.EncodeToString([]byte("mobile:13800000002:second"))
	read := func() string {
		for _, cookie := range client.GetCookies("m.mcloud.139.com") {
			if cookie.Name == "cookieTokenKey" {
				return cookie.Value
			}
		}
		return ""
	}
	client.SetAuth("Basic " + first)
	if read() != url.QueryEscape(first) {
		t.Fatal("account auth cookie missing")
	}
	client.SetAuth("basic " + second)
	if read() != url.QueryEscape(second) {
		t.Fatal("old account auth cookie retained")
	}
	if len(client.GetCookies("unrelated.example")) != 0 {
		t.Fatal("auth cookie escaped activity host")
	}
	client.SetAuth("")
	if read() != "" {
		t.Fatal("cleared authorization left a credential cookie")
	}
}
