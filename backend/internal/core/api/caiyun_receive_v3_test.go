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
			if req.Method != "POST" || req.URL.RawQuery != "" || cloudV3Header(req.Header, "isDeviceId") != "true" || cloudV3Header(req.Header, "appVersion") != "13.2.5.0" || cloudV3Header(req.Header, "jwtToken") != f.jwt {
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
	if !strings.Contains(err.Error(), "code=610") || !strings.Contains(err.Error(), "云朵记录2") {
		t.Fatalf("failed reward lost upstream diagnostics: %v", err)
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

func TestReceiveV3DoesNotSubmitDefaultDeviceAfterRegistrationFailure(t *testing.T) {
	t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "")
	posts := 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/deviceprofile/") {
			return mailTestResponse(req, 200, `{"code":500,"msg":"registration unavailable"}`), nil
		}
		if req.URL.Path == "/ycloud/signin/page/receiveV3" {
			posts++
		}
		return mailTestResponse(req, 200, `{"code":0}`), nil
	})))
	_, err := NewCaiyunAPI(client).Receive()
	if err == nil || !strings.Contains(err.Error(), "设备认证失败") || posts != 0 {
		t.Fatalf("submitted with unregistered device: posts=%d err=%v", posts, err)
	}
}

// gp 镜像：infoV3 盘点 receiveList 后 POST receiveV3 {client:app,cloudId:"",cloudType:4}。
// 614/615 视为「需 App 领取」正常返回；code 0 记录领取数；gp 票自动按域切换。
func TestReceivePendingCloudRewardsGPProbe(t *testing.T) {
	for _, tc := range []struct {
		name         string
		list         string
		ack          string
		wantPosts    int
		wantApp      bool
		wantReceived int
	}{
		{
			name:      "615 needs app",
			list:      `[{"recordId":11,"cloudType":0,"cloudNum":10},{"recordId":12,"cloudType":0,"cloudNum":20}]`,
			ack:       `{"code":615,"msg":"请前往App领取"}`,
			wantPosts: 1, wantApp: true,
		},
		{
			name:      "614 needs app",
			list:      `[{"recordId":11,"cloudType":0,"cloudNum":10},{"recordId":12,"cloudType":0,"cloudNum":20}]`,
			ack:       `{"code":614,"msg":"需App验证"}`,
			wantPosts: 1, wantApp: true,
		},
		{
			name:         "code 0 claimed",
			list:         `[{"recordId":11,"cloudType":0,"cloudNum":10},{"recordId":12,"cloudType":0,"cloudNum":20}]`,
			ack:          `{"code":0,"msg":"success","result":{"receive":30,"total":1030}}`,
			wantPosts:    1,
			wantReceived: 30,
		},
		{
			name:         "eligible records deduplicated",
			list:         `[{"recordId":11,"cloudType":0,"cloudNum":10},{"recordId":11,"cloudType":0,"cloudNum":10},{"cloudId":12,"cloudType":0,"cloudNum":20},{"recordId":13,"cloudType":1,"cloudNum":99},{"cloudType":0,"cloudNum":99}]`,
			ack:          `{"code":0,"result":{"receive":30,"total":1030}}`,
			wantPosts:    1,
			wantReceived: 30,
		},
		{
			name:      "empty list skips probe",
			list:      `[]`,
			wantPosts: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var infoCalls, posts int
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "gp.mcloud.139.com" {
					t.Fatalf("gp mirror must stay on the gp host: %s", req.URL)
				}
				if cloudV3Header(req.Header, "jwtToken") != "gp-jwt" {
					t.Fatalf("gp mirror did not use the gp ticket: %v", req.Header)
				}
				switch req.URL.Path {
				case "/ycloud/signin/page/infoV3":
					infoCalls++
					if req.Method != "GET" || req.URL.Query().Get("client") != "app" {
						t.Fatalf("wrong gp info contract: %s %s", req.Method, req.URL)
					}
					if posts > 0 && tc.wantReceived > 0 {
						return mailTestResponse(req, 200, `{"code":0,"result":{"total":1030,"toReceive":0,"receiveList":[]}}`), nil
					}
					pending := 0
					if tc.wantPosts > 0 {
						pending = 30
					}
					return mailTestResponse(req, 200, fmt.Sprintf(`{"code":0,"result":{"total":1000,"toReceive":%d,"receiveList":%s}}`, pending, tc.list)), nil
				case "/ycloud/signin/page/receiveV3":
					posts++
					var body map[string]interface{}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if req.Method != "POST" || len(body) != 3 || body["client"] != "app" || body["cloudId"] != "" || body["cloudType"] != float64(4) {
						t.Fatalf("wrong gp receive payload: %s", body)
					}
					return mailTestResponse(req, 200, tc.ack), nil
				default:
					t.Fatalf("unexpected gp request: %s", req.URL)
				}
				return nil, nil
			})))
			client.SetJWTToken("m-jwt")
			client.SetJWTTokenGP("gp-jwt")
			r, err := NewCaiyunAPI(client).ReceivePendingCloudRewardsGP()
			wantInfoCalls := 1
			if tc.wantReceived > 0 {
				wantInfoCalls++
			}
			if err != nil || r == nil || !r.IsSuccess() || infoCalls != wantInfoCalls || posts != tc.wantPosts {
				t.Fatalf("gp probe response=%+v err=%v info=%d posts=%d", r, err, infoCalls, posts)
			}
			payload, _ := resultMap(r)
			if tc.wantApp {
				if !strings.Contains(r.MessageText(), "App") || payload["appRequired"] != true ||
					payload["pendingCloud"] != 30 || payload["pendingCount"] != 2 || payload["receivedCloud"] != 0 {
					t.Fatalf("615 must be attributed to App without error: %+v %s", payload, r.MessageText())
				}
			} else if tc.wantReceived > 0 {
				if payload["receivedCloud"] != tc.wantReceived || payload["claimedCount"] != 2 || payload["total"] != 1030 || payload["pendingCount"] != 0 || payload["pendingCloud"] != 0 || payload["toReceive"] != 0 {
					t.Fatalf("code 0 must record the receipt: %+v", payload)
				}
			} else if posts != 0 && tc.wantPosts == 0 {
				t.Fatalf("empty list must not probe: %+v", payload)
			}
		})
	}
}

func TestReceivePendingCloudRewardsGPRequiresConfirmedReceipt(t *testing.T) {
	const before = `{"code":0,"result":{"total":1000,"toReceive":30,"receiveList":[{"recordId":11,"cloudType":0,"cloudNum":10},{"recordId":12,"cloudType":0,"cloudNum":20}]}}`
	const fullAck = `{"code":0,"result":{"receive":30,"total":1030}}`
	for _, tc := range []struct {
		name          string
		ack           string
		after         string
		wantReceived  int
		wantClaimed   int
		wantPending   int
		wantTotal     int
		wantInfoCalls int
		wantError     string
	}{
		{name: "records unchanged", ack: fullAck, after: before, wantPending: 30, wantTotal: 1000, wantInfoCalls: 2, wantError: "记录"},
		{name: "balance unchanged", ack: fullAck, after: `{"code":0,"result":{"total":1000,"toReceive":0,"receiveList":[]}}`, wantTotal: 1000, wantInfoCalls: 2, wantError: "余额"},
		{name: "missing confirmation list", ack: fullAck, after: `{"code":0,"result":{"total":1030}}`, wantPending: 30, wantTotal: 1000, wantInfoCalls: 2, wantError: "确认"},
		{name: "failed confirmation", ack: fullAck, after: `{"code":500,"msg":"unavailable"}`, wantPending: 30, wantTotal: 1000, wantInfoCalls: 2, wantError: "确认"},
		{name: "failed confirmation with success flag", ack: fullAck, after: `{"code":500,"success":true,"result":{"total":1030,"toReceive":0,"receiveList":[]}}`, wantPending: 30, wantTotal: 1000, wantInfoCalls: 2, wantError: "确认"},
		{name: "missing receipt total", ack: `{"code":0,"result":{"receive":30}}`, wantPending: 30, wantTotal: 1000, wantInfoCalls: 1, wantError: "数量或余额"},
		{name: "invalid receipt amount", ack: `{"code":0,"result":{"receive":"invalid","total":1030}}`, wantPending: 30, wantTotal: 1000, wantInfoCalls: 1, wantError: "数量或余额"},
		{name: "negative receipt amount", ack: `{"code":0,"result":{"receive":-1,"total":1030}}`, wantPending: 30, wantTotal: 1000, wantInfoCalls: 1, wantError: "数量或余额"},
		{name: "negative receipt total", ack: `{"code":0,"result":{"receive":30,"total":-1}}`, wantPending: 30, wantTotal: 1000, wantInfoCalls: 1, wantError: "数量或余额"},
		{name: "zero receipt retains rewards", ack: `{"code":0,"result":{"receive":0,"total":1000}}`, after: before, wantPending: 30, wantTotal: 1000, wantInfoCalls: 2, wantError: "待领取"},
		{name: "partial receipt", ack: `{"code":0,"result":{"receive":10,"total":1010}}`, after: `{"code":0,"result":{"total":1010,"toReceive":20,"receiveList":[{"recordId":12,"cloudType":0,"cloudNum":20}]}}`, wantReceived: 10, wantClaimed: 1, wantPending: 20, wantTotal: 1010, wantInfoCalls: 2, wantError: "待领取"},
		{name: "receipt exceeds removed records", ack: fullAck, after: `{"code":0,"result":{"total":1030,"toReceive":20,"receiveList":[{"recordId":12,"cloudType":0,"cloudNum":20}]}}`, wantPending: 20, wantTotal: 1030, wantInfoCalls: 2, wantError: "记录"},
		{name: "pending aggregate remains", ack: fullAck, after: `{"code":0,"result":{"total":1030,"toReceive":20,"receiveList":[]}}`, wantReceived: 30, wantClaimed: 2, wantTotal: 1030, wantInfoCalls: 2, wantError: "待领取"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			infoCalls, posts := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "gp.mcloud.139.com" || cloudV3Header(req.Header, "jwtToken") != "gp-jwt" {
					t.Fatalf("GP receipt verification changed gateway or credentials: %s", req.URL)
				}
				switch req.URL.Path {
				case "/ycloud/signin/page/infoV3":
					infoCalls++
					if posts == 0 {
						return mailTestResponse(req, 200, before), nil
					}
					return mailTestResponse(req, 200, tc.after), nil
				case "/ycloud/signin/page/receiveV3":
					posts++
					return mailTestResponse(req, 200, tc.ack), nil
				default:
					t.Fatalf("unexpected GP verification request: %s", req.URL)
					return nil, nil
				}
			})))
			client.SetJWTToken("m-jwt")
			client.SetJWTTokenGP("gp-jwt")
			response, err := NewCaiyunAPI(client).ReceivePendingCloudRewardsGP()
			if err == nil || !strings.Contains(err.Error(), tc.wantError) || response == nil || response.IsSuccess() {
				t.Fatalf("unconfirmed receipt reported as success: response=%+v err=%v", response, err)
			}
			payload, _ := resultMap(response)
			if posts != 1 || infoCalls != tc.wantInfoCalls || payload["receivedCloud"] != tc.wantReceived || payload["claimedCount"] != tc.wantClaimed || payload["pendingCloud"] != tc.wantPending || payload["total"] != tc.wantTotal {
				t.Fatalf("posts=%d info=%d payload=%v", posts, infoCalls, payload)
			}
		})
	}
}

func TestReceivePendingCloudRewardsGPRejectsInvalidInitialInfo(t *testing.T) {
	for _, info := range []string{
		`{"code":0,"result":{"total":1000,"toReceive":30}}`,
		`{"code":500,"success":true,"result":{"total":1000,"toReceive":0,"receiveList":[]}}`,
		`{"success":true,"result":{"total":1000,"toReceive":0,"receiveList":[]}}`,
	} {
		t.Run(info, func(t *testing.T) {
			posts := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/ycloud/signin/page/receiveV3" {
					posts++
				}
				return mailTestResponse(req, 200, info), nil
			})))
			if _, err := NewCaiyunAPI(client).ReceivePendingCloudRewardsGP(); err == nil || posts != 0 {
				t.Fatalf("invalid pending info must not report success or submit claims: posts=%d err=%v", posts, err)
			}
		})
	}
}

func TestReceivePendingCloudRewardsGPRejectsInvalidAcknowledgement(t *testing.T) {
	for _, ack := range []string{
		`{"code":500,"success":true,"result":{"receive":10,"total":1010}}`,
		`{"success":true,"result":{"receive":10,"total":1010}}`,
	} {
		t.Run(ack, func(t *testing.T) {
			posts, infoCalls := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/ycloud/signin/page/infoV3":
					infoCalls++
					return mailTestResponse(req, 200, `{"code":0,"result":{"total":1000,"toReceive":10,"receiveList":[{"recordId":11,"cloudType":0,"cloudNum":10}]}}`), nil
				case "/ycloud/signin/page/receiveV3":
					posts++
					return mailTestResponse(req, 200, ack), nil
				default:
					t.Fatalf("unexpected GP acknowledgement request: %s", req.URL)
					return nil, nil
				}
			})))
			if _, err := NewCaiyunAPI(client).ReceivePendingCloudRewardsGP(); err == nil || posts != 1 || infoCalls != 1 {
				t.Fatalf("invalid acknowledgement must not be credited or retried: posts=%d info=%d err=%v", posts, infoCalls, err)
			}
		})
	}
}

func TestReceivePendingCloudRewardsGPRecoversPartialReceiptWithoutDuplicateClaims(t *testing.T) {
	posts := 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/signin/page/infoV3":
			switch posts {
			case 0:
				return mailTestResponse(req, 200, `{"code":0,"result":{"total":1000,"toReceive":30,"receiveList":[{"recordId":11,"cloudType":0,"cloudNum":10},{"recordId":12,"cloudType":0,"cloudNum":20}]}}`), nil
			case 1:
				return mailTestResponse(req, 200, `{"code":0,"result":{"total":1010,"toReceive":20,"receiveList":[{"recordId":12,"cloudType":0,"cloudNum":20}]}}`), nil
			default:
				return mailTestResponse(req, 200, `{"code":0,"result":{"total":1030,"toReceive":0,"receiveList":[]}}`), nil
			}
		case "/ycloud/signin/page/receiveV3":
			posts++
			if posts == 1 {
				return mailTestResponse(req, 200, `{"code":0,"result":{"receive":10,"total":1010}}`), nil
			}
			return mailTestResponse(req, 200, `{"code":0,"result":{"receive":20,"total":1030}}`), nil
		default:
			t.Fatalf("unexpected GP receipt recovery request: %s", req.URL)
			return nil, nil
		}
	})))
	api := NewCaiyunAPI(client)
	first, err := api.ReceivePendingCloudRewardsGP()
	partial, _ := resultMap(first)
	if err == nil || partial["receivedCloud"] != 10 || partial["claimedCount"] != 1 || posts != 1 {
		t.Fatalf("partial receipt not preserved: payload=%v posts=%d err=%v", partial, posts, err)
	}
	second, err := api.ReceivePendingCloudRewardsGP()
	recovered, _ := resultMap(second)
	if err != nil || !second.IsSuccess() || recovered["receivedCloud"] != 20 || recovered["claimedCount"] != 1 || recovered["pendingCount"] != 0 || posts != 2 {
		t.Fatalf("recovery must credit only the remaining reward: payload=%v posts=%d err=%v", recovered, posts, err)
	}
	third, err := api.ReceivePendingCloudRewardsGP()
	empty, _ := resultMap(third)
	if err != nil || !third.IsSuccess() || empty["receivedCloud"] != 0 || empty["claimedCount"] != 0 || posts != 2 {
		t.Fatalf("completed rewards were claimed again: payload=%v posts=%d err=%v", empty, posts, err)
	}
}
