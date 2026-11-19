package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestFamilyCircleCreationUsesOfficialMetadataInvitesNobodyAndVerifiesOwner(t *testing.T) {
	creates := 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "group.yun.139.com" {
			t.Fatal("creation must not order a carrier plan or call another host")
		}
		switch req.URL.Path {
		case "/hcy/group/manage/circle/myGroupList":
			if creates == 0 {
				return mailTestResponse(req, 200, `{"code":"0000","data":{"pageInfo":{"list":[],"isLastPage":true}}}`), nil
			}
			return mailTestResponse(req, 200, `{"code":"0000","data":{"pageInfo":{"list":[{"id":"owned","name":"回归测试家庭圈","type":0,"creatorAccount":"13800000000","memberNum":"1"}],"isLastPage":true}}}`), nil
		case "/hcy/group/manage/type/list":
			return mailTestResponse(req, 200, `{"code":"0000","data":[{"type":0,"name":"家庭圈","topicList":[{"id":1003,"name":"家庭","defaultIconList":[{"iconStaticId":1009}]}]}]}`), nil
		case "/hcy/group/manage/create":
			var body map[string]interface{}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["account"] != "13800000000" || body["name"] != "回归测试家庭圈" || body["type"] != float64(0) || body["topicId"] != float64(1003) || body["iconStaticId"] != float64(1009) || len(body["memberList"].([]interface{})) != 0 || len(body) != 6 {
				t.Fatal("owner-only creation must use the chosen official type/topic/icon without invitations")
			}
			creates++
			return mailTestResponse(req, 200, `{"code":"0000","data":{"groupInfo":{"groupId":"owned"}}}`), nil
		default:
			t.Fatal("unexpected circle mutation")
			return nil, nil
		}
	})))
	client.SetMarketAccount("13800000000")
	cloud := NewCaiyunAPI(client)
	for i := 0; i < 2; i++ {
		group, err := cloud.CreatePersonalFamilyCircle("回归测试家庭圈", 1003, 1009)
		if err != nil || group == nil || group.ID != "owned" || group.MemberCount != "1" {
			t.Fatalf("group=%+v err=%v", group, err)
		}
	}
	if creates != 1 {
		t.Fatal("retry created another circle")
	}
}

func TestFamilyCircleCreationRejectsUnverifiedReceiptAndForeignOrMultiMemberReadback(t *testing.T) {
	for _, scenario := range []string{"missing receipt", "wrong owner", "multi member", "wrong type", "wrong icon"} {
		t.Run(scenario, func(t *testing.T) {
			creates := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(req.URL.Path, "/myGroupList"):
					if creates == 0 {
						return mailTestResponse(req, 200, `{"code":"0000","data":{"pageInfo":{"list":[],"isLastPage":true}}}`), nil
					}
					if scenario == "missing receipt" {
						return mailTestResponse(req, 200, `{"code":"0000","data":{"pageInfo":{"list":[],"isLastPage":true}}}`), nil
					}
					owner, count, kind := "13800000000", "1", "0"
					if scenario == "wrong owner" {
						owner = "foreign"
					}
					if scenario == "multi member" {
						count = "2"
					}
					if scenario == "wrong type" {
						kind = "1"
					}
					return mailTestResponse(req, 200, fmt.Sprintf(`{"code":"0000","data":{"pageInfo":{"list":[{"id":"g","name":"回归测试家庭圈","type":%q,"creatorAccount":%q,"memberNum":%q}],"isLastPage":true}}}`, kind, owner, count)), nil
				case strings.HasSuffix(req.URL.Path, "/type/list"):
					return mailTestResponse(req, 200, `{"code":"0000","data":[{"type":0,"topicList":[{"id":1003,"defaultIconList":[{"iconStaticId":1009}]}]}]}`), nil
				case strings.HasSuffix(req.URL.Path, "/manage/create"):
					if scenario == "wrong icon" {
						t.Fatal("must not create with an invented icon")
					}
					creates++
					if scenario == "missing receipt" {
						return mailTestResponse(req, 200, `{"code":"0000","data":{}}`), nil
					}
					return mailTestResponse(req, 200, `{"code":"0000","data":{"groupInfo":{"groupId":"g"}}}`), nil
				default:
					t.Fatal("unexpected family request")
					return nil, nil
				}
			})))
			client.SetMarketAccount("13800000000")
			icon := int64(1009)
			if scenario == "wrong icon" {
				icon++
			}
			if _, err := NewCaiyunAPI(client).CreatePersonalFamilyCircle("回归测试家庭圈", 1003, icon); err == nil {
				t.Fatal("unverified circle accepted")
			}
		})
	}
}

func TestFamilyCircleNumericIDsAndTypesDoNotLosePrecision(t *testing.T) {
	var group FamilyCircleGroup
	if err := json.Unmarshal([]byte(`{"id":9007199254740993,"type":0,"memberNum":1}`), &group); err != nil || group.ID != "9007199254740993" || group.Type != "0" || group.MemberCount != "1" {
		t.Fatalf("group=%+v err=%v", group, err)
	}
	for _, body := range []string{`{"id":{}}`, `{"type":false}`, `{"memberNum":1.5}`} {
		if json.Unmarshal([]byte(body), &group) == nil {
			t.Fatal("invalid scalar accepted")
		}
	}
}
