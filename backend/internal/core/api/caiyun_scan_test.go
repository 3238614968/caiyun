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

func TestScanTransportRoundTripAndPadding(t *testing.T) {
	for _, plain := range []string{`{"x":1}`, `1234567890abcdef`, `{"scanType":1,"fileInfoList":[]}`} {
		ciphertext, err := encryptScanTransport([]byte(plain))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decryptScanTransport(ciphertext)
		if err != nil || string(decoded) != plain {
			t.Fatalf("scan transport round trip = %q, %v", decoded, err)
		}
	}
	if _, err := decryptScanTransport("invalid"); err == nil {
		t.Fatal("invalid encrypted response was accepted")
	}
}

func TestCompleteFunAIScanTaskProtocol(t *testing.T) {
	var saved bool
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch req.URL.Path {
		case "/api/image/aiCamera/algorithmLayout":
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			if len(req.Header["x-yun-app-channel"]) == 0 || req.Header["x-yun-app-channel"][0] != "10000023" || payload["channelId"] != "100101" || payload["uploadToCloud"] != true || len(payload["base64"].(string)) < 1000 {
				t.Fatalf("scan algorithm fields: channel=%q, requestChannel=%v, uploadToCloud=%v", req.Header["x-yun-app-channel"], payload["channelId"], payload["uploadToCloud"])
			}
			return mailTestResponse(req, 200, `{"code":"0000","data":{"taskId":1234567890123456789}}`), nil
		case "/api/outer/async/task/resultList":
			if !strings.Contains(string(body), "1234567890123456789") {
				t.Fatalf("scan task ID lost precision: %s", body)
			}
			return mailTestResponse(req, 200, `{"data":[{"resultList":[{"fileUrlList":["https://eos.example/test.jpg"]}]}]}`), nil
		case "/adaptor/ai-camera/scans/pic/save":
			if len(req.Header["X-YUN-API-VERSION"]) == 0 || req.Header["X-YUN-API-VERSION"][0] != "3" || len(req.Header["hcy-cool-flag"]) == 0 || req.Header["hcy-cool-flag"][0] != "1" {
				t.Fatalf("scan save version=%v coolFlag=%v", req.Header["X-YUN-API-VERSION"], req.Header["hcy-cool-flag"])
			}
			decoded, err := decryptScanTransport(string(body))
			if err != nil || !strings.Contains(string(decoded), "https://eos.example/test.jpg") {
				t.Fatalf("scan save body = %s, %v", decoded, err)
			}
			saved = true
			response, err := encryptScanTransport([]byte(`{"code":"0000","data":{"scanRecordId":"record-1"}}`))
			if err != nil {
				t.Fatal(err)
			}
			return mailTestResponse(req, 200, response), nil
		default:
			t.Fatalf("unexpected scan endpoint: %s", req.URL.Path)
		}
		return nil, nil
	})))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"12345"}}`))
	client.SetJWTToken("header." + claims + ".signature")
	if err := NewCaiyunAPI(client).CompleteFunAIScanTask(); err != nil {
		t.Fatal(err)
	}
	if !saved {
		t.Fatal("scan result was not saved")
	}
}
