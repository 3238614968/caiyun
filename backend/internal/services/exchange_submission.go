package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// startExchangeLeaseHeartbeat owns no upstream work itself. It only keeps a
// claimed task from being recovered while it is still executing.
func startExchangeLeaseHeartbeat(ctx context.Context, repo *repository.ExchangeTaskRepository, taskID uint, token string, timeout time.Duration) func() bool {
	if repo == nil || taskID == 0 || strings.TrimSpace(token) == "" {
		return func() bool { return false }
	}
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	interval := timeout / 3
	if interval < time.Second {
		interval = time.Second
	}
	if interval > time.Minute {
		interval = time.Minute
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	var lost atomic.Bool
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				owned, err := repo.RenewRunning(taskID, token, time.Now())
				if err != nil {
					continue
				}
				if !owned {
					lost.Store(true)
					return
				}
			}
		}
	}()
	return func() bool {
		close(stop)
		<-done
		return lost.Load()
	}
}

func newExchangeSubmissionGuard(repo *repository.ExchangeTaskRepository, taskID uint, token string) exchangeSubmissionGuard {
	return func(ctx context.Context) error {
		if ctx == nil {
			ctx = context.Background()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return repo.WithContext(ctx).MarkSubmissionOwned(taskID, token)
	}
}

func (s *ExchangeService) persistExchangeResult(ctx context.Context, task *models.ExchangeTask, settlement repository.ExchangeSettlement) error {
	if task == nil || task.ID == 0 {
		return fmt.Errorf("refuse to persist empty exchange task")
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		err = s.exchangeTaskRepo.WithContext(ctx).FinalizeOwned(task, task.ExecutionToken, settlement)
		if err == nil || errors.Is(err, repository.ErrExchangeTaskExecutionLost) {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}
	if err != nil {
		return err
	}
	task.Outcome = string(settlement.Outcome)
	task.ExecutionToken = ""
	task.AttemptedCount++
	task.LastResult = settlement.Message
	task.Status = string(settlement.Status)
	if settlement.Outcome == models.ExchangeOutcomeAccepted {
		task.SuccessCount++
	} else {
		task.FailCount++
	}
	createExchangeSystemLog(s.taskLogRepo, task.UserID, task.ExchangeAccount.AccountID, task.PrizeName,
		exchangeAccountName(&task.ExchangeAccount), settlement.Outcome == models.ExchangeOutcomeAccepted, settlement.Message, settlement.ExecutionTimeMs)
	return nil
}

func (s *ExchangeService) persistExchangeOutcome(ctx context.Context, task *models.ExchangeTask, executionToken string, result exchangeAttemptResult, status models.ExchangeTaskStatus) error {
	if task != nil {
		task.ExecutionToken = executionToken
	}
	return s.persistExchangeResult(ctx, task, result.settlement(status))
}

func (s *ExchangeService) finalizeTaskExecution(task *models.ExchangeTask, result exchangeAttemptResult, executionToken, operationID string) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	settlement := result.settlement(exchangeTaskFinalStatus(task, result))
	if settlement.Outcome != models.ExchangeOutcomeNotSubmitted {
		settlement.OperationID = operationID
	}
	task.ExecutionToken = executionToken
	if err := s.persistExchangeResult(cleanupCtx, task, settlement); err != nil {
		return err
	}
	return taskOutcomeError(settlement.Outcome, settlement.Message)
}

func exchangeResultPreservesMonthlySeries(result exchangeAttemptResult) bool {
	return result.outcome == models.ExchangeOutcomeAccepted || result.outcome == models.ExchangeOutcomeUnknown
}
