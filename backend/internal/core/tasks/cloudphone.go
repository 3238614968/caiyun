package tasks

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/utils"
)

// CloudPhonePartyTask 云手机红包派对任务
type CloudPhonePartyTask struct {
	client *http.Client
	logger *logger.Logger
	store  Storage
	token  string
}

// NewCloudPhonePartyTask 创建云手机红包派对任务
func NewCloudPhonePartyTask(client *http.Client, log *logger.Logger, store Storage) *CloudPhonePartyTask {
	return &CloudPhonePartyTask{
		client: client,
		logger: log,
		store:  store,
	}
}

// Run 执行云手机红包派对任务
func (t *CloudPhonePartyTask) Run() error {
	t.logger.Start("------【云手机红包派对】------")

	// 第一步：登录
	if err := t.login(); err != nil {
		t.logger.Error("登录失败", err)
		return err
	}

	// 第二步：获取用户账户信息
	t.getUserAccount()

	utils.Sleep(1000)

	// 第三步：获取任务列表并签到
	taskList, err := t.getTaskList()
	if err != nil {
		t.logger.Error("获取任务列表失败", err)
		return err
	}

	// 第四步：执行签到
	if err := t.sign(taskList); err != nil {
		t.logger.Error("签到失败", err)
		return err
	}

	return nil
}

// generateCloudPhoneHeaders 生成云手机签名头（对应源码 mo() 函数）
func (t *CloudPhonePartyTask) generateCloudPhoneHeaders(token string) map[string]string {
	appId := "12345681"
	now := time.Now()
	// requestId = 时间戳字符串 + 毫秒时间戳 + 8位随机字符串
	timeStr := now.Format("20060102150405")
	timestamp := fmt.Sprintf("%d", now.UnixMilli())
	requestId := timeStr + timestamp + utils.RandomString(8)

	// sign = md5(requestId + appId + token + "e10adc3949ba59abbe56e057f20f883e" + timestamp)
	salt := "e10adc3949ba59abbe56e057f20f883e"
	raw := requestId + appId + token + salt + timestamp
	sign := strings.ToLower(utils.MD5(raw))

	return map[string]string{
		"Content-Type": "application/json",
		"sign":         sign,
		"requestId":    requestId,
		"appId":        appId,
		"token":        token,
		"timestamp":    timestamp,
	}
}

// login 登录
func (t *CloudPhonePartyTask) login() error {
	// 获取 TyrzToken
	url := fmt.Sprintf("https://caiyun.feixin.10086.cn/portal/auth/getTyrzToken.action?sourceId=001216&_=%d", time.Now().UnixMilli())
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := t.client.Get(url, headers)
	if err != nil {
		return fmt.Errorf("获取 TyrzToken 失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	var tyrzResult struct {
		RetCode string `json:"retCode"`
		Desc    string `json:"desc"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal([]byte(responseBody), &tyrzResult); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}

	if tyrzResult.RetCode != "0" || tyrzResult.Token == "" {
		return fmt.Errorf("获取 TyrzToken 失败: retCode=%s, desc=%s", tyrzResult.RetCode, tyrzResult.Desc)
	}

	// 验证 Token（使用 mo() 签名头，token 参数为空字符串）
	validateURL := "https://cpactiv.buy.139.com/cloudphone-market/user/tokenValidate"
	signHeaders := t.generateCloudPhoneHeaders("")
	validateBody := map[string]interface{}{
		"version": "1.0",
		"pintype": 13,
		"token":   tyrzResult.Token,
	}

	validateResp, err := t.client.Post(validateURL, signHeaders, validateBody)
	if err != nil {
		return fmt.Errorf("验证 Token 失败: %w", err)
	}

	validateResponseBody, err := t.client.ReadResponseBody(validateResp)
	if err != nil {
		return fmt.Errorf("读取验证响应失败: %w", err)
	}

	var validateResult struct {
		Header struct {
			Status string `json:"status"`
		} `json:"header"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(validateResponseBody), &validateResult); err != nil {
		return fmt.Errorf("解析验证响应失败: %w", err)
	}

	if validateResult.Header.Status != "200" {
		return fmt.Errorf("Token 验证失败: status=%s", validateResult.Header.Status)
	}

	t.token = validateResult.Data.Token
	return nil
}

// getUserAccount 获取用户账户信息
func (t *CloudPhonePartyTask) getUserAccount() {
	url := "https://cpactiv.buy.139.com/cloudphone-market/redpacket/userAccountInfo"
	headers := t.generateCloudPhoneHeaders(t.token)

	resp, err := t.client.Post(url, headers, map[string]interface{}{})
	if err != nil {
		t.logger.Error("获取用户信息失败", err)
		return
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		t.logger.Error("读取响应失败", err)
		return
	}

	var result struct {
		Header struct {
			Status string `json:"status"`
		} `json:"header"`
		Data struct {
			CanAmount int `json:"canAmount"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return
	}

	if result.Header.Status == "200" {
		t.logger.Info(fmt.Sprintf("可使用积分：%d", result.Data.CanAmount))
	}
}

// CloudPhonePartyTaskList 云手机红包派对任务列表
type CloudPhonePartyTaskList struct {
	ConfigTaskSignList []struct {
		TaskID     string `json:"taskId"`
		TaskName   string `json:"taskName"`
		SignAmount int    `json:"signAmount"`
		Status     int    `json:"status"`  // 0: 已签到, 1: 未签到
		IsToday    int    `json:"isToday"` // 1: 今日任务
	} `json:"configTaskSignList"`
}

// getTaskList 获取任务列表
func (t *CloudPhonePartyTask) getTaskList() (*CloudPhonePartyTaskList, error) {
	url := "https://cpactiv.buy.139.com/cloudphone-market/redpacket/configTaskList"
	headers := t.generateCloudPhoneHeaders(t.token)

	resp, err := t.client.Post(url, headers, map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Header struct {
			Status string `json:"status"`
		} `json:"header"`
		Data CloudPhonePartyTaskList `json:"data"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Header.Status != "200" {
		return nil, fmt.Errorf("获取任务列表失败: status=%s", result.Header.Status)
	}

	return &result.Data, nil
}

// sign 签到
func (t *CloudPhonePartyTask) sign(taskList *CloudPhonePartyTaskList) error {
	// 查找今日签到任务
	var todayTask *struct {
		TaskID     string `json:"taskId"`
		TaskName   string `json:"taskName"`
		SignAmount int    `json:"signAmount"`
		Status     int    `json:"status"`
		IsToday    int    `json:"isToday"`
	}

	for i := range taskList.ConfigTaskSignList {
		if taskList.ConfigTaskSignList[i].IsToday == 1 {
			todayTask = &taskList.ConfigTaskSignList[i]
			break
		}
	}

	if todayTask == nil {
		t.logger.Fail("未找到今日签到任务")
		return fmt.Errorf("未找到今日签到任务")
	}

	if todayTask.Status == 0 {
		t.logger.Success("今日已签到")
		return nil
	}

	// 执行签到
	result, err := t.userSign(todayTask.TaskID)
	if err != nil {
		return err
	}
	if result == 1 {
		t.logger.Success("签到成功", "获得", todayTask.SignAmount, "分")
		return nil
	}

	t.logger.Fail("签到失败", result)
	return fmt.Errorf("签到失败: code=%d", result)
}

// userSign 用户签到
func (t *CloudPhonePartyTask) userSign(taskID string) (int, error) {
	url := "https://cpactiv.buy.139.com/cloudphone-market/redpacket/userSign"
	headers := t.generateCloudPhoneHeaders(t.token)

	body := map[string]interface{}{}

	resp, err := t.client.Post(url, headers, body)
	if err != nil {
		return -1, fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return -1, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return -1, fmt.Errorf("解析响应失败: %w", err)
	}

	return result.Code, nil
}
