package tasks

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
)

// The reference's extra step also saves activity favourites. It changes account
// preferences but offers no rewards, so the catalog exposes it as an opt-in.
type ActivityCollectTask struct {
	api     *api.CaiyunAPI
	message string
	pending bool
	wait    func(time.Duration)
}

func NewActivityCollectTask(client *corehttp.Client) *ActivityCollectTask {
	return &ActivityCollectTask{api: api.NewCaiyunAPI(client), wait: time.Sleep}
}
func (t *ActivityCollectTask) Message() string { return t.message }
func (t *ActivityCollectTask) Pending() bool   { return t.pending }
func (t *ActivityCollectTask) Run() error {
	t.pending, t.message = false, ""
	resp, err := t.api.ActivityCentreAdverts()
	if err != nil {
		return err
	}
	if err := rewardResponseError("活动中心广告列表", resp); err != nil {
		return err
	}
	items, ok := resp.Result.([]interface{})
	if !ok {
		return fmt.Errorf("活动中心未返回有效广告列表")
	}
	seen := map[int]bool{}
	accepted, attempted := 0, 0
	var issues []error
	for _, item := range items {
		row, ok := item.(map[string]interface{})
		if !ok {
			issues = append(issues, fmt.Errorf("活动广告数据无效"))
			continue
		}
		id, err := strconv.Atoi(fmt.Sprint(row["id"]))
		if err != nil || id <= 0 {
			issues = append(issues, fmt.Errorf("活动广告缺少有效ID"))
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		if attempted >= 100 {
			t.pending = true
			break
		}
		attempted++
		result, err := t.api.CollectActivity(id)
		if err == nil {
			err = rewardResponseError(fmt.Sprintf("收藏活动%d", id), result)
		}
		if err != nil {
			issues = append(issues, err)
		} else {
			accepted++
		}
		t.wait(150 * time.Millisecond)
	}
	t.message = fmt.Sprintf("服务端接受活动收藏%d/%d项（无云朵奖励）", accepted, attempted)
	if t.pending {
		t.message += "；剩余活动待下轮处理"
	}
	if len(issues) > 0 {
		t.message += fmt.Sprintf("；失败%d项", len(issues))
	}
	return errors.Join(issues...)
}
