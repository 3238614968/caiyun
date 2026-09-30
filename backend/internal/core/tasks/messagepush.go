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

	if err := rewardResponseError("获取消息推送状态", resp); err != nil {
		return err
	}

	// 解析结果
	if resp.Result == nil {
		t.logger.Info("消息推送功能未开启")
		return nil
	}

	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		return fmt.Errorf("解析消息推送状态失败")
	}

	// 获取状态
	pushOn := responseNumber(resultMap["pushOn"])
	onDuration := 0
	secondTaskStatus := responseNumber(resultMap["secondTaskStatus"])

	if val, ok := resultMap["onDuaration"]; ok && val != nil {
		onDuration = responseNumber(val)
	} else if val, ok := resultMap["onDuration"]; ok && val != nil {
		onDuration = responseNumber(val)
	}

	// 检查是否开启
	if pushOn == 0 {
		t.logger.Error("消息通知已关闭，请前往 APP 手动打开")
		return nil
	}

	// 检查首次奖励
	firstTaskStatus := responseNumber(resultMap["firstTaskStatus"])
	// 三种奖励分别检查，未达标的连续天数任务不提交领奖请求。
	pushTaskStatus := responseNumber(resultMap["pushTaskStatus"])
	for kind, status := range []int{firstTaskStatus, secondTaskStatus, pushTaskStatus} {
		kind++
		if status != 2 {
			continue
		}
		obtainResp, err := t.api.ObtainMsgPushOnType(kind)
		if err != nil {
			t.logger.Error(fmt.Sprintf("领取消息通知奖励类型%d失败", kind), err)
			return err
		}
		if err := rewardResponseError(fmt.Sprintf("消息通知奖励类型%d领取", kind), obtainResp); err != nil {
			return err
		}
		result, ok := obtainResp.Result.(map[string]interface{})
		if ok && responseNumber(result["obtainCode"]) == 1 {
			t.logger.Success(fmt.Sprintf("消息通知奖励类型%d已领取", kind))
		} else {
			t.logger.Info(fmt.Sprintf("消息通知奖励类型%d暂无可领取奖励", kind))
		}
	}

	t.logger.Info(fmt.Sprintf("已经开启 %d 天", onDuration))
	return nil
}
