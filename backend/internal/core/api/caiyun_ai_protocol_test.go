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

func TestAICameraSeparatesJSONAndSSEHeadersAndCleansOnlyItsUpload(t *testing.T) {
	for _, tc := range []struct {
		name          string
		recognizeHTTP int
		cleanupOK     bool
	}{
		{"success", 200, true},
		{"recognition failure", 406, true},
		{"cleanup failure", 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cleaned, chatted bool
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/hcy/file/create":
					return mailTestResponse(req, 200, `{"code":0,"data":{"fileId":"camera-temp","uploadId":"camera-upload","partInfos":[{"partNumber":1,"uploadUrl":"https://upload.example/camera-temp"}]}}`), nil
				case "/camera-temp":
					if req.Method != "PUT" {
						t.Fatal("sample was not uploaded")
					}
					return mailTestResponse(req, 200, ""), nil
				case "/hcy/file/complete":
					return mailTestResponse(req, 200, `{"code":0}`), nil
				case "/api/image/aiRecognize":
					if req.Header.Get("Accept") != "application/json" {
						t.Fatal("image endpoint received SSE headers")
					}
					body, _ := io.ReadAll(req.Body)
					var payload map[string]interface{}
					if err := json.Unmarshal(body, &payload); err != nil || payload["fileId"] != "camera-temp" || payload["sendType"] != "3" {
						t.Fatalf("recognition payload=%s err=%v", body, err)
					}
					if tc.recognizeHTTP != 200 {
						return mailTestResponse(req, tc.recognizeHTTP, "<html>Not acceptable</html>"), nil
					}
					return mailTestResponse(req, 200, `{"success":true,"code":"0000"}`), nil
				case "/api/outer/assistant/chat/v2/add":
					chatted = true
					if req.Header.Get("Accept") != "text/event-stream, application/json" {
						t.Fatal("chat endpoint did not request SSE")
					}
					body, _ := io.ReadAll(req.Body)
					if !strings.Contains(string(body), "camera-temp") {
						t.Fatal("chat did not use the uploaded image")
					}
					return mailTestResponse(req, 200, "data: {\"code\":\"0000\"}\n\n"), nil
				case "/hcy/recyclebin/batchTrash":
					cleaned = true
					body, _ := io.ReadAll(req.Body)
					var payload struct {
						FileIDs []string `json:"fileIds"`
					}
					if err := json.Unmarshal(body, &payload); err != nil || len(payload.FileIDs) != 1 || payload.FileIDs[0] != "camera-temp" {
						t.Fatalf("unexpected cleanup target=%s err=%v", body, err)
					}
					if !tc.cleanupOK {
						return mailTestResponse(req, 200, `{"success":false,"code":"401"}`), nil
					}
					return mailTestResponse(req, 200, `{"success":true,"code":"0000"}`), nil
				default:
					t.Fatalf("unexpected camera request: %s", req.URL)
				}
				return nil, nil
			})))
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"user-test"}}`))
			client.SetJWTToken("header." + claims + ".signature")
			err := NewCaiyunAPI(client).CompleteAICameraTask()
			if (err != nil) != (tc.recognizeHTTP != 200 || !tc.cleanupOK) || !cleaned || chatted != (tc.recognizeHTTP == 200) {
				t.Fatalf("cleaned=%v chatted=%v err=%v", cleaned, chatted, err)
			}
		})
	}
}

func TestAIChatRequiresExactSuccessCode(t *testing.T) {
	api := &CaiyunAPI{}
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"code":"01000002","message":"0000 is not success"}`, false},
		{`{"code":"01000001"}`, false},
		{`{"code":"01000002","success":true}`, false},
		{`{"message":"0000"}`, false},
		{"data: {\"code\":\"0000\"}\n\ndata: [DONE]\n", true},
		{"data: {\"success\":true}\n\n", true},
	} {
		if got := api.isAICameraChatSuccess(tc.body); got != tc.want {
			t.Errorf("body=%s got=%t", tc.body, got)
		}
	}
}
