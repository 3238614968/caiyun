package auth

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
	"github.com/google/uuid"
)

func TestBasicSchemeIsCaseInsensitive(t *testing.T) {
	canonical := GenerateAuth("fresh-token", "13800000001", "mobile")
	lowercase := "basic " + strings.TrimPrefix(canonical, "Basic ")
	parsed, err := ParseToken(lowercase)
	if err != nil || parsed.Token != "fresh-token" {
		t.Fatalf("ParseToken(%q) = %+v, %v", lowercase, parsed, err)
	}
	if got := ensureBasicAuth(lowercase); got != canonical {
		t.Fatalf("ensureBasicAuth() = %q, want canonical Basic form", got)
	}
}

func TestAppRefreshEncryptDecrypt(t *testing.T) {
	plain := `{"clientType":"414"}`
	encrypted, err := appRefreshEncrypt(plain)
	if err != nil {
		t.Fatalf("appRefreshEncrypt failed: %v", err)
	}
	if encrypted == "" {
		t.Fatal("encrypted is empty")
	}

	decrypted, err := appRefreshDecrypt(encrypted)
	if err != nil {
		t.Fatalf("appRefreshDecrypt failed: %v", err)
	}
	if decrypted != plain {
		t.Fatalf("decrypted mismatch: got %q want %q", decrypted, plain)
	}
}

func TestNormalizeJSONString(t *testing.T) {
	if got := normalizeJSONString(`"abc=="`); got != "abc==" {
		t.Fatalf("normalize quoted: got %q", got)
	}
	if got := normalizeJSONString(`abc==`); got != "abc==" {
		t.Fatalf("normalize raw: got %q", got)
	}
}

func TestParseRefreshAuthToken(t *testing.T) {
	body := `{"success":true,"code":"0000","message":"请求成功","data":{"token":"new-token"}}`
	token, err := parseRefreshAuthToken(body)
	if err != nil {
		t.Fatalf("parseRefreshAuthToken failed: %v", err)
	}
	if token != "new-token" {
		t.Fatalf("token mismatch: %s", token)
	}

	if _, err := parseRefreshAuthToken(`{"success":false,"code":"1001","message":"bad"}`); err == nil {
		t.Fatal("expected business error")
	}
}

func TestGenerateMobileAuthorizationFromRefreshToken(t *testing.T) {
	expireAt := time.Now().Add(30 * 24 * time.Hour).UnixMilli()
	token := fmt.Sprintf("VWgoC0wt|1|RCS|%d|tail", expireAt)
	authValue := GenerateAuth(token, "13800138000", "mobile")

	info, err := ParseToken(authValue)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	if info.Platform != "mobile" || info.Phone != "13800138000" || info.Token != token {
		t.Fatalf("unexpected token info: %+v", info)
	}
	if info.AuthFull == "" || !strings.HasPrefix(info.AuthFull, "Basic ") {
		t.Fatalf("unexpected auth full: %q", info.AuthFull)
	}
}

type authRoundTripper func(*http.Request) (*http.Response, error)

func (fn authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func authTestResponse(req *http.Request, status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestTyrzLoginUsesSignedJSONAndPrefersJWTHeader(t *testing.T) {
	const serverTimestamp = "1700000000123"
	var requestIDs []string
	var nonces []string
	client := corehttp.NewClient(corehttp.WithTransport(authRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "" || req.Header.Get("jwttoken") != "" || req.Header.Get("jwtToken") != "" || req.Header.Get("Cookie") != "" {
			t.Errorf("request unexpectedly included account credentials: headers=%v", req.Header)
		}
		switch req.URL.String() {
		case tyrzTimestampURL:
			if req.Method != http.MethodGet {
				t.Errorf("timestamp method = %s, want GET", req.Method)
			}
			return authTestResponse(req, http.StatusOK, `{"result":1700000000123}`, nil), nil
		case tyrzLoginURL:
			if req.Method != http.MethodPost {
				t.Errorf("login method = %s, want POST", req.Method)
			}
			if got := req.Header.Get("Content-Type"); got != "application/json;charset=UTF-8" {
				t.Errorf("Content-Type = %q", got)
			}
			var payload struct {
				Token       string `json:"token"`
				OpenAccount int    `json:"openAccount"`
				MarketName  string `json:"marketName"`
				SourceID    string `json:"sourceId"`
			}
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Errorf("decode login body: %v", err)
			}
			if payload.Token != "original-sso" || payload.OpenAccount != 0 || payload.MarketName != "sign_in_3" || payload.SourceID != "1097" {
				t.Errorf("login body = %+v", payload)
			}
			if got := req.Header.Get("x-timestamp"); got != serverTimestamp {
				t.Errorf("x-timestamp = %q, want %q", got, serverTimestamp)
			}
			requestID := req.Header.Get("x-request-id")
			nonce := req.Header.Get("x-nonce")
			requestUUID, requestErr := uuid.Parse(requestID)
			nonceUUID, nonceErr := uuid.Parse(nonce)
			if requestErr != nil || requestUUID.Version() != 4 || nonceErr != nil || nonceUUID.Version() != 4 {
				t.Errorf("request ID and nonce must be UUIDv4: id=%q nonce=%q", requestID, nonce)
			}
			signatureInput := tyrzLoginSigningKey + requestID + serverTimestamp + nonce + tyrzLoginSigningKey
			wantSignature := md5.Sum([]byte(signatureInput))
			if got, want := req.Header.Get("x-signature"), hex.EncodeToString(wantSignature[:]); got != want {
				t.Errorf("x-signature = %q, want %q", got, want)
			}
			requestIDs = append(requestIDs, requestID)
			nonces = append(nonces, nonce)
			headers := make(http.Header)
			headers.Add("Set-Cookie", "jwtToken=cookie-jwt; Path=/")
			headers.Set("jwttoken", "header-jwt")
			return authTestResponse(req, http.StatusOK, `{"code":0,"result":{"token":"json-jwt"}}`, headers), nil
		default:
			t.Errorf("unexpected URL: %s", req.URL)
			return authTestResponse(req, http.StatusNotFound, `{}`, nil), nil
		}
	})))
	client.SetAuth("Basic stale-basic")
	client.SetJWTToken("stale-jwt")
	client.SetCookie("jwtToken", "stale-cookie", "m.mcloud.139.com")
	authenticator := NewAuth(client)

	for i := 0; i < 2; i++ {
		got, err := authenticator.TyrzLogin("original-sso")
		if err != nil {
			t.Fatalf("TyrzLogin() error = %v", err)
		}
		if got != "header-jwt" {
			t.Fatalf("TyrzLogin() = %q, want response header token", got)
		}
		if client.GetJWTToken() != "stale-jwt" {
			t.Fatalf("TyrzLogin() changed client state: JWT = %q", client.GetJWTToken())
		}
	}
	if len(requestIDs) != 2 || requestIDs[0] == requestIDs[1] || nonces[0] == nonces[1] {
		t.Fatalf("login signature inputs were not regenerated: requestIDs=%v nonces=%v", requestIDs, nonces)
	}
}

func TestTyrzLoginFallsBackToLocalTimestampAndResponseCookie(t *testing.T) {
	var loginTimestamp string
	client := corehttp.NewClient(corehttp.WithTransport(authRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == tyrzTimestampURL {
			return authTestResponse(req, http.StatusNotFound, `temporarily unavailable`, nil), nil
		}
		if req.URL.String() != tyrzLoginURL {
			return authTestResponse(req, http.StatusNotFound, `{}`, nil), nil
		}
		loginTimestamp = req.Header.Get("x-timestamp")
		headers := make(http.Header)
		headers.Add("Set-Cookie", "jwtToken=cookie-jwt; Path=/")
		return authTestResponse(req, http.StatusOK, `{"code":0,"result":{"token":"json-jwt"}}`, headers), nil
	})))
	got, err := NewAuth(client).TyrzLogin("original-sso")
	if err != nil {
		t.Fatalf("TyrzLogin() error = %v", err)
	}
	if got != "cookie-jwt" {
		t.Fatalf("TyrzLogin() = %q, want response cookie token", got)
	}
	timestamp, err := strconv.ParseInt(loginTimestamp, 10, 64)
	if err != nil {
		t.Fatalf("local fallback timestamp %q is invalid: %v", loginTimestamp, err)
	}
	now := time.Now().UnixMilli()
	if delta := now - timestamp; delta < 0 || delta > 1000 {
		t.Fatalf("fallback timestamp = %d, current time = %d", timestamp, now)
	}
	if client.GetJWTToken() != "" {
		t.Fatalf("TyrzLogin() changed client state: JWT = %q", client.GetJWTToken())
	}
}

func TestTyrzLoginRejectsJSONTokenWithoutJWTHeaderOrCookie(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(authRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() == tyrzTimestampURL {
			return authTestResponse(req, http.StatusOK, `{"result":"1700000000123"}`, nil), nil
		}
		return authTestResponse(req, http.StatusOK, `{"code":0,"result":{"token":"json-only-jwt"}}`, nil), nil
	})))
	if _, err := NewAuth(client).TyrzLogin("original-sso"); err == nil || !strings.Contains(err.Error(), "未返回 JWT") {
		t.Fatalf("TyrzLogin() error = %v, want missing JWT error", err)
	}
	if client.GetJWTToken() != "" {
		t.Fatalf("client JWT = %q, want empty", client.GetJWTToken())
	}
}

func TestTyrzLoginReportsHTTPBusinessAndJSONErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantErrSub string
	}{
		{name: "http error", status: http.StatusNotFound, body: `not found`, wantErrSub: "HTTP 状态异常"},
		{name: "business error", status: http.StatusOK, body: `{"code":1001,"message":"login rejected"}`, wantErrSub: "业务失败"},
		{name: "non JSON response", status: http.StatusOK, body: `<html>login</html>`, wantErrSub: "解析新版 tyrzLogin 响应失败"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := corehttp.NewClient(corehttp.WithTransport(authRoundTripper(func(req *http.Request) (*http.Response, error) {
				if req.URL.String() == tyrzTimestampURL {
					return authTestResponse(req, http.StatusOK, `{"result":1700000000123}`, nil), nil
				}
				return authTestResponse(req, test.status, test.body, nil), nil
			})))
			if _, err := NewAuth(client).TyrzLogin("original-sso"); err == nil || !strings.Contains(err.Error(), test.wantErrSub) {
				t.Fatalf("TyrzLogin() error = %v, want error containing %q", err, test.wantErrSub)
			}
		})
	}
}
