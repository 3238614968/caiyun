package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

type cloudV3Fixture struct {
	items                               []CloudReceiveItem
	total                               int
	posts                               []int64
	jwt, device                         string
	rejectID                            int64
	unchanged, malformedCheck, noCredit bool
}

func newCloudV3Fixture(t *testing.T, items []CloudReceiveItem, jwt, device string) (*CaiyunAPI, *cloudV3Fixture) {
	t.Helper()
	t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "")
	f := &cloudV3Fixture{items: items, total: 1000, jwt: jwt, device: device}
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/deviceprofile/") {
			return mailTestResponse(req, 200, `{"code":1}`), nil
		}
		switch req.URL.Path {
		case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling":
			return mailTestResponse(req, 200, `{"code":0}`), nil
		case "/ycloud/signin/page/infoV3":
			if f.malformedCheck && len(f.posts) > 0 {
				return mailTestResponse(req, 200, `{"code":0,"result":{}}`), nil
			}
			pending := 0
			for _, item := range f.items {
				if item.CloudType == 0 {
					pending += item.CloudNum
				}
			}
			body, _ := json.Marshal(map[string]interface{}{"code": 0, "result": map[string]interface{}{
				"total": f.total, "toReceive": pending, "receiveList": append([]CloudReceiveItem{}, f.items...),
			}})
			return mailTestResponse(req, 200, string(body)), nil
		case "/ycloud/signin/page/receiveV3":
			if req.Method != "POST" || req.URL.RawQuery != "" || cloudV3Header(req.Header, "isDeviceId") != "true" || cloudV3Header(req.Header, "appVersion") != "13.2.2.0" || cloudV3Header(req.Header, "jwtToken") != f.jwt {
				t.Errorf("wrong captured receive contract: %s %s", req.Method, req.URL)
			}
			var body struct {
				Client    string
				CloudID   int64
				CloudType int
				DeviceID  string
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Client != "app" || body.CloudType != 0 || body.DeviceID != f.device || body.CloudID <= 0 {
				t.Fatalf("wrong receive payload: %+v", body)
			}
			f.posts = append(f.posts, body.CloudID)
			if body.CloudID == f.rejectID {
				return mailTestResponse(req, 200, `{"code":610,"msg":"拒绝领取"}`), nil
			}
			for i, item := range f.items {
				if item.RewardID() == body.CloudID {
					if !f.unchanged {
						if !f.noCredit {
							f.total += item.CloudNum
						}
						f.items = append(f.items[:i], f.items[i+1:]...)
					}
					return mailTestResponse(req, 200, fmt.Sprintf(`{"code":0,"msg":"success","result":{"receive":%d,"total":%d}}`, item.CloudNum, f.total)), nil
				}
			}
			t.Fatalf("claim submitted an ID not present in receiveList: %d", body.CloudID)
		default:
			t.Fatalf("unexpected legacy/redundant request: %s", req.URL.Path)
		}
		return nil, nil
	})))
	client.SetJWTToken(jwt)
	client.SetSSOToken("fixture-sso")
	client.SetDeviceID(device)
	return NewCaiyunAPI(client), f
}

// Core Client preserves upstream header spellings before net/http transports
// normalize them; the transport fixture must compare names case-insensitively.
func cloudV3Header(headers http.Header, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func TestReceiveV3ReplaysCapturedFifteenRewardsAnd475Clouds(t *testing.T) {
	amounts := []int{5, 6, 30, 5, 60, 30, 100, 5, 6, 5, 6, 6, 6, 5, 200}
	items := []CloudReceiveItem{{CloudType: 2, CloudNum: 96}}
	for i, n := range amounts {
		items = append(items, CloudReceiveItem{RecordID: int64(3000000001 + i), CloudType: 0, CloudNum: n})
	}
	a, f := newCloudV3Fixture(t, items, "fixture-jwt", "Bfixture-device")
	r, err := a.Receive()
	if err != nil || !r.IsSuccess() || len(f.posts) != 15 || f.total != 1475 || len(f.items) != 1 {
		t.Fatalf("captured claims: response=%+v posts=%d total=%d err=%v", r, len(f.posts), f.total, err)
	}
	if payload, _ := resultMap(r); payload["receivedCloud"] != 475 || payload["claimedCount"] != 15 {
		t.Fatalf("wrong actual receipt: %+v", payload)
	}
	r, err = a.Receive()
	if err != nil || len(f.posts) != 15 {
		t.Fatalf("second run duplicated mutations: %v", err)
	}
}

func TestReceiveV3DoesNotAcceptSuccessWhenRewardRemainsOrCheckIsMissing(t *testing.T) {
	for _, mode := range []string{"unchanged", "missing list", "balance unchanged"} {
		t.Run(mode, func(t *testing.T) {
			a, f := newCloudV3Fixture(t, []CloudReceiveItem{{RecordID: 1, CloudNum: 5}}, "jwt", "Bdevice")
			f.unchanged = mode == "unchanged"
			f.malformedCheck = mode == "missing list"
			f.noCredit = mode == "balance unchanged"
			r, err := a.Receive()
			payload, _ := resultMap(r)
			if err == nil || r.IsSuccess() || payload["receivedCloud"] != 0 || len(f.posts) != 1 {
				t.Fatalf("false success: %+v %v", r, err)
			}
		})
	}
}

func TestReceiveV3PreservesPartialReceiptsAndRecoversRemainingReward(t *testing.T) {
	a, f := newCloudV3Fixture(t, []CloudReceiveItem{{RecordID: 1, CloudNum: 5}, {RecordID: 2, CloudNum: 6}}, "jwt", "Bdevice")
	f.rejectID = 2
	r, err := a.Receive()
	p, _ := resultMap(r)
	if err == nil || p["receivedCloud"] != 5 || len(f.posts) != 2 {
		t.Fatalf("partial receipt lost: %+v %v", r, err)
	}
	f.rejectID = 0
	r, err = a.Receive()
	p, _ = resultMap(r)
	if err != nil || p["receivedCloud"] != 6 || len(f.posts) != 3 || f.posts[2] != 2 {
		t.Fatalf("recovery re-claimed successful item: %+v %v", r, err)
	}
}

func TestReceiveV3KeepsAccountCredentialsAndLargeRecordIDsSeparate(t *testing.T) {
	const id int64 = 9007199254740993
	a, fa := newCloudV3Fixture(t, []CloudReceiveItem{{RecordID: id, CloudNum: 5}}, "jwt-a", "Bdevice-a")
	b, fb := newCloudV3Fixture(t, []CloudReceiveItem{{RecordID: id, CloudNum: 6}}, "jwt-b", "Bdevice-b")
	for _, a := range []*CaiyunAPI{a, b} {
		if _, err := a.Receive(); err != nil {
			t.Fatal(err)
		}
	}
	if len(fa.posts) != 1 || len(fb.posts) != 1 || fa.posts[0] != id || fb.posts[0] != id || fa.total != 1005 || fb.total != 1006 {
		t.Fatal("account or record IDs were mixed")
	}
}
