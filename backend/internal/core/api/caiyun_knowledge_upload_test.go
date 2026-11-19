package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestKnowledgeUploadChecksEachStageAndKeepsPresignedPutAnonymous(t *testing.T) {
	for _, failure := range []string{"", "create", "put", "complete", "missing file", "missing upload"} {
		t.Run(failure, func(t *testing.T) {
			puts, completes := 0, 0
			content := []byte("a real knowledge document")
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == "PUT" {
					puts++
					data, _ := io.ReadAll(req.Body)
					if req.URL.Host != "upload.example" || !bytes.Equal(data, content) || req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" || req.Header.Get("jwtToken") != "" {
						t.Fatal("upload bytes or anonymous transport changed")
					}
					status := 201
					if failure == "put" {
						status = 403
					}
					return mailTestResponse(req, status, ""), nil
				}
				var body map[string]interface{}
				_ = json.NewDecoder(req.Body).Decode(&body)
				if body["baseId"] != "owned-base" || body["sourceChannel"] != "1040000" || body["contentHash"] != sha256Hex(content) {
					t.Fatal(body)
				}
				switch {
				case strings.HasSuffix(req.URL.Path, "/createFile"):
					if failure == "create" {
						return mailTestResponse(req, 200, `{"code":"0000","success":false}`), nil
					}
					if failure == "missing file" {
						return mailTestResponse(req, 200, `{"code":"0000","data":{}}`), nil
					}
					if failure == "missing upload" {
						return mailTestResponse(req, 200, `{"code":"0000","data":{"fileId":"owned-file"}}`), nil
					}
					return mailTestResponse(req, 200, `{"code":"0000","success":true,"data":{"fileId":"owned-file","uploadId":"upload-id","partInfos":[{"uploadUrl":"https://upload.example/part"}]}}`), nil
				case strings.HasSuffix(req.URL.Path, "/complete"):
					completes++
					if body["fileId"] != "owned-file" || body["uploadId"] != "upload-id" {
						t.Fatal(body)
					}
					if failure == "complete" {
						return mailTestResponse(req, 200, `{"code":"5000"}`), nil
					}
					return mailTestResponse(req, 200, `{"code":"0000","success":true}`), nil
				default:
					t.Fatalf("unexpected %s", req.URL.Path)
				}
				return nil, nil
			})))
			client.SetAuth("Basic private")
			client.SetJWTToken("private-jwt")
			id, err := NewCaiyunAPI(client).uploadKnowledgeMaterial("owned-base", "note.txt", content)
			if (err != nil) != (failure != "") {
				t.Fatalf("id=%s err=%v", id, err)
			}
			if failure == "" && (id != "owned-file" || puts != 1 || completes != 1) {
				t.Fatal("missing upload stage")
			}
			if failure == "put" && completes != 0 {
				t.Fatal("complete called after failed PUT")
			}
		})
	}
}
