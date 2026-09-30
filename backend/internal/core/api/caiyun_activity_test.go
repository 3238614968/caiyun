package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestTokenPKAcceptInviteUsesFormBody(t *testing.T) {
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/ycloud/tokenpk/invite/accept" || req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("unexpected invite request: %s content-type=%s", req.URL, req.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(req.Body)
		if err != nil || string(body) != "code=hello%2Bworld" {
			t.Fatalf("invite body = %q, err = %v", body, err)
		}
		return mailTestResponse(req, 200, `{"code":0,"msg":"success"}`), nil
	})))
	if err := NewCaiyunAPI(client).TokenPKAcceptInvite("hello+world"); err != nil {
		t.Fatal(err)
	}
}

func TestTokenPKAcceptInviteFallsBackOnlyAfterNotFound(t *testing.T) {
	var paths []string
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		paths = append(paths, req.URL.Path)
		if req.URL.Path == "/ycloud/tokenpk/invite/accept" {
			return mailTestResponse(req, 404, `not found`), nil
		}
		return mailTestResponse(req, 200, `{"code":0,"msg":"success"}`), nil
	})))
	if err := NewCaiyunAPI(client).TokenPKAcceptInvite("code"); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[1] != "/ycloud/tokenpk/invite/acceptInvite" {
		t.Fatalf("fallback paths = %v", paths)
	}
}

func TestTokenPKAcceptInviteFallsBackOnBusinessNotFound(t *testing.T) {
	var attempts int
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return mailTestResponse(req, 200, `{"code":404,"msg":"not found"}`), nil
		}
		if req.URL.Path != "/ycloud/tokenpk/invite/acceptInvite" {
			t.Fatalf("wrong fallback endpoint: %s", req.URL.Path)
		}
		return mailTestResponse(req, 200, `{"code":0,"msg":"success"}`), nil
	})))
	if err := NewCaiyunAPI(client).TokenPKAcceptInvite("code"); err != nil || attempts != 2 {
		t.Fatalf("fallback result attempts=%d err=%v", attempts, err)
	}
}

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
