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

// WeChatTask uses only the first, free daily draw. Every response is checked;
// an absent code or a nonzero string code can never become a success.
type WeChatTask struct {
	api              *api.CaiyunAPI
	logger           *logger.Logger
	message          string
	pending, skipped bool
}

func NewWeChatTask(client *http.Client, log *logger.Logger) *WeChatTask {
	return &WeChatTask{api: api.NewCaiyunAPI(client), logger: log}
}
func (t *WeChatTask) Message() string { return t.message }
func (t *WeChatTask) Pending() bool   { return t.pending }
func (t *WeChatTask) Skipped() bool   { return t.skipped }
func (t *WeChatTask) reset()          { t.message = ""; t.pending = false; t.skipped = false }
func (t *WeChatTask) check(operation string, resp *api.CaiyunResponse) error {
	if responseCodeIs(resp, 0) {
		return nil
	}
	if resp != nil && (responseCodeIs(resp, 602) || strings.Contains(resp.MessageText(), "活动已结束")) {
		t.skipped = true
		t.message = operation + "活动已结束"
		return nil
	}
	if resp != nil {
		for _, phrase := range []string{"未绑定", "未关注", "请先关注", "尚未绑定"} {
			if strings.Contains(resp.MessageText(), phrase) {
				t.pending = true
				t.message = operation + "需绑定微信公众号：中国移动云盘"
				return nil
			}
		}
	}
	return rewardResponseError(operation, resp)
}
func (t *WeChatTask) RunSignIn() error {
	t.reset()
	resp, err := t.api.SignInWx()
	if err != nil {
		return err
	}
	if err = t.check("微信签到", resp); err != nil || t.skipped || t.pending {
		return err
	}
	// The endpoint can acknowledge a request without supplying a completion bit.
	result, _ := resp.Result.(map[string]interface{})
	confirmed := false
	for _, key := range []string{"todaySigned", "isSign", "isSigned"} {
		if value, exists := result[key]; exists && toBool(value) {
			confirmed = true
		}
	}
	t.pending = !confirmed
	if confirmed {
		t.message = "微信签到完成状态已确认"
	} else {
		t.message = "微信签到请求已接受，未返回可核验的完成状态"
	}
	return nil
}
func (t *WeChatTask) RunDraw(times int) error {
	return t.RunDrawWithInterval(times, 500*time.Millisecond)
}
func wechatRemaining(resp *api.CaiyunResponse) (int, error) {
	if err := rewardResponseError("微信抽奖次数", resp); err != nil {
		return 0, err
	}
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("微信抽奖缺少次数记录")
	}
	value, ok := result["surplusNumber"]
	if !ok {
		return 0, fmt.Errorf("微信抽奖缺少剩余次数")
	}
	n, err := strconv.Atoi(fmt.Sprint(value))
	if err != nil || n < 0 || n > 50 {
		return 0, fmt.Errorf("微信抽奖次数无效")
	}
	return n, nil
}
func (t *WeChatTask) RunDrawWithInterval(times int, _ time.Duration) error {
	t.reset()
	if times <= 0 {
		t.skipped = true
		t.message = "微信抽奖未启用"
		return nil
	}
	info, err := t.api.GetDrawInWx()
	if err != nil {
		return err
	}
	if err = t.check("微信抽奖", info); err != nil || t.skipped || t.pending {
		return err
	}
	count, err := wechatRemaining(info)
	if err != nil {
		return err
	}
	if count < 50 {
		t.message = "今日微信免费抽奖机会已使用，不消耗云朵继续抽奖"
		return nil
	}
	resp, err := t.api.WxDraw()
	if err != nil {
		return fmt.Errorf("微信抽奖提交异常（未自动重提）: %w", err)
	}
	if err = t.check("微信抽奖", resp); err != nil || t.skipped || t.pending {
		return err
	}
	after, err := t.api.GetDrawInWx()
	if err != nil {
		return fmt.Errorf("微信抽奖结果复查: %w", err)
	}
	remaining, err := wechatRemaining(after)
	if err != nil {
		return err
	}
	if remaining >= count {
		t.pending = true
		t.message = "微信抽奖已提交，剩余次数未减少，结果待确认（未自动重提）"
		return nil
	}
	t.message = fmt.Sprintf("微信免费抽奖确认1次，次数%d→%d", count, remaining)
	if result, ok := resp.Result.(map[string]interface{}); ok {
		if name, ok := result["prizeName"].(string); ok && name != "" {
			t.message += "：" + name
		}
	}
	return nil
}
