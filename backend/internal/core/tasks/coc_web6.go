package tasks

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

type CocWeb6Task struct {
	accountTaskSettings
	api     *api.CocWeb6API
	auth    string
	pending bool
	message string
	wait    func(time.Duration)
}

func NewCocWeb6Task(client *corehttp.Client, _ *logger.Logger) *CocWeb6Task {
	return &CocWeb6Task{api: api.NewCocWeb6API(client), wait: time.Sleep}
}

func (t *CocWeb6Task) SetAccountContext(phone, authorization string) *CocWeb6Task {
	t.SetAccountPhone(phone)
	t.auth = strings.TrimSpace(authorization)
	return t
}

func (t *CocWeb6Task) Pending() bool   { return t.pending }
func (t *CocWeb6Task) Message() string { return t.message }

func (t *CocWeb6Task) Run() error {
	t.pending, t.message = false, ""
	token, err := t.api.Login(t.auth, t.settingsPhone)
	if err != nil {
		return err
	}
	var parts []string
	var issues []error
	record := func(message string, err error) {
		if message != "" {
			parts = append(parts, message)
		}
		if err != nil {
			issues = append(issues, err)
		}
	}
	record(t.checkIn(token))
	record(t.draw(token))
	record(t.growth(token))
	t.message = strings.Join(parts, "；")
	return errors.Join(issues...)
}

func (t *CocWeb6Task) checkIn(token string) (string, error) {
	dates, err := t.api.ClockInDates(token)
	if err != nil {
		return "", err
	}
	today := activityNow().In(time.FixedZone("CST", 8*3600)).Format("20060102")
	containsToday := func(values []string) bool {
		for _, value := range values {
			if strings.ReplaceAll(value, "-", "") == today {
				return true
			}
		}
		return false
	}
	if containsToday(dates) {
		return fmt.Sprintf("7日打卡今日已完成，累计%d天", len(dates)), nil
	}
	_, submitErr := t.api.Prize(token, api.CocWeb6SignActivity)
	t.wait(time.Second)
	after, err := t.api.ClockInDates(token)
	if err != nil {
		return "7日打卡已提交，日期记录复查失败（未自动重提）", errors.Join(submitErr, err)
	}
	if containsToday(after) {
		return fmt.Sprintf("7日打卡日期记录已确认，累计%d天", len(after)), nil
	}
	t.pending = true
	return "7日打卡未查到今日完成记录（未自动重提）", submitErr
}

func (t *CocWeb6Task) draw(token string) (string, error) {
	count, err := t.api.DrawCount(token)
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "7日接力暂无免费抽奖次数", nil
	}
	resp, err := t.api.Prize(token, api.CocWeb6DrawActivity)
	if err != nil {
		return "7日接力抽奖提交异常（未自动重提）", err
	}
	t.wait(time.Second)
	after, err := t.api.DrawCount(token)
	if err != nil {
		return "7日接力抽奖结果待确认（未自动重提）", err
	}
	if after >= count {
		t.pending = true
		return "7日接力抽奖已提交，次数未减少，待确认（未自动重提）", nil
	}
	t.pending = t.pending || after > 0
	var result struct {
		SKU  json.RawMessage `json:"skuCode"`
		Name string          `json:"prizeName"`
	}
	_ = json.Unmarshal(resp.Data, &result)
	name := result.Name
	if strings.Trim(string(result.SKU), `"`) == "999999999999" {
		name = "未中奖"
	}
	if name == "" {
		name = "奖品以服务端记录为准"
	}
	return fmt.Sprintf("7日接力抽奖确认1次，次数%d→%d：%s", count, after, name), nil
}

func (t *CocWeb6Task) growth(token string) (string, error) {
	province := t.setting("CAIYUN_COC_WEB6_PROVINCE_CODE")
	if province == "" {
		province = "100"
	}
	level, levelErr := t.api.GrowthLevel(token, province)
	parts := []string{}
	if levelErr == nil {
		parts = append(parts, "网龄成长等级"+level)
	}
	evaluated, err := t.api.Evaluated(token)
	if err != nil {
		return strings.Join(parts, "；"), errors.Join(levelErr, err)
	}
	if evaluated {
		return strings.Join(append(parts, "网龄评价服务端已确认"), "；"), levelErr
	}
	raw := t.setting("CAIYUN_COC_WEB6_EVALUATION_JSON")
	if raw == "" {
		t.pending = true
		return strings.Join(append(parts, "网龄评价未完成，需配置此账号的真实评价内容"), "；"), levelErr
	}
	var evaluation struct {
		Score   int    `json:"score"`
		Label   string `json:"label"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &evaluation); err != nil {
		return strings.Join(parts, "；"), errors.Join(levelErr, fmt.Errorf("网龄评价配置无效: %w", err))
	}
	submitErr := t.api.Evaluate(token, evaluation.Score, evaluation.Label, evaluation.Content)
	t.wait(time.Second)
	done, checkErr := t.api.Evaluated(token)
	if checkErr != nil {
		return strings.Join(parts, "；"), errors.Join(levelErr, submitErr, checkErr)
	}
	if !done {
		t.pending = true
		return strings.Join(append(parts, "网龄评价未查到完成状态（未自动重提）"), "；"), errors.Join(levelErr, submitErr)
	}
	return strings.Join(append(parts, "网龄评价完成状态已确认"), "；"), levelErr
}
