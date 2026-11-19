package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	corehttp "caiyun/internal/core/http"

	"github.com/google/uuid"
)

// Offline vector from the App's bundled forge.min.js, encrypt(phone,"NONE").
// Runtime and regression tests require neither JavaScript nor a browser.
const knowledgeFixtureIdentity = "kQk/DBOIcTenwp0JtzQkkQsYVvb4UJQBfdR2VLwL2NUggZ7SxFKRpeoxbFoVOF8eDcuBTDGmjphknUgaDcnzICVcknGMXZ3lVMekRWYc36wxEC4pSufP61ZNbTv+bEhj677iC1elS8DA4i4irry8e50jwrdEAy4k6HUeUSNGwhw="

func TestKnowledgeEventIdentityMatchesOfficialForge(t *testing.T) {
	got, err := nativeEventIdentity("13800000000")
	if err != nil || got != knowledgeFixtureIdentity {
		t.Fatalf("official fixture mismatch: %v", err)
	}
	for _, phone := range []string{"", "1380000000", "23800000000", "1380000000a", "13800000000 ", "１３８００００００００"} {
		if _, err := nativeEventIdentity(phone); err == nil {
			t.Fatal("invalid identity was accepted")
		}
	}
}

func TestKnowledgeCreationEventMatchesSDKAndKeepsCloudCredentialsOut(t *testing.T) {
	var paths []string
	var client *corehttp.Client
	client = knowledgeEventFixtureClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch req.URL.Hostname() {
		case "dc.cmicapm.com":
			if req.URL.Host != "dc.cmicapm.com:7080" || req.URL.Scheme != "https" || req.Method != http.MethodPost {
				t.Fatal("changed official attribution destination")
			}
			for _, header := range []string{"Authorization", "jwtToken", "Cookie", "x-DeviceInfo", "x-yun-client-info", "x-yun-uni"} {
				if req.Header.Get(header) != "" {
					t.Fatalf("cloud credential or device header inherited: %s", header)
				}
			}
			for _, secret := range []string{"13800000000", "fixture-basic", "fixture-cookie", client.GetJWTToken()} {
				if strings.Contains(string(body), secret) {
					t.Fatal("raw identity or login credential leaked to attribution")
				}
			}
			if req.URL.Path == "/webid" {
				var request map[string]interface{}
				_ = json.Unmarshal(body, &request)
				if request["app_id"] != float64(10000005) || request["url"] != knowledgeCreatePage || request["user_unique_id"] != knowledgeFixtureIdentity {
					t.Fatal("web registration differs from the official SDK")
				}
				return mailTestResponse(req, 200, `{"e":0,"web_id":"fixture-web"}`), nil
			}
			if req.URL.Path != "/list" {
				t.Fatal("unexpected attribution path")
			}
			var batches []struct {
				Events []struct {
					Event     string `json:"event"`
					Params    string `json:"params"`
					Time      int64  `json:"local_time_ms"`
					SessionID string `json:"session_id"`
				} `json:"events"`
				User   map[string]string      `json:"user"`
				Header map[string]interface{} `json:"header"`
			}
			if json.Unmarshal(body, &batches) != nil || len(batches) != 1 || len(batches[0].Events) != 1 {
				t.Fatal("invalid event batch")
			}
			batch := batches[0]
			event := batch.Events[0]
			if batch.User["user_unique_id"] != knowledgeFixtureIdentity || batch.User["web_id"] != "fixture-web" || batch.User["user_unique_id_type"] != "rsa_phone_id" || event.Event != "aidialogue_click" {
				t.Fatal("wrong event attribution")
			}
			if _, err := uuid.Parse(event.SessionID); err != nil || time.Since(time.UnixMilli(event.Time)) > 5*time.Second {
				t.Fatal("invalid event time or session")
			}
			var params map[string]interface{}
			_ = json.Unmarshal([]byte(event.Params), &params)
			if params["user_domain_id"] != "fixture-user" || params["page_title"] != "创建知识库" || params["scenario"] != "知识库" || params["click_object"] != "立即创建" || params["$user_unique_id_type"] != "rsa_phone_id" || params["page_path"] != knowledgeCreatePage {
				t.Fatal("wrong knowledge creation event")
			}
			var custom map[string]string
			_ = json.Unmarshal([]byte(batch.Header["custom"].(string)), &custom)
			if custom["source_id_h5"] != "810292" || custom["source_channel_h5"] != "101" || custom["app_platform_h5"] != "App" || custom["app_version_h5"] != MarketClientVersion || batch.Header["sdk_version"] != "5.1.10.feature_tob" {
				t.Fatal("lost official source context")
			}
			return mailTestResponse(req, 200, `{"e":0}`), nil
		case "ai.yun.139.com":
			var request map[string]interface{}
			_ = json.Unmarshal(body, &request)
			if request["sourceChannel"] != "101" || request["userId"] != "fixture-user" {
				t.Fatal("wrong native knowledge identity")
			}
			switch {
			case strings.HasSuffix(req.URL.Path, "/base/list"):
				return mailTestResponse(req, 200, `{"code":"0000","data":{"baseList":[]}}`), nil
			case strings.HasSuffix(req.URL.Path, "/profilePhoto/list"):
				return mailTestResponse(req, 200, `{"code":"0000","data":[{"photoId":"official-default","type":1}]}`), nil
			case strings.HasSuffix(req.URL.Path, "/base/create"):
				if len(paths) != 5 || paths[2] != "/webid" || paths[3] != "/list" || request["openLevel"] != float64(0) || request["profilePhoto"] == nil {
					t.Fatal("creation outside the verified private flow")
				}
				return mailTestResponse(req, 200, `{"code":"0000","data":{"base":{"baseId":"created"}}}`), nil
			}
		}
		t.Fatal("unexpected knowledge request")
		return nil, nil
	})))
	client.SetCookie("jwtToken", "fixture-cookie", "dc.cmicapm.com")
	if err := NewCaiyunAPI(client).CreateFunAIKnowledgeBase(); err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeEventFailureStopsCreationAndIsNotRetried(t *testing.T) {
	for _, scenario := range []struct {
		path   string
		status int
		body   string
	}{
		{"/webid", 200, `{"e":1,"web_id":"w"}`},
		{"/webid", 200, `{"e":0}`},
		{"/webid", 200, `{"web_id":"w"}`},
		{"/webid", 200, `<html>fixture-sensitive-response</html>`},
		{"/list", 503, `{"e":0}`},
		{"/list", 200, `{"e":9,"message":"fixture-sensitive-response"}`},
		{"/list", 200, `{}`},
		{"/list", 200, `{"e":null}`},
		{"/list", 200, `{"e":"0"}`},
		{"/list", 307, `{"e":0}`},
	} {
		t.Run(fmt.Sprintf("%s/%d/%s", scenario.path, scenario.status, scenario.body), func(t *testing.T) {
			failures := 0
			client := knowledgeEventFixtureClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Hostname() == "dc.cmicapm.com" {
					if req.URL.Path == scenario.path {
						failures++
						response := mailTestResponse(req, scenario.status, scenario.body)
						response.Header.Set("Location", "https://unconfigured.example/list")
						return response, nil
					}
					if req.URL.Path != "/webid" {
						t.Fatal("submission continued after invalid web registration")
					}
					return mailTestResponse(req, 200, `{"e":0,"web_id":"w"}`), nil
				}
				if strings.HasSuffix(req.URL.Path, "/base/list") {
					return mailTestResponse(req, 200, `{"code":"0000","data":{"baseList":[]}}`), nil
				}
				if strings.HasSuffix(req.URL.Path, "/profilePhoto/list") {
					return mailTestResponse(req, 200, `{"code":"0000","data":[{"photoId":"official-default"}]}`), nil
				}
				t.Fatal("invalid event receipt must not redirect or create a library")
				return nil, nil
			})))
			err := NewCaiyunAPI(client).CreateFunAIKnowledgeBase()
			if err == nil || failures != 1 || strings.Contains(err.Error(), "fixture-sensitive-response") {
				t.Fatalf("failures=%d err=%v", failures, err)
			}
		})
	}
}

func TestKnowledgeCreationEventIsNotReplayedOnSameDayReuse(t *testing.T) {
	client := knowledgeEventFixtureClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(req.URL.Path, "/base/list") {
			t.Fatalf("reuse must not emit another creation event or create another library: %s", req.URL.Path)
		}
		name := taskKnowledgeBaseName(time.Now(), "101") + "-App"
		return mailTestResponse(req, 200, fmt.Sprintf(`{"code":"0000","data":{"baseList":[{"baseId":"today","name":%q,"baseType":1,"openLevel":0,"createBy":"fixture-user"}]}}`, name)), nil
	})))
	if err := NewCaiyunAPI(client).CreateFunAIKnowledgeBase(); err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeNewModuleEntryCreatesNewActionWithoutReplayingOldCreation(t *testing.T) {
	bases := map[string]map[string]interface{}{}
	creates, events := 0, 0
	client := knowledgeEventFixtureClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Hostname() == "dc.cmicapm.com" {
			events++
			return mailTestResponse(req, 200, `{"e":0,"web_id":"w"}`), nil
		}
		if strings.HasSuffix(req.URL.Path, "/base/list") {
			list := []map[string]interface{}{}
			for _, base := range bases {
				list = append(list, base)
			}
			body, _ := json.Marshal(map[string]interface{}{"code": "0000", "data": map[string]interface{}{"baseList": list}})
			return mailTestResponse(req, 200, string(body)), nil
		}
		if strings.HasSuffix(req.URL.Path, "/profilePhoto/list") {
			return mailTestResponse(req, 200, `{"code":"0000","data":[{"photoId":"default"}]}`), nil
		}
		if !strings.HasSuffix(req.URL.Path, "/base/create") {
			t.Fatal("unexpected scoped creation request")
		}
		var body map[string]interface{}
		_ = json.NewDecoder(req.Body).Decode(&body)
		name := body["name"].(string)
		if utf8.RuneCountInString(name) > 25 || bases[name] != nil {
			t.Fatal("scoped library name is invalid or duplicated")
		}
		creates++
		bases[name] = map[string]interface{}{"baseId": fmt.Sprint(creates), "name": name, "baseType": 1, "openLevel": 0, "createBy": "fixture-user"}
		return mailTestResponse(req, 200, fmt.Sprintf(`{"code":"0000","data":{"base":{"baseId":%q}}}`, fmt.Sprint(creates))), nil
	})))
	api := NewCaiyunAPI(client)
	for _, entries := range [][]string{{"main", "timed"}, {"timed", "main", " main "}, {"timed"}, {"timed"}} {
		if err := api.CreateFunAIKnowledgeBase(entries...); err != nil {
			t.Fatal(err)
		}
	}
	if creates != 2 || events != 4 {
		t.Fatalf("creates=%d events=%d", creates, events)
	}
}

func TestKnowledgeNativeReuseRequiresConfirmedCurrentOwner(t *testing.T) {
	for _, owner := range []string{"", "foreign-owner"} {
		t.Run(owner, func(t *testing.T) {
			client := knowledgeEventFixtureClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if !strings.HasSuffix(req.URL.Path, "/base/list") {
					t.Fatal("unconfirmed owner must not report or modify the library")
				}
				name := taskKnowledgeBaseName(time.Now(), "101") + "-App"
				return mailTestResponse(req, 200, fmt.Sprintf(`{"code":"0000","data":{"baseList":[{"baseId":"not-owned","name":%q,"baseType":1,"openLevel":0,"createBy":%q}]}}`, name, owner)), nil
			})))
			if err := NewCaiyunAPI(client).CreateFunAIKnowledgeBase(); err == nil {
				t.Fatal("unconfirmed ownership was accepted")
			}
		})
	}
}

func TestKnowledgeEventReceiptDoesNotMaskCreationFailure(t *testing.T) {
	for _, reply := range []string{`{"code":"0000"}`, `{"code":"0000","success":false,"data":{"base":{"baseId":"bad"}}}`, `{"code":"1001"}`} {
		t.Run(reply, func(t *testing.T) {
			client := knowledgeEventFixtureClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Hostname() == "dc.cmicapm.com" {
					return mailTestResponse(req, 200, `{"e":0,"web_id":"w"}`), nil
				}
				if strings.HasSuffix(req.URL.Path, "/base/list") {
					return mailTestResponse(req, 200, `{"code":"0000","data":{"baseList":[]}}`), nil
				}
				if strings.HasSuffix(req.URL.Path, "/profilePhoto/list") {
					return mailTestResponse(req, 200, `{"code":"0000","data":[{"photoId":"official-default"}]}`), nil
				}
				if !strings.HasSuffix(req.URL.Path, "/base/create") {
					t.Fatal("unexpected create request")
				}
				return mailTestResponse(req, 200, reply), nil
			})))
			if err := NewCaiyunAPI(client).CreateFunAIKnowledgeBase(); err == nil {
				t.Fatal("event acknowledgement masked a failed creation")
			}
		})
	}
}

func knowledgeEventFixtureClient(options ...corehttp.ClientOption) *corehttp.Client {
	client := corehttp.NewClient(options...)
	client.SetMarketAccount("13800000000")
	client.SetAuth("fixture-basic")
	client.SetJWTToken("h." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture-user"}}`)) + ".s")
	return client
}
