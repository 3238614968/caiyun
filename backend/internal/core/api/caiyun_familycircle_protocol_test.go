package api

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestFamilyCircleUsesAPKReleaseHostAndDistinctMethods(t *testing.T) {
	calls := 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.Host != "caiyun.feixin.10086.cn" {
			t.Fatalf("native release host changed: %s", req.URL.Host)
		}
		switch req.URL.Path {
		case "/ycloud/south/national/familyCircle/queryBackupState":
			if req.Method != "GET" || req.URL.Query().Get("userId") != "circle-user" || req.URL.Query().Get("gruopId") != "group & one" || req.Body != nil {
				t.Fatalf("invalid backup request: %s %s", req.Method, req.URL)
			}
		case "/ycloud/south/national/familyCircle/queryCircleTaskState":
			var body map[string]string
			_ = json.NewDecoder(req.Body).Decode(&body)
			if req.Method != "POST" || body["gruopId"] != "group & one" || body["userId"] != "circle-user" {
				t.Fatalf("invalid task-state request: %s %#v", req.Method, body)
			}
		default:
			t.Fatalf("unexpected native request: %s", req.URL)
		}
		return mailTestResponse(req, 200, `{"code":0,"result":1}`), nil
	})))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"circle-user"}}`))
	client.SetJWTToken("header." + claims + ".signature")
	client.SetDeviceID("Bfixture-device")
	api := NewCaiyunAPI(client)
	if _, err := api.FamilyCircleBackupState(""); err == nil {
		t.Fatal("empty group accepted")
	}
	if _, err := api.FamilyCircleTaskState(""); err == nil || calls != 0 {
		t.Fatal("empty group sent to native task service")
	}
	if _, err := api.FamilyCircleBackupState("group & one"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.FamilyCircleTaskState("group & one"); err != nil || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestFamilyCircleDiscoversOnlyAccountGroupsWithPagination(t *testing.T) {
	reads := 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		reads++
		var body struct {
			Account              string
			PageNumber, PageSize int
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Account != "13800000001" || body.PageNumber != reads || body.PageSize != 100 {
			t.Fatalf("incorrect account/page: %+v %v", body, err)
		}
		if req.URL.Host != "group.yun.139.com" || req.URL.Path != "/hcy/group/manage/circle/myGroupList" || req.Method != "POST" {
			t.Fatalf("incorrect discovery endpoint: %s %s", req.Method, req.URL)
		}
		if reads == 1 {
			return mailTestResponse(req, 200, `{"code":"0000","success":true,"data":{"pageInfo":{"list":[{"id":"owned-one","type":"1"}],"pages":2}}}`), nil
		}
		return mailTestResponse(req, 200, `{"code":"0000","success":true,"data":{"pageInfo":{"list":[{"id":"owned-one","type":"1"},{"id":"owned-two","type":"1"}],"isLastPage":true,"pages":2}}}`), nil
	})))
	client.SetMarketAccount("13800000001")
	groups, err := NewCaiyunAPI(client).FamilyCircleGroups()
	if err != nil || len(groups) != 2 || reads != 2 || groups[0].ID != "owned-one" || groups[1].ID != "owned-two" {
		t.Fatalf("groups=%+v reads=%d err=%v", groups, reads, err)
	}
}

func TestFamilyCircleMissingPayloadIsNotNoGroups(t *testing.T) {
	for _, body := range []string{`{"code":"0000"}`, `{"code":"0000","data":{"pageInfo":{}}}`, `{"code":"0000","success":false,"data":{"pageInfo":{"list":[]}}}`} {
		client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return mailTestResponse(req, 200, body), nil
		})))
		client.SetMarketAccount("13800000001")
		if _, err := NewCaiyunAPI(client).FamilyCircleGroups(); err == nil {
			t.Fatalf("invalid response accepted as an account without groups: %s", body)
		}
	}
}
