package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestCoc3MissingReceiptCannotBeRecoveredFromCookies(t *testing.T) {
	for _, body := range []string{"", " \r\n\t"} {
		t.Run(base64.StdEncoding.EncodeToString([]byte(body)), func(t *testing.T) {
			calls := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				response := mailTestResponse(req, http.StatusOK, body)
				response.Header.Add("Set-Cookie", "BSFIT_DEVICEID="+strings.Repeat("private-cookie", 8)+"; Path=/")
				response.Header.Add("Set-Cookie", "BSFIT_EXPIRATION=9999999999999; Path=/")
				response.Header.Set("Trace-Id", "private-upstream-trace")
				return response, nil
			})))
			value, err := NewCoc3API(client).Coc3Fingerprint(context.Background(), "fixture-session")
			if value != "" || !errors.Is(err, ErrCoc3FingerprintReceiptMissing) || calls != 1 {
				t.Fatalf("accepted=%v missing=%v calls=%d", value != "", errors.Is(err, ErrCoc3FingerprintReceiptMissing), calls)
			}
			if strings.Contains(err.Error(), "private-") || !strings.Contains(err.Error(), "HTTP 200") {
				t.Fatal("empty-receipt diagnostic lost HTTP context or exposed response identifiers")
			}
		})
	}
}

func TestFamilyCircleMissingRouteIsDistinctFromMissingGroup(t *testing.T) {
	for _, operation := range []string{"backup", "task"} {
		for _, status := range []int{http.StatusNotFound, http.StatusUnauthorized, http.StatusServiceUnavailable, http.StatusTemporaryRedirect} {
			t.Run(operation+"/"+http.StatusText(status), func(t *testing.T) {
				calls := 0
				client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					if req.URL.Host != "caiyun.feixin.10086.cn" {
						t.Fatal("family query followed an unconfirmed replacement host")
					}
					response := mailTestResponse(req, status, `{"code":404,"msg":"访问资源不存在","private":"private-response"}`)
					response.Header.Set("Location", "https://unconfirmed.example/?group=private-group")
					return response, nil
				})))
				claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"private-user"}}`))
				client.SetJWTToken("header." + claims + ".signature")
				cloud := NewCaiyunAPI(client)
				var err error
				if operation == "backup" {
					_, err = cloud.FamilyCircleBackupState("private-group")
				} else {
					_, err = cloud.FamilyCircleTaskState("private-group")
				}
				if err == nil || calls != 1 || errors.Is(err, ErrFamilyCircleEndpointUnavailable) != (status == http.StatusNotFound) {
					t.Fatalf("calls=%d error=%v", calls, err)
				}
				if strings.Contains(err.Error(), "private-") {
					t.Fatal("family diagnostic exposed user/group IDs or untrusted response body")
				}
			})
		}
	}
}

func TestFamilyCircleEmptySuccessEnvelopeIsNotAStateRecord(t *testing.T) {
	for _, body := range []string{"", "null", `{}`, `{"result":1}`, `{"code":0}`, `{"code":0,"result":null}`, `{"code":0,"result":{}}`, `{"code":0,"result":[]}`, `{"code":0,"result":true}`, `{"code":0,"result":1.5}`, `{"code":0,"result":""}`, `{"code":0,"result":1}trailing`} {
		client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return mailTestResponse(req, http.StatusOK, body), nil
		})))
		claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture-user"}}`))
		client.SetJWTToken("header." + claims + ".signature")
		if _, err := NewCaiyunAPI(client).FamilyCircleTaskState("fixture-group"); err == nil {
			t.Fatalf("unconfirmed state accepted: %s", body)
		}
	}
}

func TestFamilyCircleTimeoutPreservesCauseWithoutExposingGroupQuery(t *testing.T) {
	for _, phase := range []string{"request", "response-body"} {
		calls := 0
		client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			if phase == "request" {
				return nil, context.DeadlineExceeded
			}
			response := mailTestResponse(req, http.StatusOK, "")
			response.ContentLength = 1024
			response.Body = familyCircleDeadlineBody{}
			return response, nil
		})))
		claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"private-user"}}`))
		client.SetJWTToken("header." + claims + ".signature")
		_, err := NewCaiyunAPI(client).FamilyCircleBackupState("private-group")
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("%s timeout cause lost or query replayed: %v calls=%d", phase, err, calls)
		}
		if strings.Contains(err.Error(), "private-") {
			t.Fatal("timeout exposed user/group query parameters")
		}
	}
}

type familyCircleDeadlineBody struct{}

func (familyCircleDeadlineBody) Read([]byte) (int, error) { return 0, context.DeadlineExceeded }
func (familyCircleDeadlineBody) Close() error             { return nil }
