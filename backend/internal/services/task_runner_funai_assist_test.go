package services

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
)

type funaiAssistTransport func(*http.Request) (*http.Response, error)

func (f funaiAssistTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func funaiAssistReply(req *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func TestFunAIAssistUsesHelperAccountAndChecksServerCompletion(t *testing.T) {
	for _, code := range []string{"0", "513", "401"} {
		t.Run(code, func(t *testing.T) {
			accepted := false
			targetClient := corehttp.NewClient(corehttp.WithTransport(funaiAssistTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/ycloud/funai/api/invite/getcode" {
					return funaiAssistReply(req, `{"code":0,"result":"target-invite"}`), nil
				}
				if accepted {
					return funaiAssistReply(req, `{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"fuli","taskSign":1,"isComplete":1}]}}`), nil
				}
				return funaiAssistReply(req, `{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"fuli","taskSign":1,"isComplete":0}]}}`), nil
			})))
			helperClient := corehttp.NewClient(corehttp.WithTransport(funaiAssistTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/ycloud/funai/api/invite/accept" {
					body, _ := io.ReadAll(req.Body)
					if req.Header.Get("Authorization") != "Basic helper-auth" || !strings.Contains(string(body), "target-invite") {
						t.Fatal("assist used wrong account or invite code")
					}
					accepted = code == "0"
					return funaiAssistReply(req, `{"code":`+code+`,"msg":"eligibility result"}`), nil
				}
				return funaiAssistReply(req, `{"code":0}`), nil
			})))
			helperClient.SetAuth("helper-auth")
			helperClient.SetDeviceID("test-helper")
			runner := &TaskRunner{api: api.NewCaiyunAPI(targetClient)}
			_, confirmed, err := runner.assistFunAI(api.NewCaiyunAPI(helperClient))
			if code == "0" && (err != nil || !confirmed) {
				t.Fatalf("accepted assist not confirmed: %v %v", confirmed, err)
			}
			if code == "513" && (err != nil || confirmed) {
				t.Fatalf("eligibility gate was misreported: %v %v", confirmed, err)
			}
			if code == "401" && err == nil {
				t.Fatal("authentication failure was hidden")
			}
		})
	}
}
