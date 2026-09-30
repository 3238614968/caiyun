package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestCompletionActivityURLsHaveSingleYcloudPrefix(t *testing.T) {
	tests := []struct {
		path string
		call func(*CaiyunAPI) error
	}{
		{"/ycloud/prizeApi/checkPrize/getUserPrizeLogPageV2", func(a *CaiyunAPI) error { _, err := a.PrizeCenterLogPage(1, 100); return err }},
		{"/ycloud/meitu/new/authStatus", func(a *CaiyunAPI) error { _, err := a.MeituAuthStatus(); return err }},
		{"/ycloud/mcloudday/common/activityInfo", func(a *CaiyunAPI) error { _, err := a.MCloudDayActivityInfo(); return err }},
		{"/ycloud/redInvite/page/risk", func(a *CaiyunAPI) error { _, err := a.RedInviteRisk(); return err }},
		{"/ycloud/south/national/familyCircle/queryBackupState", func(a *CaiyunAPI) error { _, err := a.FamilyCircleBackupState("group-1"); return err }},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != tc.path {
					t.Fatalf("path=%s want=%s", req.URL.Path, tc.path)
				}
				return mailTestResponse(req, 200, `{"code":0,"result":{"result":[],"totalPages":1}}`), nil
			})))
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"test-user"}}`))
			client.SetJWTToken("header." + claims + ".signature")
			if err := tc.call(NewCaiyunAPI(client)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompletionResponsesUseActualUpstreamEnvelopes(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/redInvite/page/risk":
			return mailTestResponse(req, 200, `{"returnCode":"0","returnMsg":"成功","success":false,"body":999}`), nil
		case "/openapi/albumAutoBackup/getStatus":
			return mailTestResponse(req, 200, `{"resultCode":200,"resultData":{"exist":false}}`), nil
		case "/openapi/albumAutoBackup/synStatus":
			return mailTestResponse(req, 200, `{"resultCode":407,"resultMsg":"号码不在库中"}`), nil
		case "/api/outer/assistant/accredit/get":
			return mailTestResponse(req, 200, `{"code":"0000","data":{"path":"/folder-1"}}`), nil
		case "/ycloud/prizeApi/checkPrize/getUserPrizeLogPageV2":
			return mailTestResponse(req, 200, `{"code":0,"result":{"result":[{"oId":1234567890123456789,"flag":1}],"totalPages":1}}`), nil
		default:
			t.Fatalf("unexpected URL: %s", req.URL)
		}
		return nil, nil
	})))
	a := NewCaiyunAPI(client)
	risk, err := a.RedInviteRisk()
	if err != nil || RedInviteRiskPassed(risk) {
		t.Fatalf("risk passed=%v err=%v", RedInviteRiskPassed(risk), err)
	}
	status, err := a.AlbumAutoBackupStatus()
	if err != nil || fmt.Sprint(status.Code) != "0" {
		t.Fatalf("platform status=%+v err=%v", status, err)
	}
	report, err := a.ReportAlbumAutoBackupStatus("13800000001", 0)
	if err != nil || fmt.Sprint(report.Code) != "407" {
		t.Fatalf("platform report=%+v err=%v", report, err)
	}
	auth, err := a.AIStoreAccredit(7)
	if err != nil || AIStoreAccreditPath(auth) != "/folder-1" {
		t.Fatalf("AI authorization=%+v err=%v", auth, err)
	}
	page, err := a.PrizeCenterLogPage(1, 100)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].OID != "1234567890123456789" {
		t.Fatalf("prize OID precision lost: %+v err=%v", page, err)
	}
}

func TestFamilyCircleBaseOverrideIsPerClient(t *testing.T) {
	for _, host := range []string{"family.example", "m.mcloud.139.com"} {
		client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host != host {
				t.Fatalf("family host=%s want=%s", req.URL.Host, host)
			}
			return mailTestResponse(req, 200, `{"code":0}`), nil
		})))
		claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"test-user"}}`))
		client.SetJWTToken("header." + claims + ".signature")
		a := NewCaiyunAPI(client)
		if host == "family.example" {
			a.SetFamilyCircleBase("https://family.example/api")
		}
		if _, err := a.FamilyCircleBackupState(""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPrizePaginationWithoutTotalPagesStillReadsFullList(t *testing.T) {
	reads := 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		reads++
		count := 100
		if reads == 2 {
			count = 82
		}
		if reads > 2 {
			t.Fatal("pagination did not stop at short page")
		}
		items := make([]map[string]interface{}, count)
		for i := range items {
			items[i] = map[string]interface{}{"oId": fmt.Sprintf("%d-%d", reads, i), "flag": 1}
		}
		body, _ := json.Marshal(map[string]interface{}{"code": 0, "result": map[string]interface{}{"result": items}})
		return mailTestResponse(req, 200, string(body)), nil
	})))
	entries, err := NewCaiyunAPI(client).PrizeCenterAllEntries()
	if err != nil || len(entries) != 182 || reads != 2 {
		t.Fatalf("entries=%d reads=%d err=%v", len(entries), reads, err)
	}
}

func TestKnowledgeTaskReusesExistingTaskLibrary(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			created := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/api/outer/assistant/knowledge/personal/v2/base/list":
					if existing {
						return mailTestResponse(req, 200, `{"code":"0000","data":{"baseList":[{"name":"云盘任务知识库-20260929-120000"}]}}`), nil
					}
					return mailTestResponse(req, 200, `{"code":"0000","data":{"baseList":[]}}`), nil
				case "/api/outer/assistant/knowledge/personal/v2/base/create":
					created++
					return mailTestResponse(req, 200, `{"code":"0000"}`), nil
				default:
					t.Fatalf("unexpected knowledge URL: %s", req.URL)
				}
				return nil, nil
			})))
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"test-user"}}`))
			client.SetJWTToken("h." + claims + ".s")
			if err := NewCaiyunAPI(client).CreateTaskKnowledgeBase(); err != nil {
				t.Fatal(err)
			}
			if (existing && created != 0) || (!existing && created != 1) {
				t.Fatalf("existing=%v created=%d", existing, created)
			}
		})
	}
}

func TestSpaceGiftInfoHandlesBareAndWrappedStatus(t *testing.T) {
	for _, body := range []string{`{"status":0}`, `{"code":0,"result":{"status":0}}`} {
		client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/ycloud/prizeApi/checkPrize/getSpaceGiftInfo" {
				t.Fatalf("space gift URL=%s", req.URL)
			}
			return mailTestResponse(req, 200, body), nil
		})))
		resp, err := NewCaiyunAPI(client).PrizeCenterSpaceGiftInfo()
		if err != nil || !resp.IsSuccess() {
			t.Fatalf("space gift response=%+v err=%v", resp, err)
		}
		result, ok := resultMap(resp)
		if !ok || mapInt(result, "status") != 0 {
			t.Fatalf("space gift result=%v", resp.Result)
		}
	}
}
