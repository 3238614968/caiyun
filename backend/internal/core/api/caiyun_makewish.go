package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// 全网许愿赢好礼（National_MakeWish）：选奖品许愿获得首张抽奖码，
// 完成任务（auto-advance 上报）累积更多抽奖码，月底按大乐透开奖。

// MakeWishPrize 可许愿奖品。
type MakeWishPrize struct {
	MakeWishPrizeID int    `json:"makewishPrizeId"`
	PrizeID         int    `json:"prizeId"`
	PrizeName       string `json:"prizeName"`
}

// MakeWishTask 许愿活动任务。
type MakeWishTask struct {
	TaskID          int    `json:"taskId"`
	TaskCode        string `json:"taskCode"`
	TaskName        string `json:"taskName"`
	TaskState       string `json:"taskState"`
	GroupID         string `json:"groupId"`
	TaskDescription string `json:"taskDescription"`
	AutoAdvance     bool   `json:"autoAdvanceable"`
	ClickAdvance    bool   `json:"clickAdvance"`
}

// MakeWishRaffleCode 已获得抽奖码。
type MakeWishRaffleCode struct {
	CodeID     int    `json:"codeId"`
	RaffleCode string `json:"raffleCode"`
	Source     string `json:"source"`
}

// MakeWishHomeResult 活动首页状态。
type MakeWishHomeResult struct {
	Config struct {
		Period       string `json:"period"`
		CanEarnCodes bool   `json:"canEarnCodes"`
		Title        string `json:"title"`
	} `json:"config"`
	Wish *struct {
		WishID        int    `json:"wishId"`
		MakeWishPrize int    `json:"makewishPrizeId"`
		PrizeName     string `json:"prizeName"`
	} `json:"wish"`
	RaffleCodes struct {
		Period     string               `json:"period"`
		TotalCount int                  `json:"totalCount"`
		Codes      []MakeWishRaffleCode `json:"codes"`
	} `json:"raffleCodes"`
}

const makewishVersion = "13.2.2"

// MakeWishCloudExchangeEligibility reports whether the optional 300 AI-bean
// exchange is currently available to this account.
func (api *CaiyunAPI) MakeWishCloudExchangeEligibility() (bool, error) {
	body, err := api.activityGetBody(MakeWishMarketName, MobileMarketURL+"/makewish/task/cloud-exchange/info?platform=android")
	if err != nil {
		return false, err
	}
	envelope, err := decodeActivityBody("查询许愿 AI 豆兑换", body)
	if err != nil {
		return false, err
	}
	var result struct {
		CanExchange bool `json:"canExchange"`
	}
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			return false, err
		}
	}
	return result.CanExchange, nil
}

func (api *CaiyunAPI) ExchangeMakeWishCloud() error {
	body, err := api.activityPostBody(MakeWishMarketName, MobileMarketURL+"/makewish/task/cloud-exchange", map[string]string{
		"appVersion": makewishVersion, "platform": "android", "channel": "app",
	})
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("兑换许愿 AI 豆", body)
	return err
}

func makewishQuery() string {
	return "?platform=android&appVersion=" + makewishVersion + "&channel=app"
}

// MakeWishHome 查询许愿状态与抽奖码。
func (api *CaiyunAPI) MakeWishHome() (*MakeWishHomeResult, error) {
	body, err := api.activityGetBody(MakeWishMarketName, MobileMarketURL+"/makewish/page/home")
	if err != nil {
		return nil, err
	}
	envelope, err := decodeActivityBody("查询许愿活动首页", body)
	if err != nil {
		return nil, err
	}
	var result MakeWishHomeResult
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			return nil, fmt.Errorf("解析许愿活动首页失败: %w", err)
		}
	}
	return &result, nil
}

// MakeWishPrizeList 获取可选奖品列表。
func (api *CaiyunAPI) MakeWishPrizeList() ([]MakeWishPrize, error) {
	body, err := api.activityGetBody(MakeWishMarketName, MobileMarketURL+"/makewish/prize/list")
	if err != nil {
		return nil, err
	}
	envelope, err := decodeActivityBody("获取许愿奖品列表", body)
	if err != nil {
		return nil, err
	}
	var prizes []MakeWishPrize
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &prizes); err != nil {
			return nil, fmt.Errorf("解析许愿奖品列表失败: %w", err)
		}
	}
	return prizes, nil
}

// MakeWish 许愿指定奖品（首次许愿即产生抽奖码）。
func (api *CaiyunAPI) MakeWish(makeWishPrizeID int) error {
	body, err := api.activityPostBody(MakeWishMarketName, MobileMarketURL+"/makewish/wish", map[string]interface{}{
		"makewishPrizeId": makeWishPrizeID,
	})
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("许愿", body)
	return err
}

// MakeWishTaskList 获取许愿任务列表。
func (api *CaiyunAPI) MakeWishTaskList() ([]MakeWishTask, bool, error) {
	body, err := api.activityGetBody(MakeWishMarketName, MobileMarketURL+"/makewish/task/list"+makewishQuery())
	if err != nil {
		return nil, false, err
	}
	envelope, err := decodeActivityBody("获取许愿任务列表", body)
	if err != nil {
		return nil, false, err
	}
	var result struct {
		NeedAutoAdvance bool           `json:"needAutoAdvance"`
		Tasks           []MakeWishTask `json:"tasks"`
	}
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			return nil, false, fmt.Errorf("解析许愿任务列表失败: %w", err)
		}
	}
	return result.Tasks, result.NeedAutoAdvance, nil
}

// MakeWishPageClick registers a task step after the corresponding real action.
func (api *CaiyunAPI) MakeWishPageClick(taskID int) error {
	if taskID <= 0 {
		return fmt.Errorf("许愿任务 ID 必须大于 0")
	}
	body, err := api.activityPostBody(MakeWishMarketName, MobileMarketURL+"/makewish/task/page/click", map[string]interface{}{
		"platform":   "android",
		"appVersion": makewishVersion,
		"channel":    "app",
		"taskId":     taskID,
	})
	if err != nil {
		return err
	}
	_, err = decodeActivityBody("登记许愿任务步骤", body)
	return err
}

func (api *CaiyunAPI) MakeWishGenerateShareCode() (string, error) {
	body, err := api.activityPostBody(MakeWishMarketName, MobileMarketURL+"/makewish/share/generate", nil)
	if err != nil {
		return "", err
	}
	envelope, err := decodeActivityBody("生成许愿活动分享码", body)
	if err != nil {
		return "", err
	}
	var result struct {
		ShareCode string `json:"shareCode"`
	}
	if err := json.Unmarshal(envelope.Result, &result); err != nil || result.ShareCode == "" {
		return "", fmt.Errorf("许愿分享码缺失: %v", err)
	}
	return result.ShareCode, nil
}

func (api *CaiyunAPI) MakeWishCanAssist(shareCode string) (bool, error) {
	if strings.TrimSpace(shareCode) == "" {
		return false, fmt.Errorf("许愿分享码为空")
	}
	body, err := api.activityGetBody(MakeWishMarketName, MobileMarketURL+"/makewish/share/page?shareCode="+url.QueryEscape(shareCode))
	if err != nil {
		return false, err
	}
	envelope, err := decodeActivityBody("查询许愿助力资格", body)
	if err != nil {
		return false, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return false, err
	}
	return boolFromAny(result["canAssist"]), nil
}

func (api *CaiyunAPI) MakeWishAssist(shareCode string) error {
	if strings.TrimSpace(shareCode) == "" {
		return fmt.Errorf("许愿分享码为空")
	}
	body, err := api.activityPostBody(MakeWishMarketName, MobileMarketURL+"/makewish/share/access", map[string]string{"shareCode": shareCode})
	if err != nil {
		return err
	}
	envelope, err := decodeActivityBody("许愿活动助力", body)
	if err != nil {
		return err
	}
	var result struct {
		AssistStatus string `json:"assistStatus"`
	}
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			return err
		}
	}
	if result.AssistStatus != "" && !strings.EqualFold(result.AssistStatus, "SUCCESS") {
		return fmt.Errorf("许愿助力状态: %s", result.AssistStatus)
	}
	return nil
}

// MakeWishAutoAdvance 上报可自动完成的任务（advancedTaskIDs 为空时由服务端
// 检查基础任务，非空时检查指定的进阶任务）。
func (api *CaiyunAPI) MakeWishAutoAdvance(advancedTaskIDs []int) ([]MakeWishTask, error) {
	payload := map[string]interface{}{
		"platform":   "android",
		"appVersion": makewishVersion,
		"channel":    "app",
	}
	if len(advancedTaskIDs) > 0 {
		payload["advancedTasks"] = advancedTaskIDs
	}
	body, err := api.activityPostBody(MakeWishMarketName, MobileMarketURL+"/makewish/task/auto-advance", payload)
	if err != nil {
		return nil, err
	}
	envelope, err := decodeActivityBody("推进许愿任务", body)
	if err != nil {
		return nil, err
	}
	var result struct {
		AdvancedTasks []MakeWishTask `json:"advancedTasks"`
	}
	if len(envelope.Result) > 0 {
		if err := json.Unmarshal(envelope.Result, &result); err != nil {
			return nil, fmt.Errorf("解析许愿任务推进结果失败: %w", err)
		}
	}
	return result.AdvancedTasks, nil
}

// MakeWishCloudExchangeInfo 查询 AI 豆兑换抽奖码余量。
func (api *CaiyunAPI) MakeWishCloudExchangeInfo() (cost int, remaining int, err error) {
	body, err := api.activityGetBody(MakeWishMarketName, MobileMarketURL+"/makewish/task/cloud-exchange/info"+makewishQuery())
	if err != nil {
		return 0, 0, err
	}
	envelope, decodeErr := decodeActivityBody("查询豆兑换信息", body)
	if decodeErr != nil {
		return 0, 0, decodeErr
	}
	var result struct {
		Cost      json.Number `json:"cost"`
		Remaining int         `json:"remaining"`
	}
	if len(envelope.Result) > 0 {
		_ = json.Unmarshal(envelope.Result, &result)
	}
	costValue, _ := strconv.Atoi(result.Cost.String())
	return costValue, result.Remaining, nil
}

// SelectMakeWishPrize 按关键字挑选愿望奖品；未匹配时返回第一个奖品。
func SelectMakeWishPrize(prizes []MakeWishPrize, keyword string) *MakeWishPrize {
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		for i := range prizes {
			if strings.Contains(prizes[i].PrizeName, keyword) {
				return &prizes[i]
			}
		}
	}
	if len(prizes) > 0 {
		return &prizes[0]
	}
	return nil
}
