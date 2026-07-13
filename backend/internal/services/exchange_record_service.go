package services

import (
	"caiyun/internal/models"
	"context"
	"time"
)

// GetExchangeRecords 获取抢兑记录列表
func (s *ExchangeService) GetExchangeRecords(userID uint, accountID uint, productName string, status string, startDate string, endDate string, page int, limit int) ([]*models.ExchangeRecord, int64, error) {
	return s.GetExchangeRecordsContext(context.Background(), userID, accountID, productName, status, startDate, endDate, page, limit)
}

func (s *ExchangeService) GetExchangeRecordsContext(ctx context.Context, userID uint, accountID uint, productName string, status string, startDate string, endDate string, page int, limit int) ([]*models.ExchangeRecord, int64, error) {
	return s.exchangeTaskRepo.WithContext(ctx).GetRecordsWithFilter(userID, accountID, productName, status, startDate, endDate, page, limit)
}

// GetRecordStats 获取抢兑记录统计信息。userID 为 0 时返回全局统计。
func (s *ExchangeService) GetRecordStats(userID uint, startTime, endTime time.Time) (successCount, failCount int64, err error) {
	return s.GetRecordStatsContext(context.Background(), userID, startTime, endTime)
}

func (s *ExchangeService) GetRecordStatsContext(ctx context.Context, userID uint, startTime, endTime time.Time) (successCount, failCount int64, err error) {
	return s.exchangeRecordRepo.WithContext(ctx).GetStats(userID, startTime, endTime)
}

// GetFailureReasonStats 获取抢兑失败原因归类统计。
func (s *ExchangeService) GetFailureReasonStats(userID uint, startTime, endTime time.Time, limit int) (map[string]int64, error) {
	stats, err := s.exchangeRecordRepo.GetFailureReasonStats(userID, startTime, endTime, limit)
	if err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(stats))
	for _, stat := range stats {
		reason := normalizeExchangeFailureReason(stat.Message)
		result[reason] += stat.Count
	}
	return result, nil
}

func normalizeExchangeFailureReason(message string) string {
	return exchangeFailureReasonLabel(message)
}
