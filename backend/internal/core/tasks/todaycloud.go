package tasks

import (
	"fmt"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// TodayCloudTask 今日云朵任务
type TodayCloudTask struct {
	client     *http.Client
	logger     *logger.Logger
	api        *api.CaiyunAPI
	todayCount int
	totalCloud int
}

// NewTodayCloudTask 创建今日云朵任务
func NewTodayCloudTask(client *http.Client, log *logger.Logger) *TodayCloudTask {
	return &TodayCloudTask{
		client: client,
		logger: log,
		api:    api.NewCaiyunAPI(client),
	}
}

// Run 执行今日云朵任务
func (t *TodayCloudTask) Run() error {
	// 获取云朵记录（type=1 表示获得记录）
	resp, err := t.api.GetCloudRecord(1, 300, 1)
	if err != nil {
		t.logger.Fail("获取云朵记录失败")
		return err
	}

	// 解析响应 code
	code := 0
	switch v := resp.Code.(type) {
	case int:
		code = v
	case float64:
		code = int(v)
	case string:
		if v == "0" {
			code = 0
		}
	}

	if code != 0 {
		msg := resp.Message
		if msg == "" {
			msg = resp.Msg
		}
		t.logger.Error("获取云朵记录失败", fmt.Errorf("code=%d, msg=%s", code, msg))
		return fmt.Errorf("获取云朵记录失败: code=%d, msg=%s", code, msg)
	}

	// 解析结果
	if resp.Result == nil {
		t.logger.Info("今日获得0次云朵，数量共计：0")
		return nil
	}

	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.logger.Info("今日获得0次云朵，数量共计：0")
		return nil
	}

	records, ok := resultMap["records"].([]interface{})
	if !ok || len(records) == 0 {
		t.logger.Info("今日获得0次云朵，数量共计：0")
		return nil
	}

	// 筛选今日记录
	today := time.Now()
	t.todayCount = 0
	t.totalCloud = 0

	for _, record := range records {
		recordMap, ok := record.(map[string]interface{})
		if !ok {
			continue
		}

		// inserttime 为 RFC3339 字符串，例如 2026-01-28T04:58:18.000+00:00
		insertTimeStr, ok := recordMap["inserttime"].(string)
		if !ok {
			continue
		}

		num, ok := recordMap["num"].(float64)
		if !ok {
			continue
		}

		recordTime, err := time.Parse(time.RFC3339, insertTimeStr)
		if err != nil {
			continue
		}
		recordTime = recordTime.In(time.Local)

		// 仅统计今天且为正数的获得记录
		if recordTime.Day() == today.Day() &&
			recordTime.Month() == today.Month() &&
			recordTime.Year() == today.Year() &&
			num > 0 {
			t.todayCount++
			t.totalCloud += int(num)
		}
	}

	t.logger.Info(fmt.Sprintf("今日获得%d次云朵，数量共计：%d", t.todayCount, t.totalCloud))
	return nil
}

func (t *TodayCloudTask) TodayCount() int {
	return t.todayCount
}

func (t *TodayCloudTask) TotalCloud() int {
	return t.totalCloud
}
