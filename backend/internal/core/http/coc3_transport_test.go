package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

type coc3FakeDoer func(*fhttp.Request) (*fhttp.Response, error)

func (f coc3FakeDoer) Do(req *fhttp.Request) (*fhttp.Response, error) { return f(req) }

type coc3FallbackRoundTrip func(*http.Request) (*http.Response, error)

func (f coc3FallbackRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCoc3ProfileTransportPreservesExplicitIdentityAndDoesNotReplay(t *testing.T) {
	for _, fail := range []bool{false, true} {
		calls := 0
		transport := &coc3ProfileTransport{create: func() (coc3ProfileDoer, error) {
			return coc3FakeDoer(func(req *fhttp.Request) (*fhttp.Response, error) {
				calls++
				if req.GetBody != nil || req.Method != "POST" || req.Header.Get("Cookie") != "COC3_LOGIN_TOKEN=only-this-account" {
					t.Fatal("mutation became replayable or account cookie changed")
				}
				if fail {
					return nil, errors.New("ambiguous upstream result")
				}
				return &fhttp.Response{StatusCode: 200, Header: make(fhttp.Header), Body: io.NopCloser(strings.NewReader(`{"code":200}`))}, nil
			}), nil
		}}
		client := NewClient(WithTransport(transport))
		response, err := client.RequestWithOptions("POST", "https://dev.coc.10086.cn/coc3/fixture", map[string]string{"Cookie": "COC3_LOGIN_TOKEN=only-this-account"}, map[string]int{"activityId": 17766}, WithoutCookieJar(), WithoutDefaultHeaders())
		if response != nil {
			response.Body.Close()
		}
		if (err != nil) != fail || calls != 1 {
			t.Fatalf("fail=%v calls=%d err=%v", fail, calls, err)
		}
	}
}

func TestCoc3ProfileTransportRejectsRedirectAndKeepsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	transport := &coc3ProfileTransport{create: func() (coc3ProfileDoer, error) {
		return coc3FakeDoer(func(req *fhttp.Request) (*fhttp.Response, error) {
			if req.Context() != ctx {
				t.Fatal("request context discarded")
			}
			return &fhttp.Response{StatusCode: 302, Header: fhttp.Header{"Location": []string{"https://elsewhere.example/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}), nil
	}}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://dev.coc.10086.cn/coc3/fixture", strings.NewReader("{}"))
	if _, err := transport.RoundTrip(req); err == nil {
		t.Fatal("credential-bearing API redirect accepted")
	}
}

func TestCoc3ProfileTransportDoesNotChangeOtherHosts(t *testing.T) {
	called := false
	transport := &coc3ProfileTransport{fallback: coc3FallbackRoundTrip(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	}), create: func() (coc3ProfileDoer, error) { t.Fatal("COC profile used for cloud host"); return nil, nil }}
	req, _ := http.NewRequest("GET", "https://m.mcloud.139.com/fixture", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil || !called {
		t.Fatal("normal transport changed")
	}
	resp.Body.Close()
}
