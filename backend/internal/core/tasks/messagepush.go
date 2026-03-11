package tasks

import (
	"fmt"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// MessagePushRewardTask 消息推送奖励任务
type MessagePushRewardTask struct {
	client *http.Client
	logger *logger.Logger
	api    *api.CaiyunAPI
}

// NewMessagePushRewardTask 创建消息推送奖励任务
func NewMessagePushRewardTask(client *http.Client, log *logger.Logger) *MessagePushRewardTask {
	return &MessagePushRewardTask{
		client: client,
		logger: log,
		api:    api.NewCaiyunAPI(client),
	}
}

// Run 执行消息推送奖励任务
func (t *MessagePushRewardTask) Run() error {
	// 获取消息推送状态
	resp, err := t.api.GetMsgPushStatus()
	if err != nil {
		t.logger.Error("获取消息通知状态失败", err)
		return err
	}

	// 解析响应
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

	if code != 0 {
		msg := resp.Message
		if msg == "" {
			msg = resp.Msg
		}
		t.logger.Error("获取消息推送状态失败", fmt.Errorf("code=%d, msg=%s", code, msg))
		return fmt.Errorf("获取消息推送状态失败: code=%d, msg=%s", code, msg)
	}

	// 解析结果
	if resp.Result == nil {
		t.logger.Info("消息推送功能未开启")
		return nil
	}

	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.logger.Error("解析消息推送状态失败")
		return nil
	}

	// 获取状态
	pushOn := 0
	onDuration := 0
	secondTaskStatus := 0

	if val, ok := resultMap["pushOn"]; ok && val != nil {
		if f, ok := val.(float64); ok {
			pushOn = int(f)
		}
	}

	if val, ok := resultMap["onDuration"]; ok && val != nil {
		if f, ok := val.(float64); ok {
			onDuration = int(f)
		}
	}

	if val, ok := resultMap["secondTaskStatus"]; ok && val != nil {
		if f, ok := val.(float64); ok {
			secondTaskStatus = int(f)
		}
	}

	// 检查是否开启
	if pushOn == 0 {
		t.logger.Error("消息通知已关闭，请前往 APP 手动打开")
		return nil
	}

	// 检查首次奖励
	firstTaskStatus := 0
	if val, ok := resultMap["firstTaskStatus"]; ok && val != nil {
		if f, ok := val.(float64); ok {
			firstTaskStatus = int(f)
		}
	}
	if firstTaskStatus != 3 {
		t.logger.Info("首次奖励未领取，请前往 APP 手动完成")
	}

	// 检查并领取奖励
	if secondTaskStatus == 2 {
		t.logger.Info("领取奖励")
		obtainResp, err := t.api.ObtainMsgPushOn()
		if err != nil {
			t.logger.Error("领取消息通知奖励失败", err)
			return err
		}

		obtainCode := 0
		switch v := obtainResp.Code.(type) {
		case int:
			obtainCode = v
		case float64:
			obtainCode = int(v)
		case string:
			if v == "0" {
				obtainCode = 0
			}
		}

		if obtainCode == 0 {
			t.logger.Success("领取成功")
		}
		return nil
	}

	t.logger.Info(fmt.Sprintf("已经开启 %d 天", onDuration))
	return nil
}
