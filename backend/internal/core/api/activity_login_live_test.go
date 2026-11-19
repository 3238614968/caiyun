package api

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
)

// Explicit existing-account diagnostics: exchange separate single-use tickets
// for both COC protocols, then only read dates/counts. Never submit a prize.
func TestActivityLoginReadOnlyLive(t *testing.T) {
	path := os.Getenv("CAIYUN_ACTIVITY_LOGIN_AUTH_FILE")
	if path == "" {
		t.Skip("explicit existing-account login diagnostics required")
	}
	reportPath := os.Getenv("CAIYUN_ACTIVITY_LOGIN_REPORT")
	proxy, err := url.Parse(os.Getenv("CAIYUN_ACTIVITY_LOGIN_PROXY"))
	if err != nil || proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") || !filepath.IsAbs(reportPath) {
		t.Fatal("explicit proxy and absolute sanitized report path required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("existing credential file unavailable")
	}
	credential := strings.TrimSpace(string(raw))
	if strings.HasPrefix(credential, "{") {
		var doc struct{ Authorization, Auth string }
		if json.Unmarshal(raw, &doc) != nil {
			t.Fatal("invalid existing credential JSON")
		}
		credential = doc.Authorization
		if credential == "" {
			credential = doc.Auth
		}
	}
	account, err := auth.ParseToken(credential)
	if err != nil || len(account.Phone) != 11 {
		t.Fatal("invalid existing account credential")
	}
	redact := strings.NewReplacer(account.AuthFull, "[auth]", account.Auth, "[auth]", account.Token, "[token]", account.Phone, "[account]")
	report := map[string]interface{}{"account_suffix": account.Phone[7:], "observed_at": time.Now().In(time.FixedZone("CST", 8*3600)).Format(time.RFC3339), "prize_submitted": false}
	defer func() {
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil || os.WriteFile(reportPath, append(out, '\n'), 0600) != nil {
			t.Error("sanitized login report write failed")
		}
	}()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxy)
	transport.DialContext = (&net.Dialer{Timeout: 15 * time.Second}).DialContext
	defer transport.CloseIdleConnections()
	client := corehttp.NewClient(corehttp.WithTransport(transport))
	client.SetAuth(account.AuthFull)
	client.SetMarketAccount(account.Phone)
	market := NewCoc3API(client)
	login, err := market.LoginCoc3(account.AuthFull, account.Phone)
	if err != nil {
		report["coc3_error"] = redact.Replace(err.Error())
		t.Error("COC3 login not available; see sanitized report")
		if errors.Is(err, auth.ErrCredentialExpired) {
			report["credential_expired"] = true
			return
		}
	} else {
		report["coc3_login"] = login.Token != ""
		_, readErr := market.GetBeanBalance(login.Token)
		report["coc3_balance_read"] = readErr == nil
		if readErr != nil {
			report["coc3_read_error"] = redact.Replace(readErr.Error())
		}
	}
	web6 := NewCocWeb6API(client)
	token, err := web6.Login(account.AuthFull, account.Phone)
	if err != nil {
		report["web6_error"] = redact.Replace(err.Error())
		t.Error("COC web6 login not available; see sanitized report")
	} else {
		report["web6_login"] = token != ""
		dates, readErr := web6.ClockInDates(token)
		report["web6_dates_read"] = readErr == nil
		report["web6_dates"] = dates
		if readErr != nil {
			report["web6_read_error"] = redact.Replace(readErr.Error())
		}
	}
}
