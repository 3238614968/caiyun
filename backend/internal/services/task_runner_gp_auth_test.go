package services

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
)

func TestGPTasksDoNotUseMobileTokenAfterGatewayLoginFailure(t *testing.T) {
	for _, activation := range []bool{true, false} {
		for _, code := range []string{"annual_bill", "luckybag", "tasklist_gp"} {
			t.Run(fmt.Sprintf("%v/%s", activation, code), func(t *testing.T) {
				client := corehttp.NewClient(corehttp.WithTransport(coc3RunnerRoundTrip(func(req *http.Request) (*http.Response, error) {
					t.Fatalf("unauthenticated GP task was submitted: %s", req.URL)
					return nil, nil
				})))
				client.SetJWTToken("mobile-token")
				var loginErr error = errors.New("gp login network failure")
				if activation {
					loginErr = fmt.Errorf("wrapped: %w", &auth.GatewayLoginError{Gateway: "gp.mcloud.139.com", Code: 9430})
				}
				runner := &TaskRunner{httpClient: client, gpAuthErr: loginErr}
				var result *TaskResult
				if code == "tasklist_gp" {
					result = runner.runGPTaskList()
				} else {
					result = runner.runAutumnTask(code)
				}
				if activation {
					if result.Status != "pending" || !strings.Contains(result.Message, "9430") {
						t.Fatalf("%+v", result)
					}
				} else if result.Status != "failed" || !strings.Contains(result.Message, "network failure") {
					t.Fatalf("%+v", result)
				}
			})
		}
	}
}
