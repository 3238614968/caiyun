package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

type h5LoginTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func TestH5SignatureMatchesIndependentVector(t *testing.T) {
	got := h5AuthSignature("00000000-1111-4222-8333-444444444444", "1700000000123", "11111111-2222-4333-8444-555555555555")
	if got != "e45e293cef406857c28f43ba32585e3c" {
		t.Fatalf("signature=%s", got)
	}
}

func (tr h5LoginTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	u := *clone.URL
	u.Scheme = tr.target.Scheme
	u.Host = tr.target.Host
	clone.URL = &u
	return tr.base.RoundTrip(clone)
}

func TestJWTLoginUsesCapturedH5EndpointAndPreservesBothCookies(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ycloud/api/timestamp" {
			_, _ = io.WriteString(w, `{"code":0,"result":1700000000123}`)
			return
		}
		requests++
		if r.Method != "POST" || r.URL.Path != "/ycloud/auth-service/auth/tyrzLogin" || r.URL.RawQuery != "" {
			t.Errorf("unexpected login route: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body) != 4 || body["token"] != "fixture-sso" || body["openAccount"] != float64(0) || body["marketName"] != "sign_in_3" || body["sourceId"] != "1097" {
			t.Error("login body differs from HAR entry 1352")
		}
		id, nonce, timestamp := r.Header.Get("x-request-id"), r.Header.Get("x-nonce"), r.Header.Get("x-timestamp")
		if len(id) != 36 || len(nonce) != 36 || id == nonce || timestamp != "1700000000123" || r.Header.Get("x-signature") != h5AuthSignature(id, timestamp, nonce) {
			t.Error("missing or incorrect captured login signature headers")
		}
		if !strings.Contains(r.Header.Get("User-Agent"), "MCloudApp/13.2.5") {
			t.Error("login uses an inconsistent app version")
		}
		http.SetCookie(w, &http.Cookie{Name: "jwtToken", Value: "fixture-jwt", Path: "/", Secure: true})
		http.SetCookie(w, &http.Cookie{Name: "userDomainId", Value: "fixture-domain", Path: "/", Secure: true})
		_, _ = io.WriteString(w, `{"code":0,"result":{"token":"fixture-jwt","userDomainId":"fixture-domain"}}`)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	client := corehttp.NewClient(corehttp.WithTransport(h5LoginTransport{target, http.DefaultTransport}))
	jwt, err := NewAuth(client).TyrzLoginContext(context.Background(), "fixture-sso")
	if err != nil || jwt != "fixture-jwt" || requests != 1 {
		t.Fatalf("JWT login: requests=%d jwt=%q err=%v", requests, jwt, err)
	}
	values := map[string]string{}
	for _, cookie := range client.GetCookies("m.mcloud.139.com") {
		values[cookie.Name] = cookie.Value
	}
	if values["jwtToken"] != "fixture-jwt" || values["userDomainId"] != "fixture-domain" {
		t.Fatal("multiple Set-Cookie fields were not preserved")
	}
}
