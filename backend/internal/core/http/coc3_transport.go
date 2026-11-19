package http

import (
	"fmt"
	"net/http"
	"sync"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type coc3ProfileDoer interface {
	Do(*fhttp.Request) (*fhttp.Response, error)
}

type coc3ProfileTransport struct {
	fallback http.RoundTripper
	create   func() (coc3ProfileDoer, error)
	once     sync.Once
	client   coc3ProfileDoer
	err      error
}

// NewCoc3ProfileTransport matches the reference client's Chrome 131 HTTP/TLS
// profile using Go only. Account cookies remain exclusively in Client's jar;
// the lower-level client must never accumulate cookies across accounts.
func NewCoc3ProfileTransport(fallback http.RoundTripper) http.RoundTripper {
	if fallback == nil {
		fallback = newTaskTransport()
	}
	return &coc3ProfileTransport{fallback: fallback, create: func() (coc3ProfileDoer, error) {
		client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(),
			tlsclient.WithClientProfile(profiles.Chrome_131),
			tlsclient.WithRandomTLSExtensionOrder(),
			tlsclient.WithTimeoutSeconds(30),
			tlsclient.WithNotFollowRedirects())
		if err != nil {
			return nil, err
		}
		if client.GetCookieJar() != nil {
			return nil, fmt.Errorf("COC3 profile transport unexpectedly enabled its own cookie jar")
		}
		return client, nil
	}}
}

func (t *coc3ProfileTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Hostname() != "dev.coc.10086.cn" {
		return t.fallback.RoundTrip(req)
	}
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("COC3 requires HTTPS")
	}
	t.once.Do(func() { t.client, t.err = t.create() })
	if t.err != nil {
		return nil, fmt.Errorf("COC3 Chrome profile initialization failed: %w", t.err)
	}
	forwarded, err := fhttp.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), req.Body)
	if err != nil {
		return nil, err
	}
	forwarded.Header = fhttp.Header(req.Header.Clone())
	forwarded.Host = req.Host
	forwarded.ContentLength = req.ContentLength
	forwarded.TransferEncoding = append([]string(nil), req.TransferEncoding...)
	// No replay of lottery/order mutations after an ambiguous network result.
	forwarded.GetBody = nil
	resp, err := t.client.Do(forwarded)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, fmt.Errorf("COC3 API returned an unexpected redirect")
	}
	return &http.Response{
		Status: resp.Status, StatusCode: resp.StatusCode,
		Proto: resp.Proto, ProtoMajor: resp.ProtoMajor, ProtoMinor: resp.ProtoMinor,
		Header: http.Header(resp.Header), Body: resp.Body,
		ContentLength: resp.ContentLength, TransferEncoding: resp.TransferEncoding,
		Close: resp.Close, Uncompressed: resp.Uncompressed,
		Trailer: http.Header(resp.Trailer), Request: req,
	}, nil
}
