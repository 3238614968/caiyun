package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// LoginH5Session logs in on the caller's browser-session client. The cookie jar
// already contains that session's SDK cookies; all Set-Cookie fields remain in
// the same jar subsequently used for eligibility, challenge and exchange.
func LoginH5Session(ctx context.Context, client *http.Client, sso, ua, referer string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil || client.Jar == nil || strings.TrimSpace(sso) == "" {
		return "", fmt.Errorf("H5 登录缺少会话或 SSO")
	}
	isolated := *client
	isolated.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client = &isolated // same jar/transport, but credentials never follow a redirect
	timestamp := time.Now().UnixMilli()
	timeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	r, _ := http.NewRequestWithContext(timeCtx, http.MethodGet, "https://m.mcloud.139.com/ycloud/api/timestamp", nil)
	r.Header.Set("User-Agent", ua)
	r.Header.Set("Referer", referer)
	if resp, err := client.Do(r); err == nil {
		var v struct {
			Code   *int
			Result json.Number
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 65536)).Decode(&v)
		resp.Body.Close()
		if err == nil && resp.StatusCode == 200 && v.Code != nil && *v.Code == 0 {
			if n, e := v.Result.Int64(); e == nil && n > 0 {
				timestamp = n
			}
		}
	}
	cancel()
	payload, _ := json.Marshal(map[string]interface{}{"token": sso, "openAccount": 0, "marketName": "sign_in_3", "sourceId": "1097"})
	var lastErr error
	for _, host := range []string{"m.mcloud.139.com", "gp.mcloud.139.com"} {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+host+"/ycloud/auth-service/auth/tyrzLogin", bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		id, nonce, ts := uuid.NewString(), uuid.NewString(), strconv.FormatInt(timestamp, 10)
		for key, value := range map[string]string{
			"User-Agent": ua, "Referer": referer, "Origin": "https://m.mcloud.139.com", "Accept": "*/*",
			"Content-Type": "application/json;charset=UTF-8", "activityid": "sign_in_3", "appversion": "13.2.5.0",
			"X-Requested-With": "com.chinamobile.mcloud", "Cache-Control": "no-cache", "showloading": "true",
			"x-request-id": id, "x-nonce": nonce, "x-timestamp": ts, "x-signature": h5AuthSignature(id, ts, nonce),
		} {
			r.Header.Set(key, value)
		}
		resp, err := client.Do(r)
		if err != nil {
			lastErr = fmt.Errorf("H5 会话登录请求失败: host=%s", host)
			continue
		}
		var result struct {
			Code   *int
			Result struct{ Token string }
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("H5 会话登录响应无法解析: host=%s http_status=%d", host, resp.StatusCode)
			continue
		}
		if resp.StatusCode != 200 || result.Code == nil || *result.Code != 0 || result.Result.Token == "" {
			lastErr = fmt.Errorf("H5 会话登录未成功: host=%s http_status=%d code=%s", host, resp.StatusCode, loginCode(result.Code))
			continue
		}
		return result.Result.Token, nil
	}
	return "", lastErr
}

func loginCode(code *int) string {
	if code == nil {
		return "missing"
	}
	return strconv.Itoa(*code)
}
