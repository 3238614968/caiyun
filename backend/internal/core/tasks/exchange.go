package tasks

import (
	"encoding/json"
	"fmt"

	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// ExchangeMonthlyCardTask 兑换月卡任务
type ExchangeMonthlyCardTask struct {
	client *http.Client
	logger *logger.Logger
}

// NewExchangeMonthlyCardTask 创建兑换月卡任务
func NewExchangeMonthlyCardTask(client *http.Client, log *logger.Logger) *ExchangeMonthlyCardTask {
	return &ExchangeMonthlyCardTask{
		client: client,
		logger: log,
	}
}

// Run 执行兑换月卡任务
func (t *ExchangeMonthlyCardTask) Run() error {
	t.logger.Start("------【兑换月卡】------")

	// 第一步：获取兑换列表
	exchangeList, err := t.getExchangeList()
	if err != nil {
		t.logger.Error("获取兑换列表失败", err)
		return err
	}

	if len(exchangeList) == 0 {
		t.logger.Info("兑换列表为空")
		return nil
	}

	// 第二步：遍历兑换列表，尝试兑换
	for _, item := range exchangeList {
		// 跳过已兑换的
		if item.Status == 2 {
			t.logger.Info(fmt.Sprintf("%s，本月已经兑换过了", item.Name))
			continue
		}
		if item.Status == 4 {
			t.logger.Info(fmt.Sprintf("%s，本月本组已兑换，下月再来", item.Name))
			continue
		}
		if item.DailyRemainderCount <= 0 {
			t.logger.Info(fmt.Sprintf("%s，今日已经兑换完", item.Name))
			continue
		}
		if item.YearRemainderCount <= 0 {
			t.logger.Info(fmt.Sprintf("%s，本年度已经兑换完", item.Name))
			continue
		}

		// 执行兑换
		if err := t.exchange(item.PrizeID, item.Name); err != nil {
			t.logger.Error(fmt.Sprintf("兑换%s失败", item.Name), err)
		}
	}

	return nil
}

// ExchangeItem 兑换项
type ExchangeItem struct {
	PrizeID             string `json:"prizeId"`
	Name                string `json:"prizeName"`
	DailyRemainderCount int    `json:"dailyRemainderCount"`
	YearRemainderCount  int    `json:"yearRemainderCount"`
	Status              int    `json:"status"` // 0-可兑换 2-本月已兑换 4-本组已兑换
	GroupID             int    `json:"groupId"`
}

// getExchangeList 获取兑换列表
func (t *ExchangeMonthlyCardTask) getExchangeList() ([]ExchangeItem, error) {
	url := "https://mrp.mcloud.139.com/market/signin/page/exchangeList"
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := t.client.Get(url, headers)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code    int                        `json:"code"`
		Message string                     `json:"msg"`
		Result  map[string][]ExchangeItem  `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("获取兑换列表失败: code=%d, msg=%s", result.Code, result.Message)
	}

	// 合并所有分组的兑换项
	var items []ExchangeItem
	for _, group := range result.Result {
		items = append(items, group...)
	}

	return items, nil
}

// exchange 兑换
func (t *ExchangeMonthlyCardTask) exchange(prizeID, prizeName string) error {
	url := fmt.Sprintf("https://mrp.mcloud.139.com/market/signin/page/exchange?prizeId=%s&client=app&clientVersion=12.0.1&smsCode=", prizeID)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := t.client.Get(url, headers)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	switch result.Code {
	case 0:
		t.logger.Success(fmt.Sprintf("兑换%s成功，第三方会员请在本月底之前手动领取", prizeName))
		return nil
	case 2301:
		t.logger.Info(fmt.Sprintf("本月已经兑换过%s了", prizeName))
		return nil
	case 610:
		return fmt.Errorf("兑换%s失败，%d，%s，请手动登录 APP 后重试", prizeName, result.Code, result.Message)
	default:
		return fmt.Errorf("兑换%s失败: code=%d, msg=%s", prizeName, result.Code, result.Message)
	}
}
