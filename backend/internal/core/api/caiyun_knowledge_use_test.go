package api

import (
	corehttp "caiyun/internal/core/http"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestKnowledgeUseUploadsIntoOwnedLibraryAndChatsWithLibraryAttachment(t *testing.T) {
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
					reply = fmt.Sprintf(`{"code":"0000","data":{"baseList":[{"baseId":"user-library","name":"用户自己的库"},{"baseId":"task-library","name":%q,"baseType":1,"openLevel":0}]}}`, taskKnowledgeBaseName(time.Now(), "1040000"))
				case "/api/outer/assistant/knowledge/personal/v2/base/create":
					reply = `{"code":"0000","data":{"base":{"baseId":"created-base"}}}`
				case "/api/outer/assistant/knowledge/personal/v2/file/upload/createFile":
					if req.Header.Get("Accept") != "application/json" || body["sourceChannel"] != "1040000" || body["baseId"] != "task-library" || body["parentFileId"] != "" {
						t.Fatal(body)
					}
					if body["contentHash"] == "" || body["contentHashAlgorithm"] != "sha256" || body["fileList"] != nil {
						t.Fatal(body)
					}
					imported = true
					reply = `{"code":"0000","data":{"fileId":"fixture-file","rapidUpload":true}}`
					if failImport {
						reply = `{"code":"1001","message":"导入失败"}`
					}
				case "/api/outer/assistant/knowledge/personal/v2/base/resource/list":
					if imported {
						reply = `{"code":"0000","data":{"list":[{"resourceId":"ready-resource","aiStatus":1}]}}`
					} else {
						reply = `{"code":"0000","data":{"list":[]}}`
					}
				case "/api/outer/assistant/chat/config":
					if !imported || failImport {
						t.Fatal("config before successful import")
					}
				case "/api/outer/assistant/chat/v2/add":
					if body["sourceChannel"] != "1040000" || req.Header.Get("Accept") != "text/event-stream, application/json" {
						t.Fatal("wrong channel/headers")
					}
					input := body["dialogueInput"].(map[string]interface{})
					attachment := input["attachment"].(map[string]interface{})
					if attachment["attachmentTypeList"].([]interface{})[0] != float64(8) || attachment["fileList"] != nil {
						t.Fatal(attachment)
					}
					base := attachment["knowledgeBaseList"].([]interface{})[0].(map[string]interface{})
					if base["baseId"] != "task-library" || base["baseType"] != float64(1) || input["extInfo"] != `{"showBasedKnowledge":true}` {
						t.Fatal(input)
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
			wantID := "fixture-file"
			if failImport {
				wantID = ""
			}
			if id != wantID || (err != nil) != failImport || chatted == failImport {
				t.Fatalf("id=%s err=%v chatted=%v", id, err, chatted)
			}
			if failImport && !strings.Contains(err.Error(), "1001") {
				t.Fatal(err)
			}
		})
	}
}

func TestKnowledgeUseCreatesUniqueMaterialAndRejectsLateChatFailure(t *testing.T) {
	hashes := map[string]bool{}
	creates := 0
	resources := 0
	c := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/outer/assistant/knowledge/personal/v2/base/list":
			return mailTestResponse(req, 200, fmt.Sprintf(`{"code":"0000","data":{"baseList":[{"baseId":"task-owned","name":%q,"baseType":1,"openLevel":0}]}}`, taskKnowledgeBaseName(time.Now(), "1040000"))), nil
		case "/api/outer/assistant/knowledge/personal/v2/base/resource/list":
			resources++
			if resources%2 == 1 {
				return mailTestResponse(req, 200, `{"code":"0000","data":{"list":[]}}`), nil
			}
			return mailTestResponse(req, 200, `{"code":"0000","data":{"list":[{"resourceId":"ready","aiStatus":1}]}}`), nil
		case "/api/outer/assistant/knowledge/personal/v2/base/create":
			return mailTestResponse(req, 200, `{"code":"0000","data":{"base":{"baseId":"created-base"}}}`), nil
		case "/api/outer/assistant/knowledge/personal/v2/file/upload/createFile":
			creates++
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			hash := fmt.Sprint(body["contentHash"])
			if hash == "" || hashes[hash] {
				t.Fatal("knowledge material reused an old content hash")
			}
			hashes[hash] = true
			return mailTestResponse(req, 200, fmt.Sprintf(`{"code":"0000","data":{"fileId":"knowledge-%d","rapidUpload":true}}`, creates)), nil
		case "/api/outer/assistant/chat/config":
			return mailTestResponse(req, 200, `{"code":"0000"}`), nil
		case "/api/outer/assistant/chat/v2/add":
			return mailTestResponse(req, 200, "data: {\"code\":\"0000\"}\n\ndata: {\"code\":\"01000005\"}\n\n"), nil
		default:
			t.Fatalf("unexpected knowledge request: %s", req.URL.Path)
		}
		return nil, nil
	})))
	c.SetJWTToken("x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture"}}`)) + ".x")
	for i := 0; i < 2; i++ {
		id, err := NewCaiyunAPI(c).CompleteFunAIKnowledgeUse()
		if id == "" || err == nil || !strings.Contains(err.Error(), "01000005") {
			t.Fatalf("id=%s err=%v", id, err)
		}
	}
	if len(hashes) != 2 {
		t.Fatal("knowledge material was not unique across executions")
	}
}

func TestKnowledgePendingIndexRetainsMaterialAndDoesNotReplayUploadOrChat(t *testing.T) {
	reads, uploads := 0, 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/outer/assistant/knowledge/personal/v2/base/list":
			return mailTestResponse(req, 200, fmt.Sprintf(`{"code":"0000","data":{"baseList":[{"baseId":"owned","name":%q,"baseType":1,"openLevel":0}]}}`, taskKnowledgeBaseName(time.Now(), "1040000"))), nil
		case "/api/outer/assistant/knowledge/personal/v2/base/resource/list":
			reads++
			if reads == 1 {
				return mailTestResponse(req, 200, `{"code":"0000","data":{"list":null}}`), nil
			}
			return mailTestResponse(req, 200, `{"code":"0000","data":{"list":[{"resourceId":"processing","aiStatus":0}]}}`), nil
		case "/api/outer/assistant/knowledge/personal/v2/file/upload/createFile":
			uploads++
			return mailTestResponse(req, 200, `{"code":"0000","data":{"fileId":"retained-material","rapidUpload":true}}`), nil
		default:
			t.Fatalf("unready index must not chat, import, delete or register: %s", req.URL.Path)
		}
		return nil, nil
	})))
	client.SetJWTToken("h." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture"}}`)) + ".s")
	for attempt := 0; attempt < 2; attempt++ {
		id, err := NewCaiyunAPI(client).completeFunAIKnowledgeUse(func(time.Duration) {})
		var pending *KnowledgeIndexPendingError
		if !errors.As(err, &pending) {
			t.Fatalf("expected pending, got %v", err)
		}
		if attempt == 0 && id != "retained-material" {
			t.Fatal("lost pending resource identity")
		}
	}
	if uploads != 1 || reads != 14 {
		t.Fatalf("uploads=%d reads=%d", uploads, reads)
	}
}
