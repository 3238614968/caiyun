package tasks

import (
	"fmt"
	"strconv"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// RevivalRewardTask claims the monthly revival card reward when available.
type RevivalRewardTask struct {
	logger      *logger.Logger
	api         *api.CaiyunAPI
	lastMessage string
}

func NewRevivalRewardTask(client *http.Client, log *logger.Logger) *RevivalRewardTask {
	return &RevivalRewardTask{
		logger: log,
		api:    api.NewCaiyunAPI(client),
	}
}

func (t *RevivalRewardTask) Run() error {
	// 只有查询成功且明确未持有复活卡时才跳过；无效或缺失状态保留领取兜底。
	if info, infoErr := t.api.CloudPendingInfo(); infoErr == nil && responseCodeIs(info, 0) {
		if result, ok := info.Result.(map[string]interface{}); ok {
			count, countErr := strconv.Atoi(fmt.Sprint(result["cardInfo"]))
			if countErr == nil && count == 0 {
				t.lastMessage = "复活卡跳过：未持有复活卡"
				t.logger.Info(t.lastMessage)
				return nil
			}
		}
	}
	resp, err := t.api.ReceiveRevivalReward()
	if err != nil {
		t.logger.Error("领取复活卡奖励失败", err)
		return err
	}
	if resp == nil {
		err = fmt.Errorf("领取复活卡奖励失败: 响应为空")
		t.logger.Error("领取复活卡奖励失败", err)
		return err
	}

	msg := firstNonEmpty(strings.TrimSpace(resp.Msg), strings.TrimSpace(resp.Message))
	if resp.IsSuccess() {
		if msg == "" {
			msg = "复活卡奖励领取成功"
		}
		t.lastMessage = msg
		t.logger.Success(msg)
		return nil
	}

	if isRevivalRewardAlreadyClaimed(msg) {
		t.lastMessage = msg
		t.logger.Success(msg)
		return nil
	}

	if msg == "" {
		msg = fmt.Sprintf("code=%v", resp.Code)
	}
	t.lastMessage = msg
	err = fmt.Errorf("领取复活卡奖励失败: code=%v, msg=%s", resp.Code, msg)
	t.logger.Error("领取复活卡奖励失败", err)
	return err
}

func (t *RevivalRewardTask) Message() string {
	return strings.TrimSpace(t.lastMessage)
}

func isRevivalRewardAlreadyClaimed(message string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(message), " ", "")
	return strings.Contains(normalized, "复活卡") && strings.Contains(normalized, "已领取")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
