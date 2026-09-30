package api

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestUploadRandomFileGetsMissingUploadURL(t *testing.T) {
	var sequence []string
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		sequence = append(sequence, req.Method+" "+req.URL.Path)
		switch req.URL.Path {
		case "/hcy/file/create":
			body, _ := io.ReadAll(req.Body)
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil || payload["size"] != float64(11) {
				t.Fatalf("create payload=%s err=%v", body, err)
			}
			return mailTestResponse(req, 200, `{"code":0,"data":{"fileId":"file-1","uploadId":"upload-1","partInfos":[{"partNumber":1}]}}`), nil
		case "/hcy/file/getUploadUrl":
			body, _ := io.ReadAll(req.Body)
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil || payload["fileId"] != "file-1" || payload["uploadId"] != "upload-1" {
				t.Fatalf("getUploadUrl payload=%s err=%v", body, err)
			}
			return mailTestResponse(req, 200, `{"code":0,"data":{"partInfos":[{"uploadUrl":"https://upload.example/file-1"}]}}`), nil
		case "/file-1":
			if req.Method != http.MethodPut || req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
				t.Fatalf("signed upload leaked account headers: method=%s authorization=%q cookie=%q", req.Method, req.Header.Get("Authorization"), req.Header.Get("Cookie"))
			}
			body, _ := io.ReadAll(req.Body)
			if string(body) != "hello world" {
				t.Fatalf("uploaded bytes=%q", body)
			}
			return mailTestResponse(req, 200, ""), nil
		case "/hcy/file/complete":
			return mailTestResponse(req, 200, `{"code":0}`), nil
		default:
			t.Fatalf("unexpected upload request: %s", req.URL)
		}
		return nil, nil
	})))
	client.SetAuth("test-auth")
	result, err := NewFileAPI(client).UploadRandomFile(&UploadRandomFileRequest{
		ParentFileID: "/", Name: "test.txt", Content: []byte("hello world"), ContentType: "text/plain",
	})
	if err != nil || result == nil || result.FileID != "file-1" {
		t.Fatalf("upload result=%+v err=%v", result, err)
	}
	want := []string{"POST /hcy/file/create", "POST /hcy/file/getUploadUrl", "PUT /file-1", "POST /hcy/file/complete"}
	if !reflect.DeepEqual(sequence, want) {
		t.Fatalf("upload sequence=%v, want %v", sequence, want)
	}
}
