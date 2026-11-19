package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func campaignCoc3Task(t *testing.T, reply coc3RoundTrip) *Coc3Task {
	t.Helper()
	task := NewCoc3Task(corehttp.NewClient(corehttp.WithTransport(reply)), logger.NewLogger(logger.LevelError)).SetAccountContext("13800000001", "Basic fixture")
	task.tokenValue = "token"
	task.wait = func(time.Duration) {}
	return task
}

func TestCoc3CanvasRequiresBothAcceptedCodeAndCountConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, response     string
		after              int
		confirmed, failure bool
	}{
		{"win", `{"code":60000,"data":{"prizeName":"流量券"}}`, 0, true, false},
		{"no win", `{"code":"60010","data":{}}`, 0, true, false},
		{"ack only", `{"code":60000}`, 1, false, false},
		{"wrong endpoint code", `{"code":0,"data":{"prizeName":"fake"}}`, 1, false, true},
		{"risk", `{"code":-1,"data":{"risk":"true","explain":"SCORE_REFUSE"}}`, 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CAIYUN_COC3_WASM_PARAMS", "fixture")
			draws, counts, riskLookups := 0, 0, 0
			task := campaignCoc3Task(t, func(req *http.Request) (*http.Response, error) {
				var result string
				switch {
				case strings.HasSuffix(req.URL.Path, "/dfpRisk/getDfpOuterCode"):
					result = fmt.Sprintf(`{"dfp":%q,"exp":%d}`, strings.Repeat("fixture", 12), time.Now().Add(time.Hour).UnixMilli())
				case req.URL.Host == "yxfk.market.chinamobile.com:8081":
					riskLookups++
					result = `{"status":2,"data":"fixture-risk-token"}`
				case strings.HasSuffix(req.URL.Path, "/checkQualificByActivityId"):
					result = `{"resultCode":-403,"msg":"活动不在进行中"}`
					if req.URL.Query().Get("activityId") == "17456" {
						result = `{"resultCode":0,"data":{}}`
					}
				case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
					counts++
					n := 1
					if counts > 1 {
						n = tc.after
					}
					result = fmt.Sprintf(`{"code":200,"data":%d}`, n)
				case strings.HasSuffix(req.URL.Path, "/lottery/lottery"):
					if req.Header.Get("gr-risk-p3") != "fixture-risk-token" {
						t.Fatal("official SDK risk token missing from draw")
					}
					var body map[string]interface{}
					_ = json.NewDecoder(req.Body).Decode(&body)
					if body["operDfp"] != strings.Repeat("fixture", 12) {
						t.Fatal("confirmed BSFIT receipt missing from lottery body")
					}
					draws++
					result = tc.response
				default:
					t.Fatalf("unexpected %s", req.URL)
				}
				return coc3Reply(req, result), nil
			})
			message := strings.Join(task.runCampaignLottery(), "；")
			if draws != 1 || riskLookups != 1 || strings.Contains(message, "画布抽奖确认1次") != tc.confirmed || (len(task.issues) > 0) != tc.failure {
				t.Fatalf("draws=%d issues=%v message=%s", draws, task.issues, message)
			}
			if tc.name == "risk" {
				if !task.lotteryRiskStopped || counts != 1 {
					t.Fatalf("risk did not stop run: %s", message)
				}
				t.Setenv("CAIYUN_COC3_BEAN_DRAW_13800000001", "true")
				if next := strings.Join(task.runBeanLottery(), "；"); !strings.Contains(next, "本轮已触发风控") {
					t.Fatal(next)
				}
			}
			if !tc.confirmed && !tc.failure && !task.Pending() {
				t.Fatal("unconfirmed submission must stay pending")
			}
		})
	}
}

func TestCoc3EmptyFingerprintReceiptStopsBeforeLotteryAndIsNotRetried(t *testing.T) {
	t.Setenv("CAIYUN_COC3_WASM_PARAMS", "fixture")
	queries := 0
	task := campaignCoc3Task(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "yxfk.market.chinamobile.com:8081":
			return coc3Reply(req, `{"status":2,"data":"fixture-risk-token"}`), nil
		case strings.HasSuffix(req.URL.Path, "/dfpRisk/getDfpOuterCode"):
			queries++
			return coc3Reply(req, ""), nil
		case strings.HasSuffix(req.URL.Path, "/checkQualificByActivityId"):
			return coc3Reply(req, `{"resultCode":0,"data":{}}`), nil
		case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			return coc3Reply(req, `{"code":200,"data":1}`), nil
		default:
			t.Fatalf("unconfirmed BSFIT submitted another write: %s", req.URL.Path)
			return nil, nil
		}
	})
	for i := 0; i < 2; i++ {
		message := strings.Join(task.runCampaignLottery(), "；")
		if !task.Pending() || !strings.Contains(message, "BSFIT 记录未确认") || strings.Contains(message, "画布抽奖确认1次") {
			t.Fatal(message)
		}
	}
	if queries != 1 {
		t.Fatalf("fingerprint failure replayed %d times", queries)
	}
}

func TestCoc3CanvasRunsAllSDKStagesInGoAndRequiresCounterConfirmation(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CAIYUN_COC3_WASM_PARAMS", "")
	t.Setenv("CAIYUN_COC3_WASM_DIR", "")
	t.Setenv("CAIYUN_COC3_RISK_TOKEN_13800000001", "")
	t.Setenv("CAIYUN_COC3_DEVICE_ID_13800000001", "")
	risk, fp, device, draws, counts := 0, 0, 0, 0, 0
	fingerprint := strings.Repeat("fixture", 12)
	task := campaignCoc3Task(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/checkQualificByActivityId"):
			if req.URL.Query().Get("activityId") == "17766" {
				return coc3Reply(req, `{"resultCode":0,"data":{}}`), nil
			}
			return coc3Reply(req, `{"resultCode":-403,"msg":"活动不在进行中"}`), nil
		case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			counts++
			remaining := 1
			if draws > 0 {
				remaining = 0
			}
			return coc3Reply(req, fmt.Sprintf(`{"code":200,"data":%d}`, remaining)), nil
		case req.URL.Host == "yxfk.market.chinamobile.com:8081":
			risk++
			if risk == 1 {
				return coc3Reply(req, `{"status":-4,"data":"fixture-issued-id"}`), nil
			}
			return coc3Reply(req, `{"status":2,"data":"fixture-risk-token"}`), nil
		case strings.HasSuffix(req.URL.Path, "/dfpRisk/getDfpOuterCode"):
			if risk != 2 {
				t.Fatal("missing ConstID challenge exchange")
			}
			fp++
			return coc3Reply(req, fmt.Sprintf(`{"dfp":%q,"exp":%d}`, fingerprint, time.Now().Add(time.Hour).UnixMilli())), nil
		case strings.HasSuffix(req.URL.Path, "/wasm/gettoken"):
			if fp != 1 {
				t.Fatal("WASM preceded confirmed fingerprint")
			}
			device++
			return coc3Reply(req, `{"code":"0","success":true,"data":"fixture-device-token"}`), nil
		case strings.HasSuffix(req.URL.Path, "/lottery/lottery"):
			var pair []string
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if device != 1 || req.Header.Get("gr-risk-p3") != "fixture-risk-token" || body["operDfp"] != fingerprint || json.Unmarshal([]byte(req.Header.Get("wasmParams")), &pair) != nil || len(pair) != 2 || pair[0] == "" || pair[1] == "" {
				t.Fatal("draw missing a confirmed native SDK stage")
			}
			draws++
			return coc3Reply(req, `{"code":60010,"data":{}}`), nil
		default:
			t.Fatalf("unexpected native SDK request: %s", req.URL.Path)
			return nil, nil
		}
	})
	message := strings.Join(task.runCampaignLottery(), "；")
	if risk != 2 || fp != 1 || device != 1 || draws != 1 || counts != 2 || len(task.issues) != 0 || !strings.Contains(message, "画布抽奖确认1次，次数1→0") {
		t.Fatalf("risk=%d fp=%d device=%d draws=%d counts=%d message=%s", risk, fp, device, draws, counts, message)
	}
}

func TestCoc3DoesNotReplayOrderThroughCanvasEndpoint(t *testing.T) {
	task := campaignCoc3Task(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("activityId") == "17456" || req.Method != "GET" {
			t.Fatalf("ambiguous order was replayed: %s", req.URL)
		}
		return coc3Reply(req, `{"resultCode":-403,"msg":"活动已结束"}`), nil
	})
	task.orderAttempted = map[int]bool{17456: true}
	task.runCampaignLottery()
}

func TestCoc3RequiredBrowserDetectionStopsBeforeAnyDraw(t *testing.T) {
	t.Setenv("CAIYUN_COC3_WASM_PARAMS", "fixture")
	riskQueries, draws := 0, 0
	task := campaignCoc3Task(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "yxfk.market.chinamobile.com:8081":
			riskQueries++
			return coc3Reply(req, `{"status":-4,"data":"fixture-challenge"}`), nil
		case strings.HasSuffix(req.URL.Path, "/checkQualificByActivityId"):
			return coc3Reply(req, `{"resultCode":0,"data":{}}`), nil
		case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			return coc3Reply(req, `{"code":200,"data":1}`), nil
		default:
			draws++
			t.Fatalf("unconfirmed risk token must stop device/draw submission: %s", req.URL.Host)
			return nil, nil
		}
	})
	for i := 0; i < 2; i++ {
		message := strings.Join(task.runCampaignLottery(), "；")
		if !task.Pending() || !strings.Contains(message, "status=-4") || strings.Contains(message, "画布抽奖确认") {
			t.Fatal(message)
		}
	}
	if riskQueries != 2 || draws != 0 {
		t.Fatalf("queries=%d draws=%d", riskQueries, draws)
	}
}

func TestCoc3BeanDrawRequiresAccountOptInLiveZeroSKUAndActualDebit(t *testing.T) {
	for _, tc := range []struct {
		name         string
		optIn        bool
		price, after int
		expired      bool
		draws        int
		pending      bool
	}{
		{"default off", false, 0, 1, false, 0, false},
		{"confirmed", true, 0, 1, false, 1, false},
		{"ack without debit", true, 0, 2, false, 1, true},
		{"paid SKU", true, 10, 2, false, 0, false},
		{"expired", true, 0, 2, true, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CAIYUN_COC3_WASM_PARAMS", "fixture")
			t.Setenv("CAIYUN_COC3_BEAN_DRAW", "true") // a global value cannot opt in another account
			if tc.optIn {
				t.Setenv("CAIYUN_COC3_BEAN_DRAW_13800000001", "true")
			}
			draws, balances := 0, 0
			task := campaignCoc3Task(t, func(req *http.Request) (*http.Response, error) {
				if !tc.optIn {
					t.Fatalf("disabled bean draw sent %s", req.URL)
				}
				var result string
				switch {
				case strings.HasSuffix(req.URL.Path, "/getProByActId"):
					end := activityNow().Add(time.Hour).Unix()
					if tc.expired {
						end = activityNow().Add(-time.Hour).Unix()
					}
					result = fmt.Sprintf(`{"code":0,"data":{"startTime":%d,"endTime":%d,"subActivityList":[{"goodsList":[{"skuid":95408,"price":%d}]}]}}`, activityNow().Add(-24*time.Hour).Unix(), end, tc.price)
				case strings.HasSuffix(req.URL.Path, "/checkQualificByActivityId"):
					result = `{"resultCode":0}`
				case strings.HasSuffix(req.URL.Path, "/currentLottery"):
					result = `{"code":200,"data":{"skuId":95408}}`
				case strings.HasSuffix(req.URL.Path, "/getBalance"):
					balances++
					value := 2
					if balances > 1 {
						value = tc.after
					}
					result = fmt.Sprintf(`{"code":0,"data":%d}`, value)
				case strings.HasSuffix(req.URL.Path, "/secKill/cmtOrder"):
					draws++
					var payload map[string]interface{}
					_ = json.NewDecoder(req.Body).Decode(&payload)
					mix, _ := payload["mixPaymentInfo"].(map[string]interface{})
					if payload["basePayType"] != float64(32) || payload["basePayAmount"] != float64(0) || mix["digitalBean"] != float64(1) || payload["skuId"] != "95408" {
						t.Fatalf("unexpected spend: %v", payload)
					}
					result = `{"code":0,"data":{"skuName":"券"}}`
				default:
					t.Fatalf("unexpected %s", req.URL)
				}
				return coc3Reply(req, result), nil
			})
			message := strings.Join(task.runBeanLottery(), "；")
			if draws != tc.draws || task.Pending() != tc.pending || (tc.name == "confirmed" && !strings.Contains(message, "确认1次")) || (tc.name == "ack without debit" && strings.Contains(message, "确认1次")) {
				t.Fatalf("draws=%d pending=%v message=%s", draws, task.Pending(), message)
			}
		})
	}
}

func TestCoc3InventoryRequiresEveryExpectedTaskAndIgnoresDuplicates(t *testing.T) {
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/getBalance") {
			return coc3Reply(req, `{"code":0,"data":0}`), nil
		}
		return coc3Reply(req, `{"resultCode":0,"data":{"taskQueryTimeVOS":[{"taskId":3040,"completedNum":1,"totalNum":1},{"taskId":3040,"completedNum":1,"totalNum":1},{"taskId":9999,"completedNum":1,"totalNum":1}]}}`), nil
	})
	message := strings.Join(task.runInventory([]int{3040, 3041, 3026}), "；")
	if !task.Pending() || !strings.Contains(message, "确认完成1/3项") || !strings.Contains(message, "3041:未返回记录") || !strings.Contains(message, "3026:未返回记录") {
		t.Fatal(message)
	}
}

func TestCoc3FullCatalogueScanIsReadOnly(t *testing.T) {
	ids := map[string]bool{}
	task := campaignCoc3Task(t, func(req *http.Request) (*http.Response, error) {
		if req.Method != "GET" || !strings.HasSuffix(req.URL.Path, "/getProByActId") || req.Header.Get("wasmParams") != "" {
			t.Fatalf("scan mutated activity: %s", req.URL)
		}
		id := req.URL.Query().Get("activityId")
		if ids[id] {
			t.Fatal("duplicate catalogue id")
		}
		ids[id] = true
		return coc3Reply(req, fmt.Sprintf(`{"code":0,"data":{"startTime":%d,"endTime":%d,"subActivityList":[{"goodsList":[{"skuid":1,"price":0}]}]}}`, activityNow().Add(-time.Hour).Unix(), activityNow().Add(time.Hour).Unix())), nil
	})
	message := strings.Join(task.runActivityScan(), "；")
	if len(ids) != 157 || !strings.Contains(message, "在期且有0元商品157项") || len(task.issues) > 0 {
		t.Fatalf("ids=%d message=%s", len(ids), message)
	}
}
