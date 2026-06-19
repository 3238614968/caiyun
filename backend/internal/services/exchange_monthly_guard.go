package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"fmt"
	"log"
	"strings"
	"time"
)

type exchangeMonthlySeries struct {
	Key   string
	Label string
}

type monthlySeriesGuardResult struct {
	skip   bool
	reason string
}

func exchangeMonthlyWindow(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 1, 0)
}

func exchangeMonthlySeriesForProduct(product *models.Product, prizeName string) exchangeMonthlySeries {
	if product != nil {
		category := strings.TrimSpace(product.Category)
		if category != "" && !strings.HasPrefix(category, "未知分类") {
			return exchangeCategoryMonthlySeries(category)
		}
	}

	name := normalizeExchangeSeriesText(prizeName)
	switch {
	case strings.Contains(name, "音乐") ||
		strings.Contains(name, "绿钻") ||
		strings.Contains(name, "qq音乐") ||
		strings.Contains(name, "酷狗") ||
		strings.Contains(name, "酷我") ||
		strings.Contains(name, "网易云"):
		return exchangeCategoryMonthlySeries("音乐类会员")
	case strings.Contains(name, "视频") ||
		strings.Contains(name, "腾讯视频") ||
		strings.Contains(name, "爱奇艺") ||
		strings.Contains(name, "优酷") ||
		strings.Contains(name, "芒果"):
		return exchangeCategoryMonthlySeries("视频类会员")
	case strings.Contains(name, "流量"):
		return exchangeCategoryMonthlySeries("全国通用流量权益")
	case strings.Contains(name, "外卖") || strings.Contains(name, "美食"):
		return exchangeCategoryMonthlySeries("外卖美食权益")
	case strings.Contains(name, "奶茶") || strings.Contains(name, "饮品"):
		return exchangeCategoryMonthlySeries("奶茶饮品权益")
	case strings.Contains(name, "咖啡"):
		return exchangeCategoryMonthlySeries("咖啡饮品权益")
	}

	if name == "" {
		return exchangeMonthlySeries{}
	}
	return exchangeMonthlySeries{
		Key:   "product:" + name,
		Label: strings.TrimSpace(prizeName),
	}
}

func exchangeCategoryMonthlySeries(category string) exchangeMonthlySeries {
	category = strings.TrimSpace(category)
	return exchangeMonthlySeries{
		Key:   "category:" + normalizeExchangeSeriesText(category),
		Label: category,
	}
}

func normalizeExchangeSeriesText(text string) string {
	text = strings.TrimSpace(strings.ToLower(text))
	replacer := strings.NewReplacer(
		" ", "",
		"\t", "",
		"\r", "",
		"\n", "",
		"　", "",
		"-", "",
		"_", "",
	)
	return replacer.Replace(text)
}

func isExchangeMonthlySeriesLockMessage(message string) bool {
	msg := normalizeExchangeSeriesText(message)
	if msg == "" {
		return false
	}

	patterns := []string{
		"本月已兑换",
		"本月已兑",
		"本月限兑",
		"本月限购",
		"每月限兑",
		"每月限购",
		"每月只能",
		"该类商品本月",
		"该类型商品本月",
		"同系列",
		"同类商品",
		"该品类",
		"重复兑换",
		"重复兑奖",
	}
	for _, pattern := range patterns {
		if strings.Contains(msg, normalizeExchangeSeriesText(pattern)) {
			return true
		}
	}
	return false
}

func exchangeRecordLocksMonthlySeries(record *models.ExchangeRecord) bool {
	if record == nil {
		return false
	}
	if record.Status == string(models.ExchangeRecordSuccess) {
		return true
	}
	return isExchangeMonthlySeriesLockMessage(record.Message)
}

func exchangeRecordLockReason(record *models.ExchangeRecord) string {
	if record == nil {
		return "已锁定"
	}
	if record.Status == string(models.ExchangeRecordSuccess) {
		return "已成功兑换"
	}
	if strings.TrimSpace(record.Message) != "" {
		return record.Message
	}
	return "已锁定"
}

func shouldSkipExchangeMonthlySeries(
	recordRepo *repository.ExchangeRecordRepository,
	productRepo *repository.ProductRepository,
	task *models.ExchangeTask,
	now time.Time,
) (bool, string, error) {
	if recordRepo == nil || task == nil {
		return false, "", nil
	}

	taskProduct := exchangeTaskProductSnapshot(productRepo, task)
	taskSeries := exchangeMonthlySeriesForProduct(taskProduct, task.PrizeName)
	if taskSeries.Key == "" {
		return false, "", nil
	}

	start, end := exchangeMonthlyWindow(now)
	records, err := recordRepo.FindByAccountInPeriod(task.UserID, task.ExchangeAccountID, start, end)
	if err != nil {
		return false, "", err
	}

	for _, record := range records {
		if !exchangeRecordLocksMonthlySeries(record) {
			continue
		}

		recordProduct := exchangeRecordProductSnapshot(productRepo, record)
		recordSeries := exchangeMonthlySeriesForProduct(recordProduct, record.PrizeName)
		if recordSeries.Key != taskSeries.Key {
			continue
		}

		label := taskSeries.Label
		if label == "" {
			label = recordSeries.Label
		}
		if label == "" {
			label = task.PrizeName
		}
		sourceName := strings.TrimSpace(record.PrizeName)
		if sourceName == "" {
			sourceName = task.PrizeName
		}
		reason := fmt.Sprintf("本月%s已锁定（%s：%s），跳过至 %s 后再尝试",
			label,
			sourceName,
			exchangeRecordLockReason(record),
			end.Format("2006-01-02"),
		)
		return true, reason, nil
	}

	return false, "", nil
}

func exchangeMonthlySeriesLockKey(userID, exchangeAccountID uint, series exchangeMonthlySeries, now time.Time) string {
	yearMonth := now.Format("2006-01")
	return fmt.Sprintf("exchange:series:%d:%d:%s:%s", userID, exchangeAccountID, yearMonth, series.Key)
}

func (s *ExchangeService) acquireMonthlySeriesLock(task *models.ExchangeTask, now time.Time) (bool, func(), string) {
	if task == nil {
		return true, func() {}, ""
	}
	if s.lockStore == nil {
		log.Printf("【抢兑月度保护】任务 %d 未配置分布式锁，仅使用记录查询保护", task.ID)
		return true, func() {}, ""
	}

	taskProduct := exchangeTaskProductSnapshot(s.productRepo, task)
	series := exchangeMonthlySeriesForProduct(taskProduct, task.PrizeName)
	if series.Key == "" {
		return true, func() {}, ""
	}

	key := exchangeMonthlySeriesLockKey(task.UserID, task.ExchangeAccountID, series, now)
	locked, err := s.lockStore.SetNX(key, "1", 35*24*time.Hour)
	if err != nil {
		reason := fmt.Sprintf("获取本月同系列抢兑锁失败: %v", err)
		log.Printf("【抢兑月度保护】任务 %d %s", task.ID, reason)
		return false, func() {}, reason
	}
	if !locked {
		label := series.Label
		if label == "" {
			label = task.PrizeName
		}
		reason := fmt.Sprintf("本月%s已有任务正在抢兑或已锁定，跳过本次执行", label)
		log.Printf("【抢兑月度保护】任务 %d %s", task.ID, reason)
		return false, func() {}, reason
	}

	release := func() {
		if err := s.lockStore.Del(key); err != nil {
			log.Printf("【抢兑月度保护】任务 %d 释放同系列锁失败: %v", task.ID, err)
		}
	}
	return true, release, ""
}

func (s *ExchangeScheduler) acquireMonthlySeriesLock(task *models.ExchangeTask, now time.Time) (bool, func(), string) {
	if task == nil {
		return true, func() {}, ""
	}
	if s.leaseStore == nil {
		log.Printf("【抢兑月度保护】任务 %d 未配置分布式锁，仅使用记录查询保护", task.ID)
		return true, func() {}, ""
	}

	taskProduct := exchangeTaskProductSnapshot(s.productRepo, task)
	series := exchangeMonthlySeriesForProduct(taskProduct, task.PrizeName)
	if series.Key == "" {
		return true, func() {}, ""
	}

	key := exchangeMonthlySeriesLockKey(task.UserID, task.ExchangeAccountID, series, now)
	locked, err := s.leaseStore.SetNX(key, "1", 35*24*time.Hour)
	if err != nil {
		reason := fmt.Sprintf("获取本月同系列抢兑锁失败: %v", err)
		log.Printf("【抢兑月度保护】任务 %d %s", task.ID, reason)
		return false, func() {}, reason
	}
	if !locked {
		label := series.Label
		if label == "" {
			label = task.PrizeName
		}
		reason := fmt.Sprintf("本月%s已有任务正在抢兑或已锁定，跳过本次执行", label)
		log.Printf("【抢兑月度保护】任务 %d %s", task.ID, reason)
		return false, func() {}, reason
	}

	release := func() {
		if err := s.leaseStore.Del(key); err != nil {
			log.Printf("【抢兑月度保护】任务 %d 释放同系列锁失败: %v", task.ID, err)
		}
	}
	return true, release, ""
}

func exchangeTaskProductSnapshot(productRepo *repository.ProductRepository, task *models.ExchangeTask) *models.Product {
	if task == nil {
		return nil
	}
	if task.Product.ID > 0 {
		return &task.Product
	}
	if productRepo == nil || task.ProductID == 0 {
		return nil
	}
	product, err := productRepo.GetByID(task.ProductID)
	if err != nil {
		return nil
	}
	task.Product = *product
	return product
}

func exchangeRecordProductSnapshot(productRepo *repository.ProductRepository, record *models.ExchangeRecord) *models.Product {
	if record == nil {
		return nil
	}
	if record.Product != nil && record.Product.ID > 0 {
		return record.Product
	}
	if productRepo == nil || record.ProductID == 0 {
		return nil
	}
	product, err := productRepo.GetByID(record.ProductID)
	if err != nil {
		return nil
	}
	record.Product = product
	return product
}

func (s *ExchangeService) monthlySeriesSkipReason(task *models.ExchangeTask) (bool, string) {
	skip, reason, err := shouldSkipExchangeMonthlySeries(s.exchangeRecordRepo, s.productRepo, task, time.Now())
	if err != nil {
		log.Printf("【抢兑月度保护】任务 %d 查询本月同系列记录失败，继续执行: %v", task.ID, err)
		return false, ""
	}
	return skip, reason
}

func (s *ExchangeScheduler) monthlySeriesSkipReason(task *models.ExchangeTask) (bool, string) {
	skip, reason, err := shouldSkipExchangeMonthlySeries(s.exchangeRecordRepo, s.productRepo, task, time.Now())
	if err != nil {
		log.Printf("【抢兑月度保护】任务 %d 查询本月同系列记录失败，继续执行: %v", task.ID, err)
		return false, ""
	}
	return skip, reason
}

func (s *ExchangeScheduler) filterTasksByMonthlySeriesGuard(slot string, tasks []*models.ExchangeTask) []*models.ExchangeTask {
	if len(tasks) == 0 {
		return tasks
	}

	filtered := make([]*models.ExchangeTask, 0, len(tasks))
	cache := make(map[string]monthlySeriesGuardResult)
	scheduledSeries := make(map[string]string)

	for _, task := range tasks {
		if task == nil {
			continue
		}

		taskProduct := exchangeTaskProductSnapshot(s.productRepo, task)
		series := exchangeMonthlySeriesForProduct(taskProduct, task.PrizeName)
		if series.Key == "" {
			filtered = append(filtered, task)
			continue
		}

		cacheKey := fmt.Sprintf("%d:%d:%s", task.UserID, task.ExchangeAccountID, series.Key)
		if scheduledPrize, exists := scheduledSeries[cacheKey]; exists {
			reason := fmt.Sprintf("%s 同一账号同系列已安排任务「%s」，跳过「%s」，避免同月重复兑换",
				slot,
				scheduledPrize,
				task.PrizeName,
			)
			_ = s.exchangeTaskRepo.UpdateLastResult(task.ID, reason)
			log.Printf("【抢兑月度保护】任务 %d 跳过: %s", task.ID, reason)
			continue
		}

		result, ok := cache[cacheKey]
		if !ok {
			skip, reason, err := shouldSkipExchangeMonthlySeries(s.exchangeRecordRepo, s.productRepo, task, time.Now())
			if err != nil {
				log.Printf("【抢兑月度保护】任务 %d 查询本月同系列记录失败，保留执行: %v", task.ID, err)
				result = monthlySeriesGuardResult{}
			} else {
				result = monthlySeriesGuardResult{skip: skip, reason: reason}
			}
			cache[cacheKey] = result
		}

		if result.skip {
			_ = s.exchangeTaskRepo.UpdateLastResult(task.ID, result.reason)
			log.Printf("【抢兑月度保护】%s 任务 %d 跳过: %s", slot, task.ID, result.reason)
			continue
		}

		scheduledSeries[cacheKey] = task.PrizeName
		filtered = append(filtered, task)
	}

	return filtered
}
