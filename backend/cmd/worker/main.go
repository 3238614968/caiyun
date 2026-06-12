package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/concurrency"
	"caiyun/internal/constants"
	"caiyun/internal/monitor"
	"caiyun/internal/notification"
	"caiyun/internal/queue"
	"caiyun/internal/repository"
	"caiyun/internal/scheduler"
	"caiyun/internal/services"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Worker 任务执行器
type Worker struct {
	accountService *services.AccountService
	taskService    *services.TaskService
	cloudService   *services.CloudService
	taskManager    *concurrency.TaskManager
	taskMonitor    *monitor.TaskMonitor
	jobScheduler   *scheduler.Scheduler
	retryManager   *monitor.RetryManager
	taskQueue      queue.ReliableTaskQueue
	notifier       notification.Notifier
	concurrency    int
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
}

func NewWorker(
	accountService *services.AccountService,
	taskService *services.TaskService,
	cloudService *services.CloudService,
	taskManager *concurrency.TaskManager,
	taskMonitor *monitor.TaskMonitor,
	jobScheduler *scheduler.Scheduler,
	retryManager *monitor.RetryManager,
	taskQueue queue.ReliableTaskQueue,
	notifier notification.Notifier,
	concurrency int,
) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	return &Worker{
		accountService: accountService,
		taskService:    taskService,
		cloudService:   cloudService,
		taskManager:    taskManager,
		taskMonitor:    taskMonitor,
		jobScheduler:   jobScheduler,
		retryManager:   retryManager,
		taskQueue:      taskQueue,
		notifier:       notifier,
		concurrency:    concurrency,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start 启动Worker
func (w *Worker) Start() {
	// 启动任务管理器
	w.taskManager.Start()

	// 启动定时任务调度器
	w.jobScheduler.Start()

	// 启动任务监控器
	w.taskMonitor.StartCleanupJob(30*time.Minute, 24*time.Hour)

	// 启动队列监听器
	w.wg.Add(1)
	go w.queueListener()

	log.Printf("Worker已启动，并发数: %d", w.concurrency)
}

// queueListener 队列监听器
func (w *Worker) queueListener() {
	defer w.wg.Done()

	log.Println("队列监听器已启动")
	workerLimit := w.concurrency
	if workerLimit <= 0 {
		workerLimit = 1
	}
	sem := make(chan struct{}, workerLimit)
	recoverTicker := time.NewTicker(time.Minute)
	delayedTicker := time.NewTicker(10 * time.Second)
	defer recoverTicker.Stop()
	defer delayedTicker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			log.Println("队列监听器已停止")
			return
		case <-recoverTicker.C:
			recovered, err := w.taskQueue.RecoverStaleProcessing(queue.DefaultVisibilityDelay)
			if err != nil {
				log.Printf("恢复超时处理中任务失败: %v", err)
			} else if recovered > 0 {
				log.Printf("已恢复 %d 个超时处理中任务", recovered)
			}
		case <-delayedTicker.C:
			promoted, err := w.taskQueue.PromoteDueDelayed(100)
			if err != nil {
				log.Printf("恢复到期延迟任务失败: %v", err)
			} else if promoted > 0 {
				log.Printf("已恢复 %d 个到期延迟任务", promoted)
			}
		default:
			// 从队列获取任务（阻塞5秒）
			message, err := w.taskQueue.Dequeue(5 * time.Second)
			if err != nil {
				// 超时是正常的，继续循环
				continue
			}

			select {
			case sem <- struct{}{}:
			case <-w.ctx.Done():
				_ = w.taskQueue.Requeue(message)
				return
			}

			w.wg.Add(1)
			go func(msg *queue.TaskMessage) {
				defer w.wg.Done()
				defer func() { <-sem }()
				w.processQueueTask(msg)
			}(message)
		}
	}
}

// processQueueTask 处理队列任务
func (w *Worker) processQueueTask(message *queue.TaskMessage) {
	if message == nil {
		log.Println("收到空队列消息，已忽略")
		return
	}
	log.Printf("从队列获取任务: 账号ID=%d, 任务类型=%s", message.AccountID, message.TaskType)

	// 获取账号信息
	account, err := w.accountService.GetAccountByID(message.AccountID)
	if err != nil {
		log.Printf("获取账号失败: %v", err)
		w.notifier.SendTaskFailure(message.AccountID, message.TaskType, fmt.Sprintf("获取账号失败: %v", err))
		w.handleQueueTaskFailure(message, err)
		return
	}

	// 执行任务：队列消息可以指定 all/all_tasks 或具体任务类型。
	if err := w.ExecuteQueueAccountTask(account.ID, message.TaskType); err != nil {
		log.Printf("任务执行失败: %v", err)
		w.notifier.SendTaskFailure(message.AccountID, message.TaskType, err.Error())
		w.handleQueueTaskFailure(message, err)
	} else {
		log.Printf("任务执行成功: 账号ID=%d", message.AccountID)
		w.notifier.SendTaskSuccess(message.AccountID, message.TaskType, "任务执行成功")
		if err := w.taskQueue.Ack(message); err != nil {
			log.Printf("任务确认失败: account_id=%d task_type=%s err=%v", message.AccountID, message.TaskType, err)
		}
	}
}

func (w *Worker) handleQueueTaskFailure(message *queue.TaskMessage, cause error) {
	if message == nil {
		return
	}
	message.RetryCount++
	if message.RetryCount < queue.DefaultMaxAttempts {
		if err := w.taskQueue.Requeue(message); err != nil {
			log.Printf("任务重新入队失败: account_id=%d task_type=%s retry=%d err=%v", message.AccountID, message.TaskType, message.RetryCount, err)
		} else {
			log.Printf("任务已重新入队: account_id=%d task_type=%s retry=%d/%d", message.AccountID, message.TaskType, message.RetryCount, queue.DefaultMaxAttempts)
		}
		return
	}

	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	if err := w.taskQueue.DeadLetter(message, reason); err != nil {
		log.Printf("任务写入死信队列失败: account_id=%d task_type=%s err=%v", message.AccountID, message.TaskType, err)
		return
	}
	log.Printf("任务已移入死信队列: account_id=%d task_type=%s retries=%d reason=%s", message.AccountID, message.TaskType, message.RetryCount, reason)
}

// Stop 停止Worker
func (w *Worker) Stop() {
	log.Println("正在停止Worker...")
	w.cancel()

	// 停止所有组件
	w.taskManager.Stop()
	w.jobScheduler.Stop()
	w.taskMonitor.Stop()

	w.wg.Wait()
	log.Println("Worker已停止")
}

// ExecuteSingleAccount 执行单个账号的任务
func (w *Worker) ExecuteSingleAccount(accountID uint) error {
	return w.ExecuteQueueAccountTask(accountID, "all_tasks")
}

// ExecuteQueueAccountTask 执行队列指定的账号任务，支持具体任务类型。
func (w *Worker) ExecuteQueueAccountTask(accountID uint, taskType string) error {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		taskType = "all_tasks"
	}

	// 获取账号详情
	account, err := w.accountService.GetAccountByID(accountID)
	if err != nil {
		return err
	}

	// 检查账号是否激活
	if !account.IsActive {
		return fmt.Errorf("账号 %d 未激活", accountID)
	}

	// 使用重试管理器执行任务
	err = w.retryManager.ExecuteWithRetry(
		accountID,
		taskType,
		func() error {
			// 刷新Token（如果需要）
			if err := w.accountService.RefreshTokenIfNeeded(account); err != nil {
				return err
			}

			// 执行队列指定任务；taskType=all/all_tasks 时执行全部批量任务。
			_, err := w.taskService.ExecuteSelectedTaskForAccount(account, taskType)
			return err
		},
		func(progress float64, message string) {
			w.taskMonitor.UpdateTaskProgress(accountID, taskType, progress, message)
		},
	)

	return err
}

// RunAllAccounts 执行所有激活账号的任务
func (w *Worker) RunAllAccounts() error {
	// 获取所有激活账号
	accounts, err := w.accountService.GetAllActiveAccounts()
	if err != nil {
		return err
	}

	if len(accounts) == 0 {
		log.Println("没有激活账号需要执行")
		return nil
	}

	log.Printf("开始执行 %d 个激活账号的任务", len(accounts))

	// 批量提交任务到任务管理器
	if err := w.taskManager.SubmitBatchTasks(accounts); err != nil {
		return err
	}

	// 等待所有任务完成
	w.taskManager.WaitForCompletion()

	log.Println("所有账号任务执行完成")

	// 获取任务统计
	stats := w.taskManager.GetStatus()
	log.Printf("任务执行统计: %+v", stats)

	// 计算每日统计
	if err := w.cloudService.CalculateDailyStats(); err != nil {
		log.Printf("计算每日统计失败: %v", err)
	}

	return nil
}

func main() {
	// 标准库 log 默认写 stderr，会导致运行日志全部落到错误日志文件。
	log.SetOutput(os.Stdout)

	bootstrap.LoadEnvFile()

	core, err := bootstrap.InitCore()
	if err != nil {
		log.Fatal("基础依赖初始化失败:", err)
	}
	defer func() {
		if err := core.Redis.Close(); err != nil {
			log.Printf("关闭 Redis 连接失败: %v", err)
		}
	}()
	repos := core.Repository

	// 初始化Services
	accountService := services.NewAccountService(repos.Account, repos.User, core.Redis, core.Auth, repos.ExchangeAccount)
	taskService := services.NewTaskService(repos.Account, repos.TaskLog, core.TaskStore, core.Auth, repos.TaskConfig, repos.CloudStats)
	cloudService := services.NewCloudService(repos.Account, repos.CloudStats, repos.TaskLog)

	// 获取并发数配置
	concurrencyLimit := 10
	if concurrencyStr := bootstrap.GetEnv("TASK_CONCURRENCY", "10"); concurrencyStr != "" {
		if n, err := strconv.Atoi(concurrencyStr); err == nil && n > 0 {
			concurrencyLimit = n
		}
	}

	// 初始化任务监控器
	taskMonitor := monitor.NewTaskMonitor(monitor.Config{
		MaxHistory: 1000,
		Logger:     log.Default(),
	})

	// 初始化任务管理器
	taskManager := concurrency.NewTaskManager(taskService, concurrencyLimit)

	// 初始化重试管理器
	retryManager := monitor.NewRetryManager(taskMonitor, 3, 5*time.Second)

	// 初始化定时任务调度器
	jobScheduler := scheduler.NewScheduler(scheduler.Config{
		MaxResults: 100,
		Logger:     log.Default(),
	})

	// 初始化 TokenManager
	tokenManager := services.NewTokenManager(repos.Account, repos.ExchangeAccount, core.Auth)
	tokenManager.SetDistributedLockCache(core.Redis)

	// 初始化兑换中心 Service
	exchangeService := services.NewExchangeService(
		repos.Product, repos.ExchangeAccount, repos.ExchangeTask,
		repos.Account, repos.SystemConfig, repos.ExchangeRecord, repos.TaskLog, core.Auth, tokenManager,
	)

	// 初始化抢兑调度器（用于定时抢兑任务）
	exchangeScheduler := services.NewExchangeScheduler(
		repos.ExchangeTask, repos.ExchangeAccount, repos.ExchangeRecord, repos.Product, repos.SystemConfig, repos.TaskLog, tokenManager,
	)
	exchangeScheduler.SetLeaseStore(core.Redis)

	// 启动抢兑调度器
	exchangeScheduler.Start()
	log.Println("【Worker】抢兑调度器已启动")

	// 初始化任务队列
	taskQueue, err := queue.NewConfiguredTaskQueue(core.Redis)
	if err != nil {
		log.Fatalf("初始化任务队列失败: %v", err)
	}
	accountService.SetTaskQueue(taskQueue)
	log.Printf("任务队列后端: %s", queue.TaskQueueBackendFromEnv())

	// 初始化通知服务
	multiNotifier := notification.NewMultiNotifier(log.Default())
	multiNotifier.AddNotifier(notification.NewLogNotifier(log.Default()))
	// 可以添加更多通知器，如WebhookNotifier等

	// 先创建完整 Worker 实例（含 taskManager 等），再注册定时任务，避免定时回调里 taskManager 为 nil
	workerInstance := NewWorker(
		accountService,
		taskService,
		cloudService,
		taskManager,
		taskMonitor,
		jobScheduler,
		retryManager,
		taskQueue,
		multiNotifier,
		concurrencyLimit,
	)

	// 添加定时任务（必须用 workerInstance，否则回调里 taskManager 为 nil 会 panic）
	_, err = jobScheduler.AddJobWithName(
		"daily_task_execution",
		bootstrap.GetEnv("TASK_SCHEDULE", "0 8 * * *"),
		func() error {
			log.Println("定时任务开始执行...")
			return workerInstance.RunAllAccounts()
		},
		"每日定时执行所有账号任务",
	)
	if err != nil {
		log.Printf("添加定时任务失败: %v", err)
	}

	// 添加自动兑换月卡定时任务
	registerMonthlyExchangeJob(jobScheduler, exchangeService, repos.SystemConfig)

	// 添加商品自动更新定时任务
	registerAutoUpdateProductsJob(jobScheduler, exchangeService, repos.SystemConfig, repos.Account)

	// 添加账号健康检查定时任务
	registerAccountHealthCheckJob(jobScheduler, tokenManager, repos.Account, multiNotifier)

	workerInstance.Start()

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动监控API（可选）
	go startMonitoringAPI(workerInstance)

	// 等待信号
	log.Println("Worker服务已启动，按 Ctrl+C 停止")
	<-sigChan

	// 停止服务
	workerInstance.Stop()
	tokenManager.Stop()

	// 停止抢兑调度器
	exchangeScheduler.Stop()
	log.Println("Worker服务已停止")
}

// startMonitoringAPI 启动监控API（可选）
func startMonitoringAPI(worker *Worker) {
	host := bootstrap.GetEnv("WORKER_MONITOR_HOST", "127.0.0.1")
	port := bootstrap.GetEnv("WORKER_MONITOR_PORT", "8081")
	if !isLoopbackHost(host) && !bootstrap.GetBoolEnv("WORKER_MONITOR_ALLOW_PLAINTEXT", false) {
		log.Printf("监控API未启动：WORKER_MONITOR_HOST=%s 非本机地址。若已由 HTTPS 反代保护，请显式设置 WORKER_MONITOR_ALLOW_PLAINTEXT=true", host)
		return
	}
	metricsCollector := monitor.NewMetrics()
	metricsHandler := promhttp.HandlerFor(metricsCollector.Registry(), promhttp.HandlerOpts{})

	updateMetrics := func() {
		taskMonitorStats := worker.taskMonitor.GetStats()
		taskManagerStats := worker.taskManager.GetStatus()

		total := bootstrap.ToInt(taskMonitorStats["total_tasks"])
		running := bootstrap.ToInt(taskMonitorStats["active_tasks"])
		completed := bootstrap.ToInt(taskMonitorStats["completed_tasks"])
		pending := bootstrap.ToInt(taskManagerStats["pending_tasks"])

		metricsCollector.SetTaskStats(total, pending, running, completed)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"time":   time.Now().Format("2006-01-02 15:04:05"),
		})
	})
	mux.HandleFunc("/status", requireMonitorAuth(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"task_monitor": worker.taskMonitor.GetStats(),
			"task_manager": worker.taskManager.GetStatus(),
		})
	}))
	mux.Handle("/metrics", requireMonitorAuth(func(w http.ResponseWriter, r *http.Request) {
		updateMetrics()
		metricsHandler.ServeHTTP(w, r)
	}))

	srv := &http.Server{
		Addr:              net.JoinHostPort(host, port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// 周期性打印摘要，方便不接入监控系统时观察运行状态。
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				updateMetrics()
				log.Printf("任务监控统计: %+v", worker.taskMonitor.GetStats())
				log.Printf("任务管理器状态: %+v", worker.taskManager.GetStatus())
			case <-worker.ctx.Done():
				return
			}
		}
	}()

	go func() {
		<-worker.ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil && err != context.Canceled {
			log.Printf("监控API关闭失败: %v", err)
		}
	}()

	log.Printf("监控API已启动（地址: %s）", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("监控API启动失败: %v", err)
	}
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("监控API响应编码失败: %v", err)
	}
}

func requireMonitorAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(os.Getenv("WORKER_MONITOR_TOKEN"))
		if token == "" {
			http.Error(w, "monitor token is required", http.StatusUnauthorized)
			return
		}

		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == "" {
			provided = r.Header.Get("X-Monitor-Token")
		}
		if provided != token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// registerMonthlyExchangeJob 注册自动兑换月卡定时任务
func registerMonthlyExchangeJob(scheduler *scheduler.Scheduler, exchangeService *services.ExchangeService, configRepo *repository.SystemConfigRepository) {
	// 获取兑换时间配置，默认10:00
	exchangeTime := "10:00"
	if config, err := configRepo.GetByKey("exchange_monthly_time"); err == nil && config.KeyValue != "" {
		exchangeTime = config.KeyValue
	}

	// 解析时间 (HH:MM)
	parts := strings.Split(exchangeTime, ":")
	if len(parts) != 2 {
		log.Printf("【月卡兑换】时间格式错误: %s，使用默认时间 10:00", exchangeTime)
		parts = []string{"10", "00"}
	}
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])

	// 构建 cron 表达式
	cronExpr := fmt.Sprintf("%d %d * * *", minute, hour)

	_, err := scheduler.AddJobWithName(
		"monthly_exchange",
		cronExpr,
		func() error {
			// 检查是否启用自动兑换
			if config, err := configRepo.GetByKey("exchange_monthly_enabled"); err == nil {
				enabled := config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
				if !enabled {
					log.Println("【月卡兑换】自动兑换已禁用，跳过执行")
					return nil
				}
			}

			log.Println("【月卡兑换】开始执行自动兑换月卡任务...")
			exchangeService.ExecuteMonthlyExchange()
			return nil
		},
		"自动兑换月卡任务",
	)
	if err != nil {
		log.Printf("【月卡兑换】添加定时任务失败: %v", err)
	} else {
		log.Printf("【月卡兑换】定时任务已注册，执行时间: %02d:%02d", hour, minute)
	}
}

// registerAutoUpdateProductsJob 注册商品自动更新定时任务
func registerAutoUpdateProductsJob(scheduler *scheduler.Scheduler, exchangeService *services.ExchangeService, configRepo *repository.SystemConfigRepository, accountRepo *repository.AccountRepository) {
	// 每天凌晨3点更新商品
	cronExpr := constants.AutoUpdateProductsCron // 使用常量：每天凌晨3点

	_, err := scheduler.AddJobWithName(
		"auto_update_products",
		cronExpr,
		func() error {
			// 检查是否启用自动更新
			if config, err := configRepo.GetByKey("exchange_auto_update_products"); err == nil {
				enabled := config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
				if !enabled {
					log.Println("【商品更新】自动更新已禁用，跳过执行")
					return nil
				}
			}

			log.Println("【商品更新】开始执行自动更新商品任务...")

			// 获取一个有效的账号作为数据源
			accounts, err := accountRepo.GetAllActive()
			if err != nil || len(accounts) == 0 {
				log.Printf("【商品更新】没有可用的账号，跳过更新: %v", err)
				return nil
			}

			// 使用第一个账号更新商品
			if err := exchangeService.UpdateProducts(accounts[0].ID); err != nil {
				log.Printf("【商品更新】更新商品失败: %v", err)
				return err
			}

			log.Println("【商品更新】商品更新成功")
			return nil
		},
		"自动更新商品列表任务",
	)
	if err != nil {
		log.Printf("【商品更新】添加定时任务失败: %v", err)
	} else {
		log.Println("【商品更新】定时任务已注册，执行时间: 每天 03:00")
	}
}

// registerAccountHealthCheckJob 注册账号健康检查定时任务
func registerAccountHealthCheckJob(scheduler *scheduler.Scheduler, tokenManager *services.TokenManager, accountRepo *repository.AccountRepository, notifier notification.Notifier) {
	// 每6小时检查一次账号健康状态
	cronExpr := constants.AccountHealthCheckCron // 使用常量：每6小时

	_, err := scheduler.AddJobWithName(
		"account_health_check",
		cronExpr,
		func() error {
			log.Println("【账号检测】开始执行账号健康检查...")

			// 获取所有账号
			accounts, err := accountRepo.GetAll()
			if err != nil {
				log.Printf("【账号检测】获取账号列表失败: %v", err)
				return err
			}

			healthyCount := 0
			unhealthyCount := 0

			for _, account := range accounts {
				// 使用 TokenManager 检查账号 Token 状态
				tokenInfo, err := tokenManager.GetToken(account.ID)
				if err != nil {
					log.Printf("【账号检测】账号 %s (ID: %d) Token 获取失败: %v", account.Phone, account.ID, err)
					unhealthyCount++
					// 发送通知
					notifier.SendTaskFailure(account.ID, "health_check", fmt.Sprintf("Token 无效: %v", err))
					continue
				}

				// 检查 Token 健康状态
				if tokenInfo.HealthStatus == "healthy" {
					healthyCount++
					log.Printf("【账号检测】账号 %s (ID: %d) 健康状态良好", account.Phone, account.ID)
				} else {
					unhealthyCount++
					log.Printf("【账号检测】账号 %s (ID: %d) 健康状态异常: %s", account.Phone, account.ID, tokenInfo.ErrorMsg)
					// 发送通知
					notifier.SendTaskFailure(account.ID, "health_check", tokenInfo.ErrorMsg)

					// 尝试强制刷新 Token
					if _, err := tokenManager.ForceRefresh(account.ID); err != nil {
						log.Printf("【账号检测】账号 %s (ID: %d) Token 刷新失败: %v", account.Phone, account.ID, err)
					} else {
						log.Printf("【账号检测】账号 %s (ID: %d) Token 刷新成功", account.Phone, account.ID)
					}
				}
			}

			log.Printf("【账号检测】检查完成，健康账号: %d，异常账号: %d", healthyCount, unhealthyCount)
			return nil
		},
		"账号健康检查任务",
	)
	if err != nil {
		log.Printf("【账号检测】添加定时任务失败: %v", err)
	} else {
		log.Println("【账号检测】定时任务已注册，执行时间: 每6小时")
	}
}
