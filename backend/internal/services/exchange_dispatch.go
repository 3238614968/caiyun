package services

import (
	"caiyun/internal/models"
	"time"
)

// Exact fire and fallback ticks can overlap. Deduplicate task IDs per dated
// slot, while still allowing a newly created task in a later fallback check.
func (s *ExchangeScheduler) undispatchedSlotTasks(slot time.Time, tasks []*models.ExchangeTask) []*models.ExchangeTask {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()
	if s.dispatchedTasks == nil {
		s.dispatchedTasks = make(map[string]map[uint]bool)
	}
	key := slot.Format("20060102:1504")
	if s.dispatchedTasks[key] == nil {
		s.dispatchedTasks[key] = make(map[uint]bool)
	}
	for old := range s.dispatchedTasks {
		parsed, err := time.ParseInLocation("20060102:1504", old, slot.Location())
		if err != nil || parsed.Before(slot.Add(-2*time.Hour)) {
			delete(s.dispatchedTasks, old)
		}
	}
	out := make([]*models.ExchangeTask, 0, len(tasks))
	for _, task := range tasks {
		if task == nil || s.dispatchedTasks[key][task.ID] {
			continue
		}
		s.dispatchedTasks[key][task.ID] = true
		out = append(out, task)
	}
	return out
}

func (s *ExchangeScheduler) finishPreparingSlot(slot time.Time) {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()
	delete(s.preparedSlots, slot.Format("20060102:1504"))
}

func (s *ExchangeScheduler) tasksNeedingWarmup(tasks []*models.ExchangeTask) []*models.ExchangeTask {
	s.queueMutex.RLock()
	defer s.queueMutex.RUnlock()
	var out []*models.ExchangeTask
	for _, task := range tasks {
		if task == nil {
			continue
		}
		entry, ok := s.warmSessions[task.ID]
		if !ok || entry.prepared == nil || entry.accountID != task.ExchangeAccount.AccountID || time.Now().After(entry.expiresAt) {
			out = append(out, task)
			continue
		}
		if entry.prepared.sourceAuth != "" && sanitizeAuthValue(entry.prepared.sourceAuth) != sanitizeAuthValue(task.ExchangeAccount.Auth) {
			out = append(out, task)
		}
	}
	return out
}
