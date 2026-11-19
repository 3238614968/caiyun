package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/scheduler"
	"caiyun/internal/services"
)

type miguTaskConfigs interface {
	List() ([]*models.TaskConfig, error)
}

type miguActiveAccounts interface {
	FindActiveAccountsAfterID(uint, int) ([]*models.Account, error)
}

type miguOperationSubmitter interface {
	Submit(context.Context, services.SubmitOperationRequest) (*models.Operation, bool, error, error)
}

type miguWindowJobDefinition struct {
	name, taskType, schedule, description string
}

func miguWindowJobDefinitions() []miguWindowJobDefinition {
	return []miguWindowJobDefinition{
		{"migu_noon_checkin", "migu_noon", "CRON_TZ=Asia/Shanghai */20 12 * * *", "咪咕饭点打卡（北京时间12:00–13:00）"},
		{"migu_night_checkin", "migu_night", "CRON_TZ=Asia/Shanghai */20 22 * * *", "咪咕晚安打卡（北京时间22:00–23:00）"},
	}
}

// Each check-in has its own Beijing-time schedule and distributed job lease.
// Repeated dispatches share the existing daily account/task operation key;
// a restarted Worker can catch the window without replaying completed actions.
func registerMiguWindowJobs(ctx context.Context, jobScheduler *scheduler.Scheduler, configs miguTaskConfigs, accounts miguActiveAccounts, operations miguOperationSubmitter, locks workerJobLeaseStore) {
	for _, definition := range miguWindowJobDefinitions() {
		job := definition
		_, _ = registerWorkerJob(jobScheduler, job.name, job.schedule, JobNonCritical, job.description, func() error {
			return runWithWorkerJobLease(locks, job.name, 5*time.Minute, func() error {
				return submitMiguWindowTask(ctx, job.taskType, time.Now(), configs, accounts, operations)
			})
		})
	}
}

func submitMiguWindowTask(ctx context.Context, code string, at time.Time, configs miguTaskConfigs, accounts miguActiveAccounts, operations miguOperationSubmitter) error {
	beijing := at.In(time.FixedZone("Asia/Shanghai", 8*3600))
	var hour int
	switch code {
	case "migu_noon":
		hour = 12
	case "migu_night":
		hour = 22
	default:
		return fmt.Errorf("不支持的咪咕限时任务: %s", code)
	}
	if beijing.Hour() != hour {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	definitions, err := configs.List()
	if err != nil {
		return fmt.Errorf("读取咪咕定时任务开关失败: %w", err)
	}
	enabled := len(definitions) == 0
	for _, definition := range definitions {
		if definition != nil && definition.TaskType == code {
			enabled = definition.IsEnabled && definition.RunInBatch
			break
		}
	}
	if !enabled {
		return nil
	}
	const batchSize = 200
	var lastID uint
	var issues []error
	for {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(issues, err)...)
		}
		batch, err := accounts.FindActiveAccountsAfterID(lastID, batchSize)
		if err != nil {
			return errors.Join(append(issues, err)...)
		}
		if len(batch) == 0 {
			break
		}
		for _, account := range batch {
			if err := ctx.Err(); err != nil {
				return errors.Join(append(issues, err)...)
			}
			if account == nil || !account.IsActive {
				continue
			}
			operation, _, dispatchErr, err := operations.Submit(ctx, services.SubmitOperationRequest{
				UserID: account.UserID, AccountID: account.ID, OperationType: models.OperationTypeAccountTask,
				Payload:        services.AccountTaskOperationPayload{AccountID: account.ID, TaskType: code},
				IdempotencyKey: fmt.Sprintf("cron:%s:%s:%d", code, beijing.Format("2006-01-02"), account.ID),
			})
			if err != nil {
				issues = append(issues, fmt.Errorf("咪咕定时任务提交失败，账号%d: %w", account.ID, err))
			} else if dispatchErr != nil && operation != nil {
				log.Printf("【咪咕打卡】操作%s已持久化，等待队列恢复投递", operation.ID)
			}
		}
		lastID = batch[len(batch)-1].ID
		if len(batch) < batchSize {
			break
		}
	}
	return errors.Join(issues...)
}
