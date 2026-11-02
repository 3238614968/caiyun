package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The successful capture posts this event immediately before opening the
// challenge. Its Set-Cookie rotates JSESSIONID, which the exchange then uses.
// Finish it before obtaining a challenge so our own requests do not race a
// session-cookie change while solving. Telemetry failure is not an exchange.
func prepareExchangeVisit(ctx context.Context, session *exchangeHTTPSession, auth *exchangeAuthContext) error {
	if session == nil || session.client == nil {
		return fmt.Errorf("活动 HTTP 会话为空")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	session.captchaTID = ""
	body := url.Values{
		"module": {"uservisit"}, "optkeyword": {"newsignin_index_exchange_times"},
		"sourceid": {exchangeSourceID}, "marketName": {exchangeActivityID},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://m.mcloud.139.com/ycloud/visitlog/journaling", strings.NewReader(body))
	if err != nil {
		return err
	}
	for key, value := range buildExchangeHeaders(auth, session, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}) {
		req.Header.Set(key, value)
	}
	resp, err := session.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16)); err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("活动访问记录 HTTP %d", resp.StatusCode)
	}
	return nil
}
