package tasks

// 真实账号手动联调 harness。默认完全跳过，只有显式设置环境变量时才运行：
//
//	CAIYUN_MANUAL_AUTH="Basic <base64>"  提供账号凭据（必填，否则跳过）
//	CAIYUN_MANUAL_RUN=1                  真正执行动作（上传/对话/抽奖等写操作）
//
// 只设置 CAIYUN_MANUAL_AUTH 时仅做只读探测，用于验证响应解析与鉴权链路。

import (
	"os"
	"strings"
	"testing"

	"caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// TestManualCameraBackupDiag 打印 AI 相机与备份快照链路的真实响应，
// 用于定位 backup/aiCamera 未计入服务端状态的原因（只读凭据 + 探测）。
func TestManualCameraBackupDiag(t *testing.T) {
	acct, ok := loadManualAccount(t)
	if !ok {
		return
	}
	if !manualRunEnabled() {
		t.Skip("需要 CAIYUN_MANUAL_RUN=1 才会执行真实动作")
	}

	apiClient := api.NewCaiyunAPI(acct.client)
	fileAPI := api.NewFileAPI(acct.client)

	t.Logf("userDomainId=%s", acct.userDomain)

	if err := fileAPI.TriggerAlbumBackup(); err != nil {
		t.Errorf("TriggerAlbumBackup: %v", err)
	} else {
		t.Logf("TriggerAlbumBackup ok")
	}

	apiClient.PrepareActivitySession(api.TokenPKMarketName)
	if err := apiClient.TokenPKStepClick(3, "aiCamera"); err != nil {
		t.Logf("tokenpk aiCamera click: %v", err)
	}
	if err := apiClient.CompleteAICameraTask(); err != nil {
		t.Errorf("CompleteAICameraTask: %v", err)
	} else {
		t.Logf("CompleteAICameraTask ok")
	}
}
type manualAccount struct {
	client     *http.Client
	phone      string
	rawToken   string
	jwtToken   string
	ssoToken   string
	userDomain string
}

func loadManualAccount(t *testing.T) (*manualAccount, bool) {
	t.Helper()

	raw := strings.TrimSpace(os.Getenv("CAIYUN_MANUAL_AUTH"))
	if raw == "" {
		t.Skip("未设置 CAIYUN_MANUAL_AUTH，跳过真实账号联调")
	}

	info, err := auth.ParseToken(raw)
	if err != nil || info == nil || info.Phone == "" {
		t.Fatalf("解析 CAIYUN_MANUAL_AUTH 失败: %v", err)
	}

	client := http.NewClient()
	client.SetMarketAccount(info.Phone)
	client.SetAuth(info.Auth)

	authClient := http.NewClient()
	authClient.SetAuth(info.Auth)
	authMgr := auth.NewAuth(authClient)

	jwtToken, ssoToken, err := authMgr.GetJWTTokenWithSSOToken(info.Phone)
	if err != nil {
		t.Fatalf("获取 JWT/SSO token 失败: %v", err)
	}
	if jwtToken == "" {
		t.Fatalf("JWT token 为空，账号凭据可能已失效")
	}
	client.SetJWTToken(jwtToken)
	if ssoToken != "" {
		client.SetSSOToken(ssoToken)
	}

	t.Logf("认证就绪: phone=%s userDomainId=%s sso=%t", info.Phone, client.GetUserDomainID(), ssoToken != "")

	return &manualAccount{
		client:     client,
		phone:      info.Phone,
		rawToken:   info.Token,
		jwtToken:   jwtToken,
		ssoToken:   ssoToken,
		userDomain: client.GetUserDomainID(),
	}, true
}

func manualRunEnabled() bool {
	return strings.TrimSpace(os.Getenv("CAIYUN_MANUAL_RUN")) == "1"
}

func TestManualActivityProbe(t *testing.T) {
	acct, ok := loadManualAccount(t)
	if !ok {
		return
	}
	log := logger.NewLogger(logger.LevelInfo)
	_ = log
	apiClient := api.NewCaiyunAPI(acct.client)

	t.Run("tokenpk", func(t *testing.T) {
		apiClient.PrepareActivitySession(api.TokenPKMarketName)
		tasks, err := apiClient.TokenPKTaskList()
		if err != nil {
			t.Fatalf("TokenPKTaskList: %v", err)
		}
		for _, task := range tasks {
			t.Logf("  [tokenpk] id=%d state=%-8s key=%-12s %s | prizes=%s", task.ID, task.State, task.StepKey(), task.Name, task.PrizeSummary())
		}
		usedToken, stages, err := apiClient.TokenPKProgressQueryHome()
		if err != nil {
			t.Errorf("TokenPKProgressQueryHome: %v", err)
		} else {
			t.Logf("  [tokenpk] usedToken=%d stages=%d", usedToken, len(stages))
		}
	})

	t.Run("makewish", func(t *testing.T) {
		apiClient.PrepareActivitySession(api.MakeWishMarketName)
		home, err := apiClient.MakeWishHome()
		if err != nil {
			t.Fatalf("MakeWishHome: %v", err)
		}
		wish := "未许愿"
		if home.Wish != nil {
			wish = home.Wish.PrizeName
		}
		t.Logf("  [makewish] period=%s canEarn=%t wish=%s codes=%d", home.Config.Period, home.Config.CanEarnCodes, wish, home.RaffleCodes.TotalCount)

		tasks, needAdvance, err := apiClient.MakeWishTaskList()
		if err != nil {
			t.Errorf("MakeWishTaskList: %v", err)
		}
		for _, task := range tasks {
			t.Logf("  [makewish] id=%d %-18s %-8s %s", task.TaskID, task.TaskCode, task.TaskState, task.TaskName)
		}
		t.Logf("  [makewish] needAutoAdvance=%t", needAdvance)

		prizes, err := apiClient.MakeWishPrizeList()
		if err != nil {
			t.Errorf("MakeWishPrizeList: %v", err)
		} else {
			t.Logf("  [makewish] prizeCount=%d sample=%v", len(prizes), firstPrizeSample(prizes))
		}
	})

	t.Run("funai", func(t *testing.T) {
		apiClient.PrepareActivitySession(api.FunAIMarketName)
		module, err := apiClient.FunaiPageModule()
		if err != nil {
			t.Fatalf("FunaiPageModule: %v", err)
		}
		t.Logf("  [funai] moduleId=%s subtitle=%s tasks=%d", module.ModuleID, module.Subtitle, len(module.TaskIDs))
		for _, task := range module.TaskIDs {
			t.Logf("  [funai] id=%-14s complete=%d sign=%d %s", task.ID, task.IsComplete, task.TaskSign, task.TaskName)
		}
		timed, err := apiClient.FunaiTimedModules()
		if err != nil {
			t.Errorf("FunaiTimedModules: %v", err)
		} else {
			t.Logf("  [funai] timedModules=%d", len(timed))
		}
	})

	t.Run("poster", func(t *testing.T) {
		apiClient.PrepareActivitySession(api.PosterMarketName)
		tasks, err := apiClient.NewYearTaskList(api.PosterMarketName)
		if err != nil {
			t.Fatalf("NewYearTaskList: %v", err)
		}
		for _, task := range tasks {
			t.Logf("  [poster] id=%d state=%-8s key=%-8s flag=%-14s %s", task.ID, task.State, task.StepKey(), task.Flag, task.Name)
		}
		chances, err := apiClient.NewYearLotteryCount(api.PosterMarketName)
		if err != nil {
			t.Errorf("NewYearLotteryCount: %v", err)
		} else {
			t.Logf("  [poster] lotteryChances=%d", chances)
		}
	})
}

func TestManualActivityRun(t *testing.T) {
	acct, ok := loadManualAccount(t)
	if !ok {
		return
	}
	if !manualRunEnabled() {
		t.Skip("未设置 CAIYUN_MANUAL_RUN=1，跳过真实写操作")
	}

	log := logger.NewLogger(logger.LevelInfo)

	t.Run("token_pk", func(t *testing.T) {
		task := NewTokenPKTask(acct.client, log)
		task.SetStorage(NewMemoryStore()).SetAccountContext(acct.phone, acct.rawToken)
		if err := task.Run(); err != nil {
			t.Errorf("TokenPKTask.Run: %v", err)
		}
		t.Logf("token_pk message: %s", task.Message())
	})

	t.Run("make_wish", func(t *testing.T) {
		task := NewMakeWishTask(acct.client, log)
		task.SetStorage(NewMemoryStore()).SetAccountContext(acct.phone, acct.rawToken)
		if err := task.Run(); err != nil {
			t.Errorf("MakeWishTask.Run: %v", err)
		}
		t.Logf("make_wish message: %s", task.Message())
	})

	t.Run("fun_ai", func(t *testing.T) {
		task := NewFunAITask(acct.client, log)
		task.SetStorage(NewMemoryStore()).SetAccountContext(acct.phone, acct.rawToken)
		if err := task.Run(); err != nil {
			t.Errorf("FunAITask.Run: %v", err)
		}
		t.Logf("fun_ai message: %s", task.Message())
	})

	t.Run("poster_activity", func(t *testing.T) {
		task := NewPosterTask(acct.client, log)
		task.SetStorage(NewMemoryStore()).SetAccountContext(acct.phone, acct.rawToken)
		if err := task.Run(); err != nil {
			t.Errorf("PosterTask.Run: %v", err)
		}
		t.Logf("poster_activity message: %s", task.Message())
	})
}

func firstPrizeSample(prizes []api.MakeWishPrize) string {
	if len(prizes) == 0 {
		return "-"
	}
	return prizes[0].PrizeName
}
