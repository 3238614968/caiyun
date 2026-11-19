package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
)

func TestNineGridWaitsForIndexedDynamicWithoutRepeatingWrites(t *testing.T) {
	reads, waits := 0, 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/dynamic/chat/list" {
			t.Fatal("waiting for the index must only read records")
		}
		reads++
		if reads < 3 {
			return mailTestResponse(req, 200, `{"code":"0000","data":{"list":[]}}`), nil
		}
		return mailTestResponse(req, 200, `{"code":"0000","data":{"list":[{"dynamicId":"real","title":"朋友圈9图","fileCount":10}]}}`), nil
	})))
	if err := NewCaiyunAPI(client).waitNineGridRecord("real", func(time.Duration) { waits++ }); err != nil || reads != 3 || waits != 2 {
		t.Fatalf("reads=%d waits=%d err=%v", reads, waits, err)
	}
}

func TestNineGridResumesRealOutputAndDoesNotCreateOrRepeatEvents(t *testing.T) {
	result := &NineGridResult{DynamicID: "real", DynamicAccepted: true}
	for i := 0; i < 11; i++ {
		result.FileIDs = append(result.FileIDs, fmt.Sprintf("file-%d", i))
	}
	events := 0
	client := knowledgeEventFixtureClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/hcy/file/batchGet":
			files := make([]map[string]interface{}, 0, 11)
			for _, id := range result.FileIDs {
				files = append(files, map[string]interface{}{"srcFile": map[string]interface{}{"fileId": id, "size": 1234}})
			}
			body, _ := json.Marshal(map[string]interface{}{"code": "0000", "data": map[string]interface{}{"batchFileResults": files}})
			return mailTestResponse(req, 200, string(body)), nil
		case "/hcy/file/getAuditResult":
			return mailTestResponse(req, 200, `{"code":"0000","data":{"contentAuditResult":{"status":2}}}`), nil
		case "/api/dynamic/chat/list":
			return mailTestResponse(req, 200, `{"code":"0000","data":{"list":[{"dynamicId":"real","title":"朋友圈9图","fileCount":10}]}}`), nil
		case "/webid", "/list":
			if req.URL.Hostname() != "dc.cmicapm.com" || !result.FilesVerified || !result.AuditVerified || !result.DynamicVerified {
				t.Fatal("unverified artwork emitted an event")
			}
			events++
			return mailTestResponse(req, 200, `{"e":0,"web_id":"fixture-web"}`), nil
		default:
			if strings.Contains(req.URL.Path, "create") || strings.Contains(req.URL.Path, "save") {
				t.Fatal("resume created another output")
			}
			t.Fatal("unexpected resume operation")
			return nil, nil
		}
	})))
	for i := 0; i < 2; i++ {
		if err := NewCaiyunAPI(client).VerifyAndReportNineGridResult(result); err != nil {
			t.Fatal(err)
		}
	}
	if events != 2 || !result.EventAccepted {
		t.Fatalf("events=%d accepted=%v", events, result.EventAccepted)
	}
}
