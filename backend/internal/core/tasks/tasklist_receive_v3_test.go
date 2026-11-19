package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestTaskListRunsV3ActionsAndClaimsTheirRewardBubble(t *testing.T) {
	t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "")
	clicked, claimed, posts := false, false, 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/signin/task/taskListV2":
			return hiddenRewardReply(req, emptyTemporaryGroupReply(t, req)), nil
		case "/ycloud/signin/task/taskListV3":
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["marketname"] == "newsign_139mail" {
				return hiddenRewardReply(req, `{"code":0,"result":[]}`), nil
			}
			state := "WAIT"
			if clicked {
				state = "FINISH"
			}
			return hiddenRewardReply(req, fmt.Sprintf(`{"code":0,"result":[{"id":500,"name":"fixture click","enable":1,"state":%q,"stepTypeSet":["click"],"button":{"app":{"canReceive":1}}}]}`, state)), nil
		case "/ycloud/signin/task/click":
			clicked = true
			return hiddenRewardReply(req, `{"code":0}`), nil
		case "/ycloud/signin/page/infoV3":
			if clicked && !claimed {
				return hiddenRewardReply(req, `{"code":0,"result":{"total":1000,"toReceive":5,"receiveList":[{"recordId":3000000001,"cloudType":0,"cloudNum":5}]}}`), nil
			}
			total := 1000
			if claimed {
				total += 5
			}
			return hiddenRewardReply(req, fmt.Sprintf(`{"code":0,"result":{"total":%d,"toReceive":0,"receiveList":[]}}`, total)), nil
		case "/ycloud/signin/page/receiveV3":
			var body struct{ CloudID int64 }
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !clicked || claimed || body.CloudID != 3000000001 {
				t.Fatal("task ID used as reward ID or duplicate claim")
			}
			posts++
			claimed = true
			return hiddenRewardReply(req, `{"code":0,"result":{"receive":5,"total":1005}}`), nil
		case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling", "/ycloud/signin/page/doTaskPost":
			return hiddenRewardReply(req, `{"code":0}`), nil
		default:
			if strings.Contains(req.URL.Path, "/deviceprofile/") {
				return hiddenRewardReply(req, `{"code":1}`), nil
			}
			t.Fatalf("unexpected/legacy reward request: %s", req.URL.Path)
		}
		return nil, nil
	})))
	client.SetDeviceID("Bfixture-device")
	client.SetJWTToken("fixture-jwt")
	task := NewTaskListTask(client, logger.NewLogger(logger.LevelError))
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if !clicked || posts != 1 || task.ClaimedCloud() != 5 {
		t.Fatalf("click=%v posts=%d actual_gain=%d", clicked, posts, task.ClaimedCloud())
	}
	if err := task.receiveCompletedTaskRewards(); err != nil || posts != 1 || task.ClaimedCloud() != 5 {
		t.Fatal("later reward check duplicated the receipt")
	}
}

func TestTaskListSpecialRewardsPropagateFailuresAndConfirmClaims(t *testing.T) {
	for _, taskID := range []int{1003, 1021} {
		for _, tc := range []struct {
			name        string
			failFirst   bool
			confirm     bool
			wantError   bool
			wantPending bool
			wantClaims  int
		}{
			{name: "claim rejected", failFirst: true, wantError: true, wantPending: true, wantClaims: 2},
			{name: "claim unconfirmed", wantPending: true, wantClaims: 2},
			{name: "claim confirmed", confirm: true, wantClaims: 1},
			{name: "initial failure recovered", failFirst: true, confirm: true, wantClaims: 2},
		} {
			t.Run(fmt.Sprintf("%d/%s", taskID, tc.name), func(t *testing.T) {
				t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "")
				claims, canReceive := 0, 1
				client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/ycloud/signin/task/taskListV2":
						return hiddenRewardReply(req, emptyTemporaryGroupReply(t, req)), nil
					case "/ycloud/signin/task/taskListV3":
						var body map[string]interface{}
						if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
							t.Fatal(err)
						}
						if body["marketname"] == "newsign_139mail" {
							return hiddenRewardReply(req, `{"code":0,"result":[]}`), nil
						}
						return hiddenRewardReply(req, fmt.Sprintf(`{"code":0,"result":[{"id":%d,"name":"reward slot","state":"FINISH","button":{"app":{"canReceive":%d}}}]}`, taskID, canReceive)), nil
					case "/ycloud/signin/page/receiveTask":
						if req.URL.Query().Get("taskId") != fmt.Sprint(taskID) {
							t.Fatalf("wrong task reward ID: %s", req.URL)
						}
						claims++
						if tc.failFirst && (claims == 1 || !tc.confirm) {
							return hiddenRewardReply(req, `{"code":500,"msg":"claim unavailable"}`), nil
						}
						if tc.confirm {
							canReceive = 0
						}
						return hiddenRewardReply(req, `{"code":0}`), nil
					case "/ycloud/signin/page/infoV3":
						return hiddenRewardReply(req, `{"code":0,"result":{"total":1000,"toReceive":0,"receiveList":[]}}`), nil
					case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling", "/ycloud/signin/page/doTaskPost":
						return hiddenRewardReply(req, `{"code":0}`), nil
					default:
						if strings.Contains(req.URL.Path, "/deviceprofile/") {
							return hiddenRewardReply(req, `{"code":1}`), nil
						}
						t.Fatalf("unexpected special reward request: %s", req.URL)
						return nil, nil
					}
				})))
				client.SetDeviceID("Bfixture-device")
				task := NewTaskListTask(client, logger.NewLogger(logger.LevelFatal))
				err := task.Run()
				if (err != nil) != tc.wantError || task.Pending() != tc.wantPending || claims != tc.wantClaims {
					t.Fatalf("err=%v pending=%v claims=%d message=%s", err, task.Pending(), claims, task.Message())
				}
				if tc.wantError && (!strings.Contains(err.Error(), fmt.Sprintf("sign_in_3/%d", taskID)) || !strings.Contains(err.Error(), "500")) {
					t.Fatalf("reward failure lost task and business code: %v", err)
				}
				if strings.Contains(task.Message(), "待领奖") != tc.wantPending {
					t.Fatalf("pending reward missing from final message: %s", task.Message())
				}
			})
		}
	}
}

// gp 待领豆镜像并入任务列表 Message：615 归因 App 且不报错；code 0 记录领取数。
// 每次 Run 只探测一次，gp 云朵不计入 m 域 claimedCloud。
func TestTaskListMirrorsGPPendingClouds(t *testing.T) {
	for _, tc := range []struct {
		name        string
		ack         string
		confirmed   bool
		wantInfo    int
		wantMessage string
	}{
		{"615 needs app", `{"code":615,"msg":"请前往App领取"}`, false, 1, "App"},
		{"code 0 claimed", `{"code":0,"msg":"success","result":{"receive":30,"total":1030}}`, true, 2, "gp已领取30云朵"},
		{"code 0 unconfirmed", `{"code":0,"msg":"success","result":{"receive":30,"total":1030}}`, false, 2, "gp待领云朵探测失败"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "")
			gpInfo, gpPosts := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host == "gp.mcloud.139.com" {
					switch req.URL.Path {
					case "/ycloud/signin/page/infoV3":
						gpInfo++
						if tc.confirmed && gpPosts > 0 {
							return hiddenRewardReply(req, `{"code":0,"result":{"total":1030,"toReceive":0,"receiveList":[]}}`), nil
						}
						return hiddenRewardReply(req, `{"code":0,"result":{"total":1000,"toReceive":30,"receiveList":[{"recordId":11,"cloudType":0,"cloudNum":10},{"recordId":12,"cloudType":0,"cloudNum":20}]}}`), nil
					case "/ycloud/signin/page/receiveV3":
						gpPosts++
						var body map[string]interface{}
						if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
							t.Fatal(err)
						}
						if body["client"] != "app" || body["cloudId"] != "" || body["cloudType"] != float64(4) {
							t.Fatalf("wrong gp mirror payload: %s", body)
						}
						return hiddenRewardReply(req, tc.ack), nil
					default:
						t.Fatalf("unexpected gp request: %s", req.URL)
					}
				}
				switch req.URL.Path {
				case "/ycloud/signin/task/taskListV2":
					return hiddenRewardReply(req, emptyTemporaryGroupReply(t, req)), nil
				case "/ycloud/signin/task/taskListV3":
					return hiddenRewardReply(req, `{"code":0,"result":[]}`), nil
				case "/ycloud/signin/page/infoV3":
					return hiddenRewardReply(req, `{"code":0,"result":{"total":1000,"toReceive":0,"receiveList":[]}}`), nil
				case "/ycloud/signin/page/receiveV3":
					t.Fatal("m domain had no pending rewards to claim")
				case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling", "/ycloud/signin/page/doTaskPost":
					return hiddenRewardReply(req, `{"code":0}`), nil
				default:
					if strings.Contains(req.URL.Path, "/deviceprofile/") {
						return hiddenRewardReply(req, `{"code":1}`), nil
					}
					t.Fatalf("unexpected request: %s", req.URL)
				}
				return nil, nil
			})))
			client.SetDeviceID("Bfixture-device")
			client.SetJWTToken("m-jwt")
			client.SetJWTTokenGP("gp-jwt")
			task := NewTaskListTask(client, logger.NewLogger(logger.LevelError))
			if err := task.Run(); err != nil {
				t.Fatal(err)
			}
			if gpInfo != tc.wantInfo || gpPosts != 1 {
				t.Fatalf("gp mirror must probe exactly once per run: info=%d posts=%d", gpInfo, gpPosts)
			}
			if !strings.Contains(task.Message(), tc.wantMessage) || task.ClaimedCloud() != 0 {
				t.Fatalf("message=%s claimed=%d", task.Message(), task.ClaimedCloud())
			}
		})
	}
}
