package tasks

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestAccountTaskSettingsNeverReuseGlobalVerificationCode(t *testing.T) {
	t.Setenv("CAIYUN_PRIZE_SMS_CODE", "global-code")
	t.Setenv("CAIYUN_PRIZE_SMS_CODE_13800000001", "account-one-code")
	if accountTaskSetting("CAIYUN_PRIZE_SMS_CODE", "13800000001") != "account-one-code" || accountTaskSetting("CAIYUN_PRIZE_SMS_CODE", "13800000002") != "" {
		t.Fatal("verification code is not isolated by account")
	}
}

func TestRewardErrorsDoNotConvertAuthFailuresIntoSuccess(t *testing.T) {
	for _, code := range []interface{}{"401", float64(401), nil, ""} {
		if rewardResponseError("test", &api.CaiyunResponse{Code: code}, 411, 604) == nil {
			t.Fatalf("invalid/auth code was skipped: %v", code)
		}
	}
	if err := rewardResponseError("test", &api.CaiyunResponse{Code: "411"}, 411, 604); err != nil {
		t.Fatal(err)
	}
}

func TestPrizeClaimUsesConfiguredOIDWithoutSendingAnotherSMS(t *testing.T) {
	t.Setenv("CAIYUN_PRIZE_SMS_CODE_13800000001", "123456")
	t.Setenv("CAIYUN_PRIZE_OID_13800000001", "prize-two")
	called := false
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/ycloud/prize/api/acceptPrizeV2" {
			t.Fatalf("unexpected SMS or claim request: %s", req.URL.Path)
		}
		body, _ := io.ReadAll(req.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil || payload["oid"] != "prize-two" || payload["smsCode"] != "e10adc3949ba59abbe56e057f20f883e" {
			t.Fatalf("claim payload=%s err=%v", body, err)
		}
		called = true
		return hiddenRewardReply(req, `{"code":0}`), nil
	})))
	task := NewPrizeCenterTask(client, nil)
	task.SetAccountPhone("13800000001")
	parts := []string{}
	if err := task.claimFirstUnclaimed([]api.PrizeCenterEntry{{OID: "prize-one"}, {OID: "prize-two"}}, &parts); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("configured prize was not claimed")
	}
	task.SetAccountPhone("13800000002")
	called = false
	if err := task.claimFirstUnclaimed([]api.PrizeCenterEntry{{OID: "prize-two"}}, &parts); err != nil || called {
		t.Fatalf("other account reused code: called=%v err=%v", called, err)
	}
}

func TestNoticeTaskReportsOnlyExplicitAccountState(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "unset", true: "actual-state-zero"}[configured], func(t *testing.T) {
			t.Setenv("CAIYUN_APP_NOTICE_STATUS", "1")
			if configured {
				t.Setenv("CAIYUN_APP_NOTICE_STATUS_13800000001", "0")
			}
			reported := false
			client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/ycloud/signin/page/reportAppNoticeStatus":
					if !configured || req.URL.Query().Get("status") != "0" {
						t.Fatal("device notice status was fabricated")
					}
					reported = true
				case "/ycloud/signin/emailStatus/currEmailStatus":
					return hiddenRewardReply(req, `{"code":0,"result":true}`), nil
				default:
					t.Fatalf("unexpected notice request: %s", req.URL.Path)
				}
				return hiddenRewardReply(req, `{"code":0}`), nil
			})))
			task := NewNoticeSwitchTask(client, nil)
			task.SetAccountPhone("13800000001")
			if err := task.Run(); err != nil {
				t.Fatal(err)
			}
			if reported != configured {
				t.Fatalf("reported=%v want=%v", reported, configured)
			}
		})
	}
}

func TestInvalidBackupStatusNeverWritesState(t *testing.T) {
	t.Setenv("CAIYUN_ALBUM_BACKUP_STATUS_13800000001", "invalid")
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "synStatus") {
			t.Fatal("invalid config was converted into backup status zero")
		}
		return hiddenRewardReply(req, `{"resultCode":200,"resultData":{"exist":false}}`), nil
	})))
	task := NewAlbumBackupReportTask(client, nil).SetAccountContext("13800000001", "auth")
	if err := task.Run(); err == nil {
		t.Fatal("invalid backup status was accepted")
	}
}

func TestFunAIRunUsesServerCompletionAndPreservesPartialFailure(t *testing.T) {
	for _, partialFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted-but-pending", true: "partial-failure"}[partialFailure], func(t *testing.T) {
			moduleReads := 0
			client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/ycloud/funai/api/page/getmodule":
					if req.URL.Query().Get("platform") != "android" || req.URL.Query().Get("appVersion") != "13.2.2" {
						t.Fatal("missing FunAI platform parameters")
					}
					moduleReads++
					complete := 0
					if partialFailure && moduleReads > 1 {
						complete = 1
					}
					body := `{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"003","isComplete":0}]}}`
					if partialFailure {
						if complete == 1 {
							body = `{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"003","isComplete":1},{"id":"007","isComplete":0}]}}`
						} else {
							body = `{"code":0,"result":{"moduleId":"main","taskIds":[{"id":"003","isComplete":0},{"id":"007","isComplete":0}]}}`
						}
					}
					return hiddenRewardReply(req, body), nil
				case "/ycloud/funai/api/timed/getmodule":
					return hiddenRewardReply(req, `{"code":0,"result":[]}`), nil
				case "/api/image/edit/portraitMatting":
					return hiddenRewardReply(req, `{"code":"0000","data":{"taskId":"accepted-1"}}`), nil
				case "/api/image/avatar/cartoon":
					return hiddenRewardReply(req, `{"code":500,"message":"algorithm unavailable"}`), nil
				case "/ycloud/funai/api/page/lottery":
					return hiddenRewardReply(req, `{"code":500,"msg":"没有抽奖次数"}`), nil
				case "/api/outer/async/task/result":
					t.Fatal("render status must not override activity completion")
				}
				return hiddenRewardReply(req, `{"code":0}`), nil
			})))
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"test-user"}}`))
			client.SetJWTToken("h." + claims + ".s")
			client.SetDeviceID("test-device")
			task := NewFunAITask(client, logger.NewLogger(logger.LevelFatal))
			err := task.Run()
			if partialFailure {
				if err == nil {
					t.Fatal("one completed action hid another action's failure")
				}
			} else if err != nil || !task.Pending() {
				t.Fatalf("accepted but unconfirmed should be pending: err=%v pending=%v", err, task.Pending())
			}
		})
	}
}
