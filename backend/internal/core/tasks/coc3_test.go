package tasks

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"caiyun/internal/core/coc3wasm"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// coc3RoundTrip / coc3Reply —— coc3 任务的 mock transport 复用既有模式。
type coc3RoundTrip func(*http.Request) (*http.Response, error)

func (f coc3RoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func coc3Reply(req *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func newCoc3TestTask(t *testing.T, handler coc3RoundTrip) *Coc3Task {
	t.Helper()
	t.Setenv("CAIYUN_COC3_TASK_IDS_13800000001", "3040,3041,3026")
	client := corehttp.NewClient(corehttp.WithTransport(coc3RoundTrip(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/dfpRisk/getDfpOuterCode") {
			return coc3Reply(req, fmt.Sprintf(`{"dfp":%q,"exp":%d}`, strings.Repeat("fixture", 12), time.Now().Add(time.Hour).UnixMilli())), nil
		}
		if req.URL.Host == "yxfk.market.chinamobile.com:8081" {
			return coc3Reply(req, `{"status":2,"data":"fixture-risk-token"}`), nil
		}
		if strings.HasSuffix(req.URL.Path, "/slgCredits/queryGrowthValue") {
			return coc3Reply(req, `{"resultCode":-9009,"msg":"活动已结束"}`), nil
		}
		if strings.HasSuffix(req.URL.Path, "/arrange/checkQualificByActivityId") {
			switch req.URL.Query().Get("activityId") {
			case "17524", "17451", "17452", "17453":
				return coc3Reply(req, `{"resultCode":0}`), nil
			}
			return coc3Reply(req, `{"resultCode":-403,"msg":"活动不在进行中"}`), nil
		}
		return handler(req)
	})))
	task := NewCoc3Task(client, logger.NewLogger(logger.LevelError)).
		SetAccountContext("13800000001", "Basic dGVzdA==")
	task.wait = func(time.Duration) {}
	return task
}

func TestCoc3SignInFlow(t *testing.T) {
	var sawSourceID, sawTicketType interface{}
	visitSeen := 0
	signReads := 0
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "orches.yun.139.com" && strings.HasSuffix(req.URL.Path, "querySpecToken"):
			body, _ := io.ReadAll(req.Body)
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			sawSourceID = payload["toSourceId"]
			return coc3Reply(req, `{"success":true,"code":"0","data":{"token":"YZsidssolgTESTTICKET000000000000"}}`), nil
		case strings.HasSuffix(req.URL.Path, "/ssoRights/userLoginV2"):
			body, _ := io.ReadAll(req.Body)
			var payload map[string]interface{}
			_ = json.Unmarshal(body, &payload)
			sawTicketType = payload["ticketType"]
			if payload["ticket"] != "YZsidssolgTESTTICKET000000000000" {
				t.Fatalf("login ticket = %v", payload["ticket"])
			}
			return coc3Reply(req, `{"code":"0","message":"成功!","data":{"token":"coc3-token-36chars-abcdefghij","refreshToken":"rt","mobileMask":"138****0001","provinceCode":"100"}}`), nil
		case strings.HasSuffix(req.URL.Path, "/task/getTaskSignInfo"):
			signReads++
			return coc3Reply(req, fmt.Sprintf(`{"resultCode":0,"data":{"signToday":%v,"accumulateCount":2}}`, signReads > 1)), nil
		case strings.HasSuffix(req.URL.Path, "/task/accumulateSign"):
			return coc3Reply(req, `{"resultCode":0,"data":[{"skuId":84616,"awardName":"本月累签3次奖励","skuName":"2GB流量日包券"}]}`), nil
		case strings.HasSuffix(req.URL.Path, "/task/visit"):
			visitSeen++
			return coc3Reply(req, `{"resultCode":0,"data":"ok"}`), nil
		case strings.HasSuffix(req.URL.Path, "/task/getTaskList"):
			return coc3Reply(req, `{"resultCode":0,"data":{"taskQueryTimeVOS":[{"taskId":3040,"completedNum":1,"totalNum":1},{"taskId":3041,"completedNum":1,"totalNum":1},{"taskId":3026,"completedNum":1,"totalNum":1}]}}`), nil
		case strings.HasSuffix(req.URL.Path, "/digIntel/getBalance"):
			return coc3Reply(req, `{"code":0,"data":"0"}`), nil
		case strings.HasSuffix(req.URL.Path, "/lottery/residueLotteryCount"):
			return coc3Reply(req, `{"code":200,"data":0}`), nil
		default:
			t.Fatalf("unexpected coc3 endpoint: %s%s", req.URL.Host, req.URL.Path)
			return nil, nil
		}
	})
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if sawSourceID != "056002" {
		t.Fatalf("querySpecToken toSourceId = %v, want 056002", sawSourceID)
	}
	if sawTicketType != float64(11) {
		t.Fatalf("userLoginV2 ticketType = %v, want 11", sawTicketType)
	}
	msg := task.Message()
	for _, want := range []string{"签到成功", "2GB流量日包券", "任务浏览6/6组", "权益豆0"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q: %s", want, msg)
		}
	}
	if visitSeen != len(coc3Visits) {
		t.Fatalf("visit calls = %d, want %d", visitSeen, len(coc3Visits))
	}
	if task.Pending() {
		t.Fatalf("unexpected pending: %s", msg)
	}
}

func TestCoc3SignInIdempotentAndUnavailable(t *testing.T) {
	states := map[string]string{
		"/task/getTaskSignInfo": `{"resultCode":-9009,"msg":"任务已结束","data":null}`,
	}
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "orches.yun.139.com":
			return coc3Reply(req, `{"success":true,"code":"0","data":{"token":"T"}}`), nil
		case strings.HasSuffix(req.URL.Path, "/userLoginV2"):
			return coc3Reply(req, `{"code":"0","data":{"token":"tok","mobileMask":"138****0001"}}`), nil
		case strings.HasSuffix(req.URL.Path, "/task/getTaskSignInfo"):
			return coc3Reply(req, states["/task/getTaskSignInfo"]), nil
		case strings.HasSuffix(req.URL.Path, "/task/accumulateSign"):
			// 不可用任务不应走到提交
			t.Fatal("accumulateSign must not be called for unavailable task")
			return nil, nil
		case strings.HasSuffix(req.URL.Path, "/task/visit"):
			return coc3Reply(req, `{"resultCode":0}`), nil
		case strings.HasSuffix(req.URL.Path, "/getTaskList"):
			return coc3Reply(req, `{"resultCode":0,"data":{"taskQueryTimeVOS":[{"taskId":3040,"completedNum":1,"totalNum":1},{"taskId":3041,"completedNum":1,"totalNum":1},{"taskId":3026,"completedNum":1,"totalNum":1}]}}`), nil
		case strings.HasSuffix(req.URL.Path, "/getBalance"):
			return coc3Reply(req, `{"code":0,"data":"5"}`), nil
		case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			return coc3Reply(req, `{"code":200,"data":0}`), nil
		default:
			t.Fatalf("unexpected endpoint %s", req.URL.Path)
			return nil, nil
		}
	})
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(task.Message(), "不可用") {
		t.Fatalf("unavailable sign task must be reported: %s", task.Message())
	}
	if task.Pending() {
		t.Fatalf("unavailable sign task must not mark pending: %s", task.Message())
	}
}

func TestCoc3LotteryStaysPendingWhenTokenGenerationFails(t *testing.T) {
	t.Setenv("CAIYUN_COC3_WASM_PARAMS", "")
	t.Setenv("CAIYUN_COC3_WASM_DIR", "")
	origGen := coc3GenerateWasmParams
	coc3GenerateWasmParams = func(string, coc3wasm.DeviceContext) (string, error) {
		return "", fmt.Errorf("fixture generation failure")
	}
	defer func() { coc3GenerateWasmParams = origGen }()

	submitted := false
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "orches.yun.139.com":
			return coc3Reply(req, `{"success":true,"code":"0","data":{"token":"T"}}`), nil
		case strings.HasSuffix(req.URL.Path, "/userLoginV2"):
			return coc3Reply(req, `{"code":"0","data":{"token":"tok","mobileMask":"138****0001"}}`), nil
		case strings.HasSuffix(req.URL.Path, "/getTaskSignInfo"):
			return coc3Reply(req, `{"resultCode":0,"data":{"signToday":true,"accumulateCount":9}}`), nil
		case strings.HasSuffix(req.URL.Path, "/task/visit"):
			return coc3Reply(req, `{"resultCode":0}`), nil
		case strings.HasSuffix(req.URL.Path, "/getTaskList"):
			return coc3Reply(req, `{"resultCode":0,"data":{}}`), nil
		case strings.HasSuffix(req.URL.Path, "/getBalance"):
			return coc3Reply(req, `{"code":0,"data":"0"}`), nil
		case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			return coc3Reply(req, `{"code":200,"data":3}`), nil
		case strings.Contains(req.URL.Path, "/market-order/"):
			submitted = true
			t.Fatal("0元单在无 wasmParams 时不应提交")
			return nil, nil
		default:
			t.Fatalf("unexpected endpoint %s", req.URL.Path)
			return nil, nil
		}
	})
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if submitted {
		t.Fatal("submit endpoint must not be reached without wasmParams")
	}
	if !strings.Contains(task.Message(), "令牌生成失败") {
		t.Fatalf("lottery skip must be reported: %s", task.Message())
	}
}

func TestCoc3LotterySubmitsWithGeneratedWasmParams(t *testing.T) {
	t.Setenv("CAIYUN_COC3_WASM_PARAMS", "")
	t.Setenv("CAIYUN_COC3_WASM_DIR", "/tmp/coc3-wheel")
	origGen := coc3GenerateWasmParams
	coc3GenerateWasmParams = func(dir string, device coc3wasm.DeviceContext) (string, error) {
		if dir != "/tmp/coc3-wheel" {
			return "", fmt.Errorf("dir mismatch: %s", dir)
		}
		if device.BusinessToken != "tok" || device.Exchange == nil {
			return "", fmt.Errorf("current COC session missing from official device preflight")
		}
		return `["fake-aurora","fake-content"]`, nil
	}
	defer func() { coc3GenerateWasmParams = origGen }()

	var sawWasmHeader string
	remaining := map[int]int{}
	submitted := 0
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "orches.yun.139.com":
			return coc3Reply(req, `{"success":true,"code":"0","data":{"token":"T"}}`), nil
		case strings.HasSuffix(req.URL.Path, "/userLoginV2"):
			return coc3Reply(req, `{"code":"0","data":{"token":"tok","mobileMask":"138****0001"}}`), nil
		case strings.HasSuffix(req.URL.Path, "/getTaskSignInfo"):
			return coc3Reply(req, `{"resultCode":0,"data":{"signToday":true,"accumulateCount":9}}`), nil
		case strings.HasSuffix(req.URL.Path, "/task/visit"):
			return coc3Reply(req, `{"resultCode":0}`), nil
		case strings.HasSuffix(req.URL.Path, "/getTaskList"):
			return coc3Reply(req, `{"resultCode":0,"data":{}}`), nil
		case strings.HasSuffix(req.URL.Path, "/getBalance"):
			return coc3Reply(req, `{"code":0,"data":"0"}`), nil
		case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			var payload struct {
				ActivityID int `json:"activityId"`
			}
			_ = json.NewDecoder(req.Body).Decode(&payload)
			n, found := remaining[payload.ActivityID]
			if !found {
				n = 3
				remaining[payload.ActivityID] = n
			}
			return coc3Reply(req, fmt.Sprintf(`{"code":200,"data":"%d"}`, n)), nil
		case strings.HasSuffix(req.URL.Path, "/currentLottery"):
			return coc3Reply(req, `{"code":200,"data":{"skuId":95408}}`), nil
		case strings.HasSuffix(req.URL.Path, "/api/external/mixedOrder"):
			sawWasmHeader = req.Header.Get("wasmParams")
			var payload struct {
				ActivityID int `json:"activityId"`
			}
			_ = json.NewDecoder(req.Body).Decode(&payload)
			remaining[payload.ActivityID]--
			submitted++
			return coc3Reply(req, `{"code":0,"data":{"risk":false,"orderInfo":{"prizeName":"谢谢参与"}}}`), nil
		default:
			t.Fatalf("unexpected endpoint %s", req.URL.Path)
			return nil, nil
		}
	})
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if sawWasmHeader != `["fake-aurora","fake-content"]` {
		t.Fatalf("wasmParams header = %q", sawWasmHeader)
	}
	if submitted != 3*len(coc3LotteryActs) {
		t.Fatalf("string count was ignored or risk=false stopped a draw: %d", submitted)
	}
	if !strings.Contains(task.Message(), "0元抽奖共") {
		t.Fatalf("draw count must be reported: %s", task.Message())
	}
}
