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
		case "/advertapi/adv-config/adv-config/AdInfoFilter/getAdInfos":
			return mailTestResponse(req, 200, `{"returnCode":"0","body":[{"materialList":[{"materialUrl":"https://example.com/face.jpg"}]}]}`), nil
		case "/face.jpg":
			if req.Header.Get("Authorization") != "" || req.Header.Get("Cookie") != "" {
				t.Fatal("public template request leaked account credentials")
			}
			photo, err := GenerateSampleJPEG(700, 700)
			if err != nil {
				t.Fatal(err)
			}
			return mailTestResponse(req, 200, string(photo)), nil
		case "/aitools/miaoyun/image/recordUnfinishFileId":
			return mailTestResponse(req, 200, `{"success":true}`), nil
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
		case "/aitools/miaoyun/art/autoComposeArtPhoto":
			composed++
			if !strings.Contains(string(body), "image-1") {
				t.Fatalf("miaoyun art generation = %s", body)
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
