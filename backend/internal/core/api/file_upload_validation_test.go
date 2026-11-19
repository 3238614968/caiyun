package api

import (
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestFileUploadCannotSucceedFromPartialOrContradictoryResponses(t *testing.T) {
	for _, tc := range []struct {
		name, stage, body string
		status            int
	}{
		{"create HTTP failure", "create", `{"code":0,"data":{"fileId":"fake","rapidUpload":true}}`, 500},
		{"create explicit failure", "create", `{"success":false,"code":0,"data":{"fileId":"fake","rapidUpload":true}}`, 200},
		{"create conflicting business code", "create", `{"success":true,"code":"FAILED","data":{"fileId":"fake","rapidUpload":true}}`, 200},
		{"create missing ack", "create", `{"data":{"fileId":"fake","rapidUpload":true}}`, 200},
		{"URL HTTP failure", "getUploadUrl", `{"code":0,"data":{"uploadUrl":"https://upload.example/photo"}}`, 500},
		{"URL conflicting business code", "getUploadUrl", `{"success":true,"code":"FAILED","data":{"uploadUrl":"https://upload.example/photo"}}`, 200},
		{"PUT failure", "photo", "", 503},
		{"complete HTTP failure", "complete", `{"code":0}`, 500},
		{"complete explicit failure", "complete", `{"success":false,"code":0}`, 200},
		{"complete conflicting business code", "complete", `{"success":true,"code":"FAILED"}`, 200},
		{"complete missing ack", "complete", `{}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failedCalls, afterFailure := 0, false
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if failedCalls > 0 {
					afterFailure = true
				}
				if strings.HasSuffix(req.URL.Path, "/"+tc.stage) {
					failedCalls++
					return mailTestResponse(req, tc.status, tc.body), nil
				}
				switch req.URL.Path {
				case "/hcy/file/create":
					return mailTestResponse(req, 200, `{"code":0,"data":{"fileId":"photo-file","uploadId":"upload-file"}}`), nil
				case "/hcy/file/getUploadUrl":
					return mailTestResponse(req, 200, `{"code":0,"data":{"uploadUrl":"https://upload.example/photo"}}`), nil
				case "/photo":
					return mailTestResponse(req, 200, ""), nil
				case "/hcy/file/complete":
					return mailTestResponse(req, 200, `{"code":0}`), nil
				default:
					t.Fatalf("unexpected upload request: %s", req.URL.Path)
				}
				return nil, nil
			})))
			result, err := NewFileAPI(client).UploadRandomFile(&UploadRandomFileRequest{Content: []byte("unique content")})
			if err == nil || failedCalls != 1 || afterFailure {
				t.Fatalf("result=%+v failedCalls=%d afterFailure=%t err=%v", result, failedCalls, afterFailure, err)
			}
			if tc.stage == "create" && result != nil {
				t.Fatalf("create failure returned an unowned resource: %+v", result)
			}
			if tc.stage != "create" && (result == nil || result.FileID != "photo-file") {
				t.Fatalf("created file was lost after %s: %+v", tc.stage, result)
			}
		})
	}
}

func TestFileUploadPreservesHCYSuccessCodeFormats(t *testing.T) {
	for _, code := range []string{`0`, `"0"`, `"0000"`} {
		t.Run(code, func(t *testing.T) {
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/hcy/file/create":
					return mailTestResponse(req, 200, `{"code":`+code+`,"data":{"fileId":"photo","uploadId":"upload","partInfos":[{"uploadUrl":"https://upload.example/photo"}]}}`), nil
				case "/photo":
					return mailTestResponse(req, 200, ""), nil
				case "/hcy/file/complete":
					return mailTestResponse(req, 200, `{"success":true,"code":`+code+`}`), nil
				default:
					t.Fatalf("unexpected request: %s", req.URL.Path)
				}
				return nil, nil
			})))
			result, err := NewFileAPI(client).UploadRandomFile(&UploadRandomFileRequest{Content: []byte("unique content")})
			if err != nil || result == nil || result.FileID != "photo" {
				t.Fatalf("code=%s result=%+v err=%v", code, result, err)
			}
		})
	}
}
