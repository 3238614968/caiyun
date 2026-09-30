package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 趣玩AI赢好礼（National_playAI）：完成各 AI 工具体验任务累计抽奖次数，
// page/lottery 按模块抽奖。
//
// 另有邮箱版 National_playAI139mail：与云盘版共用同一后端，
// 仅 page/register、page/lottery、invite/* 可用，任务表与云盘版一致。

// FunAIMailMarketName 趣玩AI 邮箱版活动标识。
const FunAIMailMarketName = "National_playAI139mail"

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
	return api.FunaiPageModuleForMarket(FunAIMarketName)
}

// FunaiPageModuleForMarket 获取指定活动标识下的常驻模块。
func (api *CaiyunAPI) FunaiPageModuleForMarket(marketName string) (*FunaiModule, error) {
	return api.funaiGetModule(marketName, "/funai/api/page/getmodule?platform=android&appVersion=13.2.2")
}

// FunaiTimedModules 获取限时活动模块列表。
func (api *CaiyunAPI) FunaiTimedModules() ([]FunaiModule, error) {
	return api.FunaiTimedModulesForMarket(FunAIMarketName)
}

// FunaiTimedModulesForMarket 获取指定活动标识下的限时模块列表。
func (api *CaiyunAPI) FunaiTimedModulesForMarket(marketName string) ([]FunaiModule, error) {
	body, err := api.activityGetBody(marketName, MobileMarketURL+"/funai/api/timed/getmodule?platform=android")
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

// FunaiTimedTranscriptionCompleted records the timed voice transcription task.
func (api *CaiyunAPI) FunaiTimedTranscriptionCompleted(moduleID, taskID string) error {
	if moduleID == "" || taskID == "" {
		return fmt.Errorf("趣玩 AI 限时任务缺少模块或任务 ID")
	}
	body, err := api.activityPostBody(FunAIMarketName,
		MobileMarketURL+"/funai/api/timed/transcriptioncompleted",
		map[string]string{"moduleId": moduleID, "taskId": taskID})
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("完成趣玩 AI 录音转写", body)
	if err != nil && strings.Contains(err.Error(), "该任务已完成") {
		return nil
	}
	return err
}

func (api *CaiyunAPI) funaiGetModule(marketName, endpoint string) (*FunaiModule, error) {
	if strings.TrimSpace(marketName) == "" {
		marketName = FunAIMarketName
	}
	body, err := api.activityGetBody(marketName, MobileMarketURL+endpoint)
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
	if strings.TrimSpace(module.ModuleID) == "" {
		return nil, fmt.Errorf("趣玩 AI 模块缺少 moduleId")
	}
	return &module, nil
}

// FunaiRegister 登记任务参与（抓包验证：page/register {moduleId,taskId}）。
func (api *CaiyunAPI) FunaiRegister(moduleID, taskID string) error {
	return api.FunaiRegisterForMarket(FunAIMarketName, moduleID, taskID)
}

// FunaiRegisterForMarket 在指定活动标识下登记任务参与。
func (api *CaiyunAPI) FunaiRegisterForMarket(marketName, moduleID, taskID string) error {
	if strings.TrimSpace(marketName) == "" {
		marketName = FunAIMarketName
	}
	body, err := api.activityPostBody(marketName, MobileMarketURL+"/funai/api/page/register", map[string]interface{}{
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
	return api.FunaiLotteryForMarket(FunAIMarketName, moduleID)
}

// FunaiLotteryForMarket 在指定活动标识下的模块抽奖一次。
func (api *CaiyunAPI) FunaiLotteryForMarket(marketName, moduleID string) (*FunaiLotteryPrize, error) {
	if strings.TrimSpace(marketName) == "" {
		marketName = FunAIMarketName
	}
	body, err := api.activityPostBody(marketName, MobileMarketURL+"/funai/api/page/lottery", map[string]interface{}{
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

// FunaiInviteCode 生成趣玩AI 邀请码（result 是纯字符串）。
func (api *CaiyunAPI) FunaiInviteCode() (string, error) {
	body, err := api.activityGetBody(FunAIMarketName, MobileMarketURL+"/funai/api/invite/getcode")
	if err != nil {
		return "", err
	}
	envelope, err := decodeActivityBody("获取趣玩AI邀请码", body)
	if err != nil {
		return "", err
	}
	code := strings.Trim(strings.TrimSpace(string(envelope.Result)), `"`)
	if code == "" || code == "null" {
		return "", fmt.Errorf("趣玩AI 邀请码为空")
	}
	return code, nil
}

// FunaiAcceptInvite 接受趣玩AI 邀请。分享链接里参数叫 assistCode，
// 但请求体字段名是 inviteCode（活动侧原始命名）。
func (api *CaiyunAPI) FunaiAcceptInvite(inviteCode string) error {
	code := strings.TrimSpace(inviteCode)
	if code == "" {
		return fmt.Errorf("趣玩AI 邀请码为空")
	}
	body, err := api.activityPostBody(FunAIMarketName, MobileMarketURL+"/funai/api/invite/accept", map[string]interface{}{
		"inviteCode": code,
	})
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("趣玩AI助力", body)
	return err
}
