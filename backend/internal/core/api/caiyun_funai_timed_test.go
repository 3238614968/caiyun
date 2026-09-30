package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestFunaiTimedTranscriptionUsesServerModuleID(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/ycloud/funai/api/timed/transcriptioncompleted" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil || payload["moduleId"] != "2-250730-1" || payload["taskId"] != "ailuyinzhuanxie" {
			t.Fatalf("timed task payload = %s, err = %v", body, err)
		}
		return mailTestResponse(req, 200, `{"code":0,"msg":"success"}`), nil
	})))
	if err := NewCaiyunAPI(client).FunaiTimedTranscriptionCompleted("2-250730-1", "ailuyinzhuanxie"); err != nil {
		t.Fatal(err)
	}
}
