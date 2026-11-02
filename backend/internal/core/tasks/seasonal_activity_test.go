package tasks

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func seasonalPayload(t *testing.T, req *http.Request) map[string]interface{} {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestCloudPhoneRegistersOnlyProtocolTasksAndChecksBalance(t *testing.T) {
	for _, tc := range []struct {
		name    string
		already bool
		after   int
		pending bool
	}{
		{"credited", false, 100, false},
		{"already completed", true, 50, false},
		{"not credited", false, 50, true},
		{"partly credited", false, 80, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			balances, completed := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
					return hiddenRewardReply(req, `{"data":{"token":"test-sso"}}`), nil
				case "/cloudphone-market/user/tokenValidate":
					return hiddenRewardReply(req, `{"header":{"status":"200"},"data":{"token":"test-session"}}`), nil
				case "/cloudphone-market/redpacket/userAccountInfo":
					balances++
					balance := 50
					if balances == 2 {
						balance = tc.after
					}
					return hiddenRewardReply(req, fmt.Sprintf(`{"header":{"status":200},"data":{"info":{"canAmount":%d}}}`, balance)), nil
				case "/cloudphone-market/redpacket/userSign":
					p := seasonalPayload(t, req)
					if p["version"] != "SYS_CONFIG_Y" || p["platformType"] != float64(1) {
						t.Fatalf("sign payload=%v", p)
					}
					return hiddenRewardReply(req, `{"header":{"status":200},"data":{"status":1}}`), nil
				case "/cloudphone-market/redpacket/configTaskList":
					return hiddenRewardReply(req, `{"header":{"status":200},"data":{"configTaskMonthlyList":[{"id":13,"taskCode":"MONTHLY_5","prizeAmount":30},{"id":14,"taskCode":"MONTHLY_4"}],"configTaskInstallList":[{"id":"16","taskCode":"INSTALL_1","prizeAmount":"20"},{"id":16,"taskCode":"INSTALL_1"}],"configTaskBuyList":[{"id":99,"taskCode":"BUY_1"}]}}`), nil
				case "/cloudphone-market/redpacket/userCompleteTask":
					p := seasonalPayload(t, req)
					if p["taskId"] != float64(13) && p["taskId"] != float64(16) {
						t.Fatalf("cloud-instance or purchase task was executed: %v", p)
					}
					if p["version"] != "SYS_CONFIG_Y" || p["platformType"] != float64(1) {
						t.Fatalf("complete payload=%v", p)
					}
					completed++
					if tc.already {
						return hiddenRewardReply(req, `{"header":{"status":200},"data":{"status":0,"errorCode":"001","errorMsg":"已完成过"}}`), nil
					}
					return hiddenRewardReply(req, `{"header":{"status":200},"data":{"status":1}}`), nil
				default:
					t.Fatalf("unexpected endpoint: %s", req.URL)
				}
				return nil, nil
			})))
			task := NewCloudPhonePartyTask(client, nil, nil)
			task.SetAccountPhone("13800000001")
			task.delay = 0
			if err := task.Run(); err != nil {
				t.Fatal(err)
			}
			if completed != 2 || balances != 2 || task.Pending() != tc.pending || !strings.Contains(task.Message(), "其余2项") || !strings.Contains(task.Message(), "不计入云朵收益") {
				t.Fatalf("completed=%d balances=%d message=%s", completed, balances, task.Message())
			}
			if tc.already && !strings.Contains(task.Message(), "此前已完成2项") {
				t.Fatal(task.Message())
			}
		})
	}
}

func TestLanternStopsOnLimitAndNeverDrawsWrongAnswers(t *testing.T) {
	for _, tc := range []struct {
		name, submit, draw     string
		wantSubmits, wantDraws int
		wantError              bool
	}{
		{"wrong answers", `{"code":202}`, `{"code":0}`, 3, 0, false},
		{"daily cap", `{"code":"201"}`, `{"code":0}`, 1, 0, false},
		{"correct no win", `{"code":203}`, `{"code":10010020,"msg":"奖品溜走了"}`, 1, 1, false},
		{"expired auth", `{"code":401}`, `{"code":0}`, 1, 0, true},
		{"draw failed", `{"code":0}`, `{"code":401}`, 1, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			submits, draws := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(req.URL.Path, "/getIndexPuzzleCard"):
					if req.URL.Query().Get("sourceId") != "1201" {
						t.Fatal("missing lantern source")
					}
					return hiddenRewardReply(req, `{"code":0,"result":[{"id":"puzzle-one","puzzleTitleContext":"零存整取"},{"id":2,"puzzleTitleContext":""}]}`), nil
				case strings.HasSuffix(req.URL.Path, "/submitAnswered"):
					submits++
					p := seasonalPayload(t, req)
					if p["puzzleId"] != "puzzle-one" || p["sourceId"] != float64(1201) {
						t.Fatalf("wrong puzzle body: %v", p)
					}
					return hiddenRewardReply(req, tc.submit), nil
				case strings.HasSuffix(req.URL.Path, "/awarding"):
					draws++
					return hiddenRewardReply(req, tc.draw), nil
				default:
					t.Fatalf("unexpected endpoint: %s", req.URL)
				}
				return nil, nil
			})))
			task := NewLanternTask(client, nil)
			task.delay = 0
			err := task.Run()
			if (err != nil) != tc.wantError || submits != tc.wantSubmits || draws != tc.wantDraws {
				t.Fatalf("submits=%d draws=%d err=%v", submits, draws, err)
			}
			if tc.name == "wrong answers" && !task.Pending() {
				t.Fatal("uncompleted riddles must remain pending")
			}
			if tc.name == "correct no win" && !task.Pending() {
				t.Fatal("remaining unknown riddles must stay pending after a partial run")
			}
		})
	}
}

func TestLanternDrawsAtMostFourTimesAndSkipsUnknownClues(t *testing.T) {
	submits, draws := 0, 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(req.URL.Path, "/getIndexPuzzleCard"):
			rows := []map[string]interface{}{{"id": 1, "puzzleTitleContext": "图片题，无谜底"}}
			for id := 2; id <= 8; id++ {
				rows = append(rows, map[string]interface{}{"id": id, "puzzleTitleContext": "春花__月"})
			}
			data, _ := json.Marshal(map[string]interface{}{"code": 0, "result": rows})
			return hiddenRewardReply(req, string(data)), nil
		case strings.HasSuffix(req.URL.Path, "/submitAnswered"):
			submits++
			if seasonalPayload(t, req)["answered"] != "秋" {
				t.Fatal("wrong riddle answer")
			}
			return hiddenRewardReply(req, `{"code":0}`), nil
		case strings.HasSuffix(req.URL.Path, "/awarding"):
			draws++
			return hiddenRewardReply(req, `{"code":0,"result":{"prizeName":"10 AI豆"}}`), nil
		}
		t.Fatalf("unexpected endpoint: %s", req.URL)
		return nil, nil
	})))
	task := NewLanternTask(client, nil)
	task.delay = 0
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if submits != 4 || draws != 4 || !strings.Contains(task.Message(), "跳过1题") || !strings.Contains(task.Message(), "中奖4次") {
		t.Fatalf("submits=%d draws=%d message=%s", submits, draws, task.Message())
	}
	for _, clue := range []string{"", "花", "未知谜面"} {
		if len(lanternAnswers(clue)) != 0 {
			t.Fatalf("unknown clue was guessed: %q", clue)
		}
	}
	if got := lanternAnswers("春花 __ 月（填空）"); len(got) != 1 || got[0] != "秋" {
		t.Fatalf("normalized clue=%v", got)
	}
}

func TestLiveFlowerChecksBalancesAndHandlesAlreadyClaimed(t *testing.T) {
	for _, tc := range []struct {
		name, gift         string
		after              int
		pending, wantError bool
	}{
		{"first gift", `{"code":0,"result":{"prizeName":"10朵小红花"}}`, 10, false, false},
		{"already claimed", `{"code":"200110","msg":"您已参与过活动"}`, 0, false, false},
		{"not credited", `{"code":0,"result":{}}`, 0, true, false},
		{"unauthorized", `{"code":401}`, 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads, gifts := 0, 0
			client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/ycloud/liveRoomFeedback/user/info":
					reads++
					count := 0
					if reads > 1 {
						count = tc.after
					}
					return hiddenRewardReply(req, fmt.Sprintf(`{"code":0,"result":{"flowerCnt":%d,"redFlowerValidDate":"2026-12-31"}}`, count)), nil
				case "/ycloud/liveRoomFeedback/firstParticipate/give":
					gifts++
					return hiddenRewardReply(req, tc.gift), nil
				default:
					t.Fatalf("unexpected draw/SMS/watch request: %s", req.URL)
				}
				return nil, nil
			})))
			task := NewLiveFlowerTask(client, nil)
			err := task.Run()
			if (err != nil) != tc.wantError || task.Pending() != tc.pending || gifts != 1 {
				t.Fatalf("gifts=%d pending=%v err=%v", gifts, task.Pending(), err)
			}
			if !tc.wantError && reads != 2 {
				t.Fatal("gift was not checked against the balance")
			}
		})
	}
}

func TestShakeUsesNewHostAndSeparatesFragmentsFromPrizes(t *testing.T) {
	draws := 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "gp.mcloud.139.com" || req.Method != "POST" {
			t.Fatalf("old shake endpoint: %s", req.URL)
		}
		if token := req.Header.Values("jwtToken"); len(token) != 1 || token[0] != "test-jwt" {
			t.Fatal("missing marketing JWT header")
		}
		switch {
		case strings.HasSuffix(req.URL.Path, "/shakeIt"):
			if p := seasonalPayload(t, req); len(p) != 1 || p["flag"] != float64(2) || req.URL.RawQuery != "" {
				t.Fatalf("wrong shake body=%v", p)
			}
			draws++
			if draws == 1 {
				return hiddenRewardReply(req, `{"code":0,"result":{"shakePrizeconfig":{"name":"红包","fragment":"0"}}}`), nil
			}
			if draws == 2 {
				return hiddenRewardReply(req, `{"code":0,"result":{"shakePrizeconfig":{"name":"会员卡","fragment":4}}}`), nil
			}
			return hiddenRewardReply(req, `{"code":0,"result":{}}`), nil
		case strings.HasSuffix(req.URL.Path, "/fragmentPack"):
			return hiddenRewardReply(req, `{"code":0,"result":[{"fragmentNum":1,"shakePrizeconfig":{"name":"会员卡","fragment":4}}]}`), nil
		}
		t.Fatalf("unexpected endpoint: %s", req.URL)
		return nil, nil
	})))
	client.SetJWTToken("test-jwt")
	task := NewShakeTask(client, nil)
	if err := task.RunWithConfig(3, 0); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"抽奖3次", "完整奖品1次", "碎片1次", "会员卡碎片1/4"} {
		if !strings.Contains(task.Message(), text) {
			t.Fatalf("message=%s missing=%s", task.Message(), text)
		}
	}
}

func TestShakeStopsImmediatelyOnBusinessFailure(t *testing.T) {
	calls := 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		calls++
		return hiddenRewardReply(req, `{"code":90001,"msg":"未登录"}`), nil
	})))
	if err := NewShakeTask(client, nil).RunWithConfig(3, 0); err == nil || calls != 1 {
		t.Fatalf("shake auth failure swallowed or retried: calls=%d err=%v", calls, err)
	}
}

func TestTokenPKThirteenUsesImageConversationBeforeRegistering(t *testing.T) {
	var chatted, registered, cleaned bool
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/hcy/file/create":
			return hiddenRewardReply(req, `{"code":0,"data":{"fileId":"temp-image","uploadId":"temp-upload","partInfos":[{"partNumber":1,"uploadUrl":"https://upload.example/temp-image"}]}}`), nil
		case "/temp-image":
			return hiddenRewardReply(req, ""), nil
		case "/hcy/file/complete":
			return hiddenRewardReply(req, `{"code":0}`), nil
		case "/api/image/aiRecognize":
			return hiddenRewardReply(req, `{"success":true,"code":"0000"}`), nil
		case "/api/outer/assistant/chat/v2/add":
			body, _ := io.ReadAll(req.Body)
			if !strings.Contains(string(body), "temp-image") {
				t.Fatal("TokenPK13 sent a plain text conversation")
			}
			chatted = true
			return hiddenRewardReply(req, "data: {\"code\":\"0000\"}\n\n"), nil
		case "/hcy/recyclebin/batchTrash":
			cleaned = true
			return hiddenRewardReply(req, `{"success":true,"code":"0000"}`), nil
		case "/ycloud/tokenpk/task/step/click":
			if !chatted || !cleaned {
				t.Fatal("TokenPK13 registered before completing and cleaning its image action")
			}
			p := seasonalPayload(t, req)
			if p["taskId"] != float64(13) || p["key"] != "aiAssistant" {
				t.Fatalf("TokenPK13 registration=%v", p)
			}
			registered = true
			return hiddenRewardReply(req, `{"code":0}`), nil
		default:
			t.Fatalf("unexpected TokenPK13 request: %s", req.URL)
		}
		return nil, nil
	})))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"test-user"}}`))
	client.SetJWTToken("header." + claims + ".signature")
	task := NewTokenPKTask(client, logger.NewLogger(logger.LevelError))
	err := task.executeTokenPKStep(tokenPKAction{Task: api.ActivityTask{ID: 13}, Key: "aiAssistant"})
	if err != nil || !registered {
		t.Fatalf("registered=%v err=%v", registered, err)
	}
}
