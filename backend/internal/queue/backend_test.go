package queue

import "testing"

func TestTaskQueueBackendRejectsLegacyListInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	if _, err := NewTaskQueueBackend(nil, TaskQueueBackendList); err == nil {
		t.Fatal("production list queue was accepted")
	}
}

func TestTaskQueueBackendKeepsLegacyListAvailableOutsideProduction(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	queue, err := NewTaskQueueBackend(nil, TaskQueueBackendList)
	if err != nil {
		t.Fatalf("development list queue error = %v", err)
	}
	if _, ok := queue.(*TaskQueue); !ok {
		t.Fatalf("queue type = %T, want *TaskQueue", queue)
	}
}
