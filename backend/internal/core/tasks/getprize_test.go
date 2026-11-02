package tasks

import (
	"io"
	"net/http"
	"strings"
	"testing"

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
