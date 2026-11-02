package services

import (
	"caiyun/internal/core/shumei"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Replays the HAR's contract with synthetic credentials and server sessions.
// Only the public challenge images are copied from entries[1794]; no live
// requests or captured device IDs/cookies are used.
func TestExchangeCapturedFlowUsesVariablePrizeAndServerCookie(t *testing.T) {
	load := func(name string) string {
		data, err := os.ReadFile("../core/slider/testdata/har-success-" + name + ".png")
		if err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(data)
	}
	picture, puzzle := load("picture"), load("puzzle")
	for _, tc := range []struct {
		prizeID     string
		visitStatus int
	}{
		{"260630012", 200}, {"12345", 200}, {"12345", 503},
	} {
		t.Run(fmt.Sprintf("%s-%d", tc.prizeID, tc.visitStatus), func(t *testing.T) {
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture-domain"}}`))
			jwt := "header." + claims + ".signature"
			basic := base64.StdEncoding.EncodeToString([]byte("mobile:13800000001:fixture-auth"))
			var steps []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/deviceprofile/v4" {
					_, _ = io.WriteString(w, `{"code":1100,"detail":{"deviceId":"fixture-device"}}`)
					return
				}
				if r.URL.Path == "/portal/mobilecloud/index.html" {
					_, _ = io.WriteString(w, "<html></html>")
					return
				}
				if r.URL.Path == "/ycloud/api/timestamp" {
					_, _ = io.WriteString(w, `{"code":0,"result":1700000000123}`)
					return
				}
				if r.URL.Path == "/ycloud/auth-service/auth/tyrzLogin" {
					if cookie, err := r.Cookie(shumei.DeviceCookieName()); err != nil || cookie.Value != "fixture-device" {
						t.Error("login lost the registered SDK cookie")
					}
					if r.Header.Get("Authorization") != "" || r.Header.Get("x-signature") == "" {
						t.Error("login used cloud Basic defaults or omitted signature")
					}
					http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session-at-login", Path: "/", Secure: true})
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "result": map[string]string{"token": jwt}})
					return
				}
				steps = append(steps, r.URL.Path)
				if len(r.Header.Values("jwttoken")) != 1 || r.Header.Get("deviceId") != "" {
					t.Error("request did not match HAR authentication headers")
				}
				for name, want := range map[string]string{
					"jwtToken": jwt, "userDomainId": "fixture-domain", "cookieTokenKey": url.QueryEscape(basic),
					shumei.DeviceCookieName(): "fixture-device",
				} {
					cookie, err := r.Cookie(name)
					if err != nil || cookie.Value != want {
						t.Errorf("missing/mismatched current-session cookie %s", name)
					}
				}
				if cookie, err := r.Cookie("smidV2"); err != nil || len(cookie.Value) != 63 {
					t.Error("missing SDK local identifier")
				}
				switch r.URL.Path {
				case "/ycloud/signin/page/receivePrizeDetails":
					if cookie, err := r.Cookie("JSESSIONID"); err != nil || cookie.Value != "session-at-login" {
						t.Error("eligibility did not inherit login session")
					}
					if r.URL.Query().Get("prizeId") != tc.prizeID {
						t.Error("qualification used the captured rather than selected prize")
					}
					http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session-before-visit", Path: "/"})
					_, _ = io.WriteString(w, `{"code":0,"result":{"exchangeCode":0}}`)
				case "/ycloud/visitlog/journaling":
					if err := r.ParseForm(); err != nil {
						t.Error(err)
					}
					if r.PostForm.Get("optkeyword") != "newsignin_index_exchange_times" || r.PostForm.Get("sourceid") != "1097" {
						t.Error("visit event differs from HAR entry 1793")
					}
					if tc.visitStatus == 200 {
						http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: "session-after-visit", Path: "/"})
					}
					w.WriteHeader(tc.visitStatus)
					_, _ = io.WriteString(w, `{"code":0}`)
				case "/ycloud/auth-service/slide/getSlide":
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "result": map[string]interface{}{
						"picture": picture, "puzzle": puzzle, "picWidth": "680", "picHeight": "400", "puzzleWidth": "96",
					}})
				case "/ycloud/signin/page/exchangeV3":
					var payload map[string]interface{}
					decoder := json.NewDecoder(r.Body)
					decoder.UseNumber()
					if err := decoder.Decode(&payload); err != nil {
						t.Error(err)
					}
					wantCookie := "session-after-visit"
					if tc.visitStatus != 200 {
						wantCookie = "session-before-visit"
					}
					cookie, err := r.Cookie("JSESSIONID")
					if err != nil || cookie.Value != wantCookie {
						t.Errorf("exchange lost the server session: %v %v", cookie, err)
					}
					if fmt.Sprint(payload["prizeId"]) != tc.prizeID || fmt.Sprint(payload["puzzleOffset"]) != "546" || payload["deviceId"] != "Bfixture-device" || len(payload) != 6 {
						t.Errorf("exchange JSON differs from captured contract: %v", payload)
					}
					_, _ = fmt.Fprintf(w, `{"code":0,"msg":"success","result":{"oid":"fixture-order","prizeId":%s}}`, tc.prizeID)
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			t.Setenv("CAIYUN_SHUMEI_DEVICE_API", server.URL+"/deviceprofile/v4")
			t.Setenv("CAIYUN_EXCHANGE_SLIDE_SOLVER", "builtin")
			t.Setenv("CAIYUN_EXCHANGE_SLIDE_MIN_CONFIDENCE", "0.75")
			auth := &exchangeAuthContext{jwtToken: jwt, ssoToken: "fixture-sso", authorization: "Basic " + basic}
			session := newExchangeHTTPSessionContext(context.Background(), nil, auth)
			target, _ := url.Parse(server.URL)
			session.client.Transport = &exchangeRewriteRoundTripper{base: http.DefaultTransport, targetURL: target}
			if err := ensureExchangeSessionLogin(context.Background(), session, auth); err != nil {
				t.Fatal(err)
			}
			result := executeExchangeOnce(tc.prizeID, auth, session)
			wantSteps := []string{"/ycloud/signin/page/receivePrizeDetails", "/ycloud/visitlog/journaling", "/ycloud/auth-service/slide/getSlide", "/ycloud/signin/page/exchangeV3"}
			if !result.success || !reflect.DeepEqual(steps, wantSteps) {
				t.Fatalf("result=%+v steps=%v", result, steps)
			}
			if tc.visitStatus == 503 && !strings.Contains(result.message, "活动访问记录未完成") {
				t.Fatal("optional visit failure was lost")
			}
		})
	}
}
