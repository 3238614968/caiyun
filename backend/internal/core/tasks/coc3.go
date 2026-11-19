package tasks

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// Coc3Task 中国移动权益市场任务（云盘 ck → 056002 票 → coc3 token）。
//
// 默认动作不消耗云朵/权益豆/话费；权益豆订单仅在账号显式配置后执行。
//   - 累签：好礼月月领(2999) 每日签到，成功直接下发奖励（-9014 已签=幂等成功）
//   - 浏览：task/visit × 6 组（推进浏览类任务计数）
//   - 盘点：任务进度 / 权益豆余额（只读）
//   - AI豆苗：3115 签到、动态浏览/分享任务、阳光值气泡，复查记录与余额。
//   - 0 元抽奖：仅免费次数通道（basePayType=3），使用内置 wheel 资源生成令牌。
//     令牌由纯 Go WASM 运行时生成，覆盖 mixedOrder 与画布 lottery 两种提交。
//   - 一豆赢惊喜：默认关闭，按账号启用后最多消耗配置上限的权益豆并复查扣豆。
type Coc3Task struct {
	accountTaskSettings
	client             *http.Client
	logger             *logger.Logger
	api                *api.Coc3API
	phone              string
	auth               string
	tokenValue         string
	pending            bool
	message            string
	issues             []error
	wait               func(time.Duration)
	wasmError          string
	riskToken          string
	riskTokenError     string
	fingerprint        string
	fingerprintError   string
	orderAttempted     map[int]bool
	lotteryRiskStopped bool
	scanOnly           bool
	progressTaskIDs    []int
}

// coc3SignTasks 累签任务表（活动下线是常态：-9009 按不可用如实跳过）。
var coc3SignTasks = []struct {
	id   int
	name string
}{
	{2999, "好礼月月领"},
}

// coc3SignChannel 签到提交渠道码（实测 P00000067264 被接受；渠道码参与路由）。
const coc3SignChannel = "P00000067264"

// coc3Visits 任务浏览组（visitUrl 是页面代码非网址；HAR 实测 10 组中 6 组有效）。
var coc3Visits = []struct {
	channel string
	page    string
}{
	{"P00000139704", "26xysrqtyld"},
	{"P00000137248", "2026ydchl"},
	{"P00000123846", "2026xyyxyddhl"},
	{"P00000067264", "ydagrjh"},
	{"P00000123844", "wljxmh"},
	{"P00000067264", "26xysrqtyld"},
}

// coc3LotteryActs 免费次数型抽奖活动（count 模式；权益豆型刻意不列）。
var coc3LotteryActs = []struct {
	id   int
	name string
}{
	{17524, "AI你幸运礼"},
	{17451, "三日签·流量"},
	{17452, "三日签·话费"},
	{17453, "三日签·大额话费"},
}

func NewCoc3Task(client *http.Client, log *logger.Logger) *Coc3Task {
	return &Coc3Task{client: client, logger: log, api: api.NewCoc3API(client)}
}

// SetAccountContext 设置账号上下文（云盘 Authorization 原值 + 手机号）。
func (t *Coc3Task) SetAccountContext(phone, authToken string) *Coc3Task {
	t.phone = strings.TrimSpace(phone)
	t.auth = strings.TrimSpace(authToken)
	t.SetAccountPhone(phone)
	return t
}

func (t *Coc3Task) Pending() bool   { return t.pending }
func (t *Coc3Task) Message() string { return t.message }

func (t *Coc3Task) note(parts ...string) {
	t.message = strings.Join(parts, "；")
}

func (t *Coc3Task) pause(delay time.Duration) {
	if t.wait != nil {
		t.wait(delay)
		return
	}
	time.Sleep(delay)
}

func (t *Coc3Task) failure(operation string, err error) string {
	issue := fmt.Errorf("%s: %w", operation, err)
	t.issues = append(t.issues, issue)
	return issue.Error()
}

func coc3Unavailable(err error) bool {
	var business *api.Coc3BusinessError
	return errors.As(err, &business) && (business.Code == "-9009" || strings.Contains(business.Message, "活动已结束"))
}

func (t *Coc3Task) Run() error {
	t.pending = false
	t.message = ""
	t.issues = nil
	t.wasmError = ""
	t.riskToken, t.riskTokenError = "", ""
	t.fingerprint, t.fingerprintError = "", ""
	t.orderAttempted = make(map[int]bool)
	t.lotteryRiskStopped = false
	t.progressTaskIDs = nil
	if t.auth == "" || t.phone == "" {
		return fmt.Errorf("缺少账号凭据，无法换 coc3 token")
	}
	login, err := t.api.LoginCoc3(t.auth, t.phone)
	if err != nil {
		return err
	}
	t.tokenValue = login.Token
	t.logger.Info(fmt.Sprintf("coc3 登录会话已建立：%s（%s 省），继续执行并复查任务", login.MobileMask, login.ProvinceCode))

	var parts []string
	if t.scanOnly {
		parts = append(parts, t.runActivityScan()...)
		t.note(parts...)
		return errors.Join(t.issues...)
	}
	parts = append(parts, t.runSignIn()...)
	parts = append(parts, t.runSeed()...)
	if raw := t.setting("CAIYUN_COC3_TASK_IDS"); raw != "" {
		ids, configErr := coc3ConfiguredTaskIDs(raw)
		if configErr != nil {
			parts = append(parts, t.failure("任务ID配置", configErr))
		} else {
			t.progressTaskIDs = ids
		}
	}
	parts = append(parts, t.runVisits()...)
	parts = append(parts, t.runInventory(t.progressTaskIDs)...)
	parts = append(parts, t.runLottery()...)
	parts = append(parts, t.runCampaignLottery()...)
	parts = append(parts, t.runBeanLottery()...)
	t.note(parts...)
	if t.pending {
		t.message += "；仍有未完成或待确认项"
	}
	t.logger.Info("coc3: " + t.message)
	return errors.Join(t.issues...)
}

// runSignIn 累签：未签则提交；-9014/-1108 已签按幂等成功归一。
func (t *Coc3Task) runSignIn() []string {
	var parts []string
	for _, task := range coc3SignTasks {
		info, err := t.api.GetTaskSignInfo(t.tokenValue, task.id)
		if err != nil {
			parts = append(parts, t.failure(fmt.Sprintf("%s(%d)状态查询", task.name, task.id), err))
			continue
		}
		if info.Unavailable {
			parts = append(parts, fmt.Sprintf("%s(%d)不可用(%s)", task.name, task.id, info.UnavailableReason))
			continue
		}
		if info.SignToday {
			parts = append(parts, fmt.Sprintf("%s(%d)今日已签(累签%d天)", task.name, task.id, info.AccumulateCount))
			continue
		}
		t.pause(2 * time.Second)
		resp, err := t.api.AccumulateSign(t.tokenValue, task.id, coc3SignChannel)
		if err != nil {
			parts = append(parts, t.failure(fmt.Sprintf("%s(%d)签到", task.name, task.id), err))
			continue
		}
		switch {
		case resp.Success:
			t.pause(time.Second)
			confirmed, checkErr := t.api.GetTaskSignInfo(t.tokenValue, task.id)
			if checkErr != nil {
				parts = append(parts, t.failure(fmt.Sprintf("%s(%d)签到复查", task.name, task.id), checkErr))
			} else if !confirmed.SignToday {
				t.pending = true
				parts = append(parts, fmt.Sprintf("%s(%d)签到已提交，服务端尚未确认", task.name, task.id))
			} else {
				parts = append(parts, fmt.Sprintf("%s(%d)签到成功（服务端已确认）%s", task.name, task.id, parseCoc3Award(resp.Data)))
			}
		case resp.ResultCode == -9014 || resp.ResultCode == -1108 || strings.Contains(resp.Message, "已签"):
			confirmed, checkErr := t.api.GetTaskSignInfo(t.tokenValue, task.id)
			if checkErr != nil {
				parts = append(parts, t.failure(fmt.Sprintf("%s(%d)已签状态复查", task.name, task.id), checkErr))
			} else if !confirmed.SignToday {
				t.pending = true
				parts = append(parts, fmt.Sprintf("%s(%d)返回已签提示，但今日记录未确认", task.name, task.id))
			} else {
				parts = append(parts, fmt.Sprintf("%s(%d)今日已签（复查确认）", task.name, task.id))
			}
		default:
			issue := resp.BusinessError("签到")
			if coc3Unavailable(issue) {
				parts = append(parts, fmt.Sprintf("%s(%d)活动已结束", task.name, task.id))
			} else {
				parts = append(parts, t.failure(fmt.Sprintf("%s(%d)", task.name, task.id), issue))
			}
		}
	}
	return parts
}

// runVisits 任务浏览：visitUrl 是页面代码，逐组上报。
func (t *Coc3Task) runVisits() []string {
	accepted, total := 0, len(coc3Visits)
	var parts []string
	for _, v := range coc3Visits {
		resp, err := t.api.VisitTask(t.tokenValue, v.channel, v.page)
		if err != nil {
			parts = append(parts, t.failure("浏览 "+v.page, err))
		} else if strings.Contains(resp.Message+string(resp.Data), "不在配置范围内") || coc3Unavailable(resp.BusinessError("浏览")) {
			parts = append(parts, "浏览 "+v.page+"：当前活动未配置或已结束")
		} else if resp.Success {
			accepted++
		} else {
			parts = append(parts, t.failure("浏览 "+v.page, resp.BusinessError("浏览上报")))
		}
		t.pause(900 * time.Millisecond)
	}
	return append(parts, fmt.Sprintf("任务浏览%d/%d组被接受（完成状态以任务进度复查为准）", accepted, total))
}

// runInventory 只读盘点：任务进度 + 权益豆余额。
func (t *Coc3Task) runInventory(expectedIDs []int) []string {
	var parts []string
	if len(expectedIDs) == 0 {
		t.pending = true
		parts = append(parts, "当期未下发可复查的任务ID，不以历史任务ID确认完成")
	} else if resp, err := t.api.GetTaskProgress(t.tokenValue, expectedIDs); err != nil {
		parts = append(parts, t.failure("任务进度复查", err))
	} else if !resp.Success {
		parts = append(parts, t.failure("任务进度复查", resp.BusinessError("任务进度")))
	} else {
		var data struct {
			TaskQueryTimeVOS []struct {
				TaskID       json.Number `json:"taskId"`
				CompletedNum json.Number `json:"completedNum"`
				TotalNum     json.Number `json:"totalNum"`
			} `json:"taskQueryTimeVOS"`
		}
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			parts = append(parts, t.failure("任务进度数据", err))
		} else if len(data.TaskQueryTimeVOS) == 0 {
			t.pending = true
			parts = append(parts, "服务端未返回可核验的任务记录，不能确认完成")
		} else {
			progress := make([]string, 0, len(data.TaskQueryTimeVOS))
			done := 0
			seen := map[int]bool{}
			expected := map[int]bool{}
			for _, id := range expectedIDs {
				expected[id] = true
			}
			for _, v := range data.TaskQueryTimeVOS {
				id, idErr := strconv.Atoi(v.TaskID.String())
				if idErr != nil || !expected[id] || seen[id] {
					continue
				}
				seen[id] = true
				completed, ce := strconv.Atoi(v.CompletedNum.String())
				total, te := strconv.Atoi(v.TotalNum.String())
				if ce != nil || te != nil || completed < 0 || total <= 0 {
					t.pending = true
					progress = append(progress, v.TaskID.String()+":进度未下发")
					continue
				}
				progress = append(progress, fmt.Sprintf("%s:%d/%d", v.TaskID, completed, total))
				if completed >= total {
					done++
				} else {
					t.pending = true
				}
			}
			for _, id := range expectedIDs {
				if !seen[id] {
					t.pending = true
					progress = append(progress, fmt.Sprintf("%d:未返回记录", id))
				}
			}
			parts = append(parts, fmt.Sprintf("服务端确认完成%d/%d项，任务进度%s", done, len(expectedIDs), strings.Join(progress, ",")))
		}
	}
	if balance, err := t.api.GetBeanBalance(t.tokenValue); err == nil {
		parts = append(parts, fmt.Sprintf("权益豆%d", balance))
	} else {
		parts = append(parts, t.failure("权益豆余额", err))
	}
	return parts
}

// runLottery 0 元抽奖：仅免费次数；无 wasmParams 时只报次数不提交。
func (t *Coc3Task) runLottery() []string {
	var parts []string
	drawn := 0
	wasmUnavailable := false
activities:
	for _, act := range coc3LotteryActs {
		label := fmt.Sprintf("%s(%d)", act.name, act.id)
		eligible, err := t.api.Coc3LotteryEligibility(t.tokenValue, act.id)
		if err != nil {
			parts = append(parts, t.failure(label+"资格预检", err))
			continue
		}
		if !eligible.Eligible {
			parts = append(parts, label+"跳过："+eligible.Reason)
			continue
		}
		count, err := t.api.ResidueLotteryCount(t.tokenValue, act.id)
		if err != nil {
			if coc3Unavailable(err) {
				parts = append(parts, label+"活动已结束")
			} else {
				parts = append(parts, t.failure(label+"次数查询", err))
			}
			continue
		}
		if count <= 0 {
			parts = append(parts, label+"暂无免费抽奖次数")
			continue
		}
		if wasmUnavailable {
			t.pending = true
			parts = append(parts, fmt.Sprintf("%s可抽%d次（令牌生成失败，抽奖待确认：%s）", label, count, t.wasmError))
			continue
		}
		wasmParams := t.resolveWasmParams()
		if wasmParams == "" {
			wasmUnavailable, t.pending = true, true
			parts = append(parts, fmt.Sprintf("%s可抽%d次（令牌生成失败，抽奖待确认：%s）", label, count, t.wasmError))
			continue
		}
		sku, err := t.api.CurrentLotterySku(t.tokenValue, act.id)
		if err != nil {
			parts = append(parts, t.failure(label+"抽奖sku", err))
			continue
		}
		remaining := count
		for i := 0; i < count && i < 5 && remaining > 0; i++ {
			// The official wrapper performs device preflight and roto_simple in
			// the same instance for each write, using this account's COC session.
			if i > 0 {
				wasmParams = t.resolveWasmParams()
			}
			if wasmParams == "" {
				wasmUnavailable, t.pending = true, true
				parts = append(parts, fmt.Sprintf("%s可抽%d次（令牌生成失败，抽奖待确认：%s）", label, remaining, t.wasmError))
				break
			}
			if t.orderAttempted == nil {
				t.orderAttempted = make(map[int]bool)
			}
			t.orderAttempted[act.id] = true
			resp, err := t.api.SubmitCoc3ZeroOrder(t.tokenValue, wasmParams, act.id, sku)
			if err != nil {
				parts = append(parts, t.failure(label+"抽奖提交（未自动重提）", err))
				break
			}
			var risk struct {
				Risk interface{} `json:"risk"`
			}
			_ = json.Unmarshal(resp.Data, &risk)
			if toBool(risk.Risk) || strings.Contains(resp.Message, "SCORE_REFUSE") {
				t.lotteryRiskStopped = true
				t.pending = true
				parts = append(parts, label+"触发风控，停止抽奖，需App核验")
				break activities
			}
			if !resp.Success {
				parts = append(parts, t.failure(label+"抽奖", resp.BusinessError("提交")))
				break
			}
			t.pause(1500 * time.Millisecond)
			after, checkErr := t.api.ResidueLotteryCount(t.tokenValue, act.id)
			if checkErr != nil {
				parts = append(parts, t.failure(label+"抽奖结果复查（未自动重提）", checkErr))
				break
			}
			if after >= remaining {
				t.pending = true
				parts = append(parts, label+"抽奖已提交，剩余次数未减少，结果待服务端确认（未自动重提）")
				break
			}
			remaining = after
			drawn++
			if award := parseCoc3Award(resp.Data); award != "" {
				t.logger.Success(fmt.Sprintf("coc3 抽奖中奖：%s %s", act.name, award))
				parts = append(parts, fmt.Sprintf("%s(%d)★%s", act.name, act.id, award))
			}
		}
		if remaining > 0 {
			t.pending = true
		}
	}
	parts = append(parts, fmt.Sprintf("0元抽奖共%d次（已复查确认）", drawn))
	return parts
}

func coc3ConfiguredTaskIDs(raw string) ([]int, error) {
	seen := map[int]bool{}
	var ids []int
	for _, value := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		id, err := strconv.Atoi(value)
		if err != nil || id <= 0 || len(ids) >= 100 {
			return nil, fmt.Errorf("任务ID必须为正整数且最多100项")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("未配置有效任务ID")
	}
	return ids, nil
}

func parseCoc3Award(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	// 奖励形态：data 直接是数组（签到）或对象.orderInfo.prizeName（抽奖）。
	var list []struct {
		SkuName   string `json:"skuName"`
		AwardName string `json:"awardName"`
	}
	if err := json.Unmarshal(data, &list); err == nil && len(list) > 0 {
		name := list[0].SkuName
		if name == "" {
			name = list[0].AwardName
		}
		if name != "" {
			return "，奖励：" + name
		}
	}
	var obj struct {
		OrderInfo struct {
			PrizeName string `json:"prizeName"`
		} `json:"orderInfo"`
		PrizeName string `json:"prizeName"`
		SkuName   string `json:"skuName"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		name := obj.OrderInfo.PrizeName
		if name == "" {
			name = obj.PrizeName
		}
		if name == "" {
			name = obj.SkuName
		}
		if name != "" {
			return "，奖励：" + name
		}
	}
	return ""
}
