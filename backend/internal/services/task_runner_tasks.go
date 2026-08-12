package services

import "time"

func (r *TaskRunner) Run() []TaskResult {
	// 兼容旧调用方：统一转发到注册表驱动路径，避免并行维护两套任务编排逻辑。
	return r.RunSelected(defaultTaskCatalog.DefaultBatchCodes())
}

func taskResultFromErr(taskType string, startTime time.Time, err error, successMessage string) *TaskResult {
	result := &TaskResult{
		TaskType:      taskType,
		ExecutionTime: int(time.Since(startTime).Milliseconds()),
	}
	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
		return result
	}
	result.Status = "success"
	result.Message = successMessage
	return result
}

// getCurrentCloudCount 获取当前云朵总数（通过签到API）
func (r *TaskRunner) getCurrentCloudCount() int {
	cloudInfo, err := r.api.GetCloudInfo()
	if err != nil {
		return 0
	}
	if !cloudInfo.IsSuccess() {
		return 0
	}
	return cloudInfo.Result.Total
}

// GetCloudGained 获取本次执行获得的云朵数
func (r *TaskRunner) GetCloudGained() int {
	if r.finalCloudCount > r.initialCloudCount {
		return r.finalCloudCount - r.initialCloudCount
	}
	return 0
}

// GetFinalCloudCount 获取执行后的云朵总数
func (r *TaskRunner) GetFinalCloudCount() int {
	return r.finalCloudCount
}
