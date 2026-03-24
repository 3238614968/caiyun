package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/concurrency"
	"caiyun/internal/constants"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/monitor"
	"caiyun/internal/notification"
	"caiyun/internal/queue"
	"caiyun/internal/repository"
	"caiyun/internal/scheduler"
	"caiyun/internal/services"
	"caiyun/pkg/database"

	"github.com/joho/godotenv"
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
	taskQueue      *queue.TaskQueue
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
	taskQueue *queue.TaskQueue,
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

	for {
		select {
		case <-w.ctx.Done():
			log.Println("队列监听器已停止")
			return
		default:
			// 从队列获取任务（阻塞5秒）
			message, err := w.taskQueue.Dequeue(5 * time.Second)
			if err != nil {
				// 超时是正常的，继续循环
				continue
			}

			// 异步处理任务
			go w.processQueueTask(message)
		}
	}
}

// processQueueTask 处理队列任务
func (w *Worker) processQueueTask(message *queue.TaskMessage) {
	log.Printf("从队列获取任务: 账号ID=%d, 任务类型=%s", message.AccountID, message.TaskType)

	// 获取账号信息
	account, err := w.accountService.GetAccountByID(message.AccountID)
	if err != nil {
		log.Printf("获取账号失败: %v", err)
		w.notifier.SendTaskFailure(message.AccountID, message.TaskType, fmt.Sprintf("获取账号失败: %v", err))
		return
	}

	// 执行任务
	if err := w.ExecuteSingleAccount(account.ID); err != nil {
		log.Printf("任务执行失败: %v", err)
		w.notifier.SendTaskFailure(message.AccountID, message.TaskType, err.Error())
	} else {
		log.Printf("任务执行成功: 账号ID=%d", message.AccountID)
		w.notifier.SendTaskSuccess(message.AccountID, message.TaskType, "任务执行成功")
	}
}

// Stop 停止Worker
func (w *Worker) Stop() {
	log.Println("正在停止Worker...")
	w.cancel()

	// 停止所有组件
	w.taskManager.Stop()
	w.jobScheduler.Stop()
	w.taskMonitor.Stop()
	w.cancel() // 停止队列监听器

	w.wg.Wait()
	log.Println("Worker已停止")
}

// ExecuteSingleAccount 执行单个账号的任务
func (w *Worker) ExecuteSingleAccount(accountID uint) error {
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
		"all_tasks",
		func() error {
			// 刷新Token（如果需要）
			if err := w.accountService.RefreshTokenIfNeeded(account); err != nil {
				return err
			}

			// 执行所有任务
			_, err := w.taskService.ExecuteTaskForAccount(account)
			return err
		},
		func(progress float64, message string) {
			w.taskMonitor.UpdateTaskProgress(accountID, "all_tasks", progress, message)
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

// RunScheduledTask 定时任务
func (w *Worker) RunScheduledTask(schedule string) {
	// 解析定时任务表达式
	// 这里使用简单的定时器，实际生产环境可以使用更强大的调度库如 robfig/cron

	// 每天早上8点执行一次
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// 计算第一次执行时间（早上8点）
	now := time.Now()
	firstRun := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())
	if firstRun.Before(now) {
		firstRun = firstRun.Add(24 * time.Hour)
	}

	waitDuration := firstRun.Sub(now)
	log.Printf("首次任务执行时间: %s (等待 %v)", firstRun.Format("2006-01-02 15:04:05"), waitDuration)

	// 等待首次执行
	select {
	case <-time.After(waitDuration):
		// 执行任务
		if err := w.RunAllAccounts(); err != nil {
			log.Printf("定时任务执行失败: %v", err)
		}
	case <-w.ctx.Done():
		return
	}

	// 之后的定时执行
	for {
		select {
		case <-ticker.C:
			if err := w.RunAllAccounts(); err != nil {
				log.Printf("定时任务执行失败: %v", err)
			}
		case <-w.ctx.Done():
			return
		}
	}
}

func getEnv(key, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	return value
}

func main() {
	// 标准库 log 默认写 stderr，会导致运行日志全部落到错误日志文件。
	log.SetOutput(os.Stdout)

	// 加载环境变量
	if err := godotenv.Load(); err != nil {
		log.Println("未找到.env文件，使用默认配置")
	}

	// 连接数据库
	dbConfig := database.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "3306"),
		User:     getEnv("DB_USER", "root"),
		Password: getEnv("DB_PASSWORD", "root123"),
		DBName:   getEnv("DB_NAME", "caiyun"),
	}
	db, err := database.NewMySQL(dbConfig)
	if err != nil {
		log.Fatal("数据库连接失败:", err)
	}

	// 连接Redis
	redisConfig := cache.RedisConfig{
		Host:     getEnv("REDIS_HOST", "localhost"),
		Port:     getEnv("REDIS_PORT", "6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	}
	redisCache, err := cache.NewRedisCache(redisConfig)
	if err != nil {
		log.Fatal("Redis连接失败:", err)
	}

	// 初始化认证管理器
	httpClient := corehttp.NewClient()
	authMgr := auth.NewAuth(httpClient)

	// 初始化Repositories
	accountRepo := repository.NewAccountRepository(db)
	userRepo := repository.NewUserRepository(db)
	taskLogRepo := repository.NewTaskLogRepository(db)
	cloudStatsRepo := repository.NewCloudStatsRepository(db)

	// 创建Redis存储（用于任务存储）
	redisStorage := cache.NewRedisStorage(redisCache, "caiyun:task")

	// 初始化TaskConfig仓库并同步注册表定义
	taskConfigRepo := repository.NewTaskConfigRepository(db)
	schemaRepo := repository.NewSchemaRepository(db)
	if err := schemaRepo.ValidateCriticalSchema(); err != nil {
		log.Fatalf("数据库结构校验失败: %v", err)
	}
	if err := taskConfigRepo.SyncDefinitions(services.DefaultTaskConfigs()); err != nil {
		log.Fatalf("任务配置同步失败: %v", err)
	}

	// 初始化Services
	accountService := services.NewAccountService(accountRepo, userRepo, redisCache, authMgr)
	taskService := services.NewTaskService(accountRepo, taskLogRepo, redisStorage, authMgr, taskConfigRepo, cloudStatsRepo)
	cloudService := services.NewCloudService(accountRepo, cloudStatsRepo, taskLogRepo)

	// 获取并发数配置
	concurrencyLimit := 10
	if concurrencyStr := getEnv("TASK_CONCURRENCY", "10"); concurrencyStr != "" {
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

	// 初始化兑换中心相关 Repository
	productRepo := repository.NewProductRepository(db)
	exchangeAccountRepo := repository.NewExchangeAccountRepository(db)
	exchangeTaskRepo := repository.NewExchangeTaskRepository(db)
	exchangeRecordRepo := repository.NewExchangeRecordRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)

	// 初始化 TokenManager
	tokenManager := services.NewTokenManager(accountRepo, exchangeAccountRepo, authMgr)

	// 初始化兑换中心 Service
	exchangeService := services.NewExchangeService(
		productRepo, exchangeAccountRepo, exchangeTaskRepo,
		accountRepo, configRepo, exchangeRecordRepo, taskLogRepo, authMgr, tokenManager,
	)

	// 初始化抢兑调度器（用于定时抢兑任务）
	exchangeScheduler := services.NewExchangeScheduler(
		exchangeTaskRepo, exchangeAccountRepo, exchangeRecordRepo, configRepo, taskLogRepo, tokenManager,
	)

	// 启动抢兑调度器
	exchangeScheduler.Start()
	log.Println("【Worker】抢兑调度器已启动")

	// 初始化任务队列
	taskQueue := queue.NewTaskQueue(redisCache)

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
		getEnv("TASK_SCHEDULE", "0 8 * * *"),
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
	registerMonthlyExchangeJob(jobScheduler, exchangeService, configRepo)

	// 添加商品自动更新定时任务
	registerAutoUpdateProductsJob(jobScheduler, exchangeService, configRepo, accountRepo)

	// 添加账号健康检查定时任务
	registerAccountHealthCheckJob(jobScheduler, tokenManager, accountRepo, multiNotifier)

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

	// 停止抢兑调度器
	exchangeScheduler.Stop()
	log.Println("Worker服务已停止")
}

// startMonitoringAPI 启动监控API（可选）
func startMonitoringAPI(worker *Worker) {
	port := getEnv("WORKER_MONITOR_PORT", "8081")
	metricsCollector := monitor.NewMetrics()
	metricsHandler := promhttp.HandlerFor(metricsCollector.Registry(), promhttp.HandlerOpts{})

	updateMetrics := func() {
		taskMonitorStats := worker.taskMonitor.GetStats()
		taskManagerStats := worker.taskManager.GetStatus()

		total := toInt(taskMonitorStats["total_tasks"])
		running := toInt(taskMonitorStats["active_tasks"])
		completed := toInt(taskMonitorStats["completed_tasks"])
		pending := toInt(taskManagerStats["pending_tasks"])

		metricsCollector.SetTaskStats(total, pending, running, completed)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"time":   time.Now().Format("2006-01-02 15:04:05"),
		})
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"task_monitor": worker.taskMonitor.GetStats(),
			"task_manager": worker.taskManager.GetStatus(),
		})
	})
	mux.Handle("/metrics", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		updateMetrics()
		metricsHandler.ServeHTTP(w, r)
	}))

	srv := &http.Server{
		Addr:              ":" + port,
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

	log.Printf("监控API已启动（端口: %s）", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("监控API启动失败: %v", err)
	}
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("监控API响应编码失败: %v", err)
	}
}

// toInt 将常见数值类型转换为 int。
func toInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
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
