package dto

import (
	"testing"

	"caiyun/internal/models"
)

func TestHistoricalTaskProgressIsUnconfirmedRatherThanStillRunning(t *testing.T) {
	log := &models.TaskLog{TaskType: "tasklist", Status: "running", Message: "执行中", CloudGained: 11, MiguNotesGained: 40}
	response := ToTaskLogResponse(log)
	if response.Status != "pending" || response.Message != "执行已结束，结果待确认" || response.CloudGained != 11 || response.MiguNotesGained != 40 || log.Status != "running" || log.Message != "执行中" {
		t.Fatalf("history projection changed receipts or stored diagnostics: %+v", response)
	}
}
