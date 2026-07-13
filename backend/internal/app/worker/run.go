package worker

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/concurrency"
	"caiyun/internal/monitor"
	"caiyun/internal/notification"
	"caiyun/internal/repository"
	"caiyun/internal/scheduler"
	"caiyun/internal/security"
	"caiyun/internal/services"
	"caiyun/internal/version"
	"caiyun/internal/ws"
)

// Run starts the background worker and blocks until ctx is cancelled.
// Signal handling and process exit codes are owned by cmd.
func Run(ctx context.Context, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	flags := flag.NewFlagSet("caiyun worker", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("worker 不支持位置参数: %s", strings.Join(flags.Args(), " "))
	}

	bootstrap.LoadEnvFile()
	if err := security.ValidateFieldCryptoConfig(); err != nil {
		return fmt.Errorf("数据加密配置校验失败: %w", err)
	}
	closeLogger := bootstrap.ConfigureStandardLogger("worker")
	defer closeLogger()
	log.Printf("启动 caiyun-worker: %+v", version.Get())

	core, err := bootstrap.InitCore()
	if err != nil {
		return fmt.Errorf("基础依赖初始化失败: %w", err)
	}
	defer func() {
		if err := core.Close(); err != nil {
			log.Printf("关闭基础依赖失败: %v", err)
		}
	}()
	repos := core.Repository
	wsHub := ws.GetHub()
	wsHub.SetWSMessageRepository(repos.WSMessage)
	if err := wsHub.ConfigureEventBus(ctx, core.Redis, bootstrap.GetEnv("WS_EVENT_CHANNEL", "caiyun:ws:events"), bootstrap.GetEnv("INSTANCE_ID", "worker")); err != nil {
		return err
	}
	defer wsHub.Stop()

	// 初始化 API/Worker 共用业务服务，避免两个进程复制依赖装配逻辑。
	sharedServices, err := bootstrap.InitSharedServices(core)
	if err != nil {
		return fmt.Errorf("初始化共享业务服务失败: %w", err)
	}
	defer services.StopExchangeRequestControls()
	accountService := sharedServices.Account
	taskService := sharedServices.Task
	cloudService := sharedServices.Cloud
	tokenManager := sharedServices.TokenManager
	defer tokenManager.Stop()
	exchangeService := sharedServices.Exchange
	operationService := sharedServices.Operation
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

	metricsCollector := monitor.NewMetrics()

	// 初始化抢兑调度器（用于定时抢兑任务）
	exchangeScheduler := services.NewExchangeScheduler(
		repos.ExchangeTask, repos.ExchangeAccount, repos.ExchangeRecord, repos.Product, repos.SystemConfig, repos.TaskLog, tokenManager,
	)
	exchangeScheduler.SetLeaseStore(core.Redis)
	exchangeScheduler.SetMetrics(metricsCollector)
	// 初始化通知服务
	multiNotifier := notification.NewMultiNotifier(log.Default())
	multiNotifier.AddNotifier(notification.NewLogNotifier(log.Default()))
	// 可以添加更多通知器，如WebhookNotifier等

	// 先创建完整 Worker 实例（含 taskManager 等），再注册定时任务，避免定时回调里 taskManager 为 nil
	workerInstance := NewWorkerContext(
		ctx,
		accountService,
		taskService,
		cloudService,
		taskManager,
		taskMonitor,
		jobScheduler,
		retryManager,
		taskQueue,
		multiNotifier,
		metricsCollector,
		concurrencyLimit,
	)
	workerInstance.SetOperationServices(operationService, exchangeService)

	// 核心日常任务的配置错误必须阻止 Worker 启动；静默跳过会让进程
	// 看似健康但永远不执行主要业务任务。
	taskSchedule := bootstrap.GetEnv("TASK_SCHEDULE", "0 8 * * *")
	if err := addDailyTaskExecutionJob(jobScheduler, taskSchedule, func() error {
		return runWithWorkerJobLease(core.Redis, "daily_task_execution", 12*time.Hour, func() error {
			log.Println("定时任务开始执行...")
			return workerInstance.RunAllAccounts()
		})
	}); err != nil {
		return err
	}

	// 添加自动兑换月卡定时任务
	registerMonthlyExchangeJob(jobScheduler, exchangeService, repos.SystemConfig)

	// 添加商品自动更新定时任务
	registerAutoUpdateProductsJob(jobScheduler, exchangeService, repos.SystemConfig, repos.Account, core.Redis)

	// 添加账号健康检查定时任务
	registerAccountHealthCheckJob(jobScheduler, tokenManager, repos.Account, multiNotifier, core.Redis)

	// 添加历史日志/抢兑记录归档任务
	historyArchiveService := services.NewHistoryArchiveServiceFromEnv(repository.NewHistoryArchiveRepository(core.DB))
	historyArchiveService.SetLockStore(core.Redis)
	historyArchiveService.SetMetrics(metricsCollector)
	registerHistoryArchiveJob(jobScheduler, historyArchiveService, metricsCollector)

	// 所有可能失败的核心配置均校验完成后，再启动后台组件。
	exchangeScheduler.Start()
	log.Println("【Worker】抢兑调度器已启动")
	workerInstance.Start()
	// defer 按 LIFO 执行：先停抢兑调度器，再停 Worker，最后关闭共享服务/Core。
	defer workerInstance.Stop()
	defer exchangeScheduler.Stop()

	// 监控 API 由 Worker 跟踪，Stop 会取消并等待其 HTTP 服务、ticker
	// 和 shutdown goroutine 全部退出。
	workerInstance.startBackground(func() {
		startMonitoringAPI(workerInstance, core)
	})

	log.Println("Worker 服务已启动")
	<-ctx.Done()
	log.Println("Worker 收到停止请求")
	return nil
}

func addDailyTaskExecutionJob(jobScheduler *scheduler.Scheduler, schedule string, job func() error) error {
	if jobScheduler == nil {
		return fmt.Errorf("注册 daily_task_execution 失败: scheduler 未初始化")
	}
	if job == nil {
		return fmt.Errorf("注册 daily_task_execution 失败: job 不能为空")
	}
	if _, err := jobScheduler.AddJobWithName(
		"daily_task_execution",
		schedule,
		job,
		"每日定时执行所有账号任务",
	); err != nil {
		return fmt.Errorf("TASK_SCHEDULE=%q 无效，Worker 拒绝启动: %w", schedule, err)
	}
	return nil
}
