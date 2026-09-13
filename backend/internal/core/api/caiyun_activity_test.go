package api

import (
	"strings"
	"testing"
)

func TestDecodeActivityBodyAcceptsNumericAndStringCodes(t *testing.T) {
	for _, body := range []string{
		`{"code":0,"msg":"success","result":{"a":1}}`,
		`{"code":"0","msg":"success","result":[1,2]}`,
	} {
		envelope, err := decodeActivityBody("测试", body)
		if err != nil {
			t.Fatalf("decodeActivityBody(%q) error = %v", body, err)
		}
		if !envelope.OK() || len(envelope.Result) == 0 {
			t.Fatalf("decodeActivityBody(%q) = %+v, want OK with result", body, envelope)
		}
	}
}

func TestDecodeActivityBodyRejectsBusinessErrors(t *testing.T) {
	envelope, err := decodeActivityBody("任务领奖", `{"code":403,"msg":"活动已下线"}`)
	if err == nil {
		t.Fatal("decodeActivityBody() error = nil, want business error")
	}
	if envelope == nil || envelope.MessageText() != "活动已下线" {
		t.Fatalf("decodeActivityBody() envelope = %+v, want message 活动已下线", envelope)
	}
	if !strings.Contains(err.Error(), "任务领奖") {
		t.Fatalf("decodeActivityBody() error = %v, want it to mention the operation", err)
	}

	if _, err := decodeActivityBody("测试", `<html>502</html>`); err == nil {
		t.Fatal("decodeActivityBody() error = nil for non-JSON body")
	}
}

func TestActivityTaskStepKeyAndLink(t *testing.T) {
	task := ActivityTask{
		ID: 3,
		Button: map[string]interface{}{
			"other": map[string]interface{}{"ext": "openUrl", "link": "https://example.com/a"},
			"android": map[string]interface{}{
				"ext":  "aiCamera",
				"link": "https://example.com/camera",
			},
		},
	}
	if got := task.StepKey(); got != "aiCamera" {
		t.Fatalf("StepKey() = %q, want aiCamera (android platform wins)", got)
	}
	if got := task.StepLink(); got != "https://example.com/camera" {
		t.Fatalf("StepLink() = %q, want camera link", got)
	}

	fallback := ActivityTask{Button: map[string]interface{}{
		"wechat": map[string]interface{}{"ext": "backup"},
	}}
	if got := fallback.StepKey(); got != "backup" {
		t.Fatalf("StepKey() = %q, want fallback to any platform ext", got)
	}
}

func TestSelectMakeWishPrize(t *testing.T) {
	prizes := []MakeWishPrize{
		{MakeWishPrizeID: 1, PrizeName: "腾讯视频VIP会员月卡"},
		{MakeWishPrizeID: 5, PrizeName: "哔哩哔哩超级月度大会员"},
	}
	if got := SelectMakeWishPrize(prizes, "哔哩"); got == nil || got.MakeWishPrizeID != 5 {
		t.Fatalf("SelectMakeWishPrize() = %+v, want prize 5 by keyword", got)
	}
	if got := SelectMakeWishPrize(prizes, "不存在"); got == nil || got.MakeWishPrizeID != 1 {
		t.Fatalf("SelectMakeWishPrize() = %+v, want first prize when keyword misses", got)
	}
	if got := SelectMakeWishPrize(nil, ""); got != nil {
		t.Fatalf("SelectMakeWishPrize() = %+v, want nil for empty list", got)
	}
}
