package tasks

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
)

func TestPrizeCenterResendsOnlyClaimedSpaceGifts(t *testing.T) {
	oldDelay := prizeSpaceGiftResendDelay
	prizeSpaceGiftResendDelay = 0
	t.Cleanup(func() { prizeSpaceGiftResendDelay = oldDelay })

	var resends []string
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/prizeApi/checkPrize/getUserPrizeLogPageV2":
			return hiddenRewardReply(req, `{"code":0,"result":{"totalPages":1,"result":[
				{"oId":"claimed-space","marketid":"National_NewLoginGif","flag":2},
				{"oId":"unclaimed-space","marketid":"National_NewLoginGif","flag":1},
				{"oId":"missing-market","marketid":"","flag":2}
			]}}`), nil
		case "/ycloud/prizeApi/userdraw/sendSpaceGiftPrize":
			body, _ := io.ReadAll(req.Body)
			resends = append(resends, string(body))
			return hiddenRewardReply(req, `{"code":0}`), nil
		case "/ycloud/prizeApi/checkPrize/getSpaceGiftInfo":
			return hiddenRewardReply(req, `{"code":0,"result":{"status":0}}`), nil
		default:
			t.Fatalf("unexpected prize request: %s", req.URL.Path)
		}
		return nil, nil
	})))
	task := NewPrizeCenterTask(client, nil)
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if len(resends) != 1 {
		t.Fatalf("resend calls = %d, want 1: %v", len(resends), resends)
	}
	if !strings.Contains(resends[0], `"oId":"claimed-space"`) || !strings.Contains(resends[0], `"marketName":"National_NewLoginGif"`) {
		t.Fatalf("resend payload = %s", resends[0])
	}
	if !strings.Contains(task.Message(), "空间卡补发尝试1项、成功1项") {
		t.Fatalf("message = %s", task.Message())
	}
}

func TestPrizeCenterResendsBeyondOldFiftyEntryCap(t *testing.T) {
	oldDelay := prizeSpaceGiftResendDelay
	prizeSpaceGiftResendDelay = 0
	t.Cleanup(func() { prizeSpaceGiftResendDelay = oldDelay })

	// 旧版只扫前 50 条；60 条 flag=2 记录必须全部补发。
	const total = 60
	entries := make([]api.PrizeCenterEntry, 0, total)
	for i := 0; i < total; i++ {
		entries = append(entries, api.PrizeCenterEntry{
			OID:      fmt.Sprintf("space-%02d", i),
			MarketID: "National_NewLoginGif",
			Flag:     api.PrizeFlagClaimed,
		})
	}
	resends := map[string]bool{}
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/ycloud/prizeApi/userdraw/sendSpaceGiftPrize" {
			t.Fatalf("unexpected prize request: %s", req.URL.Path)
		}
		body, _ := io.ReadAll(req.Body)
		resends[string(body)] = true
		return hiddenRewardReply(req, `{"code":0}`), nil
	})))
	task := NewPrizeCenterTask(client, nil)
	attempted, accepted := task.resendSpaceGiftPrizes(entries)
	if attempted != total || accepted != total || len(resends) != total {
		t.Fatalf("attempted=%d accepted=%d unique=%d, want %d", attempted, accepted, len(resends), total)
	}
	if !resends[`{"marketName":"National_NewLoginGif","oId":"space-59"}`] {
		t.Fatal("entry beyond the old 50-entry cap was not resent")
	}
}

func TestPrizeCenterSpaceGiftResendFailureIsBestEffort(t *testing.T) {
	oldDelay := prizeSpaceGiftResendDelay
	prizeSpaceGiftResendDelay = 0
	t.Cleanup(func() { prizeSpaceGiftResendDelay = oldDelay })

	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/prizeApi/checkPrize/getUserPrizeLogPageV2":
			return hiddenRewardReply(req, `{"code":0,"result":{"totalPages":1,"result":[
				{"oId":"claimed-space","marketid":"National_NewLoginGif","flag":2}
			]}}`), nil
		case "/ycloud/prizeApi/userdraw/sendSpaceGiftPrize":
			return nil, io.ErrUnexpectedEOF
		case "/ycloud/prizeApi/checkPrize/getSpaceGiftInfo":
			return hiddenRewardReply(req, `{"code":0,"result":{"status":0}}`), nil
		default:
			t.Fatalf("unexpected prize request: %s", req.URL.Path)
		}
		return nil, nil
	})))
	task := NewPrizeCenterTask(client, nil)
	if err := task.Run(); err != nil {
		t.Fatalf("best-effort resend must not fail the task: %v", err)
	}
	if !strings.Contains(task.Message(), "空间卡补发尝试1项、成功0项") {
		t.Fatalf("message = %s", task.Message())
	}
}
