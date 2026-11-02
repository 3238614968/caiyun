package auth

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// Public protocol salt distributed in the captured H5 JavaScript, not an
// account credential. H5 signs salt + requestId + timestamp + nonce + salt;
// it does not include the login JSON body in this particular signature.
const h5SignatureSalt = "sekaMdYYLIZfbCfm"

func h5AuthSignature(requestID, timestamp, nonce string) string {
	sum := md5.Sum([]byte(h5SignatureSalt + requestID + timestamp + nonce + h5SignatureSalt))
	return hex.EncodeToString(sum[:])
}

func (a *Auth) h5LoginHeaders(ctx context.Context, ssoToken string) map[string]string {
	requestID, nonce := uuid.NewString(), uuid.NewString()
	timestamp := strconv.FormatInt(a.h5ServerTime(ctx), 10)
	return map[string]string{
		"Content-Type": "application/json;charset=UTF-8",
		"x-request-id": requestID, "x-timestamp": timestamp, "x-nonce": nonce,
		"x-signature": h5AuthSignature(requestID, timestamp, nonce), "showLoading": "true",
		"Referer": "https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin&sourceid=1097&enableShare=1&token=" + url.QueryEscape(ssoToken) + "&targetSourceId=001005",
	}
}

func (a *Auth) h5ServerTime(ctx context.Context) int64 {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	resp, err := a.client.GetWithContext(ctx, "https://m.mcloud.139.com/ycloud/api/timestamp", map[string]string{"User-Agent": authWebViewUA, "Accept": "*/*"})
	if err == nil {
		body, readErr := a.client.ReadResponseBody(resp)
		if readErr == nil && resp.StatusCode == 200 {
			var value struct {
				Code   *int
				Result json.Number
			}
			if json.Unmarshal([]byte(body), &value) == nil && value.Code != nil && *value.Code == 0 {
				if milliseconds, err := value.Result.Int64(); err == nil && milliseconds > 0 {
					return milliseconds
				}
			}
		}
	}
	// The official H5 also falls back to local milliseconds if time lookup fails.
	return time.Now().UnixMilli()
}
