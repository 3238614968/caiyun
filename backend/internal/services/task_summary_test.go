package services

import (
	"context"
	"strings"
	"testing"

	"caiyun/internal/models"
)

func TestTaskSummaryCountsOutcomesAndOnlyConfirmedGains(t *testing.T) {
	results := []TaskResult{{TaskType: "signin", Status: "success", CloudGained: 5}, {TaskType: "tasklist", Status: "pending", CloudGained: 11, MiguNotesGained: 40}, {TaskType: "coc3", Status: "failed", MiguNotesGained: 5}, {TaskType: "todaycloud", Status: "success", MiguNotesGained: -5}, {TaskType: "hidden", Status: "skipped", CloudGained: 100, MiguNotesGained: 100}}
	got := summarizeTaskResults(results)
	if got.Success != 2 || got.Failed != 1 || got.Pending != 1 || got.Gained != 16 || got.MiguNotesGained != 45 {
		t.Fatalf("summary = %+v", got)
	}
	var failure = taskBatchOutcome(results)
	operation := &models.Operation{Status: models.OperationFailed, ErrorSummary: operationErrorSummary(failure)}
	message := PublicOperationError(operation)
	if !strings.Contains(message, "部分任务未完成") || !strings.Contains(message, "成功 2 项，失败 1 项，待确认 1 项") || operation.Status != models.OperationFailed {
		t.Fatalf("partial failure was hidden or called total failure: %s", message)
	}
}

func TestBalanceReadFailureCannotTurnExistingBalanceIntoNewEarnings(t *testing.T) {
	for _, scenario := range []struct {
		before, after           int
		knownBefore, knownAfter bool
		want                    int
	}{
		{0, 12064, false, true, 0}, {12000, 0, true, false, 0},
		{12000, 12011, true, true, 11}, {0, 11, true, true, 11}, {12011, 12000, true, true, 0},
	} {
		runner := &TaskRunner{initialCloudCount: scenario.before, finalCloudCount: scenario.after, initialCloudKnown: scenario.knownBefore, finalCloudKnown: scenario.knownAfter}
		if got := runner.GetCloudGained(); got != scenario.want {
			t.Fatalf("gain = %d, want %d", got, scenario.want)
		}
	}
}

func TestTaskLogDayUsesCSTAndHalfOpenBoundaries(t *testing.T) {
	start, end, err := TaskLogDayRange("2026-11-12")
	if err != nil || start.UTC().Format("2006-01-02T15:04:05Z") != "2026-11-11T16:00:00Z" || end.UTC().Format("2006-01-02T15:04:05Z") != "2026-11-12T16:00:00Z" {
		t.Fatalf("wrong day range: %v %v %v", start, end, err)
	}
	if _, _, err := TaskLogDayRange("2026-11-32"); err == nil {
		t.Fatal("invalid date accepted")
	}
}

func TestTaskOperationContextDoesNotChangeSharedWorkerContext(t *testing.T) {
	parent := context.Background()
	one := WithTaskOperation(parent, "operation-one", 1)
	two := WithTaskOperation(parent, "operation-two", 2)
	if parent.Value(taskOperationKey{}) != nil || one.Value(taskOperationKey{}).(taskOperation).id != "operation-one" || two.Value(taskOperationKey{}).(taskOperation).attempt != 2 {
		t.Fatal("operation metadata leaked between concurrent accounts")
	}
}

func TestPublicOperationFailureNeverLeaksLegacyRawErrors(t *testing.T) {
	for _, raw := range []string{
		"任务批次存在失败项: coc3(auth=secret | body=private)、family_circle(HTTP 404)；可单独重试失败任务",
		"private auth=secret password=fixture http_status=500 body=private",
	} {
		message := PublicOperationError(&models.Operation{Status: models.OperationFailed, ErrorSummary: raw})
		if strings.Contains(message, "secret") || strings.Contains(message, "body=") || strings.Contains(message, "HTTP") || strings.Contains(message, "fixture") {
			t.Fatalf("private failure leaked: %s", message)
		}
		if strings.HasPrefix(raw, "任务批次") && !strings.Contains(message, "未完成任务") {
			t.Fatalf("legacy failure lost its task context: %s", message)
		}
	}
}
