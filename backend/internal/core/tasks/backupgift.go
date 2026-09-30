package tasks

import (
	"fmt"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// BackupGiftTask 备份好礼任务
type BackupGiftTask struct {
	client *http.Client
	logger *logger.Logger
	api    *api.CaiyunAPI
}

// NewBackupGiftTask 创建备份好礼任务
func NewBackupGiftTask(client *http.Client, log *logger.Logger) *BackupGiftTask {
	return &BackupGiftTask{
		client: client,
		logger: log,
		api:    api.NewCaiyunAPI(client),
	}
}

// Run 执行备份好礼任务
func (t *BackupGiftTask) Run() error {
	// 获取备份好礼信息
	resp, err := t.api.GetBackupGift()
	if err != nil {
		t.logger.Error("获取备份好礼信息失败", err)
		return err
	}

	if err := rewardResponseError("获取备份好礼", resp); err != nil {
		return err
	}

	// 解析结果
	if resp.Result == nil {
		t.logger.Info("备份好礼功能未开启")
		return nil
	}

	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		return fmt.Errorf("解析备份好礼状态失败")
	}

	// 获取状态
	state := 0
	curMonth := 0

	if val, ok := resultMap["state"]; ok && val != nil {
		if f, ok := val.(float64); ok {
			state = int(f)
		}
	}

	if val, ok := resultMap["curMonth"]; ok && val != nil {
		if f, ok := val.(float64); ok {
			curMonth = int(f)
		}
	}

	if curMonth == 0 {
		t.logger.Debug("本月未开启备份")
		return nil
	}

	// 根据状态处理
	switch state {
	case -1:
		t.logger.Warn("未开启备份，请前往 APP 手动开启")
		return nil

	case 1:
		t.logger.Success("本月已领取")
		return nil

	case 0:
		t.logger.Info("领取备份奖励")
		receiveResp, err := t.api.ReceiveBackupGift()
		if err != nil {
			t.logger.Error("领取备份奖励失败", err)
			return err
		}

		if err := rewardResponseError("领取备份奖励", receiveResp, 504); err != nil {
			return err
		}
		if responseCodeIs(receiveResp, 504) {
			t.logger.Info("本月备份奖励已领取")
			return nil
		}
		if responseCodeIs(receiveResp, 0) {
			// 尝试获取奖励数量
			if receiveResp.Result != nil {
				if reward, ok := receiveResp.Result.(float64); ok {
					t.logger.Success(fmt.Sprintf("领取成功，获得云朵 %d", int(reward)))
					return nil
				}
			}
			t.logger.Success("领取成功")
		}
		return nil

	default:
		t.logger.Warn(fmt.Sprintf("未知状态 %d", state))
		return nil
	}
}
