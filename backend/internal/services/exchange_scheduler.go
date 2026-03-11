package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/utils"
	"caiyun/internal/ws"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// ExchangeScheduler 抢兑调度器
// 负责管理抢兑任务的调度、提前初始化和自动切换账号
type ExchangeScheduler struct {
	exchangeTaskRepo    *repository.ExchangeTaskRepository
	exchangeAccountRepo *repository.ExchangeAccountRepository
	exchangeRecordRepo  *repository.ExchangeRecordRepository
	tokenMgr            *TokenManager
	hub                 *ws.Hub

	// 抢兑队列
	morningQueue   []*models.ExchangeTask // 上午10点抢兑队列
	eveningQueue   []*models.ExchangeTask // 下午16点抢兑队列
	queueMutex     sync.RWMutex

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
	tokenMgr *TokenManager,
) *ExchangeScheduler {
	return &ExchangeScheduler{
		exchangeTaskRepo:    exchangeTaskRepo,
		exchangeAccountRepo: exchangeAccountRepo,
		exchangeRecordRepo:  exchangeRecordRepo,
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

// checkAndPrepareExchange 检查并准备抢兑
func (s *ExchangeScheduler) checkAndPrepareExchange() {
	now := time.Now()
	hour := now.Hour()
	minute := now.Minute()
	second := now.Second()

	// 检查是否需要提前初始化（提前3秒）
	// 上午10点：9:59:57 初始化
	// 下午16点：15:59:57 初始化
	if hour == constants.MorningExchangeHour && minute == 59 && second == 60-constants.ExchangePreInitSeconds {
		if !s.isMorningRunning {
			log.Println("【抢兑调度器】准备上午10点抢兑队列...")
			s.prepareMorningQueue()
		}
	}

	if hour == constants.EveningExchangeHour && minute == 59 && second == 60-constants.ExchangePreInitSeconds {
		if !s.isEveningRunning {
			log.Println("【抢兑调度器】准备下午16点抢兑队列...")
			s.prepareEveningQueue()
		}
	}

	// 检查是否到达抢兑时间
	if hour == constants.MorningExchangeHour && minute == 0 && second == 0 {
		if !s.isMorningRunning {
			log.Println("【抢兑调度器】开始上午10点抢兑...")
			go s.executeMorningExchange()
		}
	}

	if hour == constants.EveningExchangeHour && minute == 0 && second == 0 {
		if !s.isEveningRunning {
			log.Println("【抢兑调度器】开始下午16点抢兑...")
			go s.executeEveningExchange()
		}
	}
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
			"period": "morning",
			"time":   "10:00",
			"count":  len(tasks),
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
			"period": "evening",
			"time":   "16:00",
			"count":  len(tasks),
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

	// 获取并发配置
	concurrency := constants.DefaultConcurrency

	// 为每个商品组创建执行器
	var wg sync.WaitGroup

	for prizeID, groupTasks := range taskGroups {
		wg.Add(1)
		go func(prizeID string, groupTasks []*models.ExchangeTask) {
			defer wg.Done()
			s.executeProductGroup(prizeID, groupTasks, concurrency)
		}(prizeID, groupTasks)
	}

	wg.Wait()

	log.Printf("【抢兑调度器】%s时段抢兑执行完成", period)

	// 发送完成通知
	s.hub.Broadcast(ws.Message{
		Type: "exchange_completed",
		Data: map[string]interface{}{
			"period":  period,
			"message": fmt.Sprintf("%s时段抢兑执行完成", map[string]string{"morning": "上午", "evening": "下午"}[period]),
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
func (s *ExchangeScheduler) executeProductGroup(prizeID string, tasks []*models.ExchangeTask, concurrency int) {
	log.Printf("【抢兑调度器】开始抢兑商品 %s，共 %d 个账号", prizeID, len(tasks))

	// 创建工作池
	executor := utils.NewConcurrentExecutor(concurrency)

	// 用于控制是否停止抢兑的标记
	var stopFlag sync.Map
	stopFlag.Store(prizeID, false)

	// 用于记录成功状态的映射
	successMap := make(map[uint]bool)
	var successMutex sync.Mutex

	for _, task := range tasks {
		task := task // 捕获循环变量

		executor.Submit(func() {
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

			// 执行抢兑
			success, message, execTime := s.executeTask(task)

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
			s.recordResult(task, success, message, execTime)
		})
	}

	executor.Wait()

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
	startTime := time.Now()

	// 获取兑换账号
	account, err := s.exchangeAccountRepo.GetByID(task.ExchangeAccountID)
	if err != nil {
		return false, "获取兑换账号失败", 0
	}

	// 检查账号是否启用
	if !account.IsActive {
		return false, "账号已禁用", 0
	}

	// 创建带认证的HTTP客户端
	client, err := s.tokenMgr.CreateAuthenticatedClient(account.AccountID, account.Auth)
	if err != nil {
		return false, fmt.Sprintf("获取账号 Token 失败：%v", err), 0
	}

	// 调用兑换API
	url := utils.BuildExchangeURL(task.PrizeID)
	resp, err := client.Get(url, nil)
	if err != nil {
		return false, fmt.Sprintf("请求失败：%v", err), int(time.Since(startTime).Milliseconds())
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return false, fmt.Sprintf("读取响应失败：%v", err), int(time.Since(startTime).Milliseconds())
	}

	execTime := int(time.Since(startTime).Milliseconds())

	// 解析响应
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return false, fmt.Sprintf("解析响应失败：%v", err), execTime
	}

	msg, ok := response["msg"].(string)
	if !ok {
		return false, "响应格式错误", execTime
	}

	if msg == "success" {
		return true, "兑换成功", execTime
	}

	return false, msg, execTime
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

// recordResult 记录抢兑结果
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

	// 更新任务状态
	if success {
		s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskCompleted))
		s.exchangeTaskRepo.IncrementSuccessCount(task.ID)
	} else {
		s.exchangeTaskRepo.IncrementFailCount(task.ID)
	}

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
