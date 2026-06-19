package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/utils"
)

// AICloudTask AI 云朵任务
type AICloudTask struct {
	client *http.Client
	logger *logger.Logger
	store  Storage
	userID string
}

// NewAICloudTask 创建 AI 云朵任务
func NewAICloudTask(client *http.Client, log *logger.Logger, store Storage, userID string) *AICloudTask {
	return &AICloudTask{
		client: client,
		logger: log,
		store:  store,
		userID: userID,
	}
}

// AISession AI 会话（导出以便其他包使用）
type AISession struct {
	SessionID  string `json:"sessionId"`
	DialogueID string `json:"dialogueId"`
	UserID     string `json:"userId"`
}

// Run 执行 AI 云朵任务
func (t *AICloudTask) Run(sessions []AISession) error {
	t.logger.Start("------【玩AI小天得云朵】------")

	if len(sessions) == 0 {
		t.logger.Fail("AI云朵", "没有AI云朵对话记录,请确定AI红包任务已经运行")
		return fmt.Errorf("没有AI云朵对话记录,请确定AI红包任务已经运行")
	}
	if t.userID == "" {
		t.logger.Fail("AI云朵", "获取用户ID失败")
		return fmt.Errorf("用户ID为空")
	}

	encryptedUserID, err := utils.EncryptAiUserId(t.userID)
	if err != nil {
		t.logger.Error("加密 AI 用户 ID 失败", err)
		return err
	}
	cloudNum, err := t.getCurrentMonthCloudNum()
	if err != nil {
		t.logger.Error("获取云朵数量失败", err)
		cloudNum = 0
	}

	t.logger.Info(fmt.Sprintf("本月获取云朵数量: %d", cloudNum))
	if cloudNum > 250 {
		t.logger.Info("本月已获取超过250个云朵，跳过")
		return nil
	}
	if cloudNum >= 197 {
		return t.getCloudReward(sessions[0], encryptedUserID)
	}

	for i := 0; i < 10 && i < len(sessions); i++ {
		session := sessions[i]
		if err := t.getCloudReward(session, encryptedUserID); err != nil {
			t.logger.Error("获取云朵奖励失败", err)
		}
		utils.Sleep(2000)
	}

	return nil
}

func (t *AICloudTask) getCurrentMonthCloudNum() (int, error) {
	resp, err := api.NewCaiyunAPI(t.client).GetCloudRecord(1, 300, 1)
	if err != nil {
		return 0, err
	}
	if resp == nil {
		return 0, nil
	}
	if code := parseCaiyunCode(resp.Code); code != 0 {
		return 0, fmt.Errorf("获取云朵记录失败: code=%d, msg=%s", code, resp.Message)
	}

	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		return 0, nil
	}
	records, ok := resultMap["records"].([]interface{})
	if !ok {
		return 0, nil
	}

	now := time.Now()
	total := 0
	for _, item := range records {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if fmt.Sprint(record["mark"]) != "PlayAiodd" {
			continue
		}

		recordTime := parseAICloudRecordTime(fmt.Sprint(record["inserttime"]))
		if recordTime.IsZero() {
			recordTime = parseAICloudRecordTime(fmt.Sprint(record["updatetime"]))
		}
		if recordTime.IsZero() || recordTime.Year() != now.Year() || recordTime.Month() != now.Month() {
			continue
		}

		total += toInt(record["num"])
	}
	return total, nil
}

func parseAICloudRecordTime(value string) time.Time {
	value = fmt.Sprint(value)
	if value == "" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05Z07:00",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// getCloudReward 获取云朵奖励
func (t *AICloudTask) getCloudReward(session AISession, encryptedUserID string) error {
	recordID, err := t.getRecordID(session, encryptedUserID)
	if err != nil {
		return err
	}
	if recordID == "" {
		t.logger.Debug("僵尸打开你的脑子，摇了摇头走了")
		return nil
	}
	return t.exchangeCloud(encryptedUserID, recordID)
}

// getRecordID 获取记录ID（对应 mjs getPreCloudReward）
func (t *AICloudTask) getRecordID(session AISession, encryptedUserID string) (string, error) {
	headers := map[string]string{
		"Content-Type":     "application/json",
		"X-Requested-With": "com.chinamobile.mcloud",
		"Referer":          "https://yun.139.com/ai-helper/",
	}

	body := map[string]interface{}{
		"type":       0,
		"sessionId":  session.SessionID,
		"dialogueId": session.DialogueID,
		"userId":     encryptedUserID,
	}

	resp, err := t.client.Post(
		"https://yun.139.com/mrpInfo/ycloud/aixt/playAiClouds/getPreCloudReward",
		headers,
		body,
	)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var result api.AIRecordIdResp
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code == 52105 {
		return "", nil
	}
	if result.Code != 0 {
		t.logger.Fail("抽取奖励失败", result.Code, result.Message)
		return "", fmt.Errorf("抽取奖励失败: code=%d, msg=%s", result.Code, result.Message)
	}

	t.logger.Debug("抽取奖励成功, 抽取", result.Result.CloudNum)
	return result.Result.RecordID, nil
}

// exchangeCloud 兑换云朵（对应 mjs getCloudReward）
func (t *AICloudTask) exchangeCloud(encryptedUserID, recordID string) error {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	body := map[string]interface{}{
		"userId":   encryptedUserID,
		"recordId": recordID,
	}

	resp, err := t.client.Post(
		"https://yun.139.com/mrpInfo/ycloud/aixt/playAiClouds/getCloudReward",
		headers,
		body,
	)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	var result api.AIExchangeResp
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	if result.Code != 0 {
		return fmt.Errorf("兑换云朵失败: code=%d, msg=%s", result.Code, result.Message)
	}
	return nil
}
