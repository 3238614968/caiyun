package shumei

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDeviceRegistrationKeepsTheActivityUserAgent(t *testing.T) {
	const userAgent = "Mozilla/5.0 (Linux; Android 13; wv) MCloudApp/13.2.5"
	const pageURL = "https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin&token=fixture-sso"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body deviceProfileRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if r.Header.Get("User-Agent") != userAgent || r.Header.Get("Referer") != pageURL || body.Encode != 5 || body.Compress != 2 {
			t.Error("registration changed activity identity or SDK format")
		}
		_, _ = io.WriteString(w, `{"code":1100,"detail":{"deviceId":"fixture-device"}}`)
	}))
	defer server.Close()
	id, err := FetchDeviceIDForPage(context.Background(), server.Client(), server.URL, userAgent, pageURL)
	if err != nil || id != "Bfixture-device" {
		t.Fatalf("device registration: %q %v", id, err)
	}
}

func TestDeviceRegistrationNeverFallsBackToExceptionEncoding(t *testing.T) {
	for _, reply := range []string{`{"code":1902,"msg":"profile rejected"}`, `{"code":1100,"detail":{}}`} {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			var body deviceProfileRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Encode != 5 || body.Compress != 2 {
				t.Error("downgraded to exception encoding")
			}
			_, _ = io.WriteString(w, reply)
		}))
		_, err := FetchDeviceID(context.Background(), server.Client(), server.URL)
		server.Close()
		if err == nil || requests != 1 || !strings.Contains(err.Error(), "deviceprofile returned") {
			t.Fatalf("registration failure was hidden: requests=%d err=%v", requests, err)
		}
	}
}

func TestCanceledDeviceRegistrationDoesNotSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FetchDeviceID(ctx, nil, "http://127.0.0.1:1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled registration: %v", err)
	}
}

func TestRawServerDeviceStartingWithBStillGetsProtocolPrefix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"code":1100,"detail":{"deviceId":"Braw-fixture"}}`)
	}))
	defer server.Close()
	id, err := FetchDeviceID(context.Background(), server.Client(), server.URL)
	if err != nil || id != "BBraw-fixture" {
		t.Fatalf("raw device prefix confused with protocol prefix: %q %v", id, err)
	}
}

func TestDeviceCookiesUseSDKOrganizationAndEncodedRawID(t *testing.T) {
	jar, _ := cookiejar.New(nil)
	const page = "https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin"
	SeedDeviceCookies(jar, page, "Braw+/=", "fixture-smid")
	u, _ := url.Parse(page)
	cookies := map[string]string{}
	for _, cookie := range jar.Cookies(u) {
		cookies[cookie.Name] = cookie.Value
	}
	if DeviceCookieName() != ".thumbcache_45700955f71be4ef518b0a1af26a3f40" || cookies[DeviceCookieName()] != "raw%2B%2F%3D" || cookies["smidV2"] != "fixture-smid" {
		t.Fatalf("wrong SDK cookie shape: %v", cookies)
	}
	other, _ := url.Parse("https://slw.h5cmpassport.com:9090/deviceprofile/v4")
	if len(jar.Cookies(other)) != 0 {
		t.Fatal("activity cookies leaked to registration host")
	}
	isolated, _ := cookiejar.New(nil)
	SeedDeviceCookies(isolated, page, "Bother-device", "other-smid")
	if jar.Cookies(u)[0].Value == isolated.Cookies(u)[0].Value {
		t.Fatal("account device cookie jars mixed")
	}
}
