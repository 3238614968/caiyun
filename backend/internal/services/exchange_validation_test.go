package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExchangeRejectsWeakMatchesAndUsesLatestChallengeSession(t *testing.T) {
	for _, goodSecond := range []bool{true, false} {
		t.Run(fmt.Sprint(goodSecond), func(t *testing.T) {
			slides, solves, exchanges := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/ycloud/visitlog/journaling":
					_, _ = io.WriteString(w, `{"code":0}`)
				case "/ycloud/auth-service/slide/getSlide":
					slides++
					http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: fmt.Sprintf("session-%d", slides), Path: "/"})
					w.Header().Set("x-yun-tid", fmt.Sprintf("challenge-%d", slides))
					_, _ = io.WriteString(w, `{"code":0,"result":{"puzzle":"piece","picture":"background","picWidth":680,"puzzleWidth":96}}`)
				case "/ycloud/signin/page/receivePrizeDetails":
					_, _ = io.WriteString(w, `{"code":0,"result":{"exchangeCode":"0"}}`)
				case "/api/sms/solve":
					solves++
					confidence := 0.5613
					if goodSecond && solves >= 2 {
						confidence = 0.9
					}
					_, _ = fmt.Fprintf(w, `{"code":0,"data":{"offset":196,"confidence":%v,"method":"NCC"}}`, confidence)
				case "/ycloud/signin/page/exchangeV3":
					exchanges++
					cookie, err := r.Cookie("JSESSIONID")
					if err != nil || cookie.Value != "session-2" || r.Header.Get("x-yun-tid") != "" {
						t.Errorf("exchange did not use the accepted challenge session: cookie=%v err=%v", cookie, err)
					}
					_, _ = io.WriteString(w, `{"code":0,"msg":"success","result":{"oid":"fixture-order","prizeId":12345}}`)
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
			t.Setenv("CAIYUN_EXCHANGE_SLIDE_MIN_CONFIDENCE", "0.75")
			session := newMockExchangeSession(t, server)
			session.client.Jar, _ = cookiejar.New(nil)
			result := executeExchangeOnce("12345", &exchangeAuthContext{jwtToken: "fixture-jwt"}, session)
			if goodSecond {
				if !result.success || slides != 2 || solves != 2 || exchanges != 1 {
					t.Fatalf("slides=%d solves=%d exchanges=%d result=%+v", slides, solves, exchanges, result)
				}
			} else if result.success || slides != 3 || exchanges != 0 || !strings.Contains(result.message, "置信度不足") || isExchangeRetryableFailure(result.message) {
				t.Fatalf("weak coordinates submitted or unbounded retry: slides=%d exchanges=%d result=%+v", slides, exchanges, result)
			}
		})
	}
}

func TestExchangeDeviceProfileFailureDoesNotInventDeviceHeadersOrCookies(t *testing.T) {
	profiles := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		profiles++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":1900,"msg":"device unavailable"}`)
	}))
	defer server.Close()
	t.Setenv("CAIYUN_SHUMEI_DEVICE_API", server.URL)
	auth := &exchangeAuthContext{jwtToken: "fixture-jwt"}
	session := newExchangeHTTPSessionContext(context.Background(), nil, auth)
	if profiles != 1 || session.deviceErr == nil || session.deviceID != "" {
		t.Fatalf("profiles=%d device=%q err=%v", profiles, session.deviceID, session.deviceErr)
	}
	for key, value := range buildExchangeHeaders(auth, session, nil) {
		if strings.EqualFold(key, "deviceid") {
			t.Fatalf("device profile failure produced a fallback header: %q", value)
		}
	}
	endpoint, _ := http.NewRequest("GET", "https://m.mcloud.139.com", nil)
	for _, cookie := range session.client.Jar.Cookies(endpoint.URL) {
		if strings.HasPrefix(cookie.Name, ".thumbcache_") {
			t.Fatalf("device profile failure produced a fallback cookie: %s", cookie.Name)
		}
	}
}

func TestExchangeStopsBeforeCaptchaWhenDeviceAuthenticationFails(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Error("request sent without an authenticated device")
	}))
	defer server.Close()
	session := newMockExchangeSession(t, server)
	session.deviceID = ""
	session.deviceErr = errors.New("设备认证失败，未提交兑换")
	result := executeExchangeOnce("12345", &exchangeAuthContext{jwtToken: "fixture-jwt"}, session)
	if result.success || requests != 0 || !result.stop || isExchangeRetryableFailure(result.message) {
		t.Fatalf("requests=%d result=%+v", requests, result)
	}
}

func TestExchangeStopsBeforeCaptchaWhenEligibilityIsNotMet(t *testing.T) {
	for _, code := range []int{2302, 2303, 2305} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/ycloud/signin/page/receivePrizeDetails" {
					t.Errorf("request sent despite ineligible account: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"code":0,"result":{"exchangeCode":%d}}`, code)
			}))
			defer server.Close()
			result := executeExchangeOnce("12345", &exchangeAuthContext{jwtToken: "fixture-jwt"}, newMockExchangeSession(t, server))
			if result.success || requests != 1 || !result.stop || isExchangeRetryableFailure(result.message) || !strings.Contains(result.message, fmt.Sprint(code)) {
				t.Fatalf("requests=%d result=%+v", requests, result)
			}
		})
	}
}

func TestExchangeNeverSubmitsMissingOrNullSolverCoordinates(t *testing.T) {
	for _, data := range []string{`{"confidence":0.9}`, `{"offset":null,"confidence":0.9}`} {
		t.Run(data, func(t *testing.T) {
			solves, exchanges := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/ycloud/visitlog/journaling":
					_, _ = io.WriteString(w, `{"code":0}`)
				case "/ycloud/signin/page/receivePrizeDetails":
					_, _ = io.WriteString(w, `{"code":0,"result":{"exchangeCode":"0"}}`)
				case "/ycloud/auth-service/slide/getSlide":
					_, _ = io.WriteString(w, `{"code":0,"result":{"puzzle":"piece","picture":"background","picWidth":680,"puzzleWidth":96}}`)
				case "/api/sms/solve":
					solves++
					_, _ = io.WriteString(w, `{"code":0,"data":`+data+`}`)
				case "/ycloud/signin/page/exchangeV3":
					exchanges++
					t.Error("exchange submitted without a solver coordinate")
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
			result := executeExchangeOnce("12345", &exchangeAuthContext{jwtToken: "fixture-jwt"}, newMockExchangeSession(t, server))
			if result.success || solves != exchangeSlideMaxAttempt || exchanges != 0 || !strings.Contains(result.message, "缺少 offset") {
				t.Fatalf("solves=%d exchanges=%d result=%+v", solves, exchanges, result)
			}
		})
	}
}
