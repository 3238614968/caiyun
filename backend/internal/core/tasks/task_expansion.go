package tasks

import (
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// TaskExpansionRewardTask 领取备份翻倍奖励任务。
type TaskExpansionRewardTask struct {
	base *TaskListTask
}

// NewTaskExpansionRewardTask 创建备份翻倍奖励任务。
func NewTaskExpansionRewardTask(client *http.Client, log *logger.Logger) *TaskExpansionRewardTask {
	return &TaskExpansionRewardTask{base: NewTaskListTask(client, log)}
}

// SetStorage 设置共享状态存储。
func (t *TaskExpansionRewardTask) SetStorage(store Storage) *TaskExpansionRewardTask {
	if t != nil && t.base != nil {
		t.base.SetStorage(store)
	}
	return t
}

// Run 执行备份翻倍奖励任务。
func (t *TaskExpansionRewardTask) Run() error {
	if t == nil || t.base == nil {
		return nil
	}
	t.base.receiveTaskExpansion()
	return nil
}
