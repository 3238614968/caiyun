package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type manualClaimFixture struct {
	mu            sync.Mutex
	task          models.ExchangeTask
	claimErr      error
	beforeRecover func(*models.ExchangeTask)
	beforeClaim   func(*models.ExchangeTask)
	claims        int
	claimCalls    int
	recoveryCalls int
}

func (f *manualClaimFixture) GetByID(uint) (*models.ExchangeTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	task := f.task
	return &task, nil
}

func (f *manualClaimFixture) TryMarkForManualExecution(id, userID uint, status string, updatedAt time.Time) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCalls++
	if f.beforeClaim != nil {
		f.beforeClaim(&f.task)
		f.beforeClaim = nil
	}
	if f.claimErr != nil {
		return false, "", f.claimErr
	}
	if f.task.ID != id || f.task.UserID != userID || f.task.Status != status || !f.task.UpdatedAt.Equal(updatedAt) {
		return false, "", nil
	}
	f.task.Status = "running"
	f.task.UpdatedAt = updatedAt.Add(time.Second)
	f.claims++
	return true, "new-owner", nil
}

func (f *manualClaimFixture) RecoverStaleRunningTask(id uint, before time.Time, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recoveryCalls++
	if f.beforeRecover != nil {
		f.beforeRecover(&f.task)
	}
	if f.task.ID == id && f.task.Status == "running" && f.task.UpdatedAt.Before(before) {
		f.task.Status = "pending"
		f.task.UpdatedAt = before
		return true, nil
	}
	return false, nil
}

func TestManualRetryClaimsFailedTask156AndStartsFreshRetryRound(t *testing.T) {
	now := time.Now()
	fixture := &manualClaimFixture{task: models.ExchangeTask{
		ID: 156, UserID: 7, Status: "failed", UpdatedAt: now.Add(-time.Hour),
		LastResult: "活动异常，请稍后重试！Error Code：GK | http_status=200 | code=610",
		RetryCount: 3, AttemptedCount: 4, FailCount: 4,
	}}
	task, _ := fixture.GetByID(156)
	claim, err := claimExchangeTaskManually(fixture, task, now, time.Minute)
	if err != nil || !claim.started || claim.token == "" || claim.task.Status != "running" || claim.task.RetryCount != 0 || claim.task.LastResult != "" {
		t.Fatalf("failed GK task was not restarted: %+v, %v", claim, err)
	}
	if claim.task.AttemptedCount != 4 || claim.task.FailCount != 4 {
		t.Fatal("retry erased historical attempt statistics")
	}
}

func TestConcurrentManualRetryOnlyClaimsFailedTaskOnce(t *testing.T) {
	now := time.Now()
	fixture := &manualClaimFixture{task: models.ExchangeTask{ID: 156, UserID: 7, Status: "failed", UpdatedAt: now}}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		task, _ := fixture.GetByID(156)
		wg.Add(1)
		go func() {
			defer wg.Done()
			claim, err := claimExchangeTaskManually(fixture, task, now, time.Minute)
			if err != nil {
				t.Errorf("claim error: %v", err)
			}
			if !claim.started && strings.Contains(claim.reason, "GK") {
				t.Error("conflict returned a historical upstream error")
			}
		}()
	}
	wg.Wait()
	if fixture.claims != 1 {
		t.Fatalf("manual retries started %d executions", fixture.claims)
	}
}

func TestManualRetryDoesNotClaimCompletedOrUncertainResults(t *testing.T) {
	for _, tc := range []struct{ status, result string }{
		{"completed", "兑换成功"}, {"running", "历史 GK"},
		{"failed", "兑换结果待确认"}, {"failed", "任务执行异常"},
	} {
		now := time.Now()
		fixture := &manualClaimFixture{task: models.ExchangeTask{ID: 156, UserID: 7, Status: tc.status, LastResult: tc.result, UpdatedAt: now}}
		task, _ := fixture.GetByID(156)
		claim, err := claimExchangeTaskManually(fixture, task, now, time.Minute)
		if err != nil || claim.started || claim.reason == "" || fixture.claimCalls != 0 {
			t.Fatalf("unsafe claim for %s/%s: %+v %v", tc.status, tc.result, claim, err)
		}
	}
}

func TestManualRetryUsesLatestOwnerAndResultAfterSnapshotChanges(t *testing.T) {
	now := time.Now()
	for _, changeOwner := range []bool{false, true} {
		fixture := &manualClaimFixture{task: models.ExchangeTask{ID: 156, UserID: 7, Status: "failed", UpdatedAt: now}}
		fixture.beforeClaim = func(task *models.ExchangeTask) {
			task.UpdatedAt = now.Add(time.Second)
			if changeOwner {
				task.UserID = 8
			} else {
				task.LastResult = "兑换结果待确认"
			}
		}
		task, _ := fixture.GetByID(156)
		claim, err := claimExchangeTaskManually(fixture, task, now, time.Minute)
		if claim.started || (changeOwner && !errors.Is(err, ErrExchangePermissionDenied)) || (!changeOwner && claim.reason == "") {
			t.Fatalf("updated owner/result bypassed: %+v, %v", claim, err)
		}
	}
}

func TestManualRetryDoesNotRecoverAnOwnerWhoseHeartbeatWasRenewed(t *testing.T) {
	now := time.Now()
	fixture := &manualClaimFixture{task: models.ExchangeTask{ID: 156, UserID: 7, Status: "running", UpdatedAt: now.Add(-time.Hour)}}
	fixture.beforeRecover = func(task *models.ExchangeTask) { task.UpdatedAt = now }
	task, _ := fixture.GetByID(156)
	claim, err := claimExchangeTaskManually(fixture, task, now, time.Minute)
	if err != nil || claim.started || fixture.claimCalls != 0 || fixture.recoveryCalls != 1 || !strings.Contains(claim.reason, "正在执行") {
		t.Fatalf("fresh owner was displaced: %+v, %v", claim, err)
	}
}

func TestManualRetryRecoversStaleOwnerAndReportsDuplicateActiveTask(t *testing.T) {
	now := time.Now()
	fixture := &manualClaimFixture{task: models.ExchangeTask{ID: 156, UserID: 7, Status: "running", UpdatedAt: now.Add(-time.Hour)}}
	task, _ := fixture.GetByID(156)
	claim, err := claimExchangeTaskManually(fixture, task, now, time.Minute)
	if err != nil || !claim.started || fixture.recoveryCalls != 1 {
		t.Fatalf("stale task not recovered: %+v %v", claim, err)
	}
	fixture = &manualClaimFixture{task: models.ExchangeTask{ID: 156, UserID: 7, Status: "failed", UpdatedAt: now}, claimErr: repository.ErrDuplicateActiveExchangeTask}
	task, _ = fixture.GetByID(156)
	if _, err := claimExchangeTaskManually(fixture, task, now, time.Minute); !errors.Is(err, ErrExchangeTaskAlreadyExists) {
		t.Fatalf("duplicate active task exposed as a database error: %v", err)
	}
}
