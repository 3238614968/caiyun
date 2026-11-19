package tasks

import (
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func checkRewardEligibility(t *testing.T, info, endpoint string, wantClaims int, run func(*corehttp.Client) error) {
	t.Helper()
	t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "")
	claims := 0
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/signin/page/infoV3":
			return hiddenRewardReply(req, info), nil
		case endpoint:
			claims++
			return hiddenRewardReply(req, `{"code":0,"msg":"claimed"}`), nil
		case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling":
			return hiddenRewardReply(req, `{"code":0}`), nil
		default:
			if strings.Contains(req.URL.Path, "/deviceprofile/") {
				return hiddenRewardReply(req, `{"code":1}`), nil
			}
			t.Fatalf("unexpected eligibility request: %s", req.URL)
			return nil, nil
		}
	})))
	client.SetDeviceID("Bfixture-device")
	if err := run(client); err != nil {
		t.Fatalf("eligibility check must preserve the claim fallback: %v", err)
	}
	if claims != wantClaims {
		t.Fatalf("claim requests = %d, want %d for info %s", claims, wantClaims, info)
	}
}

func TestCloudMultipleEligibilityRequiresValidInfo(t *testing.T) {
	for _, tc := range []struct {
		name, info string
		wantClaims int
	}{
		{"eligible", `{"code":0,"result":{"curMonthBackup":true,"curMonthBackupSignAccept":false}}`, 1},
		{"no backup", `{"code":0,"result":{"curMonthBackup":false,"curMonthBackupSignAccept":false}}`, 0},
		{"already claimed", `{"code":0,"result":{"curMonthBackup":true,"curMonthBackupSignAccept":true}}`, 0},
		{"numeric flags", `{"code":0,"result":{"curMonthBackup":1,"curMonthBackupSignAccept":0}}`, 1},
		{"string flags", `{"code":"0","result":{"curMonthBackup":"0","curMonthBackupSignAccept":"0"}}`, 0},
		{"business failure", `{"code":500,"result":{"curMonthBackup":false,"curMonthBackupSignAccept":true}}`, 1},
		{"conflicting success flag", `{"code":500,"success":true,"result":{"curMonthBackup":false,"curMonthBackupSignAccept":true}}`, 1},
		{"missing backup", `{"code":0,"result":{"curMonthBackupSignAccept":false}}`, 1},
		{"missing claim flag", `{"code":0,"result":{"curMonthBackup":false}}`, 1},
		{"empty result", `{"code":0,"result":{}}`, 1},
		{"null flag", `{"code":0,"result":{"curMonthBackup":null,"curMonthBackupSignAccept":false}}`, 1},
		{"invalid flag", `{"code":0,"result":{"curMonthBackup":"unknown","curMonthBackupSignAccept":false}}`, 1},
		{"malformed response", `not json`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkRewardEligibility(t, tc.info, "/ycloud/signin/page/multiple", tc.wantClaims, func(client *corehttp.Client) error {
				return NewCloudMultipleTask(client, logger.NewLogger(logger.LevelFatal)).Run()
			})
		})
	}
}

func TestRevivalRewardEligibilityRequiresValidInfo(t *testing.T) {
	for _, tc := range []struct {
		name, info string
		wantClaims int
	}{
		{"has card", `{"code":0,"result":{"cardInfo":1}}`, 1},
		{"no card", `{"code":0,"result":{"cardInfo":0}}`, 0},
		{"string count", `{"code":"0","result":{"cardInfo":"0"}}`, 0},
		{"business failure", `{"code":500,"result":{"cardInfo":0}}`, 1},
		{"conflicting success flag", `{"code":500,"success":true,"result":{"cardInfo":0}}`, 1},
		{"missing card count", `{"code":0,"result":{}}`, 1},
		{"null card count", `{"code":0,"result":{"cardInfo":null}}`, 1},
		{"invalid card count", `{"code":0,"result":{"cardInfo":"unknown"}}`, 1},
		{"fractional card count", `{"code":0,"result":{"cardInfo":0.5}}`, 1},
		{"malformed response", `not json`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkRewardEligibility(t, tc.info, "/ycloud/signin/page/receiveRevivalReward", tc.wantClaims, func(client *corehttp.Client) error {
				return NewRevivalRewardTask(client, logger.NewLogger(logger.LevelFatal)).Run()
			})
		})
	}
}
