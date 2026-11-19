package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
)

type miguRoundTrip func(*http.Request) (*http.Response, error)

func (f miguRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func miguReply(req *http.Request, raw string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw)), Request: req}
}

func decodeMiguTest(t *testing.T, encoded string) string {
	t.Helper()
	plain, err := hex.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	for i := range plain {
		plain[i] -= miguSecurityKey[i%len(miguSecurityKey)]
	}
	return string(plain)
}

func TestMiguExchangeAndSignedSongProtocol(t *testing.T) {
	var musicCE string
	mints, validations := 0, 0
	client := corehttp.NewClient(corehttp.WithTransport(miguRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "orches.yun.139.com" {
			mints++
			var payload map[string]string
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["toSourceId"] != "012008" || payload["account"] != "13800000001" || req.Header.Get("Authorization") != "Basic fixture-cloud" {
				t.Fatalf("wrong ticket source or account: %v", payload)
			}
			return miguReply(req, `{"success":true,"code":"0000","data":{"token":"one-use-fixture-ticket"}}`), nil
		}
		if req.URL.Host != "app.c.nf.migu.cn" {
			t.Fatalf("unexpected host: %s", req.URL.Host)
		}
		for _, key := range []string{"Authorization", "JwtToken", "Cookie", "x-deviceinfo", "x-yun-client-info"} {
			if req.Header.Get(key) != "" {
				t.Fatalf("cloud credential/device header leaked: %s", key)
			}
		}
		var body []byte
		if req.Body != nil {
			body, _ = io.ReadAll(req.Body)
		}
		if req.Header.Get("Sign") != miguSign(append([]byte(req.URL.RequestURI()+req.Header.Get("Ce")+req.Header.Get("Timestamp")), body...), miguCertificatePrefix) {
			t.Fatal("body/query/timestamp signature mismatch")
		}
		if req.Header.Get("Activityid") != MiguDefaultActivity || req.Header.Get("Signversion") != "V005" || req.Header.Get("Deviceid") != "0011223344556677" {
			t.Fatal("missing activity, device or protocol version")
		}
		if req.URL.Path == "/user/h5/token-validate/v3.0" {
			validations++
			if req.URL.RawQuery != "sourceId=220029&token=ANDTone-use-fixture-ticket&type=2" {
				t.Fatal("ticket must have ANDT prefix and sorted query")
			}
			if decodeMiguTest(t, req.Header.Get("Ce")) != "ip=192.0.2.10&deviceId=0011223344556677" || req.Header.Get("Pacmtoken") != "" {
				t.Fatal("validation inherited a stale session")
			}
			resp := miguReply(req, `{"code":"000000","data":{"userId":92200000001,"msisdn":"138****0001"}}`)
			resp.Header.Set("pacmtoken", "music-session-only")
			resp.Header.Set("timestamp", "1800000000500")
			return resp, nil
		}
		if req.Header.Get("Pacmtoken") != "music-session-only" || req.Header.Get("Uid") != "92200000001" || req.Header.Get("Timestamp") != "1800000000500" {
			t.Fatal("missing H5 identity or server clock calibration")
		}
		ce := req.Header.Get("Ce")
		if decodeMiguTest(t, ce) != "uid=92200000001&ip=192.0.2.10&msisdn=13800000001&deviceId=0011223344556677" {
			t.Fatal("CE must use this account's plain phone")
		}
		if musicCE != "" && musicCE != ce {
			t.Fatal("song accounting CE changed between report, query and reward")
		}
		musicCE = ce
		switch req.URL.Path {
		case "/act-message/report/song-duration/v1":
			var report struct {
				Duration      int64
				TransactionID string
			}
			if req.Method != "POST" || !strings.HasPrefix(req.Header.Get("Content-Type"), "application/json") || json.Unmarshal([]byte(decodeMiguTest(t, string(body))), &report) != nil || report.Duration != 60000 || report.TransactionID != "fixture-transaction" {
				t.Fatal("song report was not encoded as documented")
			}
		case "/act-jbzx/receive-task-rewards/v2.0":
			if req.Method != "POST" || string(body) != `{"taskId":"SONG_1"}` {
				t.Fatal("invalid reward contract")
			}
			return miguReply(req, `{"code":"000000","data":{"aiBeanNum":"2"}}`), nil
		case "/act-jbzx/song-task/v2.0":
			return miguReply(req, `{"code":"000000","data":{"songTime":"60000","tasks":[{"id":"SONG_1","status":"1"}]}}`), nil
		default:
			t.Fatalf("unexpected music path %s", req.URL.Path)
		}
		return miguReply(req, `{"code":"000000","data":{}}`), nil
	})))
	client.SetAuth("fixture-cloud")
	client.SetJWTToken("cloud-jwt")
	client.SetCookie("stale-session", "do-not-send", "app.c.nf.migu.cn")
	api := NewMiguAPI(client, "0011223344556677", "192.0.2.10", "")
	api.now = func() time.Time { return time.UnixMilli(1800000000000) }
	if _, err := api.Login("Basic fixture-cloud", "13800000001"); err != nil {
		t.Fatal(err)
	}
	if err := api.ReportSong(60000, "fixture-transaction"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.SongStatus(); err != nil {
		t.Fatal(err)
	}
	if amount, err := api.ClaimSong("SONG_1"); err != nil || amount != 2 {
		t.Fatalf("amount=%d err=%v", amount, err)
	}
	if mints != 1 || validations != 1 || client.GetJWTToken() != "cloud-jwt" {
		t.Fatal("exchange replayed a ticket or altered cloud session")
	}
}

func TestMiguNeverReplaysSensitiveGETOrLogsTicket(t *testing.T) {
	for _, path := range []string{"/user/h5/token-validate/v3.0", "/act-task/login-access", "/act-task/join-activity"} {
		t.Run(path, func(t *testing.T) {
			calls := 0
			client := corehttp.NewClient(corehttp.WithTransport(miguRoundTrip(func(req *http.Request) (*http.Response, error) {
				calls++
				return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: errors.New("fixture timeout")}
			})))
			api := NewMiguAPI(client, "0011223344556677", "127.0.0.1", "")
			_, err := api.request("GET", path, url.Values{"token": {"ANDT-secret-ticket-do-not-log"}}, nil, nil)
			if err == nil || calls != 1 || strings.Contains(err.Error(), "secret-ticket") {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestMiguLoginRequiresSessionFields(t *testing.T) {
	for _, data := range []string{`{"userId":"uid"}`, `{"pacmtoken":"session"}`, `{"userId":"uid","pacmtoken":"session","msisdn":"13800000002"}`} {
		client := corehttp.NewClient(corehttp.WithTransport(miguRoundTrip(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "orches.yun.139.com" {
				return miguReply(req, `{"success":true,"data":{"token":"ticket"}}`), nil
			}
			return miguReply(req, `{"code":"000000","data":`+data+`}`), nil
		})))
		api := NewMiguAPI(client, "0011223344556677", "127.0.0.1", "")
		if _, err := api.Login("Basic fixture", "13800000001"); err == nil || api.session != nil {
			t.Fatalf("accepted incomplete/mismatched session: %s", data)
		}
	}
}

func TestMiguAlreadyClaimedIsNotSessionExpiration(t *testing.T) {
	if MiguSessionExpired(&MiguError{Code: "200002", Info: "已领取"}) || !MiguSessionExpired(&MiguError{Code: "299999", Info: "用户未登录，请登录后再试"}) {
		t.Fatal("business error was confused with login expiry")
	}
}

func TestMiguStableNoteBalanceDoesNotConsumeRewardNotices(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(miguRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/act-jbzx/ai-bean/query-available" {
			t.Fatalf("balance verification consumed a notification: %s", req.URL.Path)
		}
		return miguReply(req, `{"code":"000000","data":{"availableNum":58}}`), nil
	})))
	api := NewMiguAPI(client, "0011223344556677", "127.0.0.1", "")
	for i := 0; i < 2; i++ {
		if balance, err := api.NoteBalance(); err != nil || balance != 58 {
			t.Fatalf("balance=%d err=%v", balance, err)
		}
	}
}

func TestMiguMusicBeanFlowUsesDocumentedPagination(t *testing.T) {
	calls := 0
	client := corehttp.NewClient(corehttp.WithTransport(miguRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != "GET" || req.URL.Path != "/act-jbzx/music-bean-flow" || req.URL.RawQuery != "pageNumber=2&pageSize=20" {
			t.Fatal("incorrect legacy flow request")
		}
		return miguReply(req, `{"code":"000000","data":[]}`), nil
	})))
	api := NewMiguAPI(client, "0011223344556677", "127.0.0.1", "")
	if data, err := api.MusicBeanFlow(2, 20); err != nil || string(data) != "[]" {
		t.Fatalf("data=%s err=%v", data, err)
	}
	for _, invalid := range [][2]int{{0, 20}, {1, 0}, {1, 101}} {
		if _, err := api.MusicBeanFlow(invalid[0], invalid[1]); err == nil {
			t.Fatal("invalid pagination accepted")
		}
	}
	if calls != 1 {
		t.Fatal("invalid pagination reached the server")
	}
}
