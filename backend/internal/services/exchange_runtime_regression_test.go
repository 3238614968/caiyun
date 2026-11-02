package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMalformedEligibilityNeverReachesCaptchaOrExchange(t *testing.T) {
	for _, body := range []string{`{}`, `{"code":0}`, `{"code":0,"result":{"exchangeCode":null}}`, `{"code":0,"result":{"exchangeCode":false}}`, `{"code":0,"result":{"exchangeCode":0.1}}`, `{"code":505,"msg":"权限不足"}`} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if !strings.HasSuffix(r.URL.Path, "receivePrizeDetails") {
				t.Fatal("unexpected submission", r.URL.Path)
			}
			io.WriteString(w, body)
		}))
		r := executeExchangeOnce("123", &exchangeAuthContext{jwtToken: "fixture"}, newMockExchangeSession(t, server))
		server.Close()
		if calls != 1 || r.success || !r.stop || !strings.Contains(r.message, "未提交兑换") {
			t.Fatalf("body=%s calls=%d result=%+v", body, calls, r)
		}
	}
}

func TestGKStopsRetriesWithoutPreventingANewScheduledRun(t *testing.T) {
	calls := 0
	for run := 0; run < 2; run++ {
		ok, _, _, retries, err := runExchangeWithRetries(context.Background(), 3, nil, func() (bool, string, int) {
			calls++
			return false, "活动异常，请稍后重试！Error Code：GK | code=610", 20
		})
		if ok || err != nil || retries != 0 {
			t.Fatalf("same rejected session retried: %v %d", err, retries)
		}
	}
	if calls != 2 {
		t.Fatal(fmt.Sprint(calls))
	}
}
