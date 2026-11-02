package tasks

import (
	"fmt"
	"strings"
	"time"

	"caiyun/internal/core/api"
)

// The task table is a state machine: a successful click acknowledges one step,
// not the entire task. Callback counters can also lag behind a real file/AI action.
func (t *TaskListTask) executeTaskSteps(item taskListItem, action func() error) error {
	// A previous task can satisfy this task too (shared AI/upload counters).
	// Re-read before performing another real action from an old batch snapshot.
	if t.api != nil {
		current, err := t.queryTaskByMarket(item.MarketName, item.GroupKey, item.Task.ID)
		if err != nil {
			return fmt.Errorf("执行前查询任务状态失败: %w", err)
		}
		if current == nil {
			return fmt.Errorf("执行前任务已不在列表中，跳过行为并等待下次确认")
		}
		item.Task = *current
	}
	stalled, actions := 0, 0
	for round := 0; round < 8; round++ {
		if isTaskCompleted(item.Task) || taskCompletedToday(item.Task) {
			return nil
		}
		// Preserve action -> doTaskPost -> click ordering from the reference.
		// Retry the real action at most once after delayed checks, not on every
		// reward-registration step (especially expensive AI and sharing calls).
		if action != nil && (actions == 0 || (stalled == 2 && actions == 1 && item.Task.ID != 522)) {
			if err := action(); err != nil {
				return fmt.Errorf("真实行为失败: %w", err)
			}
			actions++
			stalled = 0
		}
		if action != nil {
			if err := t.reportTaskAction(item); err != nil {
				return fmt.Errorf("步骤登记失败: %w", err)
			}
		} else {
			if t.gp {
				resp, err := t.api.DoTaskPostForTask(item.MarketName, item.Task.ID)
				if err != nil {
					return err
				}
				if err = rewardResponseError("GP任务登记", resp); err != nil {
					return err
				}
			}
			keys := t.getTaskClickKeys(item.Task)
			if len(keys) == 0 {
				break
			}
			for _, key := range keys {
				if err := t.api.DoTaskWithMarket(item.MarketName, key, fmt.Sprint(item.Task.ID)); err != nil {
					return err
				}
			}
		}

		t.waitForTask(1500*time.Millisecond + time.Duration(stalled)*time.Second)
		current, err := t.queryTaskByMarket(item.MarketName, item.GroupKey, item.Task.ID)
		if err != nil {
			return fmt.Errorf("登记后查询完成状态失败: %w", err)
		}
		if current == nil {
			return fmt.Errorf("登记后任务从列表消失，不能确认完成")
		}
		if isTaskCompleted(*current) || taskCompletedToday(*current) {
			t.logger.Success(fmt.Sprintf("服务端确认任务%s：%s", taskCompletionText(*current), taskListLabel(item)))
			return nil
		}
		if current.CurrStep > item.Task.CurrStep || current.Process > item.Task.Process {
			stalled = 0
		} else {
			stalled++
		}
		item.Task = *current
		if stalled >= 3 {
			break
		}
	}
	t.pending = true
	t.logger.Warn(fmt.Sprintf("任务尚未确认完成：%s（步骤%d，进度%d），下次继续", taskListLabel(item), item.Task.CurrStep, item.Task.Process))
	return nil
}

func (t *TaskListTask) waitForTask(delay time.Duration) {
	if t.wait != nil {
		t.wait(delay)
		return
	}
	time.Sleep(delay)
}

// Task 434 stays WAIT between its seven daily shares. Only the primary app/out
// button tells us today's action is complete; appcard always says "去完成".
func taskCompletedToday(task api.Task) bool {
	if task.ID != 434 || isTaskCompleted(task) {
		return false
	}
	for _, channel := range []string{"app", "out"} {
		if button, ok := task.Button[channel]; ok {
			text := strings.TrimSpace(toString(button["text"]))
			return text == "已完成" || text == "今日已完成"
		}
	}
	return false
}

func taskCompletionText(task api.Task) string {
	if taskCompletedToday(task) {
		return "今日已完成（继续按日累计）"
	}
	return "已完成"
}

func taskListLabel(item taskListItem) string {
	name := item.Task.Name
	// The sharing title contains an HTML progress span. Keep its text only.
	var clean strings.Builder
	inTag := false
	for _, r := range name {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				clean.WriteRune(r)
			}
		}
	}
	return fmt.Sprintf("%s[%s/%d]", strings.TrimSpace(clean.String()), item.MarketName, item.Task.ID)
}
