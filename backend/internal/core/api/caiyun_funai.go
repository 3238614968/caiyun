package api

import (
	"encoding/json"
	"fmt"
)

// 趣玩AI赢好礼（National_playAI）：完成各 AI 工具体验任务累计抽奖次数，
// page/lottery 按模块抽奖。

// FunaiTask 趣玩AI 任务项。
type FunaiTask struct {
	ID         string `json:"id"`
	TaskName   string `json:"taskName"`
	TaskDesc   string `json:"taskDesc"`
	IsComplete int    `json:"isComplete"`
	TaskSign   int    `json:"taskSign"`
	InnerLink  string `json:"innerLink"`
}

// FunaiModule 趣玩AI 模块（常驻模块或限时模块）。
type FunaiModule struct {
	ModuleID string      `json:"moduleId"`
	Subtitle string      `json:"subtitle"`
	Flag     int         `json:"flag"`
	TaskIDs  []FunaiTask `json:"taskIds"`
}

// FunaiLotteryPrize 抽奖结果。
type FunaiLotteryPrize struct {
	PrizeID   int    `json:"prizeId"`
	PrizeName string `json:"prizeName"`
	IsWin     int    `json:"isWin"`
}

// FunaiPageModule 获取常驻模块（含任务完成情况）。
func (api *CaiyunAPI) FunaiPageModule() (*FunaiModule, error) {
	return api.funaiGetModule("/funai/api/page/getmodule")
}

// FunaiTimedModules 获取限时活动模块列表。
func (api *CaiyunAPI) FunaiTimedModules() ([]FunaiModule, error) {
	body, err := api.activityGetBody(FunAIMarketName, MobileMarketURL+"/funai/api/timed/getmodule")
	if err != nil {
		return nil, err
	}
	envelope, err := decodeActivityBody("获取趣玩AI限时模块", body)
	if err != nil {
		return nil, err
	}
	var modules []FunaiModule
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &modules); err != nil {
			return nil, fmt.Errorf("解析趣玩AI限时模块失败: %w", err)
		}
	}
	return modules, nil
}

func (api *CaiyunAPI) funaiGetModule(endpoint string) (*FunaiModule, error) {
	body, err := api.activityGetBody(FunAIMarketName, MobileMarketURL+endpoint)
	if err != nil {
		return nil, err
	}
	envelope, err := decodeActivityBody("获取趣玩AI模块", body)
	if err != nil {
		return nil, err
	}
	var module FunaiModule
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &module); err != nil {
			return nil, fmt.Errorf("解析趣玩AI模块失败: %w", err)
		}
	}
	return &module, nil
}

// FunaiRegister 登记任务参与（抓包验证：page/register {moduleId,taskId}）。
func (api *CaiyunAPI) FunaiRegister(moduleID, taskID string) error {
	body, err := api.activityPostBody(FunAIMarketName, MobileMarketURL+"/funai/api/page/register", map[string]interface{}{
		"moduleId": moduleID,
		"taskId":   taskID,
	})
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("趣玩AI任务登记", body)
	return err
}

// FunaiLottery 在指定模块抽奖一次。
func (api *CaiyunAPI) FunaiLottery(moduleID string) (*FunaiLotteryPrize, error) {
	body, err := api.activityPostBody(FunAIMarketName, MobileMarketURL+"/funai/api/page/lottery", map[string]interface{}{
		"moduleId": moduleID,
	})
	if err != nil {
		return nil, err
	}
	envelope, err := decodeActivityBody("趣玩AI抽奖", body)
	if err != nil {
		return nil, err
	}
	var prize FunaiLotteryPrize
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &prize); err != nil {
			return nil, fmt.Errorf("解析趣玩AI抽奖结果失败: %w", err)
		}
	}
	return &prize, nil
}
