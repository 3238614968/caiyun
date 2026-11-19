package http

import (
	"errors"
	"io"
	nethttp "net/http"
	"strings"
	"testing"
	"time"
)

type cookieIsolationTransport func(*nethttp.Request) (*nethttp.Response, error)

func (f cookieIsolationTransport) RoundTrip(req *nethttp.Request) (*nethttp.Response, error) {
	return f(req)
}

func TestGetMutationCanDisableTransportRetries(t *testing.T) {
	requests := 0
	client := NewClient(WithTransport(cookieIsolationTransport(func(req *nethttp.Request) (*nethttp.Response, error) {
		requests++
		return nil, errors.New("ambiguous upstream timeout")
	})))
	if _, err := client.RequestWithOptions("GET", "https://example.com/prize", nil, nil, WithoutRetries()); err == nil || requests != 1 {
		t.Fatalf("GET mutation was replayed: requests=%d err=%v", requests, err)
	}
}

func TestExplicitCookiesDoNotDuplicateJarOrMutateSharedSession(t *testing.T) {
	requests := 0
	client := NewClient(WithTransport(cookieIsolationTransport(func(req *nethttp.Request) (*nethttp.Response, error) {
		requests++
		want := "User-Token=fresh"
		if requests == 2 {
			want = "User-Token=jar-session"
		}
		if req.Header.Get("Cookie") != want {
			t.Fatalf("request %d Cookie=%q want=%q", requests, req.Header.Get("Cookie"), want)
		}
		header := make(nethttp.Header)
		header.Add("Set-Cookie", "User-Token=other; Path=/")
		return &nethttp.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
	})))
	client.SetCookie("User-Token", "jar-session", "dev.coc.10086.cn")
	response, err := client.RequestWithOptions("POST", "https://dev.coc.10086.cn/coc/activities/prize", map[string]string{"Cookie": "User-Token=fresh"}, nil, WithoutCookieJar(), WithRequestTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if cookies := response.Cookies(); len(cookies) != 1 || cookies[0].Value != "other" {
		t.Fatal("response cookie unavailable")
	}
	response, err = client.Get("https://dev.coc.10086.cn/coc/activities/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if requests != 2 {
		t.Fatal(requests)
	}
}
