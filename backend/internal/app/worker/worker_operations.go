package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/queue"
	"caiyun/internal/services"
)

// processOperationTask executes one durable API command. A queue frame is ACKed
// only after the database state reaches a terminal status. State-update errors
// deliberately leave the frame pending for visibility-timeout recovery.
func (w *Worker) processOperationTask(message *queue.TaskMessage) {
	if message == nil || strings.TrimSpace(message.OperationID) == "" {
		return
	}
	if w.operationService == nil {
		log.Printf("operation service is not configured; leave message pending: operation_id=%s", message.OperationID)
		return
	}

	operation, claimed, err := w.operationService.TryStart(w.ctx, message.OperationID, queue.DefaultVisibilityDelay)
	if err != nil {
		if errors.Is(err, services.ErrOperationNotFound) {
			if dlqErr := w.taskQueue.DeadLetter(message, "operation not found"); dlqErr != nil {
				log.Printf("dead-letter orphan operation failed: operation_id=%s err=%v", message.OperationID, dlqErr)
			}
			return
		}
		log.Printf("claim operation failed; leave message pending: operation_id=%s err=%v", message.OperationID, err)
		return
	}
	if !claimed {
		// Terminal rows have no active owner, so an old outbox delivery can be
		// removed.  A running row is different: this frame can be the only
		// recoverable delivery after a Redis PEL move.  ACKing it while another
		// Worker still holds the database lease would turn a later Worker crash
		// into a permanently-running operation.  Leave it pending; after the
		// owner completes it is harmlessly cleaned up, and after the owner dies
		// the expired database lease permits a new claim.
		if operation != nil && operation.Terminal() {
			if err := w.taskQueue.Ack(message); err != nil {
				log.Printf("ack terminal duplicate operation failed: operation_id=%s err=%v", message.OperationID, err)
			}
		} else {
			status := "unknown"
			if operation != nil {
				status = string(operation.Status)
			}
			log.Printf("operation is still owned or queued; leave delivery pending: operation_id=%s status=%s", message.OperationID, status)
		}
		return
	}

	stopLeaseHeartbeat := w.startOperationLeaseHeartbeat(message, operation.ID, operation.ExecutionToken, queue.DefaultVisibilityDelay)
	executionErr := w.executeOperation(operation)
	if stopLeaseHeartbeat() {
		// A new Worker has acquired the fencing lease. It owns the durable
		// state now, so this Worker leaves its queue frame pending and never
		// ACKs or overwrites the newer execution's outcome.
		log.Printf("operation execution lease lost; leave message pending: operation_id=%s", operation.ID)
		return
	}
	if executionErr != nil {
		w.handleOperationFailure(message, operation, operation.ExecutionToken, executionErr)
		return
	}
	if err := w.operationService.MarkSucceeded(w.ctx, operation.ID, operation.ExecutionToken); err != nil {
		log.Printf("mark operation succeeded failed; leave message pending: operation_id=%s err=%v", operation.ID, err)
		return
	}
	if err := w.taskQueue.Ack(message); err != nil {
		log.Printf("ack completed operation failed: operation_id=%s err=%v", operation.ID, err)
	}
}

// startOperationLeaseHeartbeat renews both the database fencing lease and the
// queue-delivery visibility before either can expire.  Redis Streams tracks
// PEL idle time independently from the database row, so refreshing only
// updated_at lets stale recovery move a healthy long-running task.  A failed
// visibility refresh is logged but does not surrender the database fence: the
// delivery may already have been moved and must remain recoverable.
//
// The returned function waits for the heartbeat to stop and reports whether
// database ownership was conclusively lost.
func (w *Worker) startOperationLeaseHeartbeat(message *queue.TaskMessage, operationID, executionToken string, leaseTimeout time.Duration) func() bool {
	if w == nil || w.operationService == nil || strings.TrimSpace(operationID) == "" || strings.TrimSpace(executionToken) == "" {
		return func() bool { return false }
	}
	interval := operationLeaseRenewalInterval(leaseTimeout)
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
			case <-w.ctx.Done():
				return
			case <-ticker.C:
				owned, err := w.operationService.RenewLease(w.ctx, operationID, executionToken)
				if err != nil {
					log.Printf("renew operation lease failed: operation_id=%s err=%v", operationID, err)
					continue
				}
				if !owned {
					lost.Store(true)
					log.Printf("operation lease ownership lost: operation_id=%s", operationID)
					return
				}
				if w.taskQueue == nil || message == nil {
					continue
				}
				renewed, err := w.taskQueue.RenewVisibility(message)
				if err != nil {
					log.Printf("renew operation queue visibility failed: operation_id=%s stream_id=%s err=%v", operationID, message.StreamID, err)
					continue
				}
				if !renewed {
					log.Printf("operation queue delivery is no longer pending for this worker; preserve database lease: operation_id=%s stream_id=%s", operationID, message.StreamID)
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

func operationLeaseRenewalInterval(leaseTimeout time.Duration) time.Duration {
	if leaseTimeout <= 0 {
		leaseTimeout = queue.DefaultVisibilityDelay
	}
	interval := leaseTimeout / 3
	if interval < time.Second {
		return time.Second
	}
	if interval > time.Minute {
		return time.Minute
	}
	return interval
}

func (w *Worker) handleOperationFailure(message *queue.TaskMessage, operation *models.Operation, executionToken string, cause error) {
	if message == nil {
		return
	}
	attemptCount := message.RetryCount + 1
	if operation != nil && operation.AttemptCount > 0 {
		attemptCount = operation.AttemptCount
	}
	log.Printf("operation execution failed: operation_id=%s attempt=%d err=%v", message.OperationID, attemptCount, cause)
	if isTerminalOperationError(cause) {
		if err := w.operationService.MarkFailed(w.ctx, message.OperationID, executionToken, cause); err != nil {
			log.Printf("mark terminal operation failed failed; leave message pending: operation_id=%s err=%v", message.OperationID, err)
			return
		}
		if err := w.taskQueue.DeadLetter(message, "terminal operation error"); err != nil {
			log.Printf("dead-letter terminal operation failed: operation_id=%s err=%v", message.OperationID, err)
		}
		return
	}
	// RetryCount is only a delivery payload field and can be reset by a fresh
	// outbox enqueue.  AttemptCount is incremented by the durable CAS claim,
	// therefore it is the sole retry budget authority across Redis redelivery,
	// delayed retry and process restart.
	message.RetryCount = attemptCount
	if attemptCount < queue.DefaultMaxAttempts {
		if err := w.operationService.MarkRetryQueued(w.ctx, message.OperationID, executionToken, cause); err != nil {
			log.Printf("mark operation queued failed; leave message pending: operation_id=%s err=%v", message.OperationID, err)
			return
		}
		if err := w.taskQueue.Requeue(message); err != nil {
			// The durable row is queued, so the outbox reconciler can republish it.
			log.Printf("requeue operation failed; outbox will reconcile: operation_id=%s err=%v", message.OperationID, err)
		}
		return
	}
	if err := w.operationService.MarkFailed(w.ctx, message.OperationID, executionToken, cause); err != nil {
		log.Printf("mark operation failed failed; leave message pending: operation_id=%s err=%v", message.OperationID, err)
		return
	}
	if err := w.taskQueue.DeadLetter(message, "operation execution failed"); err != nil {
		log.Printf("dead-letter failed operation failed: operation_id=%s err=%v", message.OperationID, err)
	}
}

// isTerminalOperationError identifies validation and business-rule failures
// that cannot succeed on retry. Retrying them only creates duplicate outbox
// deliveries and hides the actionable error from the operation record.
func isTerminalOperationError(err error) bool {
	return errors.Is(err, services.ErrExchangeTaskConflict) ||
		errors.Is(err, services.ErrExchangeTaskAlreadyExists) ||
		errors.Is(err, services.ErrExchangeMonthlyLimitReached) ||
		errors.Is(err, services.ErrExchangeExecutionFailed) ||
		errors.Is(err, services.ErrExchangeBatchPartialFailure) ||
		errors.Is(err, services.ErrExchangeBatchFailed) ||
		errors.Is(err, services.ErrExchangeInvalidInput) ||
		errors.Is(err, services.ErrExchangeCloudAccountMissing) ||
		errors.Is(err, services.ErrExchangeRuleNotFound) ||
		errors.Is(err, services.ErrExchangeTaskNotFound) ||
		errors.Is(err, services.ErrExchangeProductNotFound) ||
		errors.Is(err, services.ErrExchangePermissionDenied) ||
		errors.Is(err, services.ErrExchangeAccountDisabled) ||
		errors.Is(err, services.ErrExchangeCredentialsMissing) ||
		errors.Is(err, services.ErrExchangeProductInactive)
}

func (w *Worker) executeOperation(operation *models.Operation) error {
	if operation == nil {
		return errors.New("operation is nil")
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	switch operation.OperationType {
	case models.OperationTypeAccountTask:
		var payload services.AccountTaskOperationPayload
		if err := decodeOperationPayload(operation, &payload); err != nil {
			return err
		}
		if payload.AccountID == 0 || strings.TrimSpace(payload.TaskType) == "" {
			return errors.New("invalid account task payload")
		}
		if err := w.requireAccountOwner(payload.AccountID, operation.UserID); err != nil {
			return err
		}
		return w.ExecuteQueueAccountTask(payload.AccountID, payload.TaskType)

	case models.OperationTypeAccountTaskBatch:
		var payload services.AccountTaskBatchOperationPayload
		if err := decodeOperationPayload(operation, &payload); err != nil {
			return err
		}
		if len(payload.AccountIDs) == 0 || len(payload.AccountIDs) > 1000 {
			return errors.New("invalid account batch payload")
		}
		var failures []error
		for _, accountID := range payload.AccountIDs {
			if err := w.ctx.Err(); err != nil {
				return err
			}
			if err := w.requireAccountOwner(accountID, operation.UserID); err != nil {
				failures = append(failures, err)
				continue
			}
			if err := w.ExecuteQueueAccountTask(accountID, "all_tasks"); err != nil {
				failures = append(failures, fmt.Errorf("account %d: %w", accountID, err))
			}
		}
		return errors.Join(failures...)

	case models.OperationTypeExchangeTask:
		if w.exchangeService == nil {
			return errors.New("exchange service is not configured")
		}
		var payload services.ExchangeTaskOperationPayload
		if err := decodeOperationPayload(operation, &payload); err != nil {
			return err
		}
		if payload.TaskID == 0 {
			return errors.New("invalid exchange task payload")
		}
		return w.exchangeService.ExecuteExchangeTaskContext(w.ctx, payload.TaskID, operation.UserID)

	case models.OperationTypeExchangeTaskBatch:
		if w.exchangeService == nil {
			return errors.New("exchange service is not configured")
		}
		var payload services.ExchangeTaskBatchOperationPayload
		if err := decodeOperationPayload(operation, &payload); err != nil {
			return err
		}
		if len(payload.TaskIDs) == 0 || len(payload.TaskIDs) > 1000 {
			return errors.New("invalid exchange task batch payload")
		}
		results := w.exchangeService.BatchExecuteExchangeTasksContext(w.ctx, payload.TaskIDs, operation.UserID)
		succeeded := 0
		failed := 0
		failures := make([]string, 0, len(results))
		for _, result := range results {
			if result.Success {
				succeeded++
				continue
			}
			failed++
			message := strings.TrimSpace(result.Message)
			if message == "" {
				message = "抢兑失败，请查看执行结果"
			}
			if len(failures) < 8 {
				failures = append(failures, fmt.Sprintf("任务%d：%s", result.TaskID, message))
			}
		}
		if failed > 0 {
			summary := fmt.Sprintf("批量抢兑结果：成功 %d，失败 %d，共 %d", succeeded, failed, len(results))
			if len(failures) > 0 {
				summary += "；失败项：" + strings.Join(failures, "；")
			}
			if succeeded > 0 {
				return fmt.Errorf("%w: %s", services.ErrExchangeBatchPartialFailure, summary)
			}
			return fmt.Errorf("%w: %s", services.ErrExchangeBatchFailed, summary)
		}
		return nil

	case models.OperationTypeExchangeImmediate:
		return w.executeImmediateExchangeOperation(operation)

	case models.OperationTypeExchangeMonthly:
		if w.exchangeService == nil {
			return errors.New("exchange service is not configured")
		}
		if err := w.ctx.Err(); err != nil {
			return err
		}
		return w.exchangeService.ExecuteMonthlyExchangeContext(w.ctx)

	default:
		return fmt.Errorf("unsupported operation type %q", operation.OperationType)
	}
}

func (w *Worker) executeImmediateExchangeOperation(operation *models.Operation) error {
	if w.exchangeService == nil {
		return errors.New("exchange service is not configured")
	}
	if operation.ResourceID > 0 {
		return w.exchangeService.ExecuteExchangeTaskContext(w.ctx, operation.ResourceID, operation.UserID)
	}
	var payload services.ImmediateExchangeOperationPayload
	if err := decodeOperationPayload(operation, &payload); err != nil {
		return err
	}
	if payload.ProductID == 0 || (payload.ExchangeRuleID == 0 && payload.AccountID == 0) {
		return errors.New("invalid immediate exchange payload")
	}
	options := services.ExchangeTaskCreateOptions{SourceOperationID: operation.ID, TaskType: string(models.ExchangeTaskFixed), MaxAttempts: 1, RestockCycle: "once", CalendarPolicy: "all"}
	var task *models.ExchangeTask
	var err error
	if payload.ExchangeRuleID > 0 {
		task, err = w.exchangeService.CreateExchangeTaskWithOptionsContext(w.ctx, operation.UserID, payload.ExchangeRuleID, payload.ProductID, options)
	} else {
		task, err = w.exchangeService.CreateExchangeTaskByAccountIDWithOptionsContext(w.ctx, operation.UserID, payload.AccountID, payload.ProductID, options)
	}
	if err != nil {
		return err
	}
	if err := w.operationService.SetResourceID(w.ctx, operation.ID, operation.ExecutionToken, task.ID); err != nil {
		return fmt.Errorf("link immediate exchange task: %w", err)
	}
	return w.exchangeService.ExecuteExchangeTaskContext(w.ctx, task.ID, operation.UserID)
}

func (w *Worker) requireAccountOwner(accountID, userID uint) error {
	if accountID == 0 || userID == 0 {
		return errors.New("invalid account ownership input")
	}
	account, err := w.accountService.GetAccountByID(accountID)
	if err != nil {
		return err
	}
	if account.UserID != userID {
		return errors.New("account does not belong to operation user")
	}
	return nil
}

func decodeOperationPayload(operation *models.Operation, target interface{}) error {
	if operation == nil || strings.TrimSpace(operation.Payload) == "" {
		return errors.New("operation payload is empty")
	}
	if err := json.Unmarshal([]byte(operation.Payload), target); err != nil {
		return fmt.Errorf("decode operation payload: %w", err)
	}
	return nil
}
