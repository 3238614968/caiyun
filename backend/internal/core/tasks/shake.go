package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// ShakeTask 摇一摇任务
type ShakeTask struct {
	client *http.Client
	logger *logger.Logger
}

// NewShakeTask 创建摇一摇任务
func NewShakeTask(client *http.Client, log *logger.Logger) *ShakeTask {
	return &ShakeTask{
		client: client,
		logger: log,
	}
}

// Run 执行摇一摇任务
func (t *ShakeTask) Run() error {
	return t.RunWithConfig(15, time.Second)
}

// RunWithConfig 按指定次数和间隔执行摇一摇
func (t *ShakeTask) RunWithConfig(times int, delay time.Duration) error {
	if times <= 0 {
		return nil
	}
	if delay < 0 {
		delay = 0
	}

	for i := 0; i < times; i++ {
		result, err := t.shake()
		if err != nil {
			// 静默失败，继续下一次
			continue
		}

		// 直接输出结果，保持与原 JS 版本接近的控制台展示风格
		if result.Explain != "" {
			fmt.Printf(" [摇一摇] %s\n", result.Explain)
		} else if result.Img != "" {
			fmt.Printf(" [摇一摇] %s\n", result.Img)
		}

		if i < times-1 && delay > 0 {
			time.Sleep(delay)
		}
	}

	return nil
}

// ShakeResult 摇一摇结果
type ShakeResult struct {
	ShakePrizeconfig struct {
		Title string `json:"title"`
		Name  string `json:"name"`
	} `json:"shakePrizeconfig"`
	ShakeRecommend struct {
		Explain string `json:"explain"`
		Img     string `json:"img"`
	} `json:"shakeRecommend"`
	Explain string `json:"explain"` // 有时直接在顶层
	Img     string `json:"img"`     // 有时直接在顶层
}

// shake 执行一次摇一摇
func (t *ShakeTask) shake() (*ShakeResult, error) {
	url := "https://caiyun.feixin.10086.cn/market/shake-server/shake/shakeIt?flag=1"
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := t.client.Post(url, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var apiResult struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Msg     string      `json:"msg"`
		Result  ShakeResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &apiResult); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	code := apiResult.Code
	if code != 0 {
		msg := apiResult.Message
		if msg == "" {
			msg = apiResult.Msg
		}
		return nil, fmt.Errorf("摇一摇失败: code=%d, msg=%s", code, msg)
	}

	// 兼容不同返回结构，统一映射到 Explain/Img
	result := &apiResult.Result
	if result.ShakeRecommend.Explain != "" {
		result.Explain = result.ShakeRecommend.Explain
	}
	if result.ShakeRecommend.Img != "" {
		result.Img = result.ShakeRecommend.Img
	}
	if result.ShakePrizeconfig.Title != "" || result.ShakePrizeconfig.Name != "" {
		result.Explain = result.ShakePrizeconfig.Title + result.ShakePrizeconfig.Name
	}

	return result, nil
}
