package tasks

import (
	"fmt"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// SignInTask 签到任务
type SignInTask struct {
	client *http.Client
	logger *logger.Logger
	api    *api.CaiyunAPI
}

// NewSignInTask 创建签到任务
func NewSignInTask(client *http.Client, logger *logger.Logger) *SignInTask {
	return &SignInTask{
		client: client,
		logger: logger,
		api:    api.NewCaiyunAPI(client),
	}
}

// Run 执行签到任务
func (t *SignInTask) Run() error {
	cloudInfoBefore, err := t.api.GetCloudInfo()
	if err != nil {
		t.logger.Warn("获取签到前云朵信息失败:", err)
	} else if cloudInfoBefore.IsSuccess() {
		t.logger.Info(fmt.Sprintf("当前云朵%d", cloudInfoBefore.Result.Total))
		if cloudInfoBefore.Result.ToReceive > 0 {
			t.logger.Info(fmt.Sprintf("待领取%d", cloudInfoBefore.Result.ToReceive))
		}
		if cloudInfoBefore.Result.TodaySigned() {
			t.logger.Info("网盘今日已签到")
			if cloudInfoBefore.Result.NextMonthGet > 0 {
				t.logger.Info(fmt.Sprintf("下月可领取%d个云朵", cloudInfoBefore.Result.NextMonthGet))
			}
			return nil
		}
	}

	time.Sleep(1 * time.Second)
	signInResp, err := t.api.SignIn()
	if err != nil {
		t.logger.Error("执行签到失败:", err)
		t.logger.Debug("详细错误:", err.Error())
		return err
	}
	if !signInResp.IsSuccess() {
		msg := signInResp.MessageText()
		if msg == "" {
			msg = "unknown error"
		}
		t.logger.Fail(fmt.Sprintf("签到失败: code=%s, msg=%s", signInResp.CodeText(), msg))
		return fmt.Errorf("签到失败: code=%s, msg=%s", signInResp.CodeText(), msg)
	}

	if signInResp.Result.TodaySigned() && signInResp.Result.SignInPoints > 0 {
		t.logger.Success(fmt.Sprintf("网盘签到成功，获得%d云朵", signInResp.Result.SignInPoints))
	} else if signInResp.Result.TodaySigned() {
		t.logger.Success("网盘签到成功")
	} else {
		latestInfo, latestErr := t.api.GetCloudInfo()
		if latestErr != nil {
			t.logger.Warn("签到后复查状态失败:", latestErr)
		} else if latestInfo != nil && latestInfo.IsSuccess() && latestInfo.Result.TodaySigned() {
			t.logger.Success("网盘签到成功")
		} else {
			msg := signInResp.MessageText()
			if msg == "" {
				msg = "unknown error"
			}
			t.logger.Fail(fmt.Sprintf("签到失败: code=%s, msg=%s", signInResp.CodeText(), msg))
			return fmt.Errorf("签到失败: code=%s, msg=%s", signInResp.CodeText(), msg)
		}
	}

	cloudInfoAfter, err := t.api.GetCloudInfo()
	if err != nil {
		t.logger.Warn("获取签到后云朵信息失败:", err)
		return nil
	}
	if !cloudInfoAfter.IsSuccess() {
		msg := cloudInfoAfter.MessageText()
		if msg == "" {
			msg = "unknown error"
		}
		t.logger.Warn(fmt.Sprintf("获取签到后云朵信息失败: code=%s, msg=%s", cloudInfoAfter.CodeText(), msg))
		return nil
	}

	t.logger.Info(fmt.Sprintf("当前云朵%d", cloudInfoAfter.Result.Total))
	if cloudInfoAfter.Result.ToReceive > 0 {
		t.logger.Info(fmt.Sprintf("待领取%d", cloudInfoAfter.Result.ToReceive))
	}
	if cloudInfoAfter.Result.NextMonthGet > 0 {
		t.logger.Info(fmt.Sprintf("下月可领取%d个云朵", cloudInfoAfter.Result.NextMonthGet))
	}

	return nil
}
