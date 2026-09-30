package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestPosterUploadUsesDistinctMarketAndUnsignedPut(t *testing.T) {
	var uploadedHash string
	transport := mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch req.URL.Path {
		case "/ycloud/api/cloud/ose/activity/getUploadUrl":
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil || payload["marketName"] != posterUploadMarketName || payload["fileSize"].(float64) <= 0 {
				t.Fatalf("poster reservation payload = %s, err = %v", body, err)
			}
			return mailTestResponse(req, 200, `{"code":0,"result":{"uploadUrl":"https://upload.example/presigned","uploadId":"up-1","fileId":"file-1","hashAlgorithm":"SHA256"}}`), nil
		case "/presigned":
			if req.Method != http.MethodPut || req.Header.Get("Authorization") != "" ||
				req.Header.Get("jwttoken") != "" || req.Header.Get("deviceId") != "" ||
				req.Header.Get("Content-Type") != "image/png" {
				t.Fatalf("presigned PUT leaked account headers: %v", req.Header)
			}
			hash := sha256.Sum256(body)
			uploadedHash = hex.EncodeToString(hash[:])
			return mailTestResponse(req, 200, ``), nil
		case "/ycloud/api/cloud/ose/file/complete":
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil || payload["uploadId"] != "up-1" ||
				payload["fileId"] != "file-1" || payload["contentHash"] != uploadedHash {
				t.Fatalf("poster completion payload = %s, err = %v", body, err)
			}
			return mailTestResponse(req, 200, `{"code":0,"result":{"isAddLottoryCnt":true}}`), nil
		default:
			t.Fatalf("unexpected poster request: %s", req.URL)
			return nil, nil
		}
	})
	client := corehttp.NewClient(corehttp.WithTransport(transport))
	client.SetAuth("test-account-secret")
	client.SetJWTToken("header.payload.signature")
	if err := NewCaiyunAPI(client).CompletePosterTask(); err != nil {
		t.Fatal(err)
	}
	if uploadedHash == "" {
		t.Fatal("poster content was not uploaded")
	}
}
