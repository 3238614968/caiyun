package tasks

import (
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// CloudMultipleTask claims the new sign-in center cloud multiple reward.
type CloudMultipleTask struct {
	logger      *logger.Logger
	api         *api.CaiyunAPI
	lastMessage string
}

func NewCloudMultipleTask(client *http.Client, log *logger.Logger) *CloudMultipleTask {
	return &CloudMultipleTask{
		logger: log,
		api:    api.NewCaiyunAPI(client),
	}
}

func (t *CloudMultipleTask) Run() error {
	resp, err := t.api.CloudMultiple()
	if err != nil {
		t.logger.Error("云朵翻倍失败", err)
		return err
	}
	if resp == nil {
		err = fmt.Errorf("云朵翻倍失败: 响应为空")
		t.logger.Error("云朵翻倍失败", err)
		return err
	}

	msg := firstNonEmpty(strings.TrimSpace(resp.Msg), strings.TrimSpace(resp.Message))
	if resp.IsSuccess() {
		if result, ok := resp.Result.(map[string]interface{}); ok {
			multiple := toInt(result["multiple"])
			cloudCount := toInt(result["cloudCount"])
			if multiple > 0 && cloudCount > 0 {
				msg = fmt.Sprintf("云朵翻倍成功: %d%%，获得%d云朵", multiple, cloudCount)
			}
		}
		if msg == "" {
			msg = "云朵翻倍执行成功"
		}
		t.lastMessage = msg
		t.logger.Success(msg)
		return nil
	}

	if isCloudMultipleAlreadyClaimed(msg) {
		t.lastMessage = msg
		t.logger.Success(msg)
		return nil
	}

	if msg == "" {
		msg = fmt.Sprintf("code=%v", resp.Code)
	}
	t.lastMessage = msg
	err = fmt.Errorf("云朵翻倍失败: code=%v, msg=%s", resp.Code, msg)
	t.logger.Error("云朵翻倍失败", err)
	return err
}

func (t *CloudMultipleTask) Message() string {
	return strings.TrimSpace(t.lastMessage)
}

func isCloudMultipleAlreadyClaimed(message string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(message), " ", "")
	return strings.Contains(normalized, "已领取")
}
