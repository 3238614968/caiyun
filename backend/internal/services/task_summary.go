package services

import (
	"context"
	"fmt"
	"time"

	"caiyun/internal/models"
)

type taskOperationKey struct{}
type taskOperation struct {
	id      string
	attempt int
}

func WithTaskOperation(ctx context.Context, id string, attempt int) context.Context {
	return context.WithValue(ctx, taskOperationKey{}, taskOperation{id, attempt})
}

type taskResultSummary struct {
	Success         int `json:"success"`
	Failed          int `json:"failed"`
	Pending         int `json:"pending"`
	Gained          int `json:"gained"`
	MiguNotesGained int `json:"migu_notes_gained"`
}

func summarizeTaskResults(results []TaskResult) taskResultSummary {
	var summary taskResultSummary
	for _, result := range results {
		switch result.Status {
		case "success":
			summary.Success++
		case "failed":
			summary.Failed++
		case "skipped":
			continue
		default:
			summary.Pending++
		}
		if result.CloudGained > 0 {
			summary.Gained += result.CloudGained
		}
		if result.MiguNotesGained > 0 {
			summary.MiguNotesGained += result.MiguNotesGained
		}
	}
	return summary
}

func TaskLogDayRange(date string) (time.Time, time.Time, error) {
	if date == "" {
		date = nowCST().Format("2006-01-02")
	}
	start, err := time.ParseInLocation("2006-01-02", date, cstZone)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("日期格式应为 YYYY-MM-DD")
	}
	return start, start.AddDate(0, 0, 1), nil
}

func (s *TaskService) GetTaskDailySummariesContext(ctx context.Context, userID uint, date string, page, pageSize int) ([]*models.TaskDailySummary, int64, error) {
	start, end, err := TaskLogDayRange(date)
	if err != nil {
		return nil, 0, err
	}
	return s.taskLogRepo.DailySummaries(ctx, userID, start, end, (page-1)*pageSize, pageSize)
}
