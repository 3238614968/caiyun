package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// 算力大作战（National_TokenPK）接口封装。任务状态机：
// WAIT/ING(去做) -> SUCCESS(领奖励) -> FINISH(已完成)。

// ActivityTask 是 tokenpk/newyear 两个活动共用的任务结构。
type ActivityTask struct {
	ID        int                    `json:"id"`
	Name      string                 `json:"name"`
	State     string                 `json:"state"`
	CurrStep  int                    `json:"currstep"`
	LimitType string                 `json:"limitType"`
	Flag      string                 `json:"flag"`
	Prizes    []ActivityTaskPrize    `json:"prizes"`
	Button    map[string]interface{} `json:"button"`
}

// ActivityTaskPrize 任务奖励描述。
type ActivityTaskPrize struct {
	PrizeID   int     `json:"prizeId"`
	PrizeType string  `json:"prizeType"`
	PrizeName string  `json:"prizeName"`
	Amount    float64 `json:"amount"`
}

// StepKey 返回“去完成”按钮对应的步骤 key（如 backup/aiCamera）。
// 平台按钮内容一致，优先 android，回退任意平台。
func (t ActivityTask) StepKey() string {
	for _, platform := range []string{"android", "app", "other", "harmony", "ios"} {
		if key := buttonExt(t.Button[platform]); key != "" {
			return key
		}
	}
	for _, value := range t.Button {
		if key := buttonExt(value); key != "" {
			return key
		}
	}
	return ""
}

// StepLink 返回按钮跳转链接（openUrl 类任务需要真实访问）。
func (t ActivityTask) StepLink() string {
	for _, platform := range []string{"android", "app", "other", "harmony", "ios"} {
		if link := buttonLink(t.Button[platform]); link != "" {
			return link
		}
	}
	return ""
}

func buttonExt(value interface{}) string {
	item, ok := value.(map[string]interface{})
	if !ok {
		return ""
	}
	ext, _ := item["ext"].(string)
	return strings.TrimSpace(ext)
}

func buttonLink(value interface{}) string {
	item, ok := value.(map[string]interface{})
	if !ok {
		return ""
	}
	link, _ := item["link"].(string)
	return strings.TrimSpace(link)
}

func (t ActivityTask) PrizeSummary() string {
	if len(t.Prizes) == 0 {
		return ""
	}
	names := make([]string, 0, len(t.Prizes))
	for _, prize := range t.Prizes {
		if strings.TrimSpace(prize.PrizeName) != "" {
			names = append(names, prize.PrizeName)
		}
	}
	return strings.Join(names, "、")
}

func (api *CaiyunAPI) activityTaskList(marketName, endpoint string) ([]ActivityTask, error) {
	query := url.Values{}
	query.Set("marketName", marketName)
	query.Set("platform", "android")
	query.Set("sortState", "true")
	body, err := api.activityGetBody(marketName, MobileMarketURL+endpoint+"?"+query.Encode())
	if err != nil {
		return nil, err
	}
	envelope, err := decodeActivityBody("获取活动任务列表", body)
	if err != nil {
		return nil, err
	}
	var tasks []ActivityTask
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &tasks); err != nil {
			return nil, fmt.Errorf("解析活动任务列表失败: %w", err)
		}
	}
	return tasks, nil
}

// TokenPKTaskList 获取算力大作战任务列表。
func (api *CaiyunAPI) TokenPKTaskList() ([]ActivityTask, error) {
	return api.activityTaskList(TokenPKMarketName, "/tokenpk/task/list")
}

// TokenPKStepClick 上报任务“去完成”点击，使服务端开始跟踪该步骤。
func (api *CaiyunAPI) TokenPKStepClick(taskID int, key string) error {
	return api.activityStepClick(TokenPKMarketName, "/tokenpk/task/step/click", taskID, key)
}

// TokenPKStepReserve 预约类任务（如下月登录）。
func (api *CaiyunAPI) TokenPKStepReserve(taskID int, key string) error {
	body, err := api.activityPostBody(TokenPKMarketName, MobileMarketURL+"/tokenpk/task/step/reserve", map[string]interface{}{
		"marketName": TokenPKMarketName,
		"taskId":     taskID,
		"key":        key,
		"source":     "app",
	})
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("算力大作战预约任务", body)
	return err
}

// TokenPKReceivePrize 领取已完成任务的奖励。
func (api *CaiyunAPI) TokenPKReceivePrize(taskID int) error {
	return api.activityReceivePrize(TokenPKMarketName, "/tokenpk/task/step/receivePrize", taskID)
}

// activityStepClick/activityReceivePrize 供 tokenpk 与 newyear 复用。
func (api *CaiyunAPI) activityStepClick(marketName, endpoint string, taskID int, key string) error {
	body, err := api.activityPostBody(marketName, MobileMarketURL+endpoint, map[string]interface{}{
		"marketName": marketName,
		"taskId":     taskID,
		"key":        key,
		"source":     "app",
	})
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("活动任务点击", body)
	return err
}

func (api *CaiyunAPI) activityReceivePrize(marketName, endpoint string, taskID int) error {
	body, err := api.activityPostBody(marketName, MobileMarketURL+endpoint, map[string]interface{}{
		"marketName": marketName,
		"taskId":     taskID,
		"source":     "app",
	})
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("活动任务领奖", body)
	return err
}

// TokenPKRewardStage 算力累计阶段奖励。
type TokenPKRewardStage struct {
	PhaseNo        int    `json:"phaseNo"`
	PhaseName      string `json:"phaseName"`
	TokenThreshold int    `json:"tokenThreshold"`
	RewardName     string `json:"rewardName"`
	RewardType     string `json:"rewardType"`
	Status         int    `json:"status"`
}

// TokenPKProgressQueryHome 查询算力进度与阶段奖励状态。
func (api *CaiyunAPI) TokenPKProgressQueryHome() (int64, []TokenPKRewardStage, error) {
	body, err := api.activityGetBody(TokenPKMarketName, MobileMarketURL+"/tokenpk/toplist/progress/queryHome")
	if err != nil {
		return 0, nil, err
	}
	envelope, err := decodeActivityBody("查询算力进度", body)
	if err != nil {
		return 0, nil, err
	}
	var progress struct {
		UsedToken    int64                `json:"usedToken"`
		RewardStages []TokenPKRewardStage `json:"rewardStages"`
	}
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &progress); err != nil {
			return 0, nil, fmt.Errorf("解析算力进度失败: %w", err)
		}
	}
	return progress.UsedToken, progress.RewardStages, nil
}

// TokenPKAutoReceiveLotteryChance 自动领取达标产生的抽奖次数。
func (api *CaiyunAPI) TokenPKAutoReceiveLotteryChance() (int, error) {
	body, err := api.activityPostBody(TokenPKMarketName, MobileMarketURL+"/tokenpk/toplist/progress/autoReceiveLotteryChance", nil)
	if err != nil {
		return 0, err
	}
	envelope, err := decodeActivityBody("自动领取抽奖次数", body)
	if err != nil {
		return 0, err
	}
	var result struct {
		TotalChance int `json:"totalChance"`
		RewardCount int `json:"rewardCount"`
	}
	if len(envelope.Result) > 0 {
		_ = json.Unmarshal(envelope.Result, &result)
	}
	return result.RewardCount, nil
}

// TokenPKGenerateInviteCode 生成活动邀请码（分享类任务的辅助信息）。
func (api *CaiyunAPI) TokenPKGenerateInviteCode() (string, error) {
	body, err := api.activityGetBody(TokenPKMarketName, MobileMarketURL+"/tokenpk/invite/generateInviteCode")
	if err != nil {
		return "", err
	}
	envelope, err := decodeActivityBody("生成活动邀请码", body)
	if err != nil {
		return "", err
	}
	var code string
	if len(envelope.Result) > 0 {
		_ = json.Unmarshal(envelope.Result, &code)
	}
	return code, nil
}
