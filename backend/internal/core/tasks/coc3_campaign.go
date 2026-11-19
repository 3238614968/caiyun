package tasks

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// This is the reference's vetted list, not every zero-price product in the
// discovery catalogue. Product price alone does not establish free eligibility.
var coc3CampaignActivities = []struct {
	id   int
	name string
}{
	{17456, "好礼月月领"}, {17524, "移动AI你幸运礼"}, {17761, "一豆赢惊喜"},
	{17766, "好物福利仓二期"}, {17451, "三日签·第2天"}, {17452, "三日签·第3天"},
	{17453, "三日签·话费券"}, {17518, "AI你科技礼秒杀"}, {17764, "周周有权益秒杀"},
	{17765, "周周有权益"}, {17756, "金科苹果18pro"}, {17085, "新人有好礼"},
	{17071, "盲盒兑换"}, {17238, "超级芒盒·黄金"}, {17239, "超级芒盒·钻石"},
}

//go:embed assets/coc3/activities.json
var coc3ActivityCatalogue []byte

func NewCoc3ScanTask(client *corehttp.Client, log *logger.Logger) *Coc3Task {
	task := NewCoc3Task(client, log)
	task.scanOnly = true
	return task
}

func (t *Coc3Task) riskRejected(resp *api.Coc3Response) bool {
	var data struct {
		Risk    interface{} `json:"risk"`
		Explain string      `json:"explain"`
	}
	_ = json.Unmarshal(resp.Data, &data)
	if toBool(data.Risk) || strings.Contains(data.Explain+resp.Message, "SCORE_REFUSE") {
		t.pending, t.lotteryRiskStopped = true, true
		return true
	}
	return false
}

func (t *Coc3Task) runCampaignLottery() []string {
	if t.lotteryRiskStopped {
		return []string{"画布抽奖未提交：本轮已触发风控"}
	}
	var parts []string
	for _, activity := range coc3CampaignActivities {
		// 17761 consumes a bean and has a separate, explicit opt-in path.
		// Never retry an ambiguous order via the other lottery endpoint.
		if activity.id == 17761 || t.orderAttempted[activity.id] {
			continue
		}
		label := fmt.Sprintf("%s(%d)", activity.name, activity.id)
		eligible, err := t.api.Coc3LotteryEligibility(t.tokenValue, activity.id)
		if err != nil {
			parts = append(parts, t.failure(label+"资格预检", err))
			continue
		}
		if !eligible.Eligible {
			parts = append(parts, label+"跳过："+eligible.Reason)
			continue
		}
		count, err := t.api.ResidueLotteryCount(t.tokenValue, activity.id)
		if err != nil {
			parts = append(parts, t.failure(label+"次数查询", err))
			continue
		}
		if count == 0 {
			parts = append(parts, label+"暂无免费次数")
			continue
		}
		riskToken := t.resolveRiskToken()
		if riskToken == "" {
			t.pending = true
			return append(parts, label+"官方风险令牌未确认："+t.riskTokenError)
		}
		deviceID := t.resolveFingerprint()
		if deviceID == "" {
			t.pending = true
			return append(parts, label+"官方 BSFIT 记录未确认："+t.fingerprintError)
		}
		params := t.resolveWasmParams()
		if params == "" {
			t.pending = true
			return append(parts, label+"抽奖待确认，令牌生成失败："+t.wasmError)
		}
		t.pause(3 * time.Second)
		resp, err := t.api.SubmitCoc3Lottery(t.tokenValue, params, riskToken, deviceID, activity.id)
		if err != nil {
			// No replay: a timeout does not prove that the draw was rejected.
			parts = append(parts, t.failure(label+"抽奖提交（未自动重提）", err))
			continue
		}
		if t.riskRejected(resp) {
			return append(parts, label+"触发风控，本轮停止所有抽奖，需 App 核验")
		}
		if !resp.Success {
			parts = append(parts, t.failure(label+"抽奖", resp.BusinessError("画布抽奖")))
			continue
		}
		t.pause(1500 * time.Millisecond)
		after, err := t.api.ResidueLotteryCount(t.tokenValue, activity.id)
		if err != nil {
			parts = append(parts, t.failure(label+"结果复查（未自动重提）", err))
		} else if after >= count {
			t.pending = true
			parts = append(parts, label+"抽奖已提交，次数未减少，结果待确认（未自动重提）")
		} else {
			parts = append(parts, fmt.Sprintf("%s画布抽奖确认1次，次数%d→%d%s", label, count, after, parseCoc3Award(resp.Data)))
		}
		// Reference protocol uses a single successful submission per run/IP
		// window. Do not spend every other activity's chances in a tight loop.
		t.pending = true
		return append(parts, "其余画布抽奖机会留待下轮执行")
	}
	return append(parts, "画布活动预检完成")
}

func (t *Coc3Task) runBeanLottery() []string {
	if t.setting("CAIYUN_COC3_BEAN_DRAW") != "true" {
		return []string{"一豆赢惊喜(17761)未启用：每次消耗1权益豆，可按账号配置开启"}
	}
	if t.lotteryRiskStopped {
		return []string{"权益豆抽奖未提交：本轮已触发风控"}
	}
	limit := 1
	if raw := t.setting("CAIYUN_COC3_BEAN_MAX_DRAWS"); raw != "" {
		var err error
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 5 {
			return []string{t.failure("权益豆抽奖配置", fmt.Errorf("单轮次数必须在1至5之间"))}
		}
	}
	const activityID = 17761
	config, err := t.api.Coc3ActivityConfig(t.tokenValue, activityID)
	if err != nil {
		if coc3Unavailable(err) {
			return []string{"一豆赢惊喜活动已结束"}
		}
		return []string{t.failure("权益豆抽奖活动查询", err)}
	}
	if !config.ActiveAt(activityNow()) {
		return []string{"一豆赢惊喜不在活动时间内，未消耗权益豆"}
	}
	eligible, err := t.api.Coc3LotteryEligibility(t.tokenValue, activityID)
	if err != nil {
		return []string{t.failure("权益豆抽奖资格", err)}
	}
	if !eligible.Eligible {
		return []string{"一豆赢惊喜跳过：" + eligible.Reason}
	}
	sku, err := t.api.CurrentLotterySku(t.tokenValue, activityID)
	if err != nil {
		return []string{t.failure("权益豆抽奖SKU", err)}
	}
	if !config.ZeroPriceSKUs[sku] {
		return []string{t.failure("权益豆抽奖", fmt.Errorf("当前SKU未确认价格为0，未提交订单"))}
	}
	beans, err := t.api.GetBeanBalance(t.tokenValue)
	if err != nil {
		return []string{t.failure("权益豆抽奖余额", err)}
	}
	var parts []string
	drawn := 0
	for i := 0; i < limit && beans > 0; i++ {
		params := t.resolveWasmParams()
		if params == "" {
			t.pending = true
			parts = append(parts, "权益豆抽奖令牌生成失败："+t.wasmError)
			break
		}
		t.pause(2 * time.Second)
		resp, err := t.api.SubmitCoc3BeanOrder(t.tokenValue, params, activityID, sku)
		if err != nil {
			parts = append(parts, t.failure("权益豆抽奖提交（未自动重提）", err))
			break
		}
		if t.riskRejected(resp) {
			parts = append(parts, "权益豆抽奖触发风控，停止提交")
			break
		}
		if err := resp.BusinessError("权益豆抽奖"); err != nil {
			parts = append(parts, t.failure("权益豆抽奖", err))
			break
		}
		after, err := t.api.GetBeanBalance(t.tokenValue)
		if err != nil {
			parts = append(parts, t.failure("权益豆抽奖余额复查", err))
			break
		}
		if after != beans-1 {
			t.pending = true
			parts = append(parts, "权益豆抽奖回执与扣豆不一致，结果待确认（未自动重提）")
			break
		}
		beans, drawn = after, drawn+1
		if award := parseCoc3Award(resp.Data); award != "" {
			parts = append(parts, award)
		}
	}
	return append(parts, fmt.Sprintf("权益豆抽奖确认%d次，余额%d（不计入云朵收益）", drawn, beans))
}

func (t *Coc3Task) runActivityScan() []string {
	var catalogue []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(coc3ActivityCatalogue, &catalogue); err != nil {
		return []string{t.failure("读取活动目录", err)}
	}
	live, checked := 0, 0
	var parts []string
	for _, item := range catalogue {
		config, err := t.api.Coc3ActivityConfig(t.tokenValue, item.ID)
		if err != nil {
			if !coc3Unavailable(err) {
				parts = append(parts, t.failure(fmt.Sprintf("活动%d查询", item.ID), err))
			}
			continue
		}
		checked++
		if config.ActiveAt(activityNow()) && len(config.ZeroPriceSKUs) > 0 {
			live++
			if live <= 20 {
				parts = append(parts, fmt.Sprintf("%s(%d)：0元商品%d项", item.Name, item.ID, len(config.ZeroPriceSKUs)))
			}
		}
		t.pause(100 * time.Millisecond)
	}
	return append([]string{fmt.Sprintf("活动目录%d项，配置有效%d项，在期且有0元商品%d项（只读，不自动下单）", len(catalogue), checked, live)}, parts...)
}
