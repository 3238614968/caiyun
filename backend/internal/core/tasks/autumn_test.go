package tasks

import (
	"caiyun/internal/core/api"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func autumnFixture(t *testing.T, kind string, reply func(*http.Request) string) *AutumnTask {
	job := taskStepTestJob(t, reply)
	job.client.SetJWTToken("x." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture-user"}}`)) + ".x")
	task := NewAutumnTask(job.client, kind)
	task.delay = 0
	return task
}
func TestAnnualBillUsesActualParamsAndConfirmsCompletion(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		t.Run(fmt.Sprint(confirmed), func(t *testing.T) {
			calls, draws := 0, 0
			done := map[string]int{}
			task := autumnFixture(t, "annual_bill", func(req *http.Request) string {
				if req.URL.Host != "gp.mcloud.139.com" {
					t.Fatal("bill must use GP host")
				}
				switch req.URL.Path {
				case "/ycloud/bill/toDoTask/taskInfos":
					list := []map[string]interface{}{}
					for _, code := range []string{"BROWSE_REPORT", "AI_PHOTO", "FEEDBACK"} {
						n := 0
						if confirmed {
							n = done[code]
						}
						list = append(list, map[string]interface{}{"taskCode": code, "completedCount": n, "totalCount": 1})
					}
					b, _ := json.Marshal(map[string]interface{}{"code": 0, "result": list})
					return string(b)
				case "/ycloud/bill/toDoTask/executeTask":
					var body map[string]interface{}
					_ = json.NewDecoder(req.Body).Decode(&body)
					code := body["taskCode"].(string)
					if code == "AI_PHOTO" && body["param"] != "fixture-user" {
						t.Fatal("missing userDomainId")
					}
					if code == "FEEDBACK" {
						param, ok := body["param"].(map[string]interface{})
						if !ok || param["reasonChoice"] != "B" {
							t.Fatal("feedback must retain configured answers as an object")
						}
					}
					calls++
					done[code] = 1
					return `{"code":0}`
				case "/ycloud/bill/taskLottery/queryCount":
					return fmt.Sprintf(`{"code":0,"result":%d}`, 1-draws)
				case "/ycloud/bill/taskLottery/prize":
					draws++
					return `{"code":30004,"msg":"谢谢参与"}`
				default:
					t.Fatalf("unexpected %s", req.URL.Path)
				}
				return ""
			})
			task.SetAccountPhone("13800000001")
			t.Setenv("CAIYUN_BILL_FEEDBACK_JSON_13800000001", `{"reasonChoice":"B","suggestChoice":"C"}`)
			if err := task.Run(); err != nil {
				t.Fatal(err)
			}
			if calls != 3 || draws != 1 || task.Pending() == confirmed {
				t.Fatalf("calls=%d draws=%d pending=%v", calls, draws, task.Pending())
			}
		})
	}
}
func TestBillSubmitsDefaultFeedbackButNotInvitations(t *testing.T) {
	executed := map[string]map[string]interface{}{}
	task := autumnFixture(t, "annual_bill", func(req *http.Request) string {
		switch req.URL.Path {
		case "/ycloud/bill/toDoTask/taskInfos":
			return `{"code":0,"result":[{"taskCode":"FEEDBACK","totalCount":1},{"taskCode":"SHARE_FRIENDS","totalCount":300}]}`
		case "/ycloud/bill/toDoTask/executeTask":
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			code, _ := body["taskCode"].(string)
			param, _ := body["param"].(map[string]interface{})
			executed[code] = param
			return `{"code":0}`
		case "/ycloud/bill/taskLottery/queryCount":
			return `{"code":0,"result":0}`
		default:
			t.Fatalf("unexpected mutation %s", req.URL.Path)
		}
		return ""
	})
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	param := executed["FEEDBACK"]
	if param == nil || param["reasonChoice"] != "A" || param["suggestChoice"] != "A" {
		t.Fatalf("default feedback param = %v", param)
	}
	if _, ok := executed["SHARE_FRIENDS"]; ok {
		t.Fatal("invitation task must not be submitted")
	}
	if !task.Pending() {
		t.Fatal("unfinished bill task must stay pending")
	}
}
func TestLuckybagEligibilityCountsAndFailure(t *testing.T) {
	for _, mode := range []string{"eligible", "ineligible", "malformed", "failure", "capped"} {
		t.Run(mode, func(t *testing.T) {
			draws, subscribed := 0, 0
			count := 1
			if mode == "capped" {
				count = 100
			}
			task := autumnFixture(t, "luckybag", func(req *http.Request) string {
				switch req.URL.Path {
				case "/ycloud/luckybag/common/user/info":
					if mode == "ineligible" {
						return `{"code":604}`
					}
					if mode == "malformed" {
						return `{"code":0,"result":{}}`
					}
					return fmt.Sprintf(`{"code":0,"result":{"totalLotteryCount":%d}}`, count-draws)
				case "/ycloud/luckybag/common/user/subscribe":
					subscribed++
					return `{"code":0}`
				case "/ycloud/luckybag/blindbox/lottery":
					if mode == "failure" {
						return `{"code":505,"msg":"权限不足"}`
					}
					draws++
					return `{"code":0,"result":{"prize":"fixture"}}`
				default:
					t.Fatal(req.URL.Path)
				}
				return ""
			})
			err := task.Run()
			if (err != nil) != (mode == "malformed" || mode == "failure") {
				t.Fatalf("err=%v", err)
			}
			if mode == "eligible" && (draws != 1 || task.Pending()) {
				t.Fatal("draw not confirmed")
			}
			if mode == "ineligible" && (draws != 0 || subscribed != 0) {
				t.Fatal("ineligible mutation")
			}
			if mode == "capped" && (draws != 10 || !task.Pending()) {
				t.Fatal("unbounded drawing")
			}
		})
	}
}
func TestRedCavalryUsesServerCorrectionAndRequiresConfirmedBadges(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		t.Run(fmt.Sprint(confirmed), func(t *testing.T) {
			read, game, passed := false, false, false
			answers, reviews := 0, 0
			task := autumnFixture(t, "redcavalry", func(req *http.Request) string {
				switch req.URL.Path {
				case "/ycloud/simple/redcavalry/progress":
					return fmt.Sprintf(`{"code":0,"result":{"stations":[{"stationCode":"yudu","quizPassed":%t,"badgeEarned":%t}]}}`, passed && confirmed, passed && confirmed)
				case "/ycloud/simple/redcavalry/readCard":
					read = true
				case "/ycloud/simple/redcavalry/completeGame":
					if !read {
						t.Fatal("game before reading")
					}
					game = true
				case "/ycloud/simple/redcavalry/question":
					if !game {
						t.Fatal("question before game")
					}
					return `{"code":0,"result":{"id":"question-1"}}`
				case "/ycloud/simple/redcavalry/submitAnswer":
					answers++
					var body map[string]interface{}
					_ = json.NewDecoder(req.Body).Decode(&body)
					if body["questionId"] != "question-1" {
						t.Fatal("question ID changed")
					}
					if answers == 1 {
						return `{"code":0,"result":{"correct":false,"correctAnswer":"C"}}`
					}
					if body["answer"] != "C" {
						t.Fatal("did not use server correction")
					}
					passed = true
					return `{"code":0,"result":{"correct":true}}`
				case "/ycloud/simple/redcavalry/completeReview":
					reviews++
					return `{"code":0,"result":{"allBadgesEarned":true,"heritageNo":"fixture"}}`
				default:
					t.Fatal(req.URL.Path)
				}
				return `{"code":0}`
			})
			if err := task.Run(); err != nil {
				t.Fatal(err)
			}
			if answers != 2 || task.Pending() == confirmed || (!confirmed && reviews > 0) {
				t.Fatalf("answers=%d pending=%v reviews=%d", answers, task.Pending(), reviews)
			}
		})
	}
}
func TestTimedLotteryUsesCurrentModuleAndDeduplicates(t *testing.T) {
	calls := 0
	task := autumnFixture(t, "fun_ai_timed_lottery", func(req *http.Request) string {
		if strings.HasSuffix(req.URL.Path, "getmodule") {
			return `{"code":0,"result":[{"moduleId":"current","taskIds":[{"id":"zhishiku","isComplete":1},{"id":"zhishiku","isComplete":1},{"id":"011","isComplete":0}]}]}`
		}
		if strings.HasSuffix(req.URL.Path, "lottery") {
			calls++
			var body map[string]string
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["moduleId"] != "current" || body["taskId"] != "zhishiku" {
				t.Fatal(body)
			}
			return `{"code":10001,"msg":"次数已耗尽"}`
		}
		t.Fatal(req.URL.Path)
		return ""
	})
	if err := task.Run(); err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestRedeemCodesAreAccountScopedAndNotLogged(t *testing.T) {
	calls := 0
	task := autumnFixture(t, "redeem_code", func(req *http.Request) string {
		if req.URL.Path != "/ycloud/redeemcode-service/diamond/obtain" {
			t.Fatal(req.URL.Path)
		}
		calls++
		return `{"code":0,"result":true}`
	})
	task.SetAccountPhone("13800000001")
	t.Setenv("CAIYUN_REDEEM_CODES_13800000002", "other-account")
	if err := task.Run(); err != nil || calls != 0 || !task.Pending() {
		t.Fatalf("cross-account code used: %v", err)
	}
	t.Setenv("CAIYUN_REDEEM_CODES_13800000001", "fixture-secret,fixture-secret")
	if err := task.Run(); err != nil || calls != 1 || strings.Contains(task.Message(), "fixture-secret") {
		t.Fatalf("calls=%d err=%v message=%s", calls, err, task.Message())
	}
}
func TestTokenStageClaimsOnlyEligibleAndConfirms(t *testing.T) {
	claimed := 0
	job := taskStepTestJob(t, func(req *http.Request) string {
		if strings.HasSuffix(req.URL.Path, "receiveReward") {
			claimed++
			var body map[string]int
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["phaseNo"] != 1 {
				t.Fatal(body)
			}
			return `{"code":0}`
		}
		if strings.HasSuffix(req.URL.Path, "queryHome") {
			return `{"code":0,"result":{"usedToken":100,"rewardStages":[{"phaseNo":1,"receiveStatus":1}]}}`
		}
		t.Fatal(req.URL.Path)
		return ""
	})
	task := NewTokenPKTask(job.client, job.logger)
	one := 1
	stages := []api.TokenPKRewardStage{{PhaseNo: 1, TokenThreshold: 100}, {PhaseNo: 2, TokenThreshold: 101}, {PhaseNo: 3, TokenThreshold: 1, ReceiveStatus: &one}, {PhaseNo: 1, TokenThreshold: 100}}
	n, err := task.receiveProgressRewards(100, stages)
	if err != nil || n != 1 || claimed != 1 || task.Pending() {
		t.Fatalf("confirmed=%d claims=%d err=%v", n, claimed, err)
	}
}
func TestGPTaskListNeverFallsBackToMobileOrClaimsMobileRewards(t *testing.T) {
	markets := map[string]bool{}
	clicks := 0
	job := taskStepTestJob(t, func(req *http.Request) string {
		if strings.Contains(req.URL.Path, "deviceprofile") {
			return `{"code":1}`
		}
		if req.URL.Host != "gp.mcloud.139.com" {
			t.Fatalf("GP operation leaked to %s", req.URL.Host)
		}
		switch req.URL.Path {
		case "/ycloud/signin/page/startSignIn":
			return `{"code":0}`
		case "/ycloud/signin/task/taskListV3":
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			markets[fmt.Sprint(body["marketname"])] = true
			state := "WAIT"
			if clicks > 0 {
				state = "FINISH"
			}
			return taskStepJSON(api.Task{ID: 6000, State: state, StepTypeSet: []string{"sendcloud", "click"}})
		case "/ycloud/signin/page/doTaskPost":
			return `{"code":0}`
		case "/ycloud/signin/task/click":
			clicks++
			return `{"code":0}`
		default:
			t.Fatal("unexpected GP request", req.URL.Path)
		}
		return ""
	})
	gp := NewGPTaskListTask(job.client, job.logger)
	gp.wait = job.wait
	if err := gp.Run(); err != nil {
		t.Fatal(err)
	}
	if len(markets) != 2 || clicks != 1 || gp.ClaimedCloud() != 0 {
		t.Fatalf("markets=%v clicks=%d", markets, clicks)
	}
}
