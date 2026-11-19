package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestKnowledgeNativeCreationUsesPrivateLibraryAndOfficialDefaultAvatar(t *testing.T) {
	var calls []string
	photo := map[string]interface{}{"photoId": "official-default", "type": float64(1), "url": "https://yun.139.com/default.png"}
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Path)
		info := req.Header.Get("x-yun-client-info")
		if !strings.HasPrefix(info, "4||1|13.2.5|") || !strings.Contains(info, "||android 15|||||") || req.Header.Get("x-DeviceInfo") != info || req.Header.Get("x-huawei-channelSrc") != "10252400" || req.Header.Get("x-yun-tid") == "" {
			t.Fatalf("native request descriptor incomplete")
		}
		if req.Header.Get("Origin") != "https://yun.139.com" || req.Header.Get("Referer") != "https://yun.139.com/ai-helper/?sourceid=810292" || req.Header.Get("sec-ch-ua") != "" {
			t.Fatal("native request contains a standalone desktop context")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body["sourceChannel"] != "101" || body["userId"] != "fixture" {
			t.Fatal("invalid native body")
		}
		switch {
		case strings.HasSuffix(req.URL.Path, "/base/list"):
			return mailTestResponse(req, 200, `{"code":"0000","data":{"baseList":[]}}`), nil
		case strings.HasSuffix(req.URL.Path, "/profilePhoto/list"):
			return mailTestResponse(req, 200, `{"code":"0000","data":[{"photoId":"official-default","type":1,"url":"https://yun.139.com/default.png"}]}`), nil
		case strings.HasSuffix(req.URL.Path, "/base/create"):
			if body["openLevel"] != float64(0) || !reflect.DeepEqual(body["profilePhoto"], photo) || body["name"] != "任务私人知识库" {
				t.Fatal("creation must retain the official avatar and private state")
			}
			return mailTestResponse(req, 200, `{"code":"0000","success":true,"data":{"base":{"baseId":"created"}}}`), nil
		default:
			t.Fatal("unexpected request")
			return nil, nil
		}
	})))
	client.SetJWTToken("h." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture"}}`)) + ".s")
	if err := NewCaiyunAPI(client).createNamedKnowledgeBase("101", "任务私人知识库"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 {
		t.Fatal(calls)
	}
}

func TestKnowledgeNameLimitCountsUnicodeCharactersBeforeAnyRequest(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("invalid names must not contact upstream")
		return nil, nil
	})))
	for _, name := range []string{"", "  ", strings.Repeat("库", 26)} {
		if err := NewCaiyunAPI(client).createNamedKnowledgeBase("101", name); err == nil || !strings.Contains(err.Error(), "1至25") {
			t.Fatalf("invalid name was accepted: %v", err)
		}
	}
}

func TestKnowledgeNativeAvatarFailureDoesNotCreateLibrary(t *testing.T) {
	for _, reply := range []string{`{"code":"1001"}`, `{"code":"0000","success":false,"data":[{"photoId":"photo"}]}`, `{"code":"0000","data":[]}`, `{"code":"0000","data":[{}]}`} {
		t.Run(reply, func(t *testing.T) {
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/base/list") {
					return mailTestResponse(req, 200, `{"code":"0000","data":{"baseList":[]}}`), nil
				}
				if !strings.HasSuffix(req.URL.Path, "/profilePhoto/list") {
					t.Fatal("invalid avatar must stop creation")
				}
				return mailTestResponse(req, 200, reply), nil
			})))
			client.SetJWTToken("h." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture"}}`)) + ".s")
			if err := NewCaiyunAPI(client).createNamedKnowledgeBase("101", "任务私人知识库"); err == nil {
				t.Fatal("invalid avatar was accepted")
			}
		})
	}
}
