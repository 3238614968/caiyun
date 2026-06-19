package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/ws"
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
	productRepo         *repository.ProductRepository
	configRepo          *repository.SystemConfigRepository
	taskLogRepo         *repository.TaskLogRepository
	tokenMgr            *TokenManager
	hub                 *ws.Hub
	leaseStore          schedulerLeaseStore
	leaseOwner          string

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
	stopOnce sync.Once
	loopWG   sync.WaitGroup
}

type schedulerLeaseStore interface {
	SetNX(key string, value interface{}, expiration time.Duration) (bool, error)
	Del(keys ...string) error
}

// NewExchangeScheduler 创建抢兑调度器
func NewExchangeScheduler(
	exchangeTaskRepo *repository.ExchangeTaskRepository,
	exchangeAccountRepo *repository.ExchangeAccountRepository,
	exchangeRecordRepo *repository.ExchangeRecordRepository,
	productRepo *repository.ProductRepository,
	configRepo *repository.SystemConfigRepository,
	taskLogRepo *repository.TaskLogRepository,
	tokenMgr *TokenManager,
) *ExchangeScheduler {
	return &ExchangeScheduler{
		exchangeTaskRepo:    exchangeTaskRepo,
		exchangeAccountRepo: exchangeAccountRepo,
		exchangeRecordRepo:  exchangeRecordRepo,
		productRepo:         productRepo,
		configRepo:          configRepo,
		taskLogRepo:         taskLogRepo,
		tokenMgr:            tokenMgr,
		hub:                 ws.GetHub(),
		leaseOwner:          randomLockValue(0),
		stopChan:            make(chan struct{}),
	}
}

// SetLeaseStore 启用准备阶段调度租约，减少多 Worker 副本重复预热同一抢兑时间点。
func (s *ExchangeScheduler) SetLeaseStore(store schedulerLeaseStore) {
	if s == nil {
		return
	}
	s.leaseStore = store
}

// Start 启动调度器
func (s *ExchangeScheduler) Start() {
	if s == nil {
		return
	}
	log.Println("【抢兑调度器】启动...")
	s.loopWG.Add(1)
	go func() {
		defer s.loopWG.Done()
		s.scheduleLoop()
	}()
}

// Stop 停止调度器
func (s *ExchangeScheduler) Stop() {
	if s == nil {
		return
	}
	log.Println("【抢兑调度器】停止...")
	s.stopOnce.Do(func() {
		close(s.stopChan)
	})
	s.loopWG.Wait()
}

// scheduleLoop 调度循环
func (s *ExchangeScheduler) scheduleLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			if s.isStopped() {
				return
			}
			s.checkAndPrepareExchange()
		}
	}
}

func (s *ExchangeScheduler) isStopped() bool {
	if s == nil {
		return true
	}
	select {
	case <-s.stopChan:
		return true
	default:
		return false
	}
}

// checkAndPrepareExchange 检查并准备抢兑（支持自定义时间）
func (s *ExchangeScheduler) checkAndPrepareExchange() {
	if s.isStopped() {
		return
	}
	now := time.Now()
	if hour, minute, ok := scheduledPrepareSlot(now); ok {
		if s.isStopped() {
			return
		}
		if s.claimSchedulerSlot("prepare", now, hour, minute, 10*time.Minute) {
			log.Printf("【抢兑调度器】准备 %02d:%02d 抢兑队列...", hour, minute)
			s.prepareQueueByTime(hour, minute)
		}
	}
	if hour, minute, ok := scheduledExecuteSlot(now); ok {
		if s.isStopped() {
			return
		}
		// 执行阶段不再使用整分钟租约。多副本同时触发时由 TryMarkRunning 抢占任务执行权，
		// 避免拿到租约的实例在真正执行前崩溃导致整个时间槽漏执行。
		log.Printf("【抢兑调度器】执行 %02d:%02d 抢兑...", hour, minute)
		s.executeExchangeByTime(hour, minute)
	}
}

func (s *ExchangeScheduler) claimSchedulerSlot(kind string, now time.Time, hour, minute int, ttl time.Duration) bool {
	if s == nil || s.leaseStore == nil {
		return true
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	key := fmt.Sprintf("exchange:scheduler:%s:%s:%02d%02d", kind, now.Format("20060102"), hour, minute)
	owner := s.leaseOwner
	if owner == "" {
		owner = randomLockValue(0)
	}
	ok, err := s.leaseStore.SetNX(key, owner, ttl)
	if err != nil {
		log.Printf("【抢兑调度器】获取 %s 调度租约失败，降级为本实例执行: key=%s err=%v", kind, key, err)
		return true
	}
	if !ok {
		log.Printf("【抢兑调度器】%s %02d:%02d 已由其他实例处理，跳过本实例", kind, hour, minute)
		return false
	}
	return true
}

// prepareQueueByTime 根据指定时间准备抢兑队列
func (s *ExchangeScheduler) prepareQueueByTime(hour, minute int) {
	if s.isStopped() {
		return
	}
	slot := fmt.Sprintf("%02d:%02d", hour, minute)

	// Load tasks for the target slot.
	tasks, err := s.exchangeTaskRepo.GetTasksByTime(hour, minute)
	if err != nil {
		log.Printf("【抢兑调度器】获取 %s 抢兑任务失败: %v", slot, err)
		return
	}

	log.Printf("【抢兑调度器】查询 %s 找到 %d 个任务", slot, len(tasks))

	if len(tasks) == 0 {
		return
	}

	tasks = s.filterTasksByMonthlySeriesGuard(slot, tasks)
	if len(tasks) == 0 {
		log.Printf("【抢兑调度器】%s 所有任务已被本月同系列保护跳过，本次不加入抢兑队列", slot)
		return
	}

	s.logQueuedTasks(slot, tasks)
	readyAccounts := s.preheatAccountsForTasks(slot, tasks)
	tasks = filterTasksByReadyAccounts(slot, tasks, readyAccounts)
	if len(tasks) == 0 {
		log.Printf("【抢兑调度器】%s 预热后没有可执行账号，本次不加入抢兑队列", slot)
		return
	}

	s.queueMutex.Lock()
	// Merge into the shared in-memory queue.
	s.morningQueue = mergeExchangeTasks(s.morningQueue, tasks)
	s.queueMutex.Unlock()

	log.Printf("【抢兑调度器】%s 抢兑队列已准备，共 %d 个任务", slot, len(tasks))

	s.hub.Broadcast(ws.Message{
		Type: "exchange_preparing",
		Data: map[string]interface{}{
			"time":    slot,
			"count":   len(tasks),
			"message": fmt.Sprintf("%s 抢兑即将开始，共%d个任务准备就绪", slot, len(tasks)),
		},
	})
}

func (s *ExchangeScheduler) executeExchangeByTime(hour, minute int) {
	if s.isStopped() {
		return
	}
	slot := fmt.Sprintf("%02d:%02d", hour, minute)

	s.queueMutex.Lock()
	var tasksToExecute []*models.ExchangeTask
	var remainingTasks []*models.ExchangeTask

	for _, task := range s.morningQueue {
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

	fromPreparedQueue := len(tasksToExecute) > 0
	if len(tasksToExecute) == 0 {
		var err error
		tasksToExecute, err = s.exchangeTaskRepo.GetTasksByTime(hour, minute)
		if err != nil {
			log.Printf("【抢兑调度器】补查 %s 抢兑任务失败: %v", slot, err)
			return
		}
		if len(tasksToExecute) == 0 {
			return
		}
	}

	tasksToExecute = s.filterTasksByMonthlySeriesGuard(slot, tasksToExecute)
	if len(tasksToExecute) == 0 {
		log.Printf("【抢兑调度器】%s 所有任务已被本月同系列保护跳过，本次不执行", slot)
		return
	}

	if !fromPreparedQueue {
		readyAccounts := s.preheatAccountsForTasks(slot, tasksToExecute)
		tasksToExecute = filterTasksByReadyAccounts(slot, tasksToExecute, readyAccounts)
		if len(tasksToExecute) == 0 {
			log.Printf("【抢兑调度器】%s 补查任务预热后没有可执行账号，跳过本次执行", slot)
			return
		}
	}

	if s.isStopped() {
		return
	}

	log.Printf("【抢兑调度器】开始执行 %s 抢兑，共 %d 个任务", slot, len(tasksToExecute))
	go s.executeExchangeWithAutoSwitch(tasksToExecute, slot)
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
