package api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// 校园海报/新年 AI 特别活动（National_PlayAISpecial，newyear 接口族）：
// 完成任务获得抽奖次数，newyear/blindbox/lottery 抽奖。

// NewYearTaskList 获取活动任务列表（与 tokenpk 相同结构）。
func (api *CaiyunAPI) NewYearTaskList(marketName string) ([]ActivityTask, error) {
	query := url.Values{}
	query.Set("marketName", marketName)
	query.Set("source", "app")
	body, err := api.activityGetBody(marketName, MobileMarketURL+"/newyear/task/list?"+query.Encode())
	if err != nil {
		return nil, err
	}
	envelope, err := decodeActivityBody("获取校园海报任务列表", body)
	if err != nil {
		return nil, err
	}
	var tasks []ActivityTask
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &tasks); err != nil {
			return nil, fmt.Errorf("解析校园海报任务列表失败: %w", err)
		}
	}
	return tasks, nil
}

// NewYearLotteryCount 查询当前可用抽奖次数。
func (api *CaiyunAPI) NewYearLotteryCount(marketName string) (int, error) {
	query := url.Values{}
	query.Set("marketName", marketName)
	body, err := api.activityGetBody(marketName, MobileMarketURL+"/newyear/user/info?"+query.Encode())
	if err != nil {
		return 0, err
	}
	envelope, err := decodeActivityBody("查询校园海报抽奖次数", body)
	if err != nil {
		return 0, err
	}
	var result struct {
		TotalLotteryCount int `json:"totalLotteryCount"`
	}
	if len(envelope.Result) > 0 {
		_ = json.Unmarshal(envelope.Result, &result)
	}
	return result.TotalLotteryCount, nil
}

// NewYearStepClick 上报任务步骤点击。
func (api *CaiyunAPI) NewYearStepClick(marketName string, taskID int, key string) error {
	return api.activityStepClick(marketName, "/newyear/task/step/click", taskID, key)
}

// NewYearLottery 活动抽奖一次，返回奖品名。
func (api *CaiyunAPI) NewYearLottery(marketName string) (string, error) {
	body, err := api.activityPostBody(marketName, MobileMarketURL+"/newyear/blindbox/lottery", map[string]interface{}{
		"source":     "app",
		"marketName": marketName,
	})
	if err != nil {
		return "", err
	}
	envelope, err := decodeActivityBody("校园海报活动抽奖", body)
	if err != nil {
		return "", err
	}
	var result struct {
		Prize struct {
			PrizeID   int    `json:"prizeId"`
			PrizeName string `json:"prizeName"`
		} `json:"prize"`
	}
	if len(envelope.Result) > 0 {
		_ = json.Unmarshal(envelope.Result, &result)
	}
	return result.Prize.PrizeName, nil
}
