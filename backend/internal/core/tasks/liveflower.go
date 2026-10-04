package tasks

import (
	"fmt"
	"strconv"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// LiveFlowerTask 仅领取首次参与赠花；看播和验证码领奖由账号持有人完成。
type LiveFlowerTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
	pending     bool
}

func NewLiveFlowerTask(client *http.Client, log *logger.Logger) *LiveFlowerTask {
	return &LiveFlowerTask{api: api.NewCaiyunAPI(client), logger: log}
}

func (t *LiveFlowerTask) Message() string { return t.lastMessage }
func (t *LiveFlowerTask) Pending() bool   { return t.pending }

func (t *LiveFlowerTask) Run() error {
	t.lastMessage, t.pending = "", false
	before, _, err := t.info()
	if err != nil {
		return err
	}
	gift, err := t.api.LiveFlowerFirstGift()
	if err != nil {
		return err
	}
	if !responseCodeIs(gift, 0, 200110) {
		return fmt.Errorf("直播首次赠花失败: code=%v %s", gift.Code, gift.MessageText())
	}
	after, info, err := t.info()
	if err != nil {
		return err
	}
	status := "首次赠花已到账"
	if responseCodeIs(gift, 200110) {
		status = "首次赠花此前已领取"
	} else if after <= before {
		status = "首次赠花尚未确认到账"
		t.pending = true
	}
	t.lastMessage = fmt.Sprintf("%s；红花%d→%d；有效期%v；看播与验证码领奖需在App完成", status, before, after, info["redFlowerValidDate"])
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return nil
}

func (t *LiveFlowerTask) info() (int64, map[string]interface{}, error) {
	resp, err := t.api.LiveFlowerInfo()
	if err != nil {
		return 0, nil, err
	}
	if !responseCodeIs(resp, 0) {
		return 0, nil, fmt.Errorf("直播红花查询失败: code=%v %s", resp.Code, resp.MessageText())
	}
	info, ok := resp.Result.(map[string]interface{})
	if !ok {
		return 0, nil, fmt.Errorf("直播红花响应缺少有效 result")
	}
	count, err := strconv.ParseInt(fmt.Sprint(info["flowerCnt"]), 10, 64)
	if err != nil || count < 0 {
		return 0, nil, fmt.Errorf("直播红花响应缺少有效 flowerCnt")
	}
	return count, info, nil
}
