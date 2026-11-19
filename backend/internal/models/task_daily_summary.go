package models

import "time"

// TaskDailySummary is a read projection, not a second earnings ledger.
type TaskDailySummary struct {
	AccountID       uint      `json:"account_id"`
	Phone           string    `json:"phone"`
	Remark          string    `json:"remark"`
	CloudGained     int       `json:"cloud_gained"`
	MiguNotesGained int       `json:"migu_notes_gained"`
	SuccessCount    int       `json:"success_count"`
	FailedCount     int       `json:"failed_count"`
	PendingCount    int       `json:"pending_count"`
	SkippedCount    int       `json:"skipped_count"`
	LastExecutedAt  time.Time `json:"last_executed_at"`
}
