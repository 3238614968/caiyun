package worker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/queue"
	"caiyun/internal/repository"
	"caiyun/internal/services"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// operationVisibilityQueueStub embeds the interface so the test only needs to
// implement the heartbeat capability under test.
type operationVisibilityQueueStub struct {
	queue.ReliableTaskQueue
	renewals atomic.Int32
}

func (q *operationVisibilityQueueStub) RenewVisibility(*queue.TaskMessage) (bool, error) {
	q.renewals.Add(1)
	return true, nil
}

func TestOperationLeaseRenewalIntervalIsBounded(t *testing.T) {
	cases := []struct {
		name  string
		lease time.Duration
		want  time.Duration
	}{
		{name: "default", lease: 0, want: time.Minute},
		{name: "normal", lease: 90 * time.Second, want: 30 * time.Second},
		{name: "short", lease: 2 * time.Second, want: time.Second},
		{name: "long", lease: 30 * time.Minute, want: time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := operationLeaseRenewalInterval(tc.lease); got != tc.want {
				t.Fatalf("interval(%s)=%s, want %s", tc.lease, got, tc.want)
			}
		})
	}
	if got := operationLeaseRenewalInterval(queue.DefaultVisibilityDelay); got >= queue.DefaultVisibilityDelay {
		t.Fatalf("renewal interval=%s must be before lease expiry=%s", got, queue.DefaultVisibilityDelay)
	}
}

func TestOperationLeaseHeartbeatRenewsDatabaseAndQueueVisibility(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Operation{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	op := &models.Operation{
		ID:             "lease-heartbeat-op",
		UserID:         1,
		OperationType:  models.OperationTypeAccountTask,
		Status:         models.OperationRunning,
		ExecutionToken: "lease-token",
		Payload:        "{}",
		IdempotencyKey: "lease-heartbeat-key",
		QueuedAt:       now,
		StartedAt:      &now,
	}
	if err := db.Create(op).Error; err != nil {
		t.Fatal(err)
	}

	visibility := &operationVisibilityQueueStub{}
	worker := &Worker{
		ctx:              context.Background(),
		taskQueue:        visibility,
		operationService: services.NewOperationService(repository.NewOperationRepository(db), nil),
	}
	stop := worker.startOperationLeaseHeartbeat(&queue.TaskMessage{StreamID: "1-0"}, op.ID, op.ExecutionToken, 2*time.Second)

	deadline := time.Now().Add(3 * time.Second)
	for visibility.renewals.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if visibility.renewals.Load() == 0 {
		_ = stop()
		t.Fatal("queue visibility renewal was not called")
	}
	if lost := stop(); lost {
		t.Fatal("healthy database lease was reported as lost")
	}
	var renewed models.Operation
	if err := db.First(&renewed, "id = ?", op.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !renewed.UpdatedAt.After(now) {
		t.Fatalf("database lease timestamp was not renewed: updated_at=%s initial=%s", renewed.UpdatedAt, now)
	}
}
