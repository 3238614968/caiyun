package services

import (
	"errors"
	"reflect"
	"testing"
)

func TestBatchCompletionClaimsLateRewardsBeforeStatisticsAndCleanup(t *testing.T) {
	in := []string{"signin", "receive", "todaycloud", "after_task", "poster_activity", "fun_ai"}
	want := []string{"signin", "poster_activity", "fun_ai", "receive", "todaycloud", "after_task"}
	if got := orderTaskCompletionPhases(in); !reflect.DeepEqual(got, want) || in[1] != "receive" {
		t.Fatalf("wrong completion phases: %v", got)
	}
	if got := orderTaskCompletionPhases([]string{}); len(got) != 0 {
		t.Fatal("empty batch enabled tasks")
	}
}

func TestBatchPartialFailureIsVisibleWithoutReplayingSuccesses(t *testing.T) {
	results := []TaskResult{{TaskType: "signin", Status: "success"}, {TaskType: "poster_activity", Status: "failed", Message: "upload refused"}, {TaskType: "student_perks", Status: "pending"}}
	var failure *TaskBatchFailure
	if err := taskBatchOutcome(results); !errors.As(err, &failure) || failure.Retryable() || len(failure.Tasks) != 1 {
		t.Fatalf("partial failure hidden/retriable: %v", err)
	}
	if taskBatchOutcome([]TaskResult{{Status: "pending"}, {Status: "success"}}) != nil {
		t.Fatal("qualification pending treated as execution failure")
	}
}

func TestRepeatedBatchRetriesIncompleteActionsAndRefreshesReceipts(t *testing.T) {
	codes := []string{"signin", "poster_activity", "student_perks", "receive", "todaycloud", "after_task"}
	done := []string{"signin", "receive", "todaycloud", "after_task"}
	want := []string{"poster_activity", "student_perks", "receive", "todaycloud", "after_task"}
	if got := remainingBatchTaskCodes(codes, done); !reflect.DeepEqual(got, want) {
		t.Fatalf("retry did not target unfinished tasks: %v", got)
	}
}
