package tasks

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// CloudPhonePartyTask 执行云手机红包派对的协议任务，云币与云朵分别统计。
type CloudPhonePartyTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	delay       time.Duration
	lastMessage string
	pending     bool
}

func NewCloudPhonePartyTask(client *http.Client, log *logger.Logger, _ Storage) *CloudPhonePartyTask {
	return &CloudPhonePartyTask{api: api.NewCaiyunAPI(client), logger: log, delay: time.Second}
}

func (t *CloudPhonePartyTask) Message() string { return t.lastMessage }
func (t *CloudPhonePartyTask) Pending() bool   { return t.pending }

func (t *CloudPhonePartyTask) Run() error {
	t.lastMessage, t.pending = "", false
	token, err := t.api.CloudPhoneLogin(t.settingsPhone)
	if err != nil {
		return err
	}
	before, err := t.api.CloudPhoneBalance(token)
	if err != nil {
		return err
	}
	var issues []error
	var parts []string
	sign, err := t.api.CloudPhoneSign(token)
	if err != nil {
		issues = append(issues, err)
	} else if err := cloudPhoneActionError("签到登记", sign); err != nil {
		issues = append(issues, err)
	} else {
		parts = append(parts, "签到已登记，实际签到奖励仍需云机确认")
	}
	list, err := t.api.CloudPhoneTasks(token)
	if err != nil {
		return errors.Join(append(issues, err)...)
	}
	registered, already, pending := 0, 0, 0
	var expectedCoins int64
	seen := map[string]bool{}
	for _, task := range list.Tasks() {
		// 其余任务依赖真实云机、设备开关或购买条件，不能把登记当作完成。
		if task.TaskCode != "MONTHLY_5" && task.TaskCode != "INSTALL_1" {
			pending++
			continue
		}
		if seen[task.ID.String()] {
			continue
		}
		seen[task.ID.String()] = true
		result, err := t.api.CloudPhoneCompleteTask(token, task.ID)
		if err != nil {
			issues = append(issues, fmt.Errorf("云手机/%s: %w", task.TaskCode, err))
		} else if err := cloudPhoneActionError(task.TaskCode, result); err != nil {
			issues = append(issues, err)
		} else if result.ErrorCode == "001" {
			already++
		} else {
			registered++
			if amount, err := task.PrizeAmount.Int64(); err == nil && amount > 0 {
				expectedCoins += amount
			}
		}
		if t.delay > 0 {
			time.Sleep(t.delay)
		}
	}
	after, err := t.api.CloudPhoneBalance(token)
	if err != nil {
		issues = append(issues, err)
		parts = append(parts, "云币余额复查失败，未确认到账")
	} else {
		parts = append(parts, fmt.Sprintf("云币%d→%d（变化%+d，不计入云朵收益）", before, after, after-before))
		if registered > 0 && (after <= before || (expectedCoins > 0 && after-before < expectedCoins)) {
			t.pending = true
			parts = append(parts, "协议任务已登记，云币尚未全部确认到账")
		}
	}
	parts = append(parts, fmt.Sprintf("协议任务登记%d项、此前已完成%d项", registered, already))
	if pending > 0 {
		parts = append(parts, fmt.Sprintf("其余%d项需真实云机或活动资格，仅盘点", pending))
	}
	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}

func cloudPhoneActionError(action string, result *api.CloudPhoneAction) error {
	if result != nil && (result.ErrorCode == "001" || (result.Status.String() == "1" && result.ErrorCode == "")) {
		return nil
	}
	if result == nil {
		return fmt.Errorf("云手机%s响应为空", action)
	}
	return fmt.Errorf("云手机%s未成功: status=%s code=%s %s", action, result.Status, result.ErrorCode, result.ErrorMsg)
}
