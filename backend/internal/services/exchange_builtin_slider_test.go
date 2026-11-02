package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBuiltinExchangeSolvesLocallyAndRefreshesFailedChallenge(t *testing.T) {
	load := func(name string) string {
		data, err := os.ReadFile("../core/slider/testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(data)
	}
	picture, puzzle := load("reference-picture.png"), load("reference-puzzle.png")
	flat := image.NewNRGBA(image.Rect(0, 0, 96, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 96; x++ {
			flat.SetNRGBA(x, y, color.NRGBA{40, 40, 40, 255})
		}
	}
	var flatBytes bytes.Buffer
	if err := png.Encode(&flatBytes, flat); err != nil {
		t.Fatal(err)
	}
	flatPuzzle := base64.StdEncoding.EncodeToString(flatBytes.Bytes())
	for _, tc := range []struct {
		name                     string
		invalidFirst, allInvalid bool
		solver, minConfidence    string
		wantSlides               int
		wantFailure              string
	}{
		{name: "default-local", wantSlides: 1},
		{name: "explicit-builtin", solver: "builtin", wantSlides: 1},
		{name: "refresh-challenge", invalidFirst: true, wantSlides: 2},
		{name: "stop-after-three", allInvalid: true, wantSlides: 3, wantFailure: "缺少可辨识纹理"},
		{name: "low-confidence", minConfidence: "0.95", wantSlides: 3, wantFailure: "置信度不足"},
		{name: "invalid-mode", solver: "unsupported", wantSlides: 0, wantFailure: "识别方式无效"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			slides, remoteCalls, exchanges := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/ycloud/visitlog/journaling":
					_, _ = io.WriteString(w, `{"code":0}`)
				case "/ycloud/signin/page/receivePrizeDetails":
					_, _ = io.WriteString(w, `{"code":0,"result":{"exchangeCode":"0"}}`)
				case "/ycloud/auth-service/slide/getSlide":
					slides++
					http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: fmt.Sprint(slides), Path: "/"})
					w.Header().Set("x-yun-tid", fmt.Sprint(slides))
					currentPuzzle := puzzle
					if tc.allInvalid || tc.invalidFirst && slides == 1 {
						currentPuzzle = flatPuzzle
					}
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "result": map[string]interface{}{"picture": picture, "puzzle": currentPuzzle, "picWidth": 680, "picHeight": 400, "puzzleWidth": 96}})
				case "/api/sms/solve":
					remoteCalls++
					t.Error("default solver sent images to a remote recognition service")
				case "/ycloud/signin/page/exchangeV3":
					exchanges++
					var payload map[string]interface{}
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					cookie, err := r.Cookie("JSESSIONID")
					if payload["puzzleOffset"] != float64(392) || err != nil || cookie.Value != fmt.Sprint(slides) || r.Header.Get("x-yun-tid") != "" {
						t.Errorf("local coordinate or challenge session mismatch: payload=%v cookie=%v err=%v", payload, cookie, err)
					}
					_, _ = io.WriteString(w, `{"code":0,"msg":"success","result":{"oid":"fixture-order","prizeId":12345}}`)
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			session := newMockExchangeSession(t, server)
			session.client.Jar, _ = cookiejar.New(nil)
			// Empty configuration must actually select the local implementation.
			t.Setenv("CAIYUN_EXCHANGE_SLIDE_SOLVER", tc.solver)
			threshold := tc.minConfidence
			if threshold == "" {
				threshold = "0.75"
			}
			t.Setenv("CAIYUN_EXCHANGE_SLIDE_MIN_CONFIDENCE", threshold)
			t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
			result := executeExchangeOnce("12345", &exchangeAuthContext{jwtToken: "fixture-jwt"}, session)
			if remoteCalls != 0 {
				t.Fatalf("remote calls=%d", remoteCalls)
			}
			if tc.wantFailure != "" {
				if result.success || slides != tc.wantSlides || exchanges != 0 || isExchangeRetryableFailure(result.message) || !strings.Contains(result.message, tc.wantFailure) {
					t.Fatalf("slides=%d exchanges=%d result=%+v", slides, exchanges, result)
				}
			} else {
				if !result.success || slides != tc.wantSlides || exchanges != 1 || !strings.Contains(result.message, "solver=builtin") || !strings.Contains(result.message, "solve_ms=") {
					t.Fatalf("slides=%d exchanges=%d result=%+v", slides, exchanges, result)
				}
			}
		})
	}
}
