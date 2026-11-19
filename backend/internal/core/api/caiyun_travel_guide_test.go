package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestTravelGuideValidatesEntireAIResponseWithoutRetrying(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		http int
		ok   bool
	}{
		{"JSON success", `{"code":"0000","success":true}`, 200, true},
		{"pretty JSON", "{\n  \"code\": \"0000\"\n}", 200, true},
		{"SSE success", "data: {\"code\":\"0000\"}\n\ndata: [DONE]\n\n", 200, true},
		{"multiline SSE", "event: message\r\ndata: {\r\ndata: \"code\":\"0000\"}\r\n\r\n", 200, true},
		{"empty 200", "", 200, false},
		{"whitespace 200", " \n\t", 200, false},
		{"HTML 200", "<html>upstream error</html>", 200, false},
		{"missing code", `{"message":"0000"}`, 200, false},
		{"business failure", `{"code":"01000002","success":true}`, 200, false},
		{"explicit failure", `{"code":"0000","success":false}`, 200, false},
		{"late SSE failure", "data: {\"code\":\"0000\"}\n\ndata: {\"code\":\"01000002\"}\n\n", 200, false},
		{"late SSE error event", "data: {\"success\":true}\n\nevent: error\ndata: {\"message\":\"failed\"}\n\n", 200, false},
		{"truncated SSE", "data: {\"code\":\"0000\"}\n\ndata: {", 200, false},
		{"done without ack", "data: [DONE]\n", 200, false},
		{"HTTP failure with successful body", `{"code":"0000"}`, 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != http.MethodPost || req.URL.Host != "ai.yun.139.com" || req.URL.Path != "/api/outer/assistant/chat/v2/add" {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
				}
				var body map[string]interface{}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				input, _ := body["dialogueInput"].(map[string]interface{})
				if body["userId"] != "guide-user" || body["sourceChannel"] != "101" || input["dialogue"] != "给我一份五一出行攻略" || input["enableForceNetworkSearch"] != true {
					t.Fatalf("wrong travel guide payload: %#v", body)
				}
				if req.Header.Get("Accept") != "text/event-stream, application/json" || !strings.Contains(req.Header.Get("Content-Type"), "application/json") {
					t.Fatal("AI headers do not match chat protocol")
				}
				return mailTestResponse(req, tc.http, tc.body), nil
			})))
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"guide-user"}}`))
			client.SetJWTToken("header." + claims + ".signature")
			err := NewCaiyunAPI(client).GenerateTravelGuide()
			if (err == nil) != tc.ok || calls != 1 {
				t.Fatalf("ok=%v calls=%d err=%v", tc.ok, calls, err)
			}
		})
	}
}

func TestTravelGuideRequiresAccountBeforeRequest(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("AI called without an account")
		return nil, nil
	})))
	if err := NewCaiyunAPI(client).GenerateTravelGuide(); err == nil {
		t.Fatal("missing userDomainId accepted")
	}
}

func TestTemporaryTaskListRejectsInvalidQueryResults(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"HTTP failure", `{"code":0,"result":{"beiyong1":[]}}`, 503},
		{"missing result", `{"code":0}`, 200},
		{"missing code", `{"result":{"beiyong1":[]}}`, 200},
		{"null result", `{"code":0,"result":null}`, 200},
		{"invalid JSON", "<html>failed</html>", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "taskListV2") {
					return mailTestResponse(req, tc.status, tc.body), nil
				}
				return mailTestResponse(req, 200, `{"code":0}`), nil
			})))
			client.SetJWTToken("fixture-jwt")
			client.SetDeviceID("Bfixture-device")
			if _, err := NewCaiyunAPI(client).GetTaskListV2("beiyong1"); err == nil {
				t.Fatal("invalid task query accepted as an empty activity group")
			}
		})
	}
}
