package main

import (
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/concurrency"
	"caiyun/internal/monitor"
	"caiyun/internal/notification"
	"caiyun/internal/scheduler"
	"caiyun/internal/services"
)

func main() {
	bootstrap.LoadEnvFile()
	closeLogger := bootstrap.ConfigureStandardLogger("worker")
	defer closeLogger()

	core, err := bootstrap.InitCore()
	if err != nil {
		log.Fatal("基础依赖初始化失败:", err)
	}
	defer func() {
		if err := core.Close(); err != nil {
			log.Printf("关闭基础依赖失败: %v", err)
		}
	}()
	repos := core.Repository

	// 初始化 API/Worker 共用业务服务，避免两个进程复制依赖装配逻辑。
	sharedServices, err := bootstrap.InitSharedServices(core)
	if err != nil {
		log.Fatalf("初始化共享业务服务失败: %v", err)
	}
	accountService := sharedServices.Account
	taskService := sharedServices.Task
	cloudService := sharedServices.Cloud
	tokenManager := sharedServices.TokenManager
	exchangeService := sharedServices.Exchange
	taskQueue := sharedServices.TaskQueue
	log.Printf("任务队列后端: %s", bootstrap.TaskQueueBackendName())

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
	retryManager := monitor.NewRetryManager(
		taskMonitor,
		bootstrap.GetIntEnv("WORKER_RETRY_MAX_ATTEMPTS", 3),
		bootstrap.GetDurationEnv("WORKER_RETRY_DELAY", 5*time.Second),
	)

	// 初始化定时任务调度器
	jobScheduler := scheduler.NewScheduler(scheduler.Config{
		MaxResults: 100,
		Logger:     log.Default(),
	})

	// 初始化抢兑调度器（用于定时抢兑任务）
	exchangeScheduler := services.NewExchangeScheduler(
		repos.ExchangeTask, repos.ExchangeAccount, repos.ExchangeRecord, repos.Product, repos.SystemConfig, repos.TaskLog, tokenManager,
	)
	exchangeScheduler.SetLeaseStore(core.Redis)

	// 启动抢兑调度器
	exchangeScheduler.Start()
	log.Println("【Worker】抢兑调度器已启动")

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
	sig := <-sigChan
	log.Printf("Worker收到停止信号: %s", sig)

	// 停止服务：先停抢兑调度器，避免 Worker 停止等待期间跨分钟继续触发抢兑。
	exchangeScheduler.Stop()
	workerInstance.Stop()
	tokenManager.Stop()
	log.Println("Worker服务已停止")
}
