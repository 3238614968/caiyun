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

// BlindboxTask 开盲盒任务
type BlindboxTask struct {
	client   *http.Client
	logger   *logger.Logger
	store    Storage
	jwtToken string
}

// NewBlindboxTask 创建开盲盒任务
func NewBlindboxTask(client *http.Client, log *logger.Logger, store Storage) *BlindboxTask {
	return &BlindboxTask{
		client: client,
		logger: log,
		store:  store,
	}
}

// Run 执行开盲盒任务
func (t *BlindboxTask) Run() error {
	t.logger.Start("------【开盲盒】------")

	// 第一步：登录盲盒（通过 tyrzLogin 获取 jwttoken）
	if err := t.loginBlindbox(); err != nil {
		t.logger.Error("登录异常", err)
		return err
	}

	// 第二步：上报浏览信息
	t.reportJournaling()

	// 第三步：获取盲盒用户信息
	userInfo, err := t.getBlindboxUser()
	if err != nil {
		t.logger.Error("获取盲盒用户信息失败", err)
		return err
	}

	// 检查 chanceNum 类型
	if userInfo.ChanceNum == 0 && userInfo.TaskNum >= 2 {
		t.logger.Info("今日已完成")
		return nil
	}

	if userInfo.FirstTime {
		t.logger.Success("今日首次登录，获取次数 +1")
	}

	// 第四步：开盲盒
	for i := 0; i < userInfo.ChanceNum; i++ {
		t.openBlindbox()
		utils.Sleep(666)
	}

	// 第五步：注册盲盒任务（获取额外次数）
	t.registerBlindboxTasks()

	return nil
}

// loginBlindbox 登录盲盒（通过 mail login → artifact → tyrzLogin）
func (t *BlindboxTask) loginBlindbox() error {
	// 第一步：获取 ssoToken（通过 querySpecToken）
	ssoToken, err := t.getSpecToken()
	if err != nil {
		return fmt.Errorf("获取 specToken 失败: %w", err)
	}

	// 第二步：通过 loginMail 获取 rmkey 和 sid
	rmkey, sid, err := t.loginMail(ssoToken)
	if err != nil {
		return fmt.Errorf("登录邮箱失败: %w", err)
	}

	// 第三步：获取 artifact
	artifact, err := t.getArtifact(sid, rmkey)
	if err != nil {
		return fmt.Errorf("获取 artifact 失败: %w", err)
	}

	// 第四步：通过 tyrzLogin 获取 jwttoken
	jwtToken, err := t.tyrzLogin(artifact, "National_BlindBox")
	if err != nil {
		return fmt.Errorf("登录盲盒失败: %w", err)
	}

	t.jwtToken = jwtToken
	return nil
}

// getSpecToken 获取 specToken
func (t *BlindboxTask) getSpecToken() (string, error) {
	url := "https://user-njs.yun.139.com/user/querySpecToken"
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	body := map[string]interface{}{
		"toSourceId": "001005",
	}

	resp, err := t.client.Post(url, headers, body)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code    interface{} `json:"code"`
		Success bool        `json:"success"`
		Message string      `json:"msg"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Data.Token == "" {
		return "", fmt.Errorf("获取 specToken 失败: %s", result.Message)
	}

	return result.Data.Token, nil
}

// loginMail 通过邮箱登录获取 rmkey 和 sid
func (t *BlindboxTask) loginMail(ssoToken string) (string, string, error) {
	loginURL := "https://mail.10086.cn/login/inlogin.action"
	xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
      <object>
       <string name="clientId">10804</string> 
       <string name="version">9</string>
       <string name="loginType">7</string> 
       <string name="token">%s</string> 
      </object>`, ssoToken)

	headers := map[string]string{
		"Content-Type": "application/xml",
	}

	resp, err := t.client.Post(loginURL, headers, xmlBody)
	if err != nil {
		return "", "", fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return "", "", fmt.Errorf("读取响应失败: %w", err)
	}

	// 尝试 JSON 解析（mail.10086.cn 可能返回 JSON）
	var jsonResult struct {
		Code string `json:"code"`
		Var  struct {
			Rmkey string `json:"rmkey"`
			Sid   string `json:"sid"`
		} `json:"var"`
	}
	if err := json.Unmarshal([]byte(responseBody), &jsonResult); err == nil {
		if jsonResult.Var.Rmkey != "" && jsonResult.Var.Sid != "" {
			return jsonResult.Var.Rmkey, jsonResult.Var.Sid, nil
		}
	}

	// 降级为 XML 解析
	rmkey := utils.ExtractXMLTag(responseBody, "rmkey")
	sid := utils.ExtractXMLTag(responseBody, "sid")

	if rmkey == "" || sid == "" {
		return "", "", fmt.Errorf("登录失败，rmkey 或 sid 未获取到, response: %s", responseBody)
	}

	return rmkey, sid, nil
}

// getArtifact 获取 artifact
func (t *BlindboxTask) getArtifact(sid, rmkey string) (string, error) {
	artifactURL := fmt.Sprintf(
		"https://smsrebuild1.mail.10086.cn/setting/s?func=umc:getArtifact&sid=%s&cguid=%d",
		sid, time.Now().UnixMilli(),
	)
	headers := map[string]string{
		"Content-Type": "application/json",
		"Cookie":       fmt.Sprintf("RMKEY=%s", rmkey),
	}

	resp, err := t.client.Post(artifactURL, headers, "")
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	// 响应格式: {"code":"S_OK","var":{"artifact":"xxx"}}
	var result struct {
		Code string `json:"code"`
		Msg  string `json:"msg"`
		Var  struct {
			Artifact string `json:"artifact"`
		} `json:"var"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Var.Artifact == "" {
		return "", fmt.Errorf("获取 artifact 失败: %s", result.Msg)
	}

	return result.Var.Artifact, nil
}

// generateSignHeaders 生成签名头（对应源码 ys() 函数，e=2）
func (t *BlindboxTask) generateSignHeaders(timestamp int64, queryString string) map[string]string {
	requestID := utils.GenerateUUID()
	nonce := utils.GenerateUUID()
	ts := fmt.Sprintf("%d", timestamp)

	// NA(requestId, timestamp, nonce, 2, queryString)
	// e=2 → salt = "sekaMdYYLIZfbCfm"
	salt := "sekaMdYYLIZfbCfm"
	raw := salt + requestID + ts + nonce + queryString + salt
	signature := strings.ToLower(utils.MD5(raw))

	return map[string]string{
		"x-request-id": requestID,
		"x-timestamp":  ts,
		"x-nonce":      nonce,
		"x-signature":  signature,
	}
}

// tyrzLogin 通过 tyrzLogin 获取 jwttoken
func (t *BlindboxTask) tyrzLogin(ssoToken, marketName string) (string, error) {
	queryString := fmt.Sprintf("ssoToken=%s&openAccount=0&channel=&marketName=%s&sourceId=1005", ssoToken, marketName)
	loginURL := fmt.Sprintf("https://caiyun.feixin.10086.cn/portal/auth/v2/tyrzLogin.action?%s", queryString)

	timestamp := time.Now().UnixMilli()
	signHeaders := t.generateSignHeaders(timestamp, queryString)

	headers := map[string]string{
		"Content-Type": "application/json",
	}
	for k, v := range signHeaders {
		headers[k] = v
	}

	resp, err := t.client.Get(loginURL, headers)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Result  struct {
			Token string `json:"token"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return "", fmt.Errorf("登录失败: code=%d, msg=%s", result.Code, result.Message)
	}

	return result.Result.Token, nil
}

// reportJournaling 上报浏览信息
func (t *BlindboxTask) reportJournaling() {
	events := []string{
		"National_BlindBox_userLogin",
		"National_BlindBox_login",
		"National_BlindBox_loginAppOuterEnd",
	}
	for _, event := range events {
		t.journaling(event)
		utils.Sleep(200)
	}
}

// journaling 上报单个事件（与 mjs api.journaling 一致）
func (t *BlindboxTask) journaling(eventName string) {
	url := "https://mrp.mcloud.139.com/portal/journaling"
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"jwttoken":     t.jwtToken,
	}
	body := fmt.Sprintf("account=&module=uservisit&optkeyword=%s&fromId=&flag=&fileId=&fileType=&fileExtname=&fileSize=&sourceid=1005&linkId=", eventName)
	resp, err := t.client.Post(url, headers, body)
	if err == nil && resp != nil {
		resp.Body.Close()
	}
}

// BlindboxUserInfo 盲盒用户信息
type BlindboxUserInfo struct {
	ChanceNum int  `json:"chanceNum"`
	TaskNum   int  `json:"taskNum"`
	FirstTime bool `json:"firstTime"`
}

// getBlindboxUser 获取盲盒用户信息
func (t *BlindboxTask) getBlindboxUser() (*BlindboxUserInfo, error) {
	url := "https://caiyun.feixin.10086.cn/ycloud/blindbox/user/info"
	headers := map[string]string{
		"Content-Type": "application/json",
		"accept":       "application/json",
		"jwttoken":     t.jwtToken,
	}
	body := map[string]interface{}{
		"from": "main",
	}

	resp, err := t.client.Post(url, headers, body)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code    int              `json:"code"`
		Message string           `json:"msg"`
		Result  BlindboxUserInfo `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("获取盲盒用户信息失败: code=%d, msg=%s", result.Code, result.Message)
	}

	return &result.Result, nil
}

// openBlindbox 开盲盒
func (t *BlindboxTask) openBlindbox() {
	url := "https://caiyun.feixin.10086.cn/ycloud/blindbox/draw/openBox?from=main"
	headers := map[string]string{
		"Content-Type":    "application/json",
		"accept":          "application/json",
		"jwttoken":        t.jwtToken,
		"x-requested-with": "cn.cj.pe",
		"referer":         "https://caiyun.feixin.10086.cn:7071/portal/caiyunOfficialAccount/index.html?path=blindBox&sourceid=1015",
		"origin":          "https://caiyun.feixin.10086.cn",
	}
	body := map[string]interface{}{
		"from": "main",
	}

	resp, err := t.client.Post(url, headers, body)
	if err != nil {
		t.logger.Error("开盲盒请求失败", err)
		return
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		t.logger.Error("读取响应失败", err)
		return
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Result  struct {
			PrizeName string `json:"prizeName"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		t.logger.Error("解析响应失败", err)
		return
	}

	switch result.Code {
	case 0:
		t.logger.Info("获得", result.Result.PrizeName)
	case 200103:
		t.logger.Fail("本月奖励已领完", result.Code, result.Message)
	case 200105:
		t.logger.Debug("什么都没有哦")
	case 200106:
		t.logger.Error("异常", fmt.Errorf("code=%d, msg=%s", result.Code, result.Message))
	default:
		t.logger.Warn("未知原因失败", result.Code, result.Message)
	}
}

// registerBlindboxTasks 注册盲盒任务（获取额外开盒次数）
func (t *BlindboxTask) registerBlindboxTasks() {
	// 获取盲盒任务列表
	taskList, err := t.getBlindboxTaskList()
	if err != nil {
		t.logger.Error("获取盲盒任务列表失败", err)
		return
	}

	if len(taskList) == 0 {
		return
	}

	// 过滤出未完成且非限制的任务
	for _, task := range taskList {
		if task.Status != 0 {
			continue
		}
		// 跳过包含 isLimit 的任务
		if task.Memo != "" && strings.Contains(task.Memo, "isLimit") {
			continue
		}

		t.logger.Debug("注册盲盒任务", task.TaskName)
		t.registerSingleTask(task.TaskID)
		utils.Sleep(666)

		// 注册后尝试开盒
		t.tryOpenAfterRegister()
	}
}

// BlindboxTaskItem 盲盒任务项
type BlindboxTaskItem struct {
	TaskID   string `json:"taskId"`
	TaskName string `json:"taskName"`
	Status   int    `json:"status"` // 0: 未完成, 1: 已完成
	Memo     string `json:"memo"`
}

// getBlindboxTaskList 获取盲盒任务列表
func (t *BlindboxTask) getBlindboxTaskList() ([]BlindboxTaskItem, error) {
	url := "https://caiyun.feixin.10086.cn/market/task-service/task/api/blindBox/queryTaskInfo"
	headers := map[string]string{
		"Content-Type": "application/json",
		"accept":       "application/json",
		"jwttoken":     t.jwtToken,
	}
	body := map[string]interface{}{
		"marketName": "National_BlindBox",
		"clientType": 1,
	}

	resp, err := t.client.Post(url, headers, body)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code    int                `json:"code"`
		Message string             `json:"msg"`
		Result  []BlindboxTaskItem `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("获取盲盒任务列表失败: code=%d, msg=%s", result.Code, result.Message)
	}

	return result.Result, nil
}

// registerSingleTask 注册单个盲盒任务
func (t *BlindboxTask) registerSingleTask(taskID string) {
	url := "https://caiyun.feixin.10086.cn/market/task-service/task/api/blindBox/register"
	headers := map[string]string{
		"Content-Type": "application/json",
		"accept":       "application/json",
		"jwttoken":     t.jwtToken,
	}
	body := map[string]interface{}{
		"marketName": "National_BlindBox",
		"taskId":     taskID,
	}

	resp, err := t.client.Post(url, headers, body)
	if err != nil {
		t.logger.Error("注册盲盒任务失败", err)
		return
	}
	resp.Body.Close()
}

// tryOpenAfterRegister 注册任务后尝试开盒
func (t *BlindboxTask) tryOpenAfterRegister() {
	userInfo, err := t.getBlindboxUser()
	if err != nil {
		return
	}
	if userInfo.ChanceNum <= 0 {
		return
	}
	utils.Sleep(666)
	for i := 0; i < userInfo.ChanceNum; i++ {
		t.openBlindbox()
		utils.Sleep(666)
	}
}
