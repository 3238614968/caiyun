package http

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWithoutRedirectsDoesNotForwardBodyOrChangeSharedClient(t *testing.T) {
	forwarded := 0
	client := NewClient(WithTransport(cookieIsolationTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "other.example" {
			forwarded++
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
		}
		return &http.Response{StatusCode: 307, Header: http.Header{"Location": {"https://other.example/collect"}}, Body: io.NopCloser(strings.NewReader(`{}`)), Request: req}, nil
	})))
	resp, err := client.RequestWithOptions("POST", "https://configured.example/list", nil, `{"sensitive":"fixture"}`, WithoutRedirects())
	if err != nil || resp.StatusCode != 307 || forwarded != 0 {
		t.Fatalf("redirect forwarded a restricted body: forwarded=%d err=%v", forwarded, err)
	}
	resp.Body.Close()
	resp, err = client.Post("https://configured.example/list", nil, `{}`)
	if err != nil || resp.StatusCode != 200 || forwarded != 1 {
		t.Fatalf("per-request restriction changed the shared client: forwarded=%d err=%v", forwarded, err)
	}
	resp.Body.Close()
}
