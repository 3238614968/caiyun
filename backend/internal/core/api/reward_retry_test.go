package api

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

type rewardRetryTransport func(*http.Request) (*http.Response, error)

func (f rewardRetryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRewardMutationsDoNotReplayAfterUncertainResponse(t *testing.T) {
	for _, endpoint := range []string{
		"/signin/page/startSignIn", "/signin/page/receiveTaskExpansion",
		"/signin/page/receiveTask", "/backupgift/receive", "/signin/page/multiple",
	} {
		t.Run(endpoint, func(t *testing.T) {
			calls := 0
			client := corehttp.NewClient(corehttp.WithTransport(rewardRetryTransport(func(req *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(req.URL.Path, endpoint) {
					return &http.Response{StatusCode: 200, Header: http.Header{}, Body: http.NoBody, Request: req}, nil
				}
				calls++
				return nil, errors.New("connection lost after submission")
			})))
			client.SetAuth("fixture")
			client.SetJWTToken("fixture")
			api := NewCaiyunAPI(client)
			var err error
			switch endpoint {
			case "/signin/page/startSignIn":
				_, err = api.SignIn()
			case "/signin/page/receiveTaskExpansion":
				_, err = api.ReceiveTaskExpansion("2026-11-01")
			case "/signin/page/receiveTask":
				err = api.ReceiveTaskRewardForMarket("sign_in_3", "106")
			case "/backupgift/receive":
				_, err = api.ReceiveBackupGift()
			case "/signin/page/multiple":
				_, err = api.CloudMultiple()
			}
			if err == nil || calls != 1 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
