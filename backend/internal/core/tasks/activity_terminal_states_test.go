package tasks

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestMakeWishStepRegistrationOutcome(t *testing.T) {
	for _, tc := range []struct {
		code              string
		ignore, completed bool
	}{
		// 任务已完成：服务端已终结该步骤，登记多余。
		{"96541032", true, true},
		// 当前任务步骤不支持该操作：动作已执行，无法页面上报。
		{"96541037", true, false},
		// 其他错误必须如实上报。
		{"9001", false, false},
	} {
		err := &api.ActivityBusinessError{Operation: "登记许愿任务步骤", Code: tc.code, Message: "x"}
		ignore, completed := makewishStepRegistrationOutcome(err)
		if ignore != tc.ignore || completed != tc.completed {
			t.Fatalf("code=%s ignore=%v completed=%v", tc.code, ignore, completed)
		}
	}
	if ignore, completed := makewishStepRegistrationOutcome(nil); ignore || completed {
		t.Fatalf("nil error must not be treated as a terminal step state")
	}
}

func TestActivityEndedClassification(t *testing.T) {
	if !isActivityEndedError(&api.ActivityBusinessError{Code: "602", Message: "活动已结束"}) {
		t.Fatal("602 must be classified as activity ended")
	}
	if isActivityEndedError(&api.ActivityBusinessError{Code: "500", Message: "系统繁忙"}) {
		t.Fatal("500 must not be classified as activity ended")
	}
	if !isActivityEndedError(errors.New("活动任务点击 失败: code=602 活动已结束")) {
		t.Fatal("plain-text activity ended message must be recognized")
	}
	if isActivityEndedError(nil) {
		t.Fatal("nil must not be classified as activity ended")
	}
}

func TestPosterTaskSkipsEndedActivityWithoutFailing(t *testing.T) {
	t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "fixture-device")
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/portal/yunpanpage/index.html":
			return hiddenRewardReply(req, "<html></html>"), nil
		case "/ycloud/newyear/task/list":
			return hiddenRewardReply(req, `{"code":602,"msg":"活动已结束"}`), nil
		default:
			t.Fatalf("ended activity must not trigger more requests: %s", req.URL)
		}
		return nil, nil
	})))
	task := NewPosterTask(client, logger.NewLogger(logger.LevelError))
	if err := task.Run(); err != nil {
		t.Fatalf("ended activity must not fail the task: %v", err)
	}
	if task.Pending() || !strings.Contains(task.Message(), "活动已结束") {
		t.Fatalf("pending=%v message=%s", task.Pending(), task.Message())
	}
}
