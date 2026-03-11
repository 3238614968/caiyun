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
	// 获取签到信息
	signInInfo, err := t.api.SignIn()
	if err != nil {
		t.logger.Error("获取签到信息失败:", err)
		t.logger.Debug("详细错误:", err.Error())
		return err
	}

	if signInInfo.Code != 0 {
		msg := signInInfo.Msg
		if msg == "" {
			msg = signInInfo.Message
		}
		if msg == "" {
			msg = "未知错误"
		}
		t.logger.Fail(fmt.Sprintf("获取签到信息失败: code=%d, msg=%s", signInInfo.Code, msg))
		return fmt.Errorf("获取签到信息失败: code=%d, msg=%s", signInInfo.Code, msg)
	}

	// 显示云朵信息
	t.logger.Info(fmt.Sprintf("当前云朵%d", signInInfo.Result.Total))
	if signInInfo.Result.ToReceive > 0 {
		t.logger.Info(fmt.Sprintf("待领取%d", signInInfo.Result.ToReceive))
	}

	// 检查是否已经签到
	if signInInfo.Result.TodaySignIn {
		t.logger.Info("网盘今日已签到")
	} else {
		// 再次调用签到接口触发签到
		time.Sleep(1 * time.Second)
		newInfo, err := t.api.SignIn()
		if err == nil && newInfo.Code == 0 {
			if newInfo.Result.TodaySignIn {
				t.logger.Success("网盘签到成功")
			} else {
				t.logger.Fail("网盘签到失败")
			}
		}
	}

	// 显示下月可领取云朵
	if signInInfo.Result.NextMonthGet > 0 {
		t.logger.Info(fmt.Sprintf("下月可领取%d个云朵", signInInfo.Result.NextMonthGet))
	}

	return nil
}
