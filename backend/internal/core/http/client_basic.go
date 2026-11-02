package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PostWithBasicAuth is for native services whose contract is Basic + JSON only.
// In particular, Miaoyun misroutes requests carrying cloud JWT/channel headers.
// Keep the transport and body timeout, but never attach the shared cookie jar,
// follow redirects with credentials, or automatically replay a generation POST.
func (c *Client) PostWithBasicAuth(endpoint string, body interface{}, contentType string, timeout time.Duration) (*http.Response, error) {
	if c.auth == "" {
		return nil, fmt.Errorf("原生服务缺少 Basic 鉴权")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Basic "+c.auth)
	req.Header.Set("Content-Type", contentType)
	requestClient := *c.client
	requestClient.Jar = nil
	requestClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if timeout > 0 {
		requestClient.Timeout = timeout
	}
	return requestClient.Do(req)
}
