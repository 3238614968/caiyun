package tasks

import (
	"fmt"
	"time"
)

// handleMemoryPhotoTask only supplies the missing real JPEG uploads. The caller
// registers the action and confirms FINISH separately, including when process
// has reached ten but the server has not finished the task's remaining steps.
func (t *TaskListTask) handleMemoryPhotoTask(item taskListItem) error {
	current, err := t.queryTaskByMarket(item.MarketName, item.GroupKey, item.Task.ID)
	if err != nil {
		return fmt.Errorf("读取回忆上传进度失败: %w", err)
	}
	if current == nil {
		return fmt.Errorf("回忆上传任务已不在服务端列表中")
	}
	if isTaskCompleted(*current) || isTaskDisabledByServer(*current) {
		return nil
	}
	if current.Process < 0 {
		return fmt.Errorf("回忆上传进度无效: %d", current.Process)
	}
	const target = 10
	for count := current.Process; count < target; count++ {
		if err := t.handlePhotoUploadTask(); err != nil {
			return fmt.Errorf("回忆照片上传失败: %w", err)
		}
		if count+1 < target {
			t.waitForTask(500 * time.Millisecond)
		}
	}
	return nil
}
