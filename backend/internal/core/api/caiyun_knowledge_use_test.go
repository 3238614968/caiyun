package api

import (
	corehttp "caiyun/internal/core/http"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestKnowledgeUseImportsOwnedFileAndChatsWithLibraryAttachment(t *testing.T) {
	for _, failImport := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "import failure"}[failImport], func(t *testing.T) {
			imported, chatted := false, false
			c := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				var body map[string]interface{}
				if req.Body != nil {
					_ = json.NewDecoder(req.Body).Decode(&body)
				}
				reply := `{"code":"0000","data":{}}`
				switch req.URL.Path {
				case "/api/outer/assistant/knowledge/personal/v2/base/list":
					reply = `{"code":"0000","data":{"baseList":[{"baseId":"user-library","name":"用户自己的库"},{"baseId":"task-library","name":"云朵中心任务知识库"}]}}`
				case "/hcy/file/create":
					reply = `{"success":true,"data":{"fileId":"fixture-file","rapidUpload":true}}`
				case "/api/outer/assistant/knowledge/personal/v2/base/file/batchImport":
					if req.Header.Get("Accept") != "application/json" || body["sourceChannel"] != "1040000" || body["baseId"] != "task-library" || body["parentFileId"] != "" {
						t.Fatal(body)
					}
					files := body["fileList"].([]interface{})
					f := files[0].(map[string]interface{})
					if f["type"] != "file" || f["fileId"] != "fixture-file" {
						t.Fatal(files)
					}
					imported = true
					if failImport {
						reply = `{"code":"1001","message":"导入失败"}`
					}
				case "/api/outer/assistant/chat/config":
					if !imported || failImport {
						t.Fatal("config before successful import")
					}
				case "/api/outer/assistant/chat/v2/add":
					if body["sourceChannel"] != "1040023" || req.Header.Get("Accept") != "text/event-stream, application/json" {
						t.Fatal("wrong chat channel/headers")
					}
					input := body["dialogueInput"].(map[string]interface{})
					attachment := input["attachment"].(map[string]interface{})
					file := attachment["fileList"].([]interface{})[0].(map[string]interface{})
					if file["id"] != "task-library" || file["category"] != "knowledgeBase" || file["type"] != float64(8) {
						t.Fatal(file)
					}
					chatted = true
					reply = "data: {\"code\":\"0000\"}\n"
				default:
					t.Fatalf("unexpected %s", req.URL.Path)
				}
				return mailTestResponse(req, 200, reply), nil
			})))
			c.SetJWTToken("x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture"}}`)) + ".x")
			id, err := NewCaiyunAPI(c).CompleteFunAIKnowledgeUse()
			if id != "fixture-file" || (err != nil) != failImport || chatted == failImport {
				t.Fatalf("id=%s err=%v chatted=%v", id, err, chatted)
			}
			if failImport && !strings.Contains(err.Error(), "导入失败") {
				t.Fatal(err)
			}
		})
	}
}
