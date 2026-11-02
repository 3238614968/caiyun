package tasks

import (
	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AutumnTask covers independent activities added by the October reference.
// Their prizes and badges are never reported as cloud balance gains.
type AutumnTask struct {
	accountTaskSettings
	api     *api.CaiyunAPI
	kind    string
	pending bool
	message string
	delay   time.Duration
}

func NewAutumnTask(client *corehttp.Client, kind string) *AutumnTask {
	return &AutumnTask{api: api.NewCaiyunAPI(client), kind: kind, delay: 800 * time.Millisecond}
}
func (t *AutumnTask) Pending() bool   { return t.pending }
func (t *AutumnTask) Message() string { return t.message }
func (t *AutumnTask) pause() {
	if t.delay > 0 {
		time.Sleep(t.delay)
	}
}
func (t *AutumnTask) Run() error {
	t.pending = false
	t.message = ""
	switch t.kind {
	case "annual_bill":
		return t.bill()
	case "luckybag":
		return t.luckybag()
	case "redcavalry":
		return t.redcavalry()
	case "fun_ai_timed_lottery":
		return t.timedLottery()
	case "redeem_code":
		return t.redeemCodes()
	}
	return fmt.Errorf("未知活动 %s", t.kind)
}
func campaignResult[T any](resp *api.CaiyunResponse, err error) (T, error) {
	var result T
	if err != nil {
		return result, err
	}
	if resp == nil || !responseCodeIs(resp, 0) {
		return result, rewardResponseError("查询活动", resp)
	}
	if resp.Result == nil {
		return result, fmt.Errorf("活动响应缺少 result")
	}
	data, err := json.Marshal(resp.Result)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(data, &result)
	return result, err
}

type billTask struct {
	Code      string `json:"taskCode"`
	Completed int    `json:"completedCount"`
	Total     int    `json:"totalCount"`
}

func (t *AutumnTask) bill() error {
	initial, err := campaignResult[[]billTask](t.api.BillTasks())
	if err != nil {
		return err
	}
	// 默认问卷答案与前端示例一致：服务端只要求 param 是对象；保留按账号覆盖。
	feedback := map[string]interface{}{
		"reasonChoice":  "A",
		"reasonDesc":    "",
		"suggestChoice": "A",
		"suggestDesc":   "",
	}
	if raw := t.setting("CAIYUN_BILL_FEEDBACK_JSON"); raw != "" {
		var configured map[string]interface{}
		if err = json.Unmarshal([]byte(raw), &configured); err != nil {
			return fmt.Errorf("账单问卷配置不是 JSON 对象: %w", err)
		}
		for key, value := range configured {
			feedback[key] = value
		}
	}
	var issues []error
	for _, task := range initial {
		if task.Total > 0 && task.Completed >= task.Total {
			continue
		}
		switch task.Code {
		case "BROWSE_REPORT", "MEMORY_ALBUM", "AI_PHOTO", "FEEDBACK":
		default:
			continue
		}
		resp, e := t.api.BillExecute(task.Code, feedback)
		if e == nil {
			e = rewardResponseError("年度账单/"+task.Code, resp, 10003)
		}
		if e != nil {
			issues = append(issues, e)
		}
		t.pause()
	}
	latest, e := campaignResult[[]billTask](t.api.BillTasks())
	if e != nil {
		issues = append(issues, e)
		t.pending = true
	}
	done := 0
	var left []string
	for _, task := range latest {
		if task.Total > 0 && task.Completed >= task.Total {
			done++
		} else {
			left = append(left, task.Code)
		}
	}
	t.pending = t.pending || len(left) > 0
	// queryCount can be either a number or an object; malformed counts fail closed.
	countResp, e := t.api.BillLotteryCount()
	count := 0
	if e == nil {
		count, e = campaignCount(countResp, "lotteryCount", "count", "remainingCount")
	}
	if e != nil {
		issues = append(issues, e)
	}
	draws := 0
	for draws < count && draws < 10 {
		resp, e := t.api.BillDraw()
		if e != nil {
			issues = append(issues, e)
			break
		}
		if responseCodeIs(resp, 30001) {
			break
		}
		if !responseCodeIs(resp, 0, 30004) {
			issues = append(issues, rewardResponseError("年度账单抽奖", resp))
			break
		}
		draws++
		t.pause()
	}
	remaining, e := t.api.BillLotteryCount()
	if e == nil {
		var n int
		n, e = campaignCount(remaining, "lotteryCount", "count", "remainingCount")
		t.pending = t.pending || n > 0
	}
	if e != nil {
		issues = append(issues, e)
	}
	t.message = fmt.Sprintf("年度账单确认完成%d/%d项；抽奖%d次", done, len(latest), draws)
	if len(left) > 0 {
		t.message += "；待完成：" + strings.Join(left, "、")
	}
	return errors.Join(issues...)
}
func campaignCount(resp *api.CaiyunResponse, keys ...string) (int, error) {
	if err := rewardResponseError("查询抽奖次数", resp); err != nil {
		return 0, err
	}
	value := resp.Result
	if data, ok := value.(map[string]interface{}); ok {
		value = nil
		for _, key := range keys {
			if v, exists := data[key]; exists {
				value = v
				break
			}
		}
	}
	switch value.(type) {
	case json.Number, float64, int, string:
	default:
		return 0, fmt.Errorf("抽奖次数字段缺失或格式错误")
	}
	n, err := strconv.Atoi(fmt.Sprint(value))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("抽奖次数无效")
	}
	return n, nil
}

func (t *AutumnTask) redeemCodes() error {
	values := strings.FieldsFunc(t.setting("CAIYUN_REDEEM_CODES"), func(r rune) bool { return r == ',' || r == '\n' || r == ';' })
	if len(values) == 0 {
		t.pending = true
		t.message = "未提供此账号的兑换码"
		return nil
	}
	seen := map[string]bool{}
	accepted, already := 0, 0
	for _, code := range values {
		code = strings.TrimSpace(code)
		if code == "" || seen[code] {
			continue
		}
		if len(seen) >= 20 {
			t.pending = true
			break
		}
		seen[code] = true
		resp, err := t.api.RedeemActivityCode(code)
		if err != nil {
			return fmt.Errorf("兑换码请求失败（第%d项）", len(seen))
		}
		if responseCodeIs(resp, 0) {
			accepted++
		} else if resp != nil && (strings.Contains(resp.MessageText(), "已兑换") || strings.Contains(resp.MessageText(), "已领取")) {
			already++
		} else {
			return fmt.Errorf("兑换码未被接受（第%d项）", len(seen))
		}
		t.pause()
	}
	t.message = fmt.Sprintf("兑换码服务端接受%d项、此前已领取%d项", accepted, already)
	return nil
}
func (t *AutumnTask) luckybag() error {
	resp, err := t.api.LuckybagInfo()
	if err != nil {
		return err
	}
	if responseCodeIs(resp, 604) {
		t.message = "开福袋仅限移动用户，此账号不符合资格"
		return nil
	}
	count, err := campaignCount(resp, "totalLotteryCount")
	if err != nil {
		return err
	}
	sub, err := t.api.LuckybagSubscribe()
	if err != nil {
		return err
	}
	if err = rewardResponseError("福袋订阅", sub); err != nil {
		return err
	}
	draws := 0
	for draws < count && draws < 10 {
		r, e := t.api.LuckybagDraw()
		if e != nil {
			return e
		}
		if responseCodeIs(r, 10001) {
			break
		}
		if e = rewardResponseError("开福袋", r); e != nil {
			return e
		}
		draws++
		t.pause()
	}
	final, err := t.api.LuckybagInfo()
	if err != nil {
		return err
	}
	remaining, err := campaignCount(final, "totalLotteryCount")
	if err != nil {
		return err
	}
	t.pending = remaining > 0
	t.message = fmt.Sprintf("开福袋%d次；服务端剩余%d次（奖品不计入云朵收益）", draws, remaining)
	return nil
}

type redStation struct {
	Code        string `json:"stationCode"`
	QuizPassed  bool   `json:"quizPassed"`
	BadgeEarned bool   `json:"badgeEarned"`
}
type redProgress struct {
	Stations  []redStation `json:"stations"`
	AllBadges bool         `json:"allBadgesEarned"`
	Heritage  interface{}  `json:"heritageNo"`
}

func (t *AutumnTask) redcavalry() error {
	initial, err := campaignResult[redProgress](t.api.RedCavalryProgress())
	if err != nil {
		return err
	}
	if len(initial.Stations) == 0 {
		return fmt.Errorf("红色骑兵未下发站点")
	}
	var issues []error
	for _, station := range initial.Stations {
		if station.QuizPassed && station.BadgeEarned {
			continue
		}
		if station.Code == "" {
			issues = append(issues, fmt.Errorf("缺少站点编号"))
			continue
		}
		if e := t.redStation(station.Code); e != nil {
			issues = append(issues, e)
		}
	}
	latest, err := campaignResult[redProgress](t.api.RedCavalryProgress())
	if err != nil {
		return errors.Join(append(issues, err)...)
	}
	done := 0
	for _, s := range latest.Stations {
		if s.QuizPassed && s.BadgeEarned {
			done++
		}
	}
	t.pending = done != len(initial.Stations)
	t.message = fmt.Sprintf("红色骑兵确认通关%d/%d站", done, len(initial.Stations))
	if !t.pending {
		review, e := campaignResult[redProgress](t.api.RedCavalryReview())
		if e != nil {
			issues = append(issues, e)
		} else {
			t.pending = !review.AllBadges
			if review.Heritage != nil {
				t.message += fmt.Sprintf("；传承编号%v", review.Heritage)
			}
		}
	}
	return errors.Join(issues...)
}
func (t *AutumnTask) redStation(station string) error {
	for _, game := range []bool{false, true} {
		r, e := t.api.RedCavalryPrepare(station, game)
		if e != nil {
			return e
		}
		if e = rewardResponseError("红色骑兵前置步骤", r); e != nil {
			return e
		}
		t.pause()
	}
	q, e := campaignResult[struct {
		ID interface{} `json:"id"`
	}](t.api.RedCavalryQuestion(station))
	if e != nil {
		return e
	}
	if q.ID == nil {
		return fmt.Errorf("题目缺少 ID")
	}
	answer := "A"
	for attempt := 0; attempt < 2; attempt++ {
		r, e := campaignResult[struct {
			Correct bool   `json:"correct"`
			Answer  string `json:"correctAnswer"`
		}](t.api.RedCavalryAnswer(station, q.ID, answer))
		if e != nil {
			return e
		}
		if r.Correct {
			return nil
		}
		next := strings.ToUpper(strings.TrimSpace(r.Answer))
		if len(next) != 1 || !strings.Contains("ABCD", next) || next == answer {
			return nil
		}
		answer = next
		t.pause()
	}
	return nil
}

func (t *AutumnTask) timedLottery() error {
	modules, err := t.api.FunaiTimedModules()
	if err != nil {
		return err
	}
	draws := 0
	var issues []error
	seen := map[string]bool{}
	for _, module := range modules {
		for _, task := range module.TaskIDs {
			key := module.ModuleID + "/" + task.ID
			if seen[key] || task.IsComplete != 1 {
				continue
			}
			seen[key] = true
			r, e := t.api.FunaiTimedLottery(module.ModuleID, task.ID)
			if e != nil {
				issues = append(issues, e)
				continue
			}
			if responseCodeIs(r, 0) {
				draws++
			} else if !responseCodeIs(r, 10001) && !isNoLotteryChanceError(fmt.Errorf("%s", r.MessageText())) {
				issues = append(issues, rewardResponseError("限时抽奖/"+key, r))
			}
			t.pause()
		}
	}
	t.message = fmt.Sprintf("限时活动抽奖%d次（各已完成任务最多尝试一次）", draws)
	return errors.Join(issues...)
}
