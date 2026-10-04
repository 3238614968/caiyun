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
