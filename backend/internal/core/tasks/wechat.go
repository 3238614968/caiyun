package tasks

import (
	"fmt"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// WeChatTask 微信任务
type WeChatTask struct {
	client *http.Client
	logger *logger.Logger
	api    *api.CaiyunAPI
}

// NewWeChatTask 创建微信任务
func NewWeChatTask(client *http.Client, logger *logger.Logger) *WeChatTask {
	return &WeChatTask{
		client: client,
		logger: logger,
		api:    api.NewCaiyunAPI(client),
	}
}

// RunSignIn 执行微信签到
func (t *WeChatTask) RunSignIn() error {
	resp, err := t.api.SignInWx()
	if err != nil {
		t.logger.Error("微信签到失败:", err)
		return err
	}

	// 检查响应
	code := 0
	switch v := resp.Code.(type) {
	case int:
		code = v
	case float64:
		code = int(v)
	case string:
		if v == "0" {
			code = 0
		}
	}

	if code == 0 {
		t.logger.Success("微信签到成功")
		return nil
	}

	// 常见错误：账号未绑定微信公众号
	msg := resp.Message
	if msg == "" {
		msg = resp.Msg
	}
	if msg == "" {
		msg = "未知错误"
	}
	t.logger.Fail("微信签到失败")
	t.logger.Fail("当前账号没有绑定微信公众号【中国移动云盘】")
	return fmt.Errorf("微信签到失败: %s", msg)
}

// RunDraw 执行微信抽奖（每天只抽一次，因为只有第一次免费）
func (t *WeChatTask) RunDraw(configTimes int) error {
	return t.RunDrawWithInterval(configTimes, 500*time.Millisecond)
}

// RunDrawWithInterval 按配置次数和间隔执行微信抽奖
func (t *WeChatTask) RunDrawWithInterval(configTimes int, interval time.Duration) error {
	if configTimes <= 0 {
		return nil
	}
	if interval < 0 {
		interval = 0
	}

	// 先获取抽奖信息
	drawInfo, err := t.api.GetDrawInWx()
	if err != nil {
		t.logger.Error("获取微信抽奖信息失败:", err)
		return err
	}

	// 检查是否可以抽奖
	code := 0
	switch v := drawInfo.Code.(type) {
	case int:
		code = v
	case float64:
		code = int(v)
	case string:
		if v == "0" {
			code = 0
		}
	}

	if code != 0 {
		msg := drawInfo.Message
		if msg == "" {
			msg = drawInfo.Msg
		}
		t.logger.Info(fmt.Sprintf("无法抽奖: %s", msg))
		return nil
	}

	// 解析剩余次数
	surplusNumber := 0
	if drawInfo.Result != nil {
		if resultMap, ok := drawInfo.Result.(map[string]interface{}); ok {
			if surplus, ok := resultMap["surplusNumber"].(float64); ok {
				surplusNumber = int(surplus)
			}
		}
	}

	// JS 逻辑：need = configTimes - (50 - surplusNumber)
	alreadyDrawn := 50 - surplusNumber
	needDrawTimes := configTimes - alreadyDrawn

	if needDrawTimes < 1 {
		t.logger.Info(fmt.Sprintf("剩余微信抽奖次数%d，已完成", surplusNumber))
		return nil
	}

	t.logger.Info(fmt.Sprintf("剩余微信抽奖次数%d", surplusNumber))

	// 执行抽奖
	for i := 0; i < needDrawTimes; i++ {
		resp, err := t.api.WxDraw()
		if err != nil {
			t.logger.Error("微信抽奖失败:", err)
			continue
		}

		code := 0
		switch v := resp.Code.(type) {
		case int:
			code = v
		case float64:
			code = int(v)
		case string:
			if v == "0" {
				code = 0
			}
		}

		if code == 0 {
			// 解析奖励信息
			prizeName := ""
			if resp.Result != nil {
				if resultMap, ok := resp.Result.(map[string]interface{}); ok {
					if name, ok := resultMap["prizeName"].(string); ok {
						prizeName = name
					}
				}
			}
			if prizeName != "" {
				t.logger.Success(fmt.Sprintf("微信抽奖成功，获得【%s】", prizeName))
			} else {
				t.logger.Success("微信抽奖成功")
			}
		} else {
			msg := resp.Message
			if msg == "" {
				msg = resp.Msg
			}
			t.logger.Fail(fmt.Sprintf("微信抽奖失败: %s", msg))
		}

		if i < needDrawTimes-1 && interval > 0 {
			time.Sleep(interval)
		}
	}

	return nil
}
