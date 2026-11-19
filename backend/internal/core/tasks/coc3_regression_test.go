package tasks

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/coc3wasm"
)

func TestCoc3DoesNotConfirmAcknowledgementsWithoutServerRecords(t *testing.T) {
	t.Setenv("CAIYUN_COC3_WASM_PARAMS", "fixture")
	submits := 0
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/getTaskSignInfo"):
			return coc3Reply(req, `{"resultCode":0,"data":{"signToday":false}}`), nil
		case strings.HasSuffix(req.URL.Path, "/accumulateSign"):
			return coc3Reply(req, `{"resultCode":0}`), nil
		case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			return coc3Reply(req, `{"code":200,"data":"3"}`), nil
		case strings.HasSuffix(req.URL.Path, "/currentLottery"):
			return coc3Reply(req, `{"code":200,"data":{"skuId":"123"}}`), nil
		case strings.HasSuffix(req.URL.Path, "/mixedOrder"):
			submits++
			return coc3Reply(req, `{"code":0}`), nil
		default:
			t.Fatalf("unexpected %s", req.URL.Path)
			return nil, nil
		}
	})
	task.tokenValue = "token"
	message := strings.Join(append(task.runSignIn(), task.runLottery()...), "；")
	if !task.Pending() || len(task.issues) != 0 || submits != len(coc3LotteryActs) || strings.Contains(message, "签到成功") || !strings.Contains(message, "0元抽奖共0次") {
		t.Fatalf("pending=%v submits=%d issues=%v message=%s", task.Pending(), submits, task.issues, message)
	}
}

func TestCoc3StopsFurtherOrdersOnServerRiskSignal(t *testing.T) {
	for _, risk := range []string{`true`, `"true"`} {
		t.Run(risk, func(t *testing.T) {
			t.Setenv("CAIYUN_COC3_WASM_PARAMS", "fixture")
			submits, counts := 0, 0
			task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
					counts++
					return coc3Reply(req, `{"code":200,"data":3}`), nil
				case strings.HasSuffix(req.URL.Path, "/currentLottery"):
					return coc3Reply(req, `{"code":200,"data":{"skuId":123}}`), nil
				case strings.HasSuffix(req.URL.Path, "/mixedOrder"):
					submits++
					return coc3Reply(req, fmt.Sprintf(`{"code":0,"data":{"risk":%s}}`, risk)), nil
				default:
					t.Fatalf("unexpected %s", req.URL.Path)
					return nil, nil
				}
			})
			message := strings.Join(task.runLottery(), "；")
			if !task.Pending() || submits != 1 || counts != 1 || !strings.Contains(message, "触发风控") || !strings.Contains(message, "共0次") {
				t.Fatalf("pending=%v submits=%d counts=%d message=%s", task.Pending(), submits, counts, message)
			}
		})
	}
}

func TestCoc3RejectedVisitsAreNotCountedAsAccepted(t *testing.T) {
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		return coc3Reply(req, `{"resultCode":"-516","message":"登录失效"}`), nil
	})
	message := strings.Join(task.runVisits(), "；")
	if !strings.Contains(message, "任务浏览0/6") || len(task.issues) != len(coc3Visits) || !strings.Contains(message, "-516") {
		t.Fatalf("issues=%v message=%s", task.issues, message)
	}
}

func TestCoc3UnknownProgressAndTokenFailureStayPending(t *testing.T) {
	t.Setenv("CAIYUN_COC3_WASM_PARAMS", "")
	t.Setenv("CAIYUN_COC3_WASM_DIR", "")
	original := coc3GenerateWasmParams
	coc3GenerateWasmParams = func(string, coc3wasm.DeviceContext) (string, error) { return "", errors.New("invalid wheel module") }
	t.Cleanup(func() { coc3GenerateWasmParams = original })
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/getTaskList"):
			return coc3Reply(req, `{"resultCode":0,"data":{}}`), nil
		case strings.HasSuffix(req.URL.Path, "/getBalance"):
			return coc3Reply(req, `{"code":0,"data":"0"}`), nil
		case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			return coc3Reply(req, `{"code":200,"data":2}`), nil
		default:
			t.Fatalf("unexpected mutation %s", req.URL.Path)
			return nil, nil
		}
	})
	message := strings.Join(append(task.runInventory([]int{3040, 3041, 3026}), task.runLottery()...), "；")
	if !task.Pending() || len(task.issues) != 0 || !strings.Contains(message, "不能确认完成") || !strings.Contains(message, "令牌生成失败") {
		t.Fatalf("pending=%v issues=%v message=%s", task.Pending(), task.issues, message)
	}
}

func TestCoc3EmbeddedWheelGeneratesHeaderWithoutExternalAssets(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	value, err := generateCoc3WasmParams("")
	if err != nil {
		t.Fatal(err)
	}
	var pair []string
	if err := json.Unmarshal([]byte(value), &pair); err != nil || len(pair) != 2 || pair[0] == "" || pair[1] == "" {
		t.Fatal("embedded wheel did not return aurora/content")
	}
}
