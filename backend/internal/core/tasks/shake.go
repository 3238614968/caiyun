package tasks

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// ShakeTask 全网抽抽乐；完整奖品与碎片分开记录。
type ShakeTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

func NewShakeTask(client *http.Client, log *logger.Logger) *ShakeTask {
	return &ShakeTask{api: api.NewCaiyunAPI(client), logger: log}
}

func (t *ShakeTask) Message() string { return t.lastMessage }

func (t *ShakeTask) Run() error { return t.RunWithConfig(3, 1500*time.Millisecond) }

func (t *ShakeTask) RunWithConfig(times int, delay time.Duration) error {
	t.lastMessage = ""
	if times <= 0 {
		t.lastMessage = "抽奖次数为0，未发起抽奖"
		return nil
	}
	draws, wins, fragments, unknown := 0, 0, 0, 0
	var prizes []string
	for i := 0; i < times; i++ {
		resp, err := t.api.ShakeDraw()
		if err != nil {
			return err
		}
		if !responseCodeIs(resp, 0) {
			return fmt.Errorf("抽抽乐第%d次失败: code=%v %s", i+1, resp.Code, resp.MessageText())
		}
		result, ok := resp.Result.(map[string]interface{})
		if !ok {
			return fmt.Errorf("抽抽乐响应缺少有效 result")
		}
		draws++
		if prize, ok := result["shakePrizeconfig"].(map[string]interface{}); ok {
			if name, _ := prize["name"].(string); name != "" {
				need, parseErr := strconv.Atoi(fmt.Sprint(prize["fragment"]))
				if parseErr == nil && need == 0 {
					wins++
					prizes = append(prizes, name)
				} else if parseErr == nil && need > 0 {
					fragments++
					prizes = append(prizes, name+"（碎片）")
				} else {
					unknown++
					prizes = append(prizes, name+"（类型待确认）")
				}
			}
		}
		if i < times-1 && delay > 0 {
			time.Sleep(delay)
		}
	}
	parts := []string{fmt.Sprintf("抽奖%d次、完整奖品%d次、碎片%d次", draws, wins, fragments)}
	if unknown > 0 {
		parts = append(parts, fmt.Sprintf("奖品类型待确认%d次", unknown))
	}
	if len(prizes) > 0 {
		parts = append(parts, strings.Join(prizes, "、"))
	}
	resp, err := t.api.ShakeFragments()
	if err != nil {
		return err
	}
	if !responseCodeIs(resp, 0) {
		return fmt.Errorf("抽抽乐碎片复查失败: code=%v %s", resp.Code, resp.MessageText())
	}
	items, ok := resp.Result.([]interface{})
	if !ok {
		return fmt.Errorf("抽抽乐碎片响应缺少有效 result")
	}
	for _, item := range items {
		row, _ := item.(map[string]interface{})
		prize, _ := row["shakePrizeconfig"].(map[string]interface{})
		if have := responseNumber(row["fragmentNum"]); have > 0 {
			parts = append(parts, fmt.Sprintf("%v碎片%d/%d", prize["name"], have, responseNumber(prize["fragment"])))
		}
	}
	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return nil
}
