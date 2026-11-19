package tasks

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestTokenPKExhaustedPrizeRemainsPendingWithoutFailingOrClaimingTwice(t *testing.T) {
	t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "fixture-device")
	claims := 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/portal/yunClound/index.html":
			return hiddenRewardReply(req, ""), nil
		case "/ycloud/tokenpk/task/list":
			return hiddenRewardReply(req, `{"code":0,"result":[{"id":9,"name":"已完成任务奖励","state":"SUCCESS"}]}`), nil
		case "/ycloud/tokenpk/task/step/receivePrize":
			claims++
			return hiddenRewardReply(req, `{"code":9001,"msg":"奖品已耗尽"}`), nil
		case "/ycloud/tokenpk/toplist/progress/autoReceiveLotteryChance":
			return hiddenRewardReply(req, `{"code":0,"result":{}}`), nil
		case "/ycloud/tokenpk/toplist/progress/queryRemainChance":
			return hiddenRewardReply(req, `{"code":0,"result":0}`), nil
		case "/ycloud/tokenpk/toplist/progress/queryHome":
			return hiddenRewardReply(req, `{"code":0,"result":{"usedToken":0}}`), nil
		case "/ycloud/tokenpk/toplist/rank/queryRankHome":
			return hiddenRewardReply(req, `{"code":0,"result":{"myRank":-1,"myToken":0}}`), nil
		default:
			t.Fatalf("unexpected TokenPK request: %s", req.URL)
		}
		return nil, nil
	})))
	task := NewTokenPKTask(client, logger.NewLogger(logger.LevelError))
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if claims != 1 || !task.Pending() || !strings.Contains(task.Message(), "1项奖励暂不可领") {
		t.Fatalf("claims=%d pending=%v message=%s", claims, task.Pending(), task.Message())
	}
}

func TestTokenPKDrawsAllRemainingLotteryChances(t *testing.T) {
	t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "fixture-device")
	draws := 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/portal/yunClound/index.html":
			return hiddenRewardReply(req, ""), nil
		case "/ycloud/tokenpk/task/list":
			return hiddenRewardReply(req, `{"code":0,"result":[]}`), nil
		case "/ycloud/tokenpk/toplist/progress/queryHome":
			return hiddenRewardReply(req, `{"code":0,"result":{"usedToken":0}}`), nil
		case "/ycloud/tokenpk/toplist/progress/autoReceiveLotteryChance":
			return hiddenRewardReply(req, `{"code":0,"result":{}}`), nil
		case "/ycloud/tokenpk/toplist/progress/queryRemainChance":
			if req.Method != "GET" {
				t.Fatalf("queryRemainChance method = %s, want GET", req.Method)
			}
			return hiddenRewardReply(req, `{"code":0,"result":2}`), nil
		case "/ycloud/tokenpk/toplist/progress/lottery":
			if req.Method != "POST" {
				t.Fatalf("lottery method = %s, want POST", req.Method)
			}
			if req.Body != nil {
				body, _ := io.ReadAll(req.Body)
				if len(body) != 0 {
					t.Fatalf("lottery must have no request body, got %s", body)
				}
			}
			draws++
			if draws == 1 {
				return hiddenRewardReply(req, `{"code":0,"result":{"win":true,"prizeName":"10G空间卡","remainChance":1}}`), nil
			}
			return hiddenRewardReply(req, `{"code":0,"result":{"win":false,"prizeName":"","remainChance":0}}`), nil
		case "/ycloud/tokenpk/toplist/rank/queryRankHome":
			return hiddenRewardReply(req, `{"code":0,"result":{"myRank":-1,"myToken":0}}`), nil
		default:
			t.Fatalf("unexpected TokenPK request: %s", req.URL)
		}
		return nil, nil
	})))
	task := NewTokenPKTask(client, logger.NewLogger(logger.LevelError))
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if draws != 2 {
		t.Fatalf("lottery draws = %d, want 2", draws)
	}
	if !strings.Contains(task.Message(), "榜单抽奖2次、中奖1次") {
		t.Fatalf("message = %s", task.Message())
	}
}

func TestTokenPKLotteryBatchCapAndFailureIsBestEffort(t *testing.T) {
	for _, tc := range []struct {
		name      string
		lotteryOK bool
		wantDraws int
		wantCalls int
	}{
		{"cap", true, 20, 20},
		// 首次抽奖失败即停止：1 次尝试、0 次成功。
		{"failure", false, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draws := 0
			client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/ycloud/tokenpk/toplist/progress/queryRemainChance":
					return hiddenRewardReply(req, `{"code":0,"result":25}`), nil
				case "/ycloud/tokenpk/toplist/progress/lottery":
					draws++
					if !tc.lotteryOK {
						return hiddenRewardReply(req, `{"code":500,"msg":"服务异常"}`), nil
					}
					return hiddenRewardReply(req, `{"code":0,"result":{"win":false}}`), nil
				default:
					t.Fatalf("unexpected TokenPK request: %s", req.URL)
				}
				return nil, nil
			})))
			task := NewTokenPKTask(client, logger.NewLogger(logger.LevelError))
			gotDraws, wins := task.drawTokenPKLottery()
			if gotDraws != tc.wantDraws || draws != tc.wantCalls || wins != 0 {
				t.Fatalf("draws=%d calls=%d wins=%d, want draws=%d calls=%d wins=0", gotDraws, draws, wins, tc.wantDraws, tc.wantCalls)
			}
		})
	}
}

func TestTokenPKTreatsEvery9001AsUnavailablePrize(t *testing.T) {
	for _, tc := range []struct {
		code, message string
		want          bool
	}{
		// 9001 在不同活动周期会回“奖品已耗尽”或“奖品发放失败”，语义一致。
		{"9001", "奖品已耗尽", true}, {"9001", "奖品发放失败", true},
		{"401", "权限不足", false}, {"9002", "参数无效", false},
	} {
		err := &api.ActivityBusinessError{Code: tc.code, Message: tc.message}
		if got := isTokenPKPrizeUnavailable(err); got != tc.want {
			t.Fatalf("code=%s message=%s got=%v", tc.code, tc.message, got)
		}
	}
}
