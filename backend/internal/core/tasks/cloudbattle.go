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

// CloudBattleTask 云朵大作战任务（合成1T）
type CloudBattleTask struct {
	client   *http.Client
	logger   *logger.Logger
	jwtToken string
}

// NewCloudBattleTask 创建云朵大作战任务
func NewCloudBattleTask(client *http.Client, log *logger.Logger) *CloudBattleTask {
	return &CloudBattleTask{
		client: client,
		logger: log,
	}
}

// Run 执行云朵大作战任务
func (t *CloudBattleTask) Run() error {
	return t.RunWithGameTime(30)
}

// RunWithGameTime 按指定游戏时长执行云朵大作战任务
func (t *CloudBattleTask) RunWithGameTime(gameTime int) error {
	if gameTime <= 0 {
		gameTime = 30
	}

	t.logger.Start("------【云朵大作战】------")

	// 第一步：登录云朵大作战
	if err := t.login(); err != nil {
		t.logger.Error("登录云朵大作战失败", err)
		return err
	}

	// 第二步：获取云朵大作战信息
	info, err := t.getHecheng1T()
	if err != nil {
		t.logger.Error("获取云朵大作战信息失败", err)
		return err
	}

	// 打印信息
	t.printInfo(info)

	// 第三步：执行游戏（根据今日剩余次数）
	curr := info.Info.Curr
	if curr <= 0 {
		t.logger.Info("今日已完成")
		return nil
	}

	for i := 0; i < curr; i++ {
		if err := t.playGame(gameTime); err != nil {
			t.logger.Error("游戏失败", err)
		}
	}

	t.logger.Success("完成云朵大作战")
	return nil
}

// login 登录云朵大作战
func (t *CloudBattleTask) login() error {
	// 获取 specToken
	ssoToken, err := t.getSpecToken()
	if err != nil {
		return fmt.Errorf("获取 specToken 失败: %w", err)
	}

	// 通过 tyrzLogin 登录，marketName=hecheng1T, sourceId=1169
	jwtToken, err := t.tyrzLogin(ssoToken)
	if err != nil {
		return fmt.Errorf("tyrzLogin 失败: %w", err)
	}

	t.jwtToken = jwtToken
	return nil
}

// getSpecToken 获取 specToken
func (t *CloudBattleTask) getSpecToken() (string, error) {
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

// generateSignHeaders 生成签名头（对应源码 ys() 函数）
// e=1 表示使用 seedMdYYLIZfbCxg 盐值（默认，用于 beinvite/finish）
// e=2 表示使用 sekaMdYYLIZfbCfm 盐值（用于 tyrzLogin）
func (t *CloudBattleTask) generateSignHeaders(timestamp int64, queryString string) map[string]string {
	return generateSignHeadersWithSalt(timestamp, queryString, 1)
}

// generateSignHeadersForLogin 生成登录用签名头（e=2）
func (t *CloudBattleTask) generateSignHeadersForLogin(timestamp int64, queryString string) map[string]string {
	return generateSignHeadersWithSalt(timestamp, queryString, 2)
}

// generateSignHeadersWithSalt 通用签名头生成
func generateSignHeadersWithSalt(timestamp int64, queryString string, saltType int) map[string]string {
	requestID := utils.GenerateUUID()
	nonce := utils.GenerateUUID()
	ts := fmt.Sprintf("%d", timestamp)

	// e=1 → salt = "seedMdYYLIZfbCxg"
	// e=2 → salt = "sekaMdYYLIZfbCfm"
	salt := "seedMdYYLIZfbCxg"
	if saltType == 2 {
		salt = "sekaMdYYLIZfbCfm"
	}
	raw := salt + requestID + ts + nonce + queryString + salt
	signature := strings.ToLower(utils.MD5(raw))

	return map[string]string{
		"x-request-id": requestID,
		"x-timestamp":  ts,
		"x-nonce":      nonce,
		"x-signature":  signature,
	}
}

// tyrzLogin 通过 tyrzLogin 登录云朵大作战
func (t *CloudBattleTask) tyrzLogin(ssoToken string) (string, error) {
	queryString := fmt.Sprintf("ssoToken=%s&openAccount=0&channel=&marketName=hecheng1T&sourceId=1169", ssoToken)
	url := fmt.Sprintf("https://caiyun.feixin.10086.cn/portal/auth/v2/tyrzLogin.action?%s", queryString)

	timestamp := time.Now().UnixMilli()
	// tyrzLogin 使用 e=2 盐值
	signHeaders := t.generateSignHeadersForLogin(timestamp, queryString)

	headers := map[string]string{
		"Content-Type": "application/json",
	}
	for k, v := range signHeaders {
		headers[k] = v
	}

	resp, err := t.client.Get(url, headers)
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
		return "", fmt.Errorf("登录云朵大作战失败: code=%d, msg=%s", result.Code, result.Message)
	}

	return result.Result.Token, nil
}

// Hecheng1TInfo 云朵大作战信息
type Hecheng1TInfo struct {
	Info struct {
		Invite   int   `json:"invite"`
		Exchange int   `json:"exchange"`
		Succ     int   `json:"succ"`
		LastSucc int64 `json:"lastSucc"`
		Curr     int   `json:"curr"`
	} `json:"info"`
	History []struct {
		Count int `json:"count"`
		Rank  int `json:"rank"`
	} `json:"history"`
}

// getHecheng1T 获取云朵大作战信息
func (t *CloudBattleTask) getHecheng1T() (*Hecheng1TInfo, error) {
	url := "https://caiyun.feixin.10086.cn/market/signin/hecheng1T/info"
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
		Code    int           `json:"code"`
		Message string        `json:"msg"`
		Result  Hecheng1TInfo `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("获取云朵大作战信息失败: code=%d, msg=%s", result.Code, result.Message)
	}

	return &result.Result, nil
}

// printInfo 打印云朵大作战信息
func (t *CloudBattleTask) printInfo(info *Hecheng1TInfo) {
	if len(info.History) > 0 {
		t.logger.Debug("本月排名", info.History[0].Rank)
		t.logger.Debug("本月成功次数", info.History[0].Count)
	}
	t.logger.Debug("今日剩余次数", info.Info.Curr)
	t.logger.Debug("今日可兑换次数", info.Info.Exchange)
	t.logger.Debug("今日可被邀请次数", info.Info.Invite)
}

// getServerTimestamp 获取服务器时间戳
func (t *CloudBattleTask) getServerTimestamp() int64 {
	url := "https://caiyun.feixin.10086.cn/portal/ajax/tools/opRequest.action"
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded; charset=UTF-8",
	}

	resp, err := t.client.Post(url, headers, "op=currentTimeMillis")
	if err != nil {
		return time.Now().UnixMilli()
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return time.Now().UnixMilli()
	}

	var result struct {
		Code   int   `json:"code"`
		Result int64 `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return time.Now().UnixMilli()
	}

	if result.Result > 0 {
		return result.Result
	}
	return time.Now().UnixMilli()
}

// journaling 上报事件（与 mjs api.journaling 一致）
func (t *CloudBattleTask) journaling(eventName string) {
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
	utils.Sleep(utils.RandomInt(50, 200))
}

// playGame 执行一局游戏
func (t *CloudBattleTask) playGame(gameTime int) error {
	// 上报事件
	t.journaling("synthesisonet_pv")
	t.journaling("synthesisonet_cookie")
	t.journaling("synthesisonet_cookie_notApp")
	t.journaling("synthesisonet_inviterUserPlayGame")
	t.journaling("synthesisonet_playGame")
	t.journaling("synthesisonet_playGame_isOts")

	// 获取服务器时间戳
	serverTime := t.getServerTimestamp()

	// 开始游戏
	if err := t.beinviteHecheng1T(serverTime); err != nil {
		return fmt.Errorf("开始游戏失败: %w", err)
	}

	t.logger.Debug("云朵大作战游戏开始，等待游戏结束中", gameTime)

	// 等待游戏时间（每3秒上报一次点击事件）
	rounds := (gameTime + 2) / 3
	for i := 0; i < rounds; i++ {
		t.journaling("synthesisonet_game_tap")
		utils.Sleep(3 * utils.RandomInt(990, 1010))
	}

	// 结束游戏
	t.journaling("synthesisonet_finish_gameSuc")
	serverTime = t.getServerTimestamp()
	if err := t.finishHecheng1T(serverTime); err != nil {
		return fmt.Errorf("结束游戏失败: %w", err)
	}

	t.logger.Debug("云朵大作战游戏结束")
	return nil
}

// beinviteHecheng1T 开始游戏
func (t *CloudBattleTask) beinviteHecheng1T(serverTime int64) error {
	url := "https://caiyun.feixin.10086.cn/market/signin/hecheng1T/beinvite?inviter="
	// beinvite 使用 e=1 盐值（默认）
	signHeaders := t.generateSignHeaders(serverTime, "")
	headers := map[string]string{
		"Content-Type": "application/json",
		"jwttoken":     t.jwtToken,
		"referer":      "https://caiyun.feixin.10086.cn:7071/portal/synthesisonet/index.html?sourceid=1005",
	}
	for k, v := range signHeaders {
		headers[k] = v
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

	if result.Code != 0 {
		return fmt.Errorf("开始游戏失败: code=%d, msg=%s", result.Code, result.Message)
	}

	return nil
}

// finishHecheng1T 结束游戏
func (t *CloudBattleTask) finishHecheng1T(serverTime int64) error {
	url := "https://caiyun.feixin.10086.cn/market/signin/hecheng1T/finish?flag=true"
	// finish 使用 e=1 盐值（默认）
	signHeaders := t.generateSignHeaders(serverTime, "")
	headers := map[string]string{
		"Content-Type": "application/json",
		"jwttoken":     t.jwtToken,
		"referer":      "https://caiyun.feixin.10086.cn:7071/portal/synthesisonet/index.html?sourceid=1005",
	}
	for k, v := range signHeaders {
		headers[k] = v
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

	if result.Code != 0 {
		return fmt.Errorf("结束游戏失败: code=%d, msg=%s", result.Code, result.Message)
	}

	return nil
}
