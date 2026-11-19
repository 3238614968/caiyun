package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
)

func TestKnowledgeCreationDoesNotReuseYesterdayOrAcceptEmptyAcknowledgement(t *testing.T) {
	for _, response := range []string{
		`{"code":"0000","data":{"base":{"baseId":"created-today"}}}`,
		`{"code":"0000"}`,
		`{"code":"0000","success":false,"data":{"base":{"baseId":"created-today"}}}`,
	} {
		t.Run(response, func(t *testing.T) {
			creates := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/base/list") {
					return mailTestResponse(req, 200, `{"code":"0000","data":{"baseList":[{"baseId":"yesterday","name":"云盘任务知识库-20261111-AI","baseType":1,"openLevel":0}]}}`), nil
				}
				if !strings.HasSuffix(req.URL.Path, "/base/create") {
					t.Fatal(req.URL.Path)
				}
				creates++
				var body map[string]interface{}
				_ = json.NewDecoder(req.Body).Decode(&body)
				if body["name"] != "云盘任务知识库-20261112-AI" || body["sourceChannel"] != "1040000" || body["openLevel"] != float64(0) {
					t.Fatal(body)
				}
				return mailTestResponse(req, 200, response), nil
			})))
			client.SetJWTToken("h." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture"}}`)) + ".s")
			err := NewCaiyunAPI(client).createKnowledgeBaseForSource(time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC), "1040000")
			wantSuccess := response == `{"code":"0000","data":{"base":{"baseId":"created-today"}}}`
			if (err == nil) != wantSuccess || creates != 1 {
				t.Fatalf("creates=%d err=%v", creates, err)
			}
		})
	}
}
