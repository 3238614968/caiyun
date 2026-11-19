package api

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestCocWeb6AuroraMatchesReferenceAESAndWrapsASCIIHexKey(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	entropy := io.MultiReader(bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7}), rand.Reader)
	body, headers, err := encryptCocWeb6Login("offline", &private.PublicKey, entropy)
	if err != nil {
		t.Fatal(err)
	}
	// Python reference's AES-ECB/PKCS7 with key=b"0001020304050607".
	if body["cocEnContent"] != "jRpB5o0vHfwHpcWc96kjzu14zoOpvLQsdSZRhDGMG76Rh5ibDly4iBzjDI5J8lFh" {
		t.Fatalf("AES differs from reference: %v", body)
	}
	wrapped, err := base64.StdEncoding.DecodeString(headers["coc-aurora"])
	if err != nil {
		t.Fatal(err)
	}
	key, err := rsa.DecryptPKCS1v15(rand.Reader, private, wrapped)
	if err != nil || string(key) != "0001020304050607" {
		t.Fatalf("key=%q err=%v", key, err)
	}
	der, _ := base64.StdEncoding.DecodeString(cocWeb6PublicKey)
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.(*rsa.PublicKey).N.BitLen() != 2048 || headers["Coc-En-Version"] != "1486234121417629696" || headers["coc-aurora-v"] != "2" {
		t.Fatal("invalid official aurora profile")
	}
	if _, _, err := encryptCocWeb6Login("offline", &private.PublicKey, strings.NewReader("short")); err == nil {
		t.Fatal("failed entropy must not produce a request")
	}
}

func TestCocWeb6LoginRequiresBusinessSuccessAndFreshCookie(t *testing.T) {
	for _, tc := range []struct {
		name, body, cookie string
		status             int
		ok                 bool
	}{
		{"valid", `{"code":"0"}`, "User-Token=web6-session; Path=/; HttpOnly", 200, true},
		{"ack without cookie", `{"code":0}`, "", 200, false},
		{"deleted cookie", `{"code":0}`, "User-Token=old; Max-Age=0", 200, false},
		{"expired cookie", `{"code":0}`, "User-Token=old; Expires=Thu, 01 Jan 1970 00:00:01 GMT", 200, false},
		{"explicit refusal with cookie", `{"code":0,"success":false}`, "User-Token=unused", 200, false},
		{"task envelope with cookie", `{"resultCode":0}`, "User-Token=unused", 200, false},
		{"rejected login", `{"code":"10","message":"归属地校验失败"}`, "User-Token=old", 200, false},
		{"bad HTTP", `{"code":0}`, "User-Token=old", 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tickets, logins := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(coc3APIRoundTrip(func(req *http.Request) (*http.Response, error) {
				reply := `{"code":"0","success":true,"data":{"token":"one-use-ticket"}}`
				status, headers := 200, make(http.Header)
				switch req.URL.Path {
				case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
					tickets++
					var payload map[string]string
					_ = json.NewDecoder(req.Body).Decode(&payload)
					if payload["toSourceId"] != "056002" {
						t.Fatalf("ticket payload=%v", payload)
					}
				case "/coc/user/loginCheck":
					logins++
					if req.Header.Get("Authorization") != "" || req.Header.Get("jwtToken") != "" || req.Header.Get("deviceId") != "" {
						t.Fatal("cloud credentials or device headers leaked into the independent web6 session")
					}
					var payload map[string]string
					_ = json.NewDecoder(req.Body).Decode(&payload)
					if payload["cocEnContent"] == "" || req.Header.Get("Coc-En-Version") != "1486234121417629696" || req.Header.Get("coc-aurora") == "" {
						t.Fatal("unencrypted login")
					}
					if req.Header.Get("wasmParams") != "" || strings.Contains(req.Header.Get("Cookie"), "COC3_LOGIN_TOKEN") {
						t.Fatal("mixed COC3 and web6 sessions")
					}
					reply, status = tc.body, tc.status
					if tc.cookie != "" {
						headers.Add("Set-Cookie", tc.cookie)
					}
				default:
					t.Fatalf("unexpected %s", req.URL)
				}
				return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(reply)), Request: req}, nil
			})))
			client.SetAuth("Basic unrelated-cloud-auth")
			client.SetJWTToken("unrelated-cloud-jwt")
			token, err := NewCocWeb6API(client).Login("Basic fixture", "13800000001")
			if (err == nil) != tc.ok || tickets != 1 || logins != 1 || (tc.ok && token != "web6-session") {
				t.Fatalf("token=%q err=%v tickets=%d logins=%d", token, err, tickets, logins)
			}
		})
	}
}

func TestCocWeb6EvaluationStatusIsNotInvertedOrAssumed(t *testing.T) {
	for _, tc := range []struct {
		data        string
		done, valid bool
	}{
		{`false`, false, true}, {`0`, false, true}, {`"0"`, false, true},
		{`true`, true, true}, {`1`, true, true}, {`"1"`, true, true},
		{`null`, false, false}, {`{}`, false, false}, {`"unknown"`, false, false},
	} {
		t.Run(tc.data, func(t *testing.T) {
			client := corehttp.NewClient(corehttp.WithTransport(coc3APIRoundTrip(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0,"data":` + tc.data + `}`)), Request: req}, nil
			})))
			done, err := NewCocWeb6API(client).Evaluated("token")
			if (err == nil) != tc.valid || done != tc.done {
				t.Fatalf("done=%v err=%v", done, err)
			}
		})
	}
}
