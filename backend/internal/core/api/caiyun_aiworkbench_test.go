package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestAIWorkbenchUsesMailChannelAndWritesDraft(t *testing.T) {
	chatCount, wrote := 0, false
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		switch req.URL.Path {
		case "/api/outer/assistant/chat/v2/add":
			chatCount++
			if payload["sourceChannel"] != "10175" || payload["userId"] != "12345" {
				t.Fatalf("AI workbench chat fields: sourceChannel=%v userId=%v", payload["sourceChannel"], payload["userId"])
			}
			return mailTestResponse(req, 200, "data: {\"code\":\"0000\",\"dialogueId\":\"dialogue-1\"}\n"), nil
		case "/api/outer/assistant/aiwriting/keyword/generate", "/api/outer/assistant/chat/config/get":
			return mailTestResponse(req, 200, `{"code":"0000"}`), nil
		case "/api/outer/assistant/aiwriting/chat/update":
			if payload["sourceChannel"] != "10175" || payload["dialogueId"] != "dialogue-1" || !strings.Contains(payload["htmlContent"].(string), "云盘使用心得") {
				t.Fatalf("AI writing payload = %v", payload)
			}
			wrote = true
			return mailTestResponse(req, 200, `{"code":"0000"}`), nil
		default:
			t.Fatalf("unexpected workbench endpoint: %s", req.URL)
		}
		return nil, nil
	})))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"12345"}}`))
	client.SetJWTToken("header." + claims + ".signature")
	if err := NewCaiyunAPI(client).CompleteAIWorkbenchTask(); err != nil {
		t.Fatal(err)
	}
	if chatCount != 2 || !wrote {
		t.Fatalf("chatCount=%d wrote=%v", chatCount, wrote)
	}
}
