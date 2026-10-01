package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

type mailRoundTripFunc func(*http.Request) (*http.Response, error)

func (f mailRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func mailTestResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}
}

func TestMail139LoginComposeAndReportProtocol(t *testing.T) {
	var ssoSeen, loginSeen, composeSeen, reportSeen bool
	transport := mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch req.URL.Path {
		case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
			ssoSeen = true
			var payload map[string]string
			if err := json.Unmarshal(body, &payload); err != nil || payload["toSourceId"] != "001003" || payload["account"] != "13800000001" {
				t.Fatalf("mail SSO payload = %s, err = %v", body, err)
			}
			if req.Header.Get("Main-Authorization") != "Basic test-auth" {
				t.Fatal("mail SSO request missing account authorization")
			}
			return mailTestResponse(req, 200, `{"data":{"token":"mail-sso"}}`), nil
		case "/login/inlogin.action":
			loginSeen = true
			if !strings.Contains(string(body), `<string name="clientId">10805</string>`) ||
				!strings.Contains(string(body), `<string name="token">mail-sso</string>`) {
				t.Fatalf("mail login XML = %s", body)
			}
			return mailTestResponse(req, 200, `{"code":"S_OK","var":{"sid":"mail-sid","rmkey":"mail-rmkey"}}`), nil
		case "/RmWeb/mail":
			composeSeen = true
			if req.Header.Get("Cookie") != "RMKEY=mail-rmkey" || !strings.Contains(req.URL.RawQuery, "func=mbox:compose") ||
				!strings.Contains(string(body), `13800000002@139.com`) || !strings.Contains(string(body), `&amp;`) {
				t.Fatalf("compose protocol mismatch: url=%s body=%s", req.URL, body)
			}
			return mailTestResponse(req, 200, `{"code":"S_OK","var":{"tid":"delivery-id"}}`), nil
		case "/mw2/file/disk":
			reportSeen = true
			return mailTestResponse(req, 500, `upstream unavailable`), nil
		default:
			t.Fatalf("unexpected upstream path: %s", req.URL.Path)
			return nil, nil
		}
	})
	client := corehttp.NewClient(corehttp.WithTransport(transport))
	api := NewCaiyunAPI(client)
	session, err := api.Login139Mail("13800000001", "test-auth")
	if err != nil {
		t.Fatal(err)
	}
	if err := api.Send139Mail(session, "13800000001", "13800000002@139.com", "问候", "A&B"); err != nil {
		t.Fatal(err)
	}
	if err := api.Report139MailTask(session); err == nil {
		t.Fatal("HTTP 500 must be reported separately from successful compose")
	}
	if !ssoSeen || !loginSeen || !composeSeen || !reportSeen {
		t.Fatalf("incomplete mail flow: sso=%t login=%t compose=%t report=%t", ssoSeen, loginSeen, composeSeen, reportSeen)
	}
}
