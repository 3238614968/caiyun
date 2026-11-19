package api

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func activityTestHeader(req *http.Request, name string) string {
	for key, values := range req.Header {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func TestCloudPhoneUsesDedicatedSSOAndSignedSession(t *testing.T) {
	for _, status := range []string{`200`, `"200"`} {
		t.Run(status, func(t *testing.T) {
			var paths []string
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				paths = append(paths, req.URL.Path)
				body, _ := io.ReadAll(req.Body)
				var payload map[string]interface{}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				if req.URL.Host == "orches.yun.139.com" {
					if payload["account"] != "13800000001" || payload["toSourceId"] != "001216" || activityTestHeader(req, "Authorization") != "Basic test-auth" {
						t.Fatalf("wrong dedicated SSO request: %s", body)
					}
					return mailTestResponse(req, 200, `{"data":{"token":"dedicated-sso"}}`), nil
				}
				if req.URL.Host != "cpactiv.buy.139.com" {
					t.Fatalf("unexpected host: %s", req.URL)
				}
				token := activityTestHeader(req, "token")
				rid, ts := activityTestHeader(req, "requestId"), activityTestHeader(req, "timestamp")
				expected := fmt.Sprintf("%x", md5.Sum([]byte(rid+"12345681"+token+"e10adc3949ba59abbe56e057f20f883e"+ts)))
				if len(rid) != 35 || activityTestHeader(req, "sign") != expected || activityTestHeader(req, "x-channelSrc") != "red-mcloudapp-001" {
					t.Fatal("cloud phone signature or channel headers are invalid")
				}
				switch req.URL.Path {
				case "/cloudphone-market/user/tokenValidate":
					if len(payload) != 3 || payload["version"] != "1.0" || payload["pintype"] != float64(13) || payload["token"] != "dedicated-sso" || token != "" {
						t.Fatalf("validation must use only the SSO login fields: %s", body)
					}
					return mailTestResponse(req, 200, `{"header":{"status":`+status+`},"data":{"token":"cloud-session"}}`), nil
				case "/cloudphone-market/redpacket/userAccountInfo":
					if token != "cloud-session" {
						t.Fatal("account request did not use the cloud phone session")
					}
					return mailTestResponse(req, 200, `{"header":{"status":`+status+`},"data":{"info":{"canAmount":"50"}}}`), nil
				default:
					t.Fatalf("unexpected request: %s", req.URL)
				}
				return nil, nil
			})))
			client.SetAuth("test-auth")
			client.SetSSOToken("cloud-main-sso")
			a := NewCaiyunAPI(client)
			token, err := a.CloudPhoneLogin("13800000001")
			if err != nil || token != "cloud-session" {
				t.Fatalf("login token=%s err=%v", token, err)
			}
			balance, err := a.CloudPhoneBalance(token)
			if err != nil || balance != 50 || len(paths) != 3 || client.GetSSOToken() != "cloud-main-sso" {
				t.Fatalf("balance=%d calls=%d err=%v", balance, len(paths), err)
			}
		})
	}
}

func TestCloudPhoneRejectsMissingBalanceAndBusinessErrors(t *testing.T) {
	for _, body := range []string{
		`{"header":{"status":401,"errMsg":"expired"},"data":{"info":{"canAmount":50}}}`,
		`{"header":{"status":200},"data":{"info":{}}}`,
		`{"header":{"status":200},"data":{"info":{"canAmount":-1}}}`,
		`{"header":{"status":200},"data":null}`,
		`{"data":{"info":{"canAmount":50}}}`,
	} {
		client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return mailTestResponse(req, 200, body), nil
		})))
		if _, err := NewCaiyunAPI(client).CloudPhoneBalance("session"); err == nil {
			t.Fatalf("invalid account response was accepted: %s", body)
		}
	}
}

// 旧链 tokenValidate 被拒（401 族）或无 token 时，SSO 已单次消费，
// 必须重新 querySpecToken 并走 cloud.139.com/uniphone 的 h5 新链。
func TestCloudPhoneLoginFallsBackToCloud139(t *testing.T) {
	for _, old := range []string{
		`{"header":{"status":401,"errMsg":"expired"},"data":null}`,
		`{"header":{"status":4001024,"errMsg":"invalid token"},"data":null}`,
		`{"header":{"status":4001025,"errMsg":"invalid token"},"data":null}`,
		`{"header":{"status":500022,"errMsg":"session rejected"},"data":null}`,
		`{"header":{"status":200},"data":{"token":""}}`,
	} {
		t.Run(old, func(t *testing.T) {
			ssoCalls, h5Calls, marketRetry := 0, 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				body, _ := io.ReadAll(req.Body)
				var payload map[string]interface{}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				switch req.URL.Host {
				case "orches.yun.139.com":
					ssoCalls++
					if payload["account"] != "13800000001" || payload["toSourceId"] != "001216" {
						t.Fatalf("wrong dedicated SSO request: %s", body)
					}
					return mailTestResponse(req, 200, fmt.Sprintf(`{"data":{"token":"sso-%d"}}`, ssoCalls)), nil
				case "cpactiv.buy.139.com":
					if req.URL.Path != "/cloudphone-market/user/tokenValidate" || payload["version"] != "1.0" || payload["pintype"] != float64(13) || payload["token"] != "sso-1" {
						t.Fatalf("wrong legacy validation: %s", body)
					}
					return mailTestResponse(req, 200, old), nil
				case "cloud.139.com":
					if req.URL.Path != "/uniphone/user/tokenValidate" {
						marketRetry++
						t.Fatalf("login fallback must use /uniphone, got %s", req.URL.Path)
					}
					h5Calls++
					if len(payload) != 3 || payload["version"] != "1.2" || payload["pintype"] != float64(5) || payload["token"] != "sso-2" {
						t.Fatalf("h5 validation must consume a fresh SSO: %s", body)
					}
					token := activityTestHeader(req, "token")
					rid, ts := activityTestHeader(req, "requestId"), activityTestHeader(req, "timestamp")
					expected := fmt.Sprintf("%x", md5.Sum([]byte(rid+"3099801"+token+"esdkabncgfsfkmlondn"+ts)))
					if token != "" || len(rid) != 35 || activityTestHeader(req, "sign") != expected || activityTestHeader(req, "appId") != "3099801" {
						t.Fatal("h5 signature headers are invalid")
					}
					if activityTestHeader(req, "platform") != "h5" || activityTestHeader(req, "x-channelSrc") != "02040" ||
						activityTestHeader(req, "x-kpcc-clientid") != "X3qUAu6hA1yO6CegzcMSrQJknuZ7aEs1" ||
						activityTestHeader(req, "x-kpcc-clientkey") != "AqXZ3iQtf713yN70ydXRZkCkxdeGo3jo" {
						t.Fatal("h5 channel headers are missing")
					}
					return mailTestResponse(req, 200, `{"header":{"status":200},"data":{"token":"h5-session"}}`), nil
				default:
					t.Fatalf("unexpected host: %s", req.URL)
				}
				return nil, nil
			})))
			token, err := NewCaiyunAPI(client).CloudPhoneLogin("13800000001")
			if err != nil || token != "h5-session" || ssoCalls != 2 || h5Calls != 1 || marketRetry != 0 {
				t.Fatalf("fallback token=%s err=%v sso=%d h5=%d marketRetry=%d", token, err, ssoCalls, h5Calls, marketRetry)
			}
		})
	}
}

// market 层 401 族状态码自动用 cloud.139.com/cloudphone-market 同路径重试，
// 签名仍用 market appId/salt；非鉴权失败不触发新链。
func TestCloudPhoneCallRetriesMarketOnCloud139(t *testing.T) {
	for _, tc := range []struct {
		name      string
		oldStatus string
		retry     bool
	}{
		{"401 retries", "401", true},
		{"4001024 retries", "4001024", true},
		{"500 does not retry", "500", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var legacy, mirrored int
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				token := activityTestHeader(req, "token")
				rid, ts := activityTestHeader(req, "requestId"), activityTestHeader(req, "timestamp")
				expected := fmt.Sprintf("%x", md5.Sum([]byte(rid+"12345681"+token+"e10adc3949ba59abbe56e057f20f883e"+ts)))
				if token != "session" || activityTestHeader(req, "sign") != expected || activityTestHeader(req, "appId") != "12345681" {
					t.Fatal("market retry must keep the market signature")
				}
				if req.URL.Path != "/cloudphone-market/redpacket/userAccountInfo" {
					t.Fatalf("unexpected path: %s", req.URL.Path)
				}
				switch req.URL.Host {
				case "cpactiv.buy.139.com":
					legacy++
					return mailTestResponse(req, 200, `{"header":{"status":`+tc.oldStatus+`,"errMsg":"rejected"},"data":null}`), nil
				case "cloud.139.com":
					mirrored++
					return mailTestResponse(req, 200, `{"header":{"status":200},"data":{"info":{"canAmount":"50"}}}`), nil
				default:
					t.Fatalf("unexpected host: %s", req.URL)
				}
				return nil, nil
			})))
			balance, err := NewCaiyunAPI(client).CloudPhoneBalance("session")
			if tc.retry {
				if err != nil || balance != 50 || legacy != 1 || mirrored != 1 {
					t.Fatalf("fallback balance=%d err=%v legacy=%d mirrored=%d", balance, err, legacy, mirrored)
				}
			} else if err == nil || legacy != 1 || mirrored != 0 {
				t.Fatalf("non-auth failure must not retry: balance=%d err=%v legacy=%d mirrored=%d", balance, err, legacy, mirrored)
			}
		})
	}
}
