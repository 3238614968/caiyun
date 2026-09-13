package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/envutil"
	"caiyun/internal/models"
	"caiyun/internal/monitor"
	"caiyun/internal/repository"
	"caiyun/internal/ws"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
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
	metrics             *monitor.Metrics
	runningTimeout      time.Duration

	// 抢兑队列
	preparedQueue []*models.ExchangeTask
	queueMutex    sync.RWMutex

	// 每副本独立的调度去重状态：preparedSlots 记录本副本已预热入队的时间槽，
	// scheduledFires 记录已挂上精确触发定时器的时间槽，防止重复预热与重复触发。
	// 跨副本的执行去重仍由 TryMarkRunning 兜底。
	preparedSlots  map[string]bool
	scheduledFires map[string]*time.Timer

	// 停止信号
	stopChan        chan struct{}
	stopOnce        sync.Once
	loopWG          sync.WaitGroup
	executionCtx    context.Context
	cancelExecution context.CancelFunc
	executionWG     sync.WaitGroup
}

const defaultExchangeTaskRunningTimeout = 15 * time.Minute

type schedulerLeaseStore interface {
	SetNX(key string, value interface{}, expiration time.Duration) (bool, error)
	DelIfValue(key, value string) (bool, error)
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
	eventHubs ...*ws.Hub,
) *ExchangeScheduler {
	var eventHub *ws.Hub
	if len(eventHubs) > 0 {
		eventHub = eventHubs[0]
	}
	executionCtx, cancelExecution := context.WithCancel(context.Background())
	return &ExchangeScheduler{
		exchangeTaskRepo:    exchangeTaskRepo,
		exchangeAccountRepo: exchangeAccountRepo,
		exchangeRecordRepo:  exchangeRecordRepo,
		productRepo:         productRepo,
		configRepo:          configRepo,
		taskLogRepo:         taskLogRepo,
		tokenMgr:            tokenMgr,
		hub:                 eventHub,
		leaseOwner:          randomLockValue(0),
		stopChan:            make(chan struct{}),
		runningTimeout:      exchangeTaskRunningTimeoutFromEnv(),
		executionCtx:        executionCtx,
		cancelExecution:     cancelExecution,
		preparedSlots:       make(map[string]bool),
		scheduledFires:      make(map[string]*time.Timer),
	}
}

// SetEventHub attaches the process-owned realtime delivery hub.
func (s *ExchangeScheduler) SetEventHub(eventHub *ws.Hub) {
	if s != nil {
		s.hub = eventHub
	}
}

// SetLeaseStore 启用准备阶段调度租约，减少多 Worker 副本重复预热同一抢兑时间点。
func (s *ExchangeScheduler) SetLeaseStore(store schedulerLeaseStore) {
	if s == nil {
		return
	}
	s.leaseStore = store
}

// SetMetrics 绑定 Prometheus 指标收集器。
func (s *ExchangeScheduler) SetMetrics(metrics *monitor.Metrics) {
	if s == nil {
		return
	}
	s.metrics = metrics
}

func (s *ExchangeScheduler) SetRunningTimeout(timeout time.Duration) {
	if s == nil || timeout <= 0 {
		return
	}
	s.runningTimeout = timeout
}

// Start 启动调度器
func (s *ExchangeScheduler) Start() {
	if s == nil {
		return
	}
	log.Println("【抢兑调度器】启动...")
	s.recoverStaleRunningTasks("startup")
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
		if s.cancelExecution != nil {
			s.cancelExecution()
		}
	})
	s.stopScheduledFires()
	s.loopWG.Wait()
	// Running exchange attempts share executionCtx.  Cancel it before waiting so
	// a process shutdown does not leave a scheduler task sleeping between retry
	// attempts or waiting for the request pacing limiter.
	s.executionWG.Wait()
}

// scheduleLoop 调度循环
func (s *ExchangeScheduler) scheduleLoop() {
	wakeTimer := time.NewTimer(nextSchedulerWakeDelay(time.Now()))
	recoverTicker := time.NewTicker(time.Minute)
	defer wakeTimer.Stop()
	defer recoverTicker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-recoverTicker.C:
			if s.isStopped() {
				return
			}
			s.recoverStaleRunningTasks("ticker")
		case <-wakeTimer.C:
			if s.isStopped() {
				return
			}
			s.checkAndPrepareExchange()
			wakeTimer.Reset(nextSchedulerWakeDelay(time.Now()))
		}
	}
}

func (s *ExchangeScheduler) recoverStaleRunningTasks(source string) {
	if s == nil || s.exchangeTaskRepo == nil {
		return
	}
	timeout := s.runningTimeout
	if timeout <= 0 {
		timeout = exchangeTaskRunningTimeoutFromEnv()
		s.runningTimeout = timeout
	}
	recovered, err := s.exchangeTaskRepo.RecoverStaleRunning(timeout)
	if err != nil {
		log.Printf("【抢兑调度器】恢复超时 running 任务失败 source=%s err=%v", source, err)
		return
	}
	if recovered > 0 {
		log.Printf("【抢兑调度器】已恢复 %d 个超时 running 抢兑任务 source=%s", recovered, source)
	}
}

func exchangeTaskRunningTimeoutFromEnv() time.Duration {
	return envutil.Duration("EXCHANGE_TASK_RUNNING_TIMEOUT", defaultExchangeTaskRunningTimeout)
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
		// 每个副本各自预热队列与 JWT（预热只产生本副本内存状态与连接准备，
		// 无跨副本副作用），避免未抢到 prepare 租约的副本在 :00 触发时才
		// 同步预热，把首个请求拖慢数秒。跨副本执行去重由 TryMarkRunning 保证。
		if s.markPreparedOnce(now, hour, minute) {
			log.Printf("【抢兑调度器】准备 %02d:%02d 抢兑队列...", hour, minute)
			s.prepareQueueByTime(hour, minute)
			s.schedulePreciseFire(now, hour, minute)
		}
	}
	if hour, minute, ok := scheduledExecuteSlot(now); ok {
		if s.isStopped() {
			return
		}
		// 执行阶段不再使用整分钟租约。多副本同时触发时由 TryMarkRunning 抢占任务执行权，
		// 避免拿到租约的实例在真正执行前崩溃导致整个时间槽漏执行。
		log.Printf("【抢兑调度器】执行 %02d:%02d 抢兑（兜底 tick）...", hour, minute)
		s.executeExchangeByTime(hour, minute)
	}
}

// markPreparedOnce 保证同一副本对同一时间槽只预热入队一次（按日期+时分去重）。
func (s *ExchangeScheduler) markPreparedOnce(now time.Time, hour, minute int) bool {
	executeTime := now.Add(time.Duration(constants.ExchangePreInitSeconds) * time.Second)
	key := fmt.Sprintf("%s:%02d%02d", executeTime.Format("20060102"), hour, minute)

	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()
	if s.preparedSlots[key] {
		return false
	}
	s.preparedSlots[key] = true

	// 轻量清理：只保留当天的槽位标记，避免长期运行内存增长。
	today := executeTime.Format("20060102")
	for existing := range s.preparedSlots {
		if !strings.HasPrefix(existing, today+":") && existing != key {
			delete(s.preparedSlots, existing)
		}
	}
	return true
}

// schedulePreciseFire 在准备阶段（默认提前 ExchangePreInitSeconds 秒）为整点
// 抢兑挂一个精确定时器，到点（如 12:00:00.000）直接派发，不再依赖粗粒度唤醒
// tick 的秒级抖动；兜底 :00 tick 仍可捕获准备之后新建的迟到任务。
func (s *ExchangeScheduler) schedulePreciseFire(now time.Time, hour, minute int) {
	executeTime := now.Add(time.Duration(constants.ExchangePreInitSeconds) * time.Second)
	slotStart := time.Date(
		executeTime.Year(), executeTime.Month(), executeTime.Day(),
		hour, minute, 0, 0, executeTime.Location(),
	)
	delay := time.Until(slotStart)
	// 时钟纠偏：整点以服务端时间为准，EXCHANGE_FIRE_OFFSET 正值=延后发、
	// 负值=提前发（默认 0），用于补偿本机与服务端的时钟偏差。
	delay += exchangeFireOffset()
	// 只在合理窗口内挂精确触发器，过远或已过点交给兜底 tick。
	if delay <= 0 || delay > 2*time.Minute {
		return
	}

	key := fmt.Sprintf("fire:%s:%02d%02d", slotStart.Format("20060102"), hour, minute)
	s.queueMutex.Lock()
	if _, exists := s.scheduledFires[key]; exists {
		s.queueMutex.Unlock()
		return
	}
	timer := time.AfterFunc(delay, func() {
		s.queueMutex.Lock()
		delete(s.scheduledFires, key)
		s.queueMutex.Unlock()
		if s.isStopped() {
			return
		}
		log.Printf("【抢兑调度器】精确触发 %02d:%02d 抢兑", hour, minute)
		s.executeExchangeByTime(hour, minute)
	})
	s.scheduledFires[key] = timer
	s.queueMutex.Unlock()
}

// stopScheduledFires 停止并清空所有已挂起的精确触发定时器。
func (s *ExchangeScheduler) stopScheduledFires() {
	s.queueMutex.Lock()
	defer s.queueMutex.Unlock()
	for key, timer := range s.scheduledFires {
		if timer != nil {
			timer.Stop()
		}
		delete(s.scheduledFires, key)
	}
}

// exchangeFireOffset 读取整点精确触发的时钟纠偏量（EXCHANGE_FIRE_OFFSET）。
// 用 time.ParseDuration 解析以支持负值（envutil.Duration 只接受正数），
// 限制在 ±5s 防止误配。
func exchangeFireOffset() time.Duration {
	raw := strings.TrimSpace(os.Getenv("EXCHANGE_FIRE_OFFSET"))
	if raw == "" {
		return 0
	}
	offset, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}
	const cap = 5 * time.Second
	if offset > cap {
		return cap
	}
	if offset < -cap {
		return -cap
	}
	return offset
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
	tasks, skipped, err := s.exchangeTaskRepo.GetTasksByTimeAtWithSkips(hour, minute, time.Now())
	if err != nil {
		log.Printf("【抢兑调度器】获取 %s 抢兑任务失败: %v", slot, err)
		return
	}
	s.reportScheduleSkips(slot, skipped)
	s.reportScheduleMatched(slot, tasks)

	log.Printf("【抢兑调度器】查询 %s 找到 %d 个可执行任务，策略跳过 %d 个任务", slot, len(tasks), len(skipped))

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
	s.preparedQueue = mergeExchangeTasks(s.preparedQueue, tasks)
	s.queueMutex.Unlock()

	log.Printf("【抢兑调度器】%s 抢兑队列已准备，共 %d 个任务", slot, len(tasks))

	s.broadcast(ws.Message{
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
	now := time.Now()

	s.queueMutex.Lock()
	var tasksToExecute []*models.ExchangeTask
	var remainingTasks []*models.ExchangeTask

	for _, task := range s.preparedQueue {
		if taskMatchesExchangeSlot(task, hour, minute, now) {
			tasksToExecute = append(tasksToExecute, task)
		} else {
			remainingTasks = append(remainingTasks, task)
		}
	}

	s.preparedQueue = remainingTasks
	s.queueMutex.Unlock()

	fromPreparedQueue := len(tasksToExecute) > 0
	if len(tasksToExecute) == 0 {
		var err error
		var skipped []repository.ExchangeTaskScheduleSkip
		tasksToExecute, skipped, err = s.exchangeTaskRepo.GetTasksByTimeAtWithSkips(hour, minute, now)
		if err != nil {
			log.Printf("【抢兑调度器】补查 %s 抢兑任务失败: %v", slot, err)
			return
		}
		s.reportScheduleSkips(slot, skipped)
		s.reportScheduleMatched(slot, tasksToExecute)
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
	s.executionWG.Add(1)
	go func() {
		defer s.executionWG.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("【抢兑调度器】%s 抢兑执行 panic: %v", slot, r)
			}
		}()
		s.executeExchangeWithAutoSwitch(tasksToExecute, slot)
	}()
}

// executeExchangeWithAutoSwitch 执行抢兑（带自动切换账号功能）

func (s *ExchangeScheduler) reportScheduleSkips(slot string, skipped []repository.ExchangeTaskScheduleSkip) {
	for _, item := range skipped {
		cycle := strings.TrimSpace(item.RestockCycle)
		if cycle == "" {
			cycle = "daily"
		}
		policy := strings.TrimSpace(item.CalendarPolicy)
		if policy == "" {
			policy = "all"
		}
		log.Printf("【抢兑调度器】%s 任务 %d 因策略不匹配跳过: %s", slot, item.TaskID, item.Reason)
		if s.metrics != nil {
			s.metrics.IncExchangeScheduleSkip(normalizeScheduleSkipMetricReason(item.Reason), cycle, policy)
		}
	}
}

func (s *ExchangeScheduler) reportScheduleMatched(slot string, tasks []*models.ExchangeTask) {
	if s.metrics == nil {
		return
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		cycle := strings.TrimSpace(task.RestockCycle)
		if cycle == "" {
			cycle = "daily"
		}
		policy := strings.TrimSpace(task.CalendarPolicy)
		if policy == "" {
			policy = "all"
		}
		s.metrics.IncExchangeScheduleMatchedTask(
			slot,
			cycle,
			policy,
			strings.TrimSpace(task.ScheduledExchangeTime) != "" || strings.TrimSpace(task.RestockTimes) != "",
			strings.TrimSpace(task.CustomCron) != "",
		)
	}
}

func normalizeScheduleSkipMetricReason(reason string) string {
	reason = strings.TrimSpace(reason)
	switch {
	case reason == "":
		return "unknown"
	case strings.Contains(reason, "cron"):
		return "cron_mismatch"
	case strings.Contains(reason, "补货周期") || strings.Contains(reason, "仅一次"):
		return "cycle_mismatch"
	case strings.Contains(reason, "日历策略"):
		return "calendar_policy"
	case strings.Contains(reason, "时间"):
		return "time_mismatch"
	default:
		return "other"
	}
}
