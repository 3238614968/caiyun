package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/ws"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"
)

// ExchangeScheduler 抢兑调度器
// 负责管理抢兑任务的调度、提前初始化和自动切换账号
type ExchangeScheduler struct {
	exchangeTaskRepo    *repository.ExchangeTaskRepository
	exchangeAccountRepo *repository.ExchangeAccountRepository
	exchangeRecordRepo  *repository.ExchangeRecordRepository
	configRepo          *repository.SystemConfigRepository
	taskLogRepo         *repository.TaskLogRepository
	tokenMgr            *TokenManager
	hub                 *ws.Hub

	// 抢兑队列
	morningQueue []*models.ExchangeTask // 上午10点抢兑队列
	eveningQueue []*models.ExchangeTask // 下午16点抢兑队列
	queueMutex   sync.RWMutex

	// 执行状态
	isMorningRunning bool
	isEveningRunning bool
	statusMutex      sync.RWMutex

	// 停止信号
	stopChan chan struct{}
}

// NewExchangeScheduler 创建抢兑调度器
func NewExchangeScheduler(
	exchangeTaskRepo *repository.ExchangeTaskRepository,
	exchangeAccountRepo *repository.ExchangeAccountRepository,
	exchangeRecordRepo *repository.ExchangeRecordRepository,
	configRepo *repository.SystemConfigRepository,
	taskLogRepo *repository.TaskLogRepository,
	tokenMgr *TokenManager,
) *ExchangeScheduler {
	return &ExchangeScheduler{
		exchangeTaskRepo:    exchangeTaskRepo,
		exchangeAccountRepo: exchangeAccountRepo,
		exchangeRecordRepo:  exchangeRecordRepo,
		configRepo:          configRepo,
		taskLogRepo:         taskLogRepo,
		tokenMgr:            tokenMgr,
		hub:                 ws.GetHub(),
		stopChan:            make(chan struct{}),
	}
}

// Start 启动调度器
func (s *ExchangeScheduler) Start() {
	log.Println("【抢兑调度器】启动...")
	go s.scheduleLoop()
}

// Stop 停止调度器
func (s *ExchangeScheduler) Stop() {
	log.Println("【抢兑调度器】停止...")
	close(s.stopChan)
}

// scheduleLoop 调度循环
func (s *ExchangeScheduler) scheduleLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAndPrepareExchange()
		case <-s.stopChan:
			return
		}
	}
}

// checkAndPrepareExchange 检查并准备抢兑（支持自定义时间）
func (s *ExchangeScheduler) checkAndPrepareExchange() {
	now := time.Now()
	if hour, minute, ok := scheduledPrepareSlot(now); ok {
		log.Printf("【抢兑调度器】准备 %02d:%02d 抢兑队列...", hour, minute)
		s.prepareQueueByTime(hour, minute)
	}
	if hour, minute, ok := scheduledExecuteSlot(now); ok {
		log.Printf("【抢兑调度器】执行 %02d:%02d 抢兑...", hour, minute)
		s.executeExchangeByTime(hour, minute)
	}
}

// prepareQueueByTime 根据指定时间准备抢兑队列
func (s *ExchangeScheduler) prepareQueueByTime(hour, minute int) {
	// 获取该时间点的任务
	tasks, err := s.exchangeTaskRepo.GetTasksByTime(hour, minute)
	if err != nil {
		log.Printf("【抢兑调度器】获取 %02d:%02d 抢兑任务失败: %v", hour, minute, err)
		return
	}

	log.Printf("【抢兑调度器】查询 %02d:%02d 找到 %d 个任务", hour, minute, len(tasks))

	if len(tasks) == 0 {
		return
	}

	s.queueMutex.Lock()
	// 将任务添加到队列（合并到morningQueue统一处理）
	s.morningQueue = mergeExchangeTasks(s.morningQueue, tasks)
	s.queueMutex.Unlock()

	log.Printf("【抢兑调度器】%02d:%02d 抢兑队列已准备，共 %d 个任务", hour, minute, len(tasks))

	// 发送WebSocket通知
	s.hub.Broadcast(ws.Message{
		Type: "exchange_preparing",
		Data: map[string]interface{}{
			"time":    fmt.Sprintf("%02d:%02d", hour, minute),
			"count":   len(tasks),
			"message": fmt.Sprintf("%02d:%02d 抢兑即将开始，共%d个任务准备就绪", hour, minute, len(tasks)),
		},
	})
}

// executeExchangeByTime 根据指定时间执行抢兑
func (s *ExchangeScheduler) executeExchangeByTime(hour, minute int) {
	s.queueMutex.Lock()
	// 从队列中筛选出当前时间需要执行的任务
	var tasksToExecute []*models.ExchangeTask
	var remainingTasks []*models.ExchangeTask

	for _, task := range s.morningQueue {
		// 检查任务是否匹配当前时间
		et1 := task.ExchangeAccount.ExchangeTime1
		et2 := task.ExchangeAccount.ExchangeTime2
		timeStr := fmt.Sprintf("%02d:%02d:00", hour, minute)

		if et1 == timeStr || et2 == timeStr {
			tasksToExecute = append(tasksToExecute, task)
		} else {
			remainingTasks = append(remainingTasks, task)
		}
	}

	s.morningQueue = remainingTasks
	s.queueMutex.Unlock()

	if len(tasksToExecute) == 0 {
		var err error
		tasksToExecute, err = s.exchangeTaskRepo.GetTasksByTime(hour, minute)
		if err != nil {
			log.Printf("【抢兑调度器】补查 %02d:%02d 抢兑任务失败: %v", hour, minute, err)
			return
		}
		if len(tasksToExecute) == 0 {
			return
		}
	}

	log.Printf("【抢兑调度器】开始执行 %02d:%02d 抢兑，共 %d 个任务", hour, minute, len(tasksToExecute))
	go s.executeExchangeWithAutoSwitch(tasksToExecute, fmt.Sprintf("%02d:%02d", hour, minute))
}

// prepareMorningQueue 准备上午抢兑队列
func (s *ExchangeScheduler) prepareMorningQueue() {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()

	// 获取所有启用的抢兑任务（上午10点）
	tasks, err := s.exchangeTaskRepo.GetTasksByTime(constants.MorningExchangeHour, constants.MorningExchangeMinute)
	if err != nil {
		log.Printf("【抢兑调度器】获取上午抢兑任务失败: %v", err)
		return
	}

	s.morningQueue = tasks
	log.Printf("【抢兑调度器】上午抢兑队列已准备，共 %d 个任务", len(tasks))

	// 发送WebSocket通知
	s.hub.Broadcast(ws.Message{
		Type: "exchange_preparing",
		Data: map[string]interface{}{
			"period":  "morning",
			"time":    "10:00",
			"count":   len(tasks),
			"message": fmt.Sprintf("上午10点抢兑即将开始，共%d个任务准备就绪", len(tasks)),
		},
	})
}

// prepareEveningQueue 准备下午抢兑队列
func (s *ExchangeScheduler) prepareEveningQueue() {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()

	// 获取所有启用的抢兑任务（下午16点）
	tasks, err := s.exchangeTaskRepo.GetTasksByTime(constants.EveningExchangeHour, constants.EveningExchangeMinute)
	if err != nil {
		log.Printf("【抢兑调度器】获取下午抢兑任务失败: %v", err)
		return
	}

	s.eveningQueue = tasks
	log.Printf("【抢兑调度器】下午抢兑队列已准备，共 %d 个任务", len(tasks))

	// 发送WebSocket通知
	s.hub.Broadcast(ws.Message{
		Type: "exchange_preparing",
		Data: map[string]interface{}{
			"period":  "evening",
			"time":    "16:00",
			"count":   len(tasks),
			"message": fmt.Sprintf("下午16点抢兑即将开始，共%d个任务准备就绪", len(tasks)),
		},
	})
}

// executeMorningExchange 执行上午抢兑
func (s *ExchangeScheduler) executeMorningExchange() {
	s.statusMutex.Lock()
	s.isMorningRunning = true
	s.statusMutex.Unlock()

	defer func() {
		s.statusMutex.Lock()
		s.isMorningRunning = false
		s.statusMutex.Unlock()
	}()

	s.queueMutex.RLock()
	tasks := make([]*models.ExchangeTask, len(s.morningQueue))
	copy(tasks, s.morningQueue)
	s.queueMutex.RUnlock()

	if len(tasks) == 0 {
		log.Println("【抢兑调度器】上午抢兑队列为空")
		return
	}

	log.Printf("【抢兑调度器】开始执行上午抢兑，共 %d 个任务", len(tasks))
	s.executeExchangeWithAutoSwitch(tasks, "morning")
}

// executeEveningExchange 执行下午抢兑
func (s *ExchangeScheduler) executeEveningExchange() {
	s.statusMutex.Lock()
	s.isEveningRunning = true
	s.statusMutex.Unlock()

	defer func() {
		s.statusMutex.Lock()
		s.isEveningRunning = false
		s.statusMutex.Unlock()
	}()

	s.queueMutex.RLock()
	tasks := make([]*models.ExchangeTask, len(s.eveningQueue))
	copy(tasks, s.eveningQueue)
	s.queueMutex.RUnlock()

	if len(tasks) == 0 {
		log.Println("【抢兑调度器】下午抢兑队列为空")
		return
	}

	log.Printf("【抢兑调度器】开始执行下午抢兑，共 %d 个任务", len(tasks))
	s.executeExchangeWithAutoSwitch(tasks, "evening")
}

// executeExchangeWithAutoSwitch 执行抢兑（带自动切换账号功能）
func (s *ExchangeScheduler) executeExchangeWithAutoSwitch(tasks []*models.ExchangeTask, period string) {
	// 按商品ID分组任务
	taskGroups := s.groupTasksByProduct(tasks)

	// 获取并发配置，并在所有商品组之间共享全局并发上限
	concurrency := s.getConfiguredConcurrency()
	limiter := make(chan struct{}, concurrency)

	// 为每个商品组创建执行器
	var wg sync.WaitGroup

	for prizeID, groupTasks := range taskGroups {
		wg.Add(1)
		go func(prizeID string, groupTasks []*models.ExchangeTask) {
			defer wg.Done()
			s.executeProductGroup(prizeID, groupTasks, limiter)
		}(prizeID, groupTasks)
	}

	wg.Wait()

	log.Printf("【抢兑调度器】%s时段抢兑执行完成", period)

	// 发送完成通知
	s.hub.Broadcast(ws.Message{
		Type: "exchange_completed",
		Data: map[string]interface{}{
			"period":  period,
			"message": fmt.Sprintf("%s 抢兑执行完成", humanizeExchangePeriod(period)),
		},
	})
}

// groupTasksByProduct 按商品ID分组任务
func (s *ExchangeScheduler) groupTasksByProduct(tasks []*models.ExchangeTask) map[string][]*models.ExchangeTask {
	groups := make(map[string][]*models.ExchangeTask)
	for _, task := range tasks {
		groups[task.PrizeID] = append(groups[task.PrizeID], task)
	}
	return groups
}

// executeProductGroup 执行商品组的抢兑（自动切换账号）
func (s *ExchangeScheduler) executeProductGroup(prizeID string, tasks []*models.ExchangeTask, limiter chan struct{}) {
	log.Printf("【抢兑调度器】开始抢兑商品 %s，共 %d 个账号", prizeID, len(tasks))

	// 用于控制是否停止抢兑的标记
	var stopFlag sync.Map
	stopFlag.Store(prizeID, false)

	// 用于记录成功状态的映射
	successMap := make(map[uint]bool)
	var successMutex sync.Mutex
	var wg sync.WaitGroup

	for _, task := range tasks {
		if task == nil {
			continue
		}
		task := task // 捕获循环变量
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 检查是否需要停止
			if shouldStop, _ := stopFlag.Load(prizeID); shouldStop.(bool) {
				return
			}

			// 检查该任务是否已经成功
			successMutex.Lock()
			if successMap[task.ID] {
				successMutex.Unlock()
				return
			}
			successMutex.Unlock()

			limiter <- struct{}{}
			// 执行抢兑
			success, message, execTime := s.executeTask(task)
			<-limiter

			if success {
				// 抢兑成功，记录成功状态
				successMutex.Lock()
				successMap[task.ID] = true
				successMutex.Unlock()

				log.Printf("【抢兑调度器】任务 %d 抢兑成功，账号: %d，商品: %s",
					task.ID, task.ExchangeAccountID, task.PrizeName)

				// 继续执行下一个账号（自动切换）
				// 注意：这里不停止，继续尝试其他账号
			} else {
				// 检查是否需要停止抢兑
				if s.shouldStopExchange(message) {
					log.Printf("【抢兑调度器】商品 %s 抢兑停止，原因: %s", prizeID, message)
					stopFlag.Store(prizeID, true)
				}
			}

			// 记录结果
			s.finalizeTaskResult(task, success, message, execTime)
		}()
	}

	wg.Wait()

	// 统计结果
	successCount := 0
	for _, success := range successMap {
		if success {
			successCount++
		}
	}

	log.Printf("【抢兑调度器】商品 %s 抢兑完成，成功 %d/%d 个账号", prizeID, successCount, len(tasks))
}

// executeTask 执行单个抢兑任务
func (s *ExchangeScheduler) executeTask(task *models.ExchangeTask) (bool, string, int) {
	s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskRunning))

	// 检查商品是否可抢兑（通过任务预加载的商品信息）
	if task.Product.ID > 0 {
		if !task.Product.IsActive {
			return false, "商品已下架，无法抢兑", 0
		}
		if task.Product.StockStatus != "available" || task.Product.DailyRemainderCount <= 0 {
			return false, "商品已售罄，无法抢兑", 0
		}
	}

	// 获取兑换账号
	account, err := s.exchangeAccountRepo.GetByID(task.ExchangeAccountID)
	if err != nil {
		return false, "获取兑换账号失败", 0
	}

	// 检查账号是否启用
	if !account.IsActive {
		return false, "账号已禁用", 0
	}

	return performExchange(account, task.PrizeID, s.tokenMgr)
}

// shouldStopExchange 判断是否应该停止抢兑
func (s *ExchangeScheduler) shouldStopExchange(message string) bool {
	// 以下情况应该停止抢兑
	stopPatterns := []string{
		"奖品单日已耗尽",
		"奖品已兑完",
		"今日已兑换",
	}

	for _, pattern := range stopPatterns {
		if contains(message, pattern) {
			return true
		}
	}

	return false
}

// recordResult 记录抢兑结果，并与手动执行路径保持一致地更新尝试次数和最后结果。
func (s *ExchangeScheduler) recordResult(task *models.ExchangeTask, success bool, message string, execTime int) {
	status := "failed"
	if success {
		status = "success"
	}

	record := &models.ExchangeRecord{
		UserID:            task.UserID,
		ExchangeAccountID: task.ExchangeAccountID,
		ExchangeTaskID:    &task.ID,
		ProductID:         task.ProductID,
		PrizeID:           task.PrizeID,
		PrizeName:         task.PrizeName,
		Status:            status,
		Message:           message,
		ExecutionTimeMs:   execTime,
	}

	if err := s.exchangeRecordRepo.Create(record); err != nil {
		log.Printf("【抢兑调度器】记录抢兑结果失败: %v", err)
	}
	s.exchangeTaskRepo.UpdateAttempt(task.ID, success, message)
	createExchangeSystemLog(
		s.taskLogRepo,
		task.UserID,
		task.ExchangeAccount.AccountID,
		task.PrizeName,
		exchangeAccountName(&task.ExchangeAccount),
		success,
		message,
		execTime,
	)

	// 发送WebSocket通知
	s.hub.SendToUser(task.UserID, ws.Message{
		Type: "exchange_result",
		Data: map[string]interface{}{
			"task_id":      task.ID,
			"prize_name":   task.PrizeName,
			"success":      success,
			"message":      message,
			"execution_ms": execTime,
		},
	})
}

func (s *ExchangeScheduler) finalizeTaskResult(task *models.ExchangeTask, success bool, message string, execTime int) {
	s.recordResult(task, success, message, execTime)

	if success {
		if isSingleRunExchangeTask(task.TaskType) {
			if task.AttemptedCount+1 >= task.MaxAttempts {
				s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskCompleted))
			} else {
				s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
			}
			return
		}

		s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
		return
	}

	if s.shouldStopExchange(message) {
		s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskCompleted))
		return
	}

	s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
}

func scheduledPrepareSlot(now time.Time) (int, int, bool) {
	if constants.ExchangePreInitSeconds <= 0 || now.Second() != 60-constants.ExchangePreInitSeconds {
		return 0, 0, false
	}

	target := now.Add(time.Duration(constants.ExchangePreInitSeconds) * time.Second)
	return target.Hour(), target.Minute(), true
}

func scheduledExecuteSlot(now time.Time) (int, int, bool) {
	if now.Second() != 0 {
		return 0, 0, false
	}
	return now.Hour(), now.Minute(), true
}

func mergeExchangeTasks(existing []*models.ExchangeTask, incoming []*models.ExchangeTask) []*models.ExchangeTask {
	if len(incoming) == 0 {
		return existing
	}

	seen := make(map[uint]struct{}, len(existing)+len(incoming))
	merged := make([]*models.ExchangeTask, 0, len(existing)+len(incoming))
	for _, task := range existing {
		if task == nil {
			continue
		}
		seen[task.ID] = struct{}{}
		merged = append(merged, task)
	}
	for _, task := range incoming {
		if task == nil {
			continue
		}
		if _, ok := seen[task.ID]; ok {
			continue
		}
		seen[task.ID] = struct{}{}
		merged = append(merged, task)
	}
	return merged
}

func humanizeExchangePeriod(period string) string {
	switch period {
	case "morning":
		return "上午"
	case "evening":
		return "下午"
	default:
		return period
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
func (s *ExchangeScheduler) getConfiguredConcurrency() int {
	if s.configRepo == nil {
		return constants.DefaultConcurrency
	}

	config, err := s.configRepo.GetByKey(constants.ConfigKeyExchangeConcurrency)
	if err != nil || config == nil || config.KeyValue == "" {
		return constants.DefaultConcurrency
	}

	concurrency, err := strconv.Atoi(config.KeyValue)
	if err != nil || concurrency <= 0 {
		return constants.DefaultConcurrency
	}
	if concurrency > 1000 {
		return 1000
	}
	return concurrency
}
