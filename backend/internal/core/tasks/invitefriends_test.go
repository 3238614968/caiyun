package tasks

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestInviteConfirmsRewardsAndNeverClaimsUnrelatedClouds(t *testing.T) {
	for _, mode := range []string{"already-complete", "confirmed", "pending", "query-failed"} {
		t.Run(mode, func(t *testing.T) {
			posts, queries := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/market/signin/public/cloudRecord":
					queries++
					if mode == "query-failed" {
						return hiddenRewardReply(r, `{"code":505,"msg":"权限不足"}`), nil
					}
					count := 19
					if mode == "already-complete" || mode == "confirmed" && posts == 1 {
						count = 20
					}
					rows := make([]CloudRecord, count)
					for i := range rows {
						rows[i] = CloudRecord{Mark: "fxnrplus5", UpdateTime: time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")}
					}
					body, _ := json.Marshal(map[string]interface{}{"code": 0, "result": map[string]interface{}{"records": rows}})
					return hiddenRewardReply(r, string(body)), nil
				case "/datacenter/":
					posts++
					body, _ := io.ReadAll(r.Body)
					values, err := url.ParseQuery(string(body))
					if err != nil {
						t.Fatal(err)
					}
					decoded, err := base64.StdEncoding.DecodeString(values.Get("data"))
					if err != nil || !json.Valid(decoded) || !strings.Contains(string(decoded), "fixture-sso") {
						t.Fatal("sharing form was corrupted or contained a fabricated SSO token")
					}
					return hiddenRewardReply(r, `{}`), nil
				default:
					t.Errorf("invitation must not claim account-wide rewards: %s", r.URL.Path)
					return nil, fmt.Errorf("unexpected endpoint")
				}
			})))
			client.SetSSOToken("fixture-sso")
			task := NewInviteFriendsTask(client, logger.NewLogger(logger.LevelError)).SetPhone("13800000001")
			err := task.Run()
			if mode == "query-failed" {
				if err == nil || posts != 0 {
					t.Fatalf("query failure must stop sharing: posts=%d err=%v", posts, err)
				}
				return
			}
			wantPosts := 1
			if mode == "already-complete" {
				wantPosts = 0
			}
			if err != nil || posts != wantPosts || queries != wantPosts+1 || task.Pending() != (mode == "pending") {
				t.Fatalf("posts=%d queries=%d pending=%v err=%v", posts, queries, task.Pending(), err)
			}
		})
	}
}

// shareFixture 构造“本月已分享19次、第N次上报后满20次”的响应。
func shareFixture(t *testing.T, posts *int) hiddenRewardRoundTrip {
	return func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/market/signin/public/cloudRecord":
			count := 19
			if *posts > 0 {
				count = 20
			}
			rows := make([]CloudRecord, count)
			for i := range rows {
				rows[i] = CloudRecord{Mark: "fxnrplus5", UpdateTime: time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")}
			}
			body, _ := json.Marshal(map[string]interface{}{"code": 0, "result": map[string]interface{}{"records": rows}})
			return hiddenRewardReply(r, string(body)), nil
		default:
			t.Fatalf("unexpected endpoint: %s", r.URL.Path)
		}
		return nil, nil
	}
}

func TestInviteFriendsRetriesTransientShareFailure(t *testing.T) {
	oldDelay := inviteShareRetryDelay
	inviteShareRetryDelay = 1
	t.Cleanup(func() { inviteShareRetryDelay = oldDelay })

	posts, failed := 0, false
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/datacenter/" {
			posts++
			if !failed {
				failed = true
				return nil, errors.New("dial tcp: lookup datacenter.mail.10086.cn on 127.0.0.53:53: server misbehaving")
			}
			return hiddenRewardReply(r, `{}`), nil
		}
		return shareFixture(t, &posts)(r)
	})))
	client.SetSSOToken("fixture-sso")
	task := NewInviteFriendsTask(client, logger.NewLogger(logger.LevelError)).SetPhone("13800000001")
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if posts != 2 || task.Pending() {
		t.Fatalf("posts=%d pending=%v message=%s", posts, task.Pending(), task.Message())
	}
}

func TestInviteFriendsKeepsPendingWhenShareReportStaysUnavailable(t *testing.T) {
	oldDelay := inviteShareRetryDelay
	inviteShareRetryDelay = 1
	t.Cleanup(func() { inviteShareRetryDelay = oldDelay })

	posts := 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/datacenter/" {
			posts++
			return nil, errors.New("dial tcp: lookup datacenter.mail.10086.cn on 127.0.0.53:53: server misbehaving")
		}
		return shareFixture(t, &posts)(r)
	})))
	client.SetSSOToken("fixture-sso")
	task := NewInviteFriendsTask(client, logger.NewLogger(logger.LevelError)).SetPhone("13800000001")
	if err := task.Run(); err != nil {
		t.Fatalf("unreachable share collector must stay retryable, not fail the batch: %v", err)
	}
	if posts != inviteShareAttempts || !task.Pending() || !strings.Contains(task.Message(), "待下次重试") {
		t.Fatalf("posts=%d pending=%v message=%s", posts, task.Pending(), task.Message())
	}
}
