package tasks

import (
	"encoding/json"
	"fmt"

	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/utils"
)

// GardenCheckinTask 果园签到任务
type GardenCheckinTask struct {
	client *http.Client
	logger *logger.Logger
}

// NewGardenCheckinTask 创建果园签到任务
func NewGardenCheckinTask(client *http.Client, log *logger.Logger) *GardenCheckinTask {
	return &GardenCheckinTask{
		client: client,
		logger: log,
	}
}

// Run 执行果园签到任务
func (t *GardenCheckinTask) Run() error {
	t.logger.Start("------【果园签到】------")

	// 第一步：初始化果园
	treeInfo, err := t.initTree()
	if err != nil {
		t.logger.Error("初始化果园失败", err)
		return err
	}

	t.logger.Info(fmt.Sprintf("拥有%d级果树，当前水滴%d", treeInfo.TreeLevel, treeInfo.CollectWater))

	// 第二步：获取签到信息
	checkinInfo, err := t.getCheckinInfo()
	if err != nil {
		t.logger.Error("获取签到信息失败", err)
		return err
	}

	// 检查是否已签到
	if checkinInfo == true {
		t.logger.Info("今日果园已签到")
		return nil
	}

	// 第三步：执行签到
	if err := t.checkin(); err != nil {
		t.logger.Error("果园签到失败", err)
		return err
	}

	// 第四步：领取场景水滴
	if err := t.claimSceneWater(); err != nil {
		t.logger.Error("领取场景水滴失败", err)
	}

	// 第五步：浇水
	if err := t.watering(); err != nil {
		t.logger.Error("浇水失败", err)
	}

	return nil
}

// TreeInfo 果树信息
type TreeInfo struct {
	NickName     string `json:"nickName"`
	UID          string `json:"uid"`
	TreeLevel    int    `json:"treeLevel"`
	CollectWater int    `json:"collectWater"`
}

// initTree 初始化果园
func (t *GardenCheckinTask) initTree() (*TreeInfo, error) {
	url := "https://happy.mail.10086.cn/jsp/cn/garden/user/initTree.do"
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
		Code         int    `json:"code"`
		Message      string `json:"msg"`
		NickName     string `json:"nickName"`
		UID          string `json:"uid"`
		TreeLevel    int    `json:"treeLevel"`
		CollectWater int    `json:"collectWater"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &TreeInfo{
		NickName:     result.NickName,
		UID:          result.UID,
		TreeLevel:    result.TreeLevel,
		CollectWater: result.CollectWater,
	}, nil
}

// getCheckinInfo 获取签到信息
func (t *GardenCheckinTask) getCheckinInfo() (bool, error) {
	url := "https://happy.mail.10086.cn/jsp/cn/garden/task/checkinInfo.do"
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := t.client.Get(url, headers)
	if err != nil {
		return false, fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return false, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		TodayCheckin bool `json:"todayCheckin"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return false, fmt.Errorf("解析响应失败: %w", err)
	}

	return result.TodayCheckin, nil
}

// checkin 签到
func (t *GardenCheckinTask) checkin() error {
	url := "https://happy.mail.10086.cn/jsp/cn/garden/task/checkin.do"
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

	if result.Code != 1 {
		t.logger.Fail(fmt.Sprintf("果园签到失败: code=%d, msg=%s", result.Code, result.Message))
		return fmt.Errorf("果园签到失败: code=%d, msg=%s", result.Code, result.Message)
	}

	t.logger.Success("果园签到成功")
	return nil
}

// claimSceneWater 领取场景水滴
func (t *GardenCheckinTask) claimSceneWater() error {
	// 获取已完成的场景
	cartoons, err := t.getCartoons()
	if err != nil {
		return err
	}

	// 获取未完成的场景
	scenes := []string{"cloud", "color", "widget", "mail"}
	for _, scene := range scenes {
		if !contains(cartoons, scene) {
			if err := t.clickCartoon(scene); err != nil {
				t.logger.Error(fmt.Sprintf("领取场景水滴%s失败", scene), err)
			} else {
				t.logger.Debug(fmt.Sprintf("领取场景水滴%s", scene))
			}
			utils.Sleep(5000)
		}
	}

	return nil
}

// getCartoons 获取已完成的场景列表
func (t *GardenCheckinTask) getCartoons() ([]string, error) {
	url := "https://happy.mail.10086.cn/jsp/cn/garden/user/gotCartoons.do"
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

	var result []string
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return []string{}, nil // 解析失败返回空数组
	}

	return result, nil
}

// clickCartoon 点击场景领取水滴
func (t *GardenCheckinTask) clickCartoon(scene string) error {
	url := fmt.Sprintf("https://happy.mail.10086.cn/jsp/cn/garden/user/clickCartoon.do?cartoonType=%s", scene)
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

	if result.Code != 1 && result.Code != -1 && result.Code != -2 {
		return fmt.Errorf("领取场景水滴失败: code=%d, msg=%s", result.Code, result.Message)
	}

	return nil
}

// watering 给果树浇水
func (t *GardenCheckinTask) watering() error {
	url := "https://happy.mail.10086.cn/jsp/cn/garden/user/watering.do?isFast=1"
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
		Water   int    `json:"water"`
		Upgrade bool   `json:"upgrade"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 1 {
		t.logger.Fail(fmt.Sprintf("浇水失败: code=%d, msg=%s", result.Code, result.Message))
		return nil
	}

	upgradeStr := "未升级"
	if result.Upgrade {
		upgradeStr = "升级"
	}
	t.logger.Success(fmt.Sprintf("浇水成功，消耗%d水滴，%s", result.Water, upgradeStr))
	return nil
}

// contains 检查字符串切片是否包含某个元素
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
