package tasks

import (
	"encoding/json"
	"fmt"
	"strings"

	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/utils"
)

// RedPacketTask AI 红包任务
type RedPacketTask struct {
	client   *http.Client
	logger   *logger.Logger
	auth     AuthClient
	userID   string
	sessions *[]AISession
}

// AuthClient 认证客户端接口
type AuthClient interface {
	GetSSOToken(userID string) (string, error)
	LoginMailWithSSO(ssoToken string) error
}

// NewRedPacketTask 创建 AI 红包任务
func NewRedPacketTask(
	client *http.Client,
	log *logger.Logger,
	auth AuthClient,
	userID string,
	sessions *[]AISession,
) *RedPacketTask {
	return &RedPacketTask{
		client:   client,
		logger:   log,
		auth:     auth,
		userID:   userID,
		sessions: sessions,
	}
}

// 红包 API 基础 URL（与 mjs Za 一致）
const redpacketBaseURL = "https://caiyun.feixin.10086.cn/market/lanternriddles/answeredPuzzles"

// AI 聊天 API 基础 URL（与 mjs lu 一致）
const aiChatBaseURL = "https://ai.yun.139.com"

// Run 执行 AI 红包任务
func (t *RedPacketTask) Run() error {
	t.logger.Start("------【AI红包】------")

	// 第一步：上报浏览信息（与 mjs A2 一致）
	t.reportJournaling()

	// 第二步：获取 userId（与 mjs _f 一致）
	if err := t.ensureUserID(); err != nil {
		return err
	}

	// 第三步：循环答题（最多4次，与 mjs P2 循环一致）
	for i := 0; i < 4; i++ {
		done, err := t.answerPuzzle()
		if err != nil {
			t.logger.Error("AI红包", err)
			return err
		}
		if done {
			break
		}
	}

	return nil
}

// reportJournaling 上报浏览信息（与 mjs A2 一致）
// mjs: api.journaling(name, sourceId, extra)
// → POST https://mrp.mcloud.139.com/portal/journaling
//
//	body: "account=&module=uservisit&optkeyword=<name>&fromId=&flag=&fileId=&fileType=&fileExtname=&fileSize=&sourceid=<sourceId>&linkId="
func (t *RedPacketTask) reportJournaling() {
	marketName := "&marketName=National_LanternRiddlesal_LanternRiddles"
	events := []string{
		"National_LanternRiddles_client_all",
		"National_LanternRiddles_pv",
		"National_LanternRiddles_client_isApp",
	}

	for _, event := range events {
		url := "https://mrp.mcloud.139.com/portal/journaling"
		headers := map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		}
		body := fmt.Sprintf("account=&module=uservisit&optkeyword=%s&fromId=&flag=&fileId=&fileType=&fileExtname=&fileSize=&sourceid=1008&linkId=%s", event, marketName)

		resp, err := t.client.Post(url, headers, body)
		if err != nil {
			t.logger.Error("上报失败", event, err)
			continue
		}
		resp.Body.Close()
		utils.Sleep(200)
	}
}

// ensureUserID 确保 userID 已设置（与 mjs _f 一致）
func (t *RedPacketTask) ensureUserID() error {
	if strings.TrimSpace(t.userID) != "" {
		return nil
	}
	return fmt.Errorf("AI 红包缺少 userID")
}

// PuzzleInfo 谜题信息
type PuzzleInfo struct {
	ID     string `json:"id"`
	Puzzle string `json:"puzzle"`
}

// answerPuzzle 单次答题流程（与 mjs P2 一致）
func (t *RedPacketTask) answerPuzzle() (bool, error) {
	utils.Sleep(1000)

	// 第一步：获取谜题列表（与 mjs I2 → api.getIndexPuzzleCard 一致）
	puzzles, err := t.getPuzzles()
	if err != nil || len(puzzles) == 0 {
		return false, nil
	}

	// 随机选择一个谜题
	puzzle := puzzles[utils.RandomInt(0, len(puzzles)-1)]
	t.logger.Debug("谜面 -> ", puzzle.Puzzle)
	utils.Sleep(200)

	// 第二步：获取 AI 答案（与 mjs _2 → api.addChat 一致）
	answer, exitMsg, needMatch, failMsg, err := t.getAIAnswer(puzzle.Puzzle)
	if err != nil {
		return false, err
	}

	if exitMsg != "" {
		return false, fmt.Errorf("%s", exitMsg)
	}

	if failMsg != "" {
		t.logger.Fail("AI红包", failMsg)
		return false, nil
	}

	if answer == "" {
		t.logger.Debug("获取AI聊天消息失败")
		return false, nil
	}

	// 处理答案（与 mjs N2 一致）
	if needMatch {
		answer = extractAnswer(answer)
	}

	if answer == "" {
		t.logger.Fail("AI红包", "未获取到谜底", answer)
		return false, nil
	}

	t.logger.Debug("谜底 -> ", answer)
	utils.Sleep(400)

	// 第三步：提交答案（与 mjs B2 → api.submitAnswered 一致）
	result, err := t.submitAnswer(puzzle.ID, answer)
	if result == -1 {
		return true, nil // 已经全部答完
	}
	if result != 0 {
		return false, nil
	}

	utils.Sleep(400)

	// 第四步：领取奖励（与 mjs O2 → api.getAwarding 一致）
	prize, err := t.claimReward(puzzle.ID)
	if err != nil {
		return false, err
	}

	if prize == -1 {
		return true, nil
	}

	prizeStr, ok := prize.(string)
	if ok {
		if prizeStr != "谢谢参与" {
			t.logger.Success("获得", prizeStr)
			return true, nil
		}
		t.logger.Debug("获得", prizeStr)
	}

	return false, nil
}

// getPuzzles 获取谜题列表（与 mjs api.getIndexPuzzleCard 一致）
// GET https://caiyun.feixin.10086.cn/market/lanternriddles/answeredPuzzles/getIndexPuzzleCard
func (t *RedPacketTask) getPuzzles() ([]PuzzleInfo, error) {
	url := redpacketBaseURL + "/getIndexPuzzleCard"

	resp, err := t.client.Get(url, nil)
	if err != nil {
		return nil, fmt.Errorf("获取谜题失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// mjs 返回数组，每项有 puzzleTitleContext 和 id
	// 过滤 puzzleTitleContext 存在且长度 < 6 的
	var rawList []struct {
		ID                 string `json:"id"`
		PuzzleTitleContext string `json:"puzzleTitleContext"`
	}
	if err := json.Unmarshal([]byte(responseBody), &rawList); err != nil {
		// 可能是包装在 result 中
		var wrapped struct {
			Code   int `json:"code"`
			Result []struct {
				ID                 string `json:"id"`
				PuzzleTitleContext string `json:"puzzleTitleContext"`
			} `json:"result"`
		}
		if err2 := json.Unmarshal([]byte(responseBody), &wrapped); err2 != nil {
			return nil, fmt.Errorf("解析响应失败: %w, body: %s", err, responseBody)
		}
		rawList = wrapped.Result
	}

	if len(rawList) == 0 {
		t.logger.Fail("未获取到谜底列表，跳过任务")
		return nil, nil
	}

	// 过滤（与 mjs I2 一致：puzzleTitleContext 存在且长度 < 6）
	var puzzles []PuzzleInfo
	for _, item := range rawList {
		if item.PuzzleTitleContext != "" && len([]rune(item.PuzzleTitleContext)) < 6 {
			puzzles = append(puzzles, PuzzleInfo{
				ID:     item.ID,
				Puzzle: item.PuzzleTitleContext,
			})
		}
	}

	return puzzles, nil
}

// getAIAnswer 获取 AI 答案（与 mjs _2 → api.addChat 一致）
// POST https://ai.yun.139.com/api/outer/assistant/chat/add
func (t *RedPacketTask) getAIAnswer(puzzle string) (string, string, bool, string, error) {
	url := aiChatBaseURL + "/api/outer/assistant/chat/add"

	headers := map[string]string{
		"Content-Type":      "application/json",
		"Accept":            "text/event-stream",
		"x-yun-api-version": "v4",
		"Origin":            "https://yun.139.com",
		"X-Requested-With":  "com.chinamobile.mcloud",
		"Referer":           "https://yun.139.com/",
	}

	body := map[string]interface{}{
		"userId":    t.userID,
		"sessionId": "",
		"content": map[string]interface{}{
			"dialogue":      puzzle,
			"prompt":        "",
			"timestamp":     fmt.Sprintf("%d", utils.CurrentTimestamp()),
			"commands":      "",
			"resourceType":  "0",
			"resourceId":    "",
			"dialogueType":  "0",
			"sourceChannel": "101",
			"extInfo":       `{"h5Version":"1.3.0"}`,
			"imageSortType": 1,
		},
		"applicationType": "chat",
		"applicationId":   "",
	}

	resp, err := t.client.Post(url, headers, body)
	if err != nil {
		return "", "", false, "", fmt.Errorf("AI聊天请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return "", "", false, "", fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析 SSE 响应（与 mjs pO 一致）
	// 响应格式: 多行 "data:..." ，取第一个 data 行解析 JSON
	var chatResult struct {
		Code    string `json:"code"`
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			SessionID  string `json:"sessionId"`
			DialogueID string `json:"dialogueId"`
			FlowResult *struct {
				OutContent string `json:"outContent"`
			} `json:"flowResult"`
			LeadCopy *struct {
				LinkName string `json:"linkName"`
			} `json:"leadCopy"`
		} `json:"data"`
	}

	// 解析 SSE 响应（支持多事件、多行 data 拼接、注释行过滤等边界场景）
	if err := parseSSEResponse(responseBody, &chatResult); err != nil {
		return "", "", false, "", fmt.Errorf("解析AI响应失败: %w", err)
	}

	// 保存会话（与 mjs j2 一致）
	if chatResult.Data.SessionID != "" {
		t.saveSession(chatResult.Data.SessionID, chatResult.Data.DialogueID)
	}

	if !chatResult.Success {
		t.logger.Fail("获取AI聊天消息失败", chatResult.Code, chatResult.Message)
		// code 10000007 或 01000004 → exitMsg
		if chatResult.Code == "10000007" || chatResult.Code == "01000004" {
			return "", chatResult.Message, false, "", nil
		}
		return "", "", false, "", nil
	}

	// 与 mjs _2 返回逻辑一致
	if chatResult.Data.FlowResult != nil {
		return chatResult.Data.FlowResult.OutContent, "", true, "", nil
	}
	if chatResult.Data.LeadCopy != nil {
		return chatResult.Data.LeadCopy.LinkName, "", false, "", nil
	}

	// failMsg
	failJSON, _ := json.Marshal(chatResult)
	return "", "", false, string(failJSON), nil
}

// extractAnswer 从 AI 响应中提取答案（与 mjs N2 一致）
// 格式: "...<p>...——答案</p>..."
func extractAnswer(response string) string {
	if !strings.Contains(response, "<p>") || !strings.Contains(response, "——") {
		return ""
	}

	// 提取 <p> 标签内容
	parts := strings.SplitN(response, "<p>", 2)
	if len(parts) < 2 {
		return ""
	}

	// 去掉 </p> 及之后的内容
	content := parts[1]
	if idx := strings.Index(content, "</p"); idx >= 0 {
		content = content[:idx]
	}

	// 按 —— 分割，取后半部分
	dashParts := strings.SplitN(content, "——", 2)
	if len(dashParts) < 2 {
		// 尝试多个连续破折号
		for _, sep := range []string{"———", "——", "—"} {
			dashParts = strings.SplitN(content, sep, 2)
			if len(dashParts) >= 2 {
				break
			}
		}
	}

	if len(dashParts) < 2 {
		return ""
	}

	answer := strings.TrimSpace(dashParts[1])
	// 去掉 <br/> 标签
	answer = strings.ReplaceAll(answer, "<br/>", "")
	answer = strings.ReplaceAll(answer, "<br />", "")

	// 取 / 前面的部分
	if idx := strings.Index(answer, "/"); idx >= 0 {
		answer = answer[:idx]
	}

	return strings.TrimSpace(answer)
}

// submitAnswer 提交答案（与 mjs B2 → api.submitAnswered 一致）
// GET https://caiyun.feixin.10086.cn/market/lanternriddles/answeredPuzzles/submitAnswered?puzzleId=<id>&answered=<answer>
func (t *RedPacketTask) submitAnswer(puzzleID, answer string) (int, error) {
	url := fmt.Sprintf("%s/submitAnswered?puzzleId=%s&answered=%s",
		redpacketBaseURL, puzzleID, answer)

	resp, err := t.client.Get(url, nil)
	if err != nil {
		return -1, fmt.Errorf("提交答案失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return -1, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return -1, fmt.Errorf("解析响应失败: %w", err)
	}

	switch result.Code {
	case 0:
		t.logger.Debug("回答问题成功")
		return 0, nil
	case 201:
		t.logger.Fail("回答问题成功，但", result.Msg)
		return -1, nil
	default:
		t.logger.Fail("回答问题失败", result.Code, result.Msg)
		return 1, nil
	}
}

// claimReward 领取奖励（与 mjs O2 → api.getAwarding 一致）
// POST https://caiyun.feixin.10086.cn/market/lanternriddles/answeredPuzzles/awarding
func (t *RedPacketTask) claimReward(puzzleID string) (interface{}, error) {
	url := redpacketBaseURL + "/awarding"
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	body := map[string]interface{}{
		"puzzleId": puzzleID,
	}

	resp, err := t.client.Post(url, headers, body)
	if err != nil {
		return -1, fmt.Errorf("领取奖励失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return -1, fmt.Errorf("读取响应失败: %w", err)
	}

	var result struct {
		Code   int    `json:"code"`
		Msg    string `json:"msg"`
		Result struct {
			PrizeName string `json:"prizeName"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return -1, fmt.Errorf("解析响应失败: %w", err)
	}

	switch result.Code {
	case 0:
		return result.Result.PrizeName, nil
	case 10010020:
		t.logger.Fail("可能你需要去 APP 手动完成一次", result.Code, result.Msg)
		return -1, nil
	default:
		t.logger.Fail("打开红包失败", result.Code, result.Msg)
		return 1, nil
	}
}

// saveSession 保存会话（与 mjs j2 一致）
func (t *RedPacketTask) saveSession(sessionID, dialogueID string) {
	if t.sessions == nil {
		return
	}

	session := AISession{
		SessionID:  sessionID,
		DialogueID: dialogueID,
		UserID:     t.userID,
	}

	*t.sessions = append(*t.sessions, session)
	t.logger.Debug("保存会话", sessionID, dialogueID)
}

// parseSSEResponse 解析 SSE 格式的响应体
// 按 SSE 协议规范处理：
// 1. 以空行分隔事件
// 2. 同一事件内多行 data: 拼接
// 3. 忽略注释行（以 : 开头）和非 data 行
// 4. 如果不是 SSE 格式，回退到直接 JSON 解析
func parseSSEResponse(body string, result interface{}) error {
	// 按空行分割为多个事件块
	events := strings.Split(body, "\n\n")

	// 提取每个事件块中的 data 内容
	var dataEntries []string
	for _, event := range events {
		lines := strings.Split(event, "\n")
		var dataParts []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			// 忽略空行
			if trimmed == "" {
				continue
			}
			// 忽略注释行（以 : 开头）
			if strings.HasPrefix(trimmed, ":") {
				continue
			}
			// 只处理 data: 前缀的行
			if strings.HasPrefix(trimmed, "data:") {
				value := strings.TrimPrefix(trimmed, "data:")
				value = strings.TrimSpace(value)
				dataParts = append(dataParts, value)
			}
			// 非 data: 行直接忽略
		}
		if len(dataParts) > 0 {
			// 同一事件内多行 data: 值拼接为完整字符串
			dataEntries = append(dataEntries, strings.Join(dataParts, ""))
		}
	}

	// 如果没有任何 data: 行，回退到直接 JSON 解析整个 body
	if len(dataEntries) == 0 {
		return json.Unmarshal([]byte(body), result)
	}

	// 从后向前遍历事件，找到最后一个可成功解析 JSON 的事件
	for i := len(dataEntries) - 1; i >= 0; i-- {
		if err := json.Unmarshal([]byte(dataEntries[i]), result); err == nil {
			return nil
		}
	}

	// 所有事件都解析失败，回退到直接 JSON 解析整个 body
	return json.Unmarshal([]byte(body), result)
}
