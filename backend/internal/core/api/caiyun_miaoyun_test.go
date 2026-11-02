package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestCompleteFunAIMiaoyunTaskProtocol(t *testing.T) {
	var folders, uploaded, verified, composed int
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasPrefix(req.URL.Path, "/aitools/miaoyun/") {
			if req.Header.Get("Authorization") != "Basic test-auth" {
				t.Fatal("missing Basic auth")
			}
			for _, header := range []string{"jwtToken", "x-yun-uni", "x-yun-client-info", "Cookie", "x-yun-app-channel"} {
				if req.Header.Get(header) != "" {
					t.Fatalf("Miaoyun inherited cloud header %s", header)
				}
			}
		}
		var body []byte
		if req.Body != nil {
			body, _ = io.ReadAll(req.Body)
		}
		switch req.URL.Path {
		case "/hcy/file/create":
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["type"] == "folder" {
				folders++
				return mailTestResponse(req, 200, fmt.Sprintf(`{"success":true,"data":{"fileId":"folder-%d"}}`, folders)), nil
			}
			if payload["parentFileId"] != "folder-4" || payload["contentType"] != "image/jpeg" {
				t.Fatalf("miaoyun upload fields = parent:%v contentType:%v", payload["parentFileId"], payload["contentType"])
			}
			uploaded++
			return mailTestResponse(req, 200, `{"success":true,"data":{"fileId":"photo-1","rapidUpload":true}}`), nil
		case "/aitools/miaoyun/image/recordUnfinishFileId":
			return mailTestResponse(req, 200, `{"success":true}`), nil
		case "/aitools/miaoyun/image/getUnfinishFileId":
			return mailTestResponse(req, 200, `{"code":"0000","data":{"taskId":"task-1"}}`), nil
		case "/aitools/miaoyun/image/verifyMainImage":
			verified++
			if !strings.Contains(string(body), "photo-1") {
				t.Fatalf("miaoyun verification = %s", body)
			}
			return mailTestResponse(req, 200, `{"data":{"masterId":"master-1","taskId":"task-1"}}`), nil
		case "/aitools/miaoyun/image/createImage":
			if !strings.Contains(string(body), "master-1") || !strings.Contains(string(body), "folder-5") {
				t.Fatalf("miaoyun image creation = %s", body)
			}
			return mailTestResponse(req, 200, `{"data":{"imageId":"image-1"}}`), nil
		case "/aitools/miaoyun/model/queryModelList":
			return mailTestResponse(req, 200, `{"code":"0000","data":{"modelList":[{"list":[{"modelType":2007,"poseId":"pose-from-server"}]}]}}`), nil
		case "/aitools/miaoyun/art/autoComposeArtPhoto":
			composed++
			if !strings.Contains(string(body), "image-1") {
				t.Fatalf("miaoyun art generation = %s", body)
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil || payload["modelType"] != float64(2007) || payload["poseId"] != "pose-from-server" {
				t.Fatalf("art request ignored live model: %s", body)
			}
			return mailTestResponse(req, 200, `{"success":true}`), nil
		default:
			t.Fatalf("unexpected miaoyun endpoint: %s", req.URL)
		}
		return nil, nil
	})))
	client.SetAuth("test-auth")
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"12345"}}`))
	client.SetJWTToken("header." + claims + ".signature")
	if err := NewCaiyunAPI(client).CompleteFunAIMiaoyunTask(); err != nil {
		t.Fatal(err)
	}
	if folders != 5 || uploaded != 1 || verified != 1 || composed != 1 {
		t.Fatalf("folders=%d uploaded=%d verified=%d composed=%d", folders, uploaded, verified, composed)
	}
}

func TestMiaoyunPreservesDownstreamErrorInsteadOfBlamingTemplate(t *testing.T) {
	calls := 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return mailTestResponse(req, 200, `{"code":"100500","success":false,"message":"下游服务响应异常"}`), nil
	})))
	client.SetAuth("fixture-auth")
	_, err := NewCaiyunAPI(client).verifyMiaoyunPhoto("fixture-user", "fixture-file")
	if err == nil || !strings.Contains(err.Error(), "100500") || !strings.Contains(err.Error(), "下游服务响应异常") || calls != 1 {
		t.Fatalf("downstream error lost: calls=%d err=%v", calls, err)
	}
}

func TestMiaoyunRejectsMissingOrFailedModelLists(t *testing.T) {
	for _, payload := range []string{`{"code":"0000","data":{"modelList":[]}}`, `{"code":"500","message":"下游失败","data":{"modelList":[{"list":[{"modelType":1001}]}]}}`} {
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			t.Fatal(err)
		}
		if _, _, err := selectMiaoyunModel(result); err == nil {
			t.Fatal("invalid model list accepted")
		}
	}
}
