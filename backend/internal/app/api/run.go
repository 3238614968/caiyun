package api

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/handlers"
	"caiyun/internal/middleware"
	"caiyun/internal/monitor"
	"caiyun/internal/security"
	"caiyun/internal/services"
	"caiyun/internal/version"
	"caiyun/internal/ws"
	"caiyun/pkg/jwt"

	"github.com/gin-gonic/gin"
)

// Run starts the HTTP API and blocks until ctx is cancelled or the server
// stops unexpectedly. Signal handling and exit codes belong to cmd.
func Run(ctx context.Context, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	flags := flag.NewFlagSet("caiyun api", flag.ContinueOnError)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("api 不支持位置参数: %s", strings.Join(flags.Args(), " "))
	}

	bootstrap.LoadEnvFile()
	if err := security.ValidateFieldCryptoConfig(); err != nil {
		return fmt.Errorf("数据加密配置校验失败: %w", err)
	}
	closeLogger := bootstrap.ConfigureStandardLogger("api")
	defer closeLogger()
	log.Printf("启动 caiyun-api: %+v", version.Get())

	core, err := bootstrap.InitCore()
	if err != nil {
		return fmt.Errorf("基础依赖初始化失败: %w", err)
	}
	defer func() {
		if err := core.Close(); err != nil {
			log.Printf("关闭基础依赖失败: %v", err)
		}
	}()

	// Access JWT 保持短时效；长期登录由可撤销、每次轮换的 refresh session 提供。
	jwtExpiry := bootstrap.GetDurationEnv("JWT_ACCESS_TTL", 15*time.Minute)
	refreshExpiry := bootstrap.GetDurationEnv("JWT_REFRESH_TTL", 30*24*time.Hour)
	if jwtExpiry > time.Hour || refreshExpiry <= jwtExpiry {
		return fmt.Errorf("JWT TTL 配置无效：JWT_ACCESS_TTL 必须不超过 1h，JWT_REFRESH_TTL 必须大于 access TTL")
	}
	jwtManager, err := newJWTManagerFromEnv()
	if err != nil {
		return err
	}
	repos := core.Repository

	// 初始化服务层。
	passwordResetConfig := services.PasswordResetConfig{
		SMTP: services.SMTPConfig{
			Host:     bootstrap.GetEnv("SMTP_HOST", ""),
			Port:     bootstrap.GetEnv("SMTP_PORT", "587"),
			Username: bootstrap.GetEnv("SMTP_USERNAME", ""),
			Password: bootstrap.GetEnv("SMTP_PASSWORD", ""),
			From:     bootstrap.GetEnv("SMTP_FROM", ""),
			FromName: bootstrap.GetEnv("SMTP_FROM_NAME", "移动云盘"),
			UseTLS:   bootstrap.GetBoolEnv("SMTP_USE_TLS", false),
		},
	}
	authService := services.NewAuthServiceWithPasswordResetCache(
		repos.User,
		jwtManager,
		jwtExpiry,
		passwordResetConfig,
		core.Redis,
		services.WithRefreshSessionRepository(repos.RefreshSession, refreshExpiry),
	)

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

	adminService := services.NewAdminService(repos.User, repos.Account, repos.TaskLog, repos.TaskConfig)
	productService := services.NewProductService(repos.Product, repos.Account)
	announcementService := services.NewAnnouncementService(repos.Announcement)

	// 初始化任务监控器，并注册为 API 进程可见的全局实例。
	taskMonitor := monitor.NewTaskMonitor(monitor.Config{Logger: log.Default(), MaxHistory: 1000})
	taskMonitor.StartCleanupJob(5*time.Minute, 30*time.Minute)
	defer taskMonitor.Stop()
	monitor.SetGlobalTaskMonitor(taskMonitor)
	metricsCollector := monitor.NewMetrics()

	// 定时同步基础监控指标到 Prometheus。
	metricsCtx, stopMetrics := context.WithCancel(ctx)
	metricsDone := make(chan struct{})
	defer func() {
		// An unexpected ListenAndServe failure does not cancel the parent ctx.
		// Explicitly cancel and join before repositories/Core are closed.
		stopMetrics()
		<-metricsDone
	}()
	go func() {
		defer close(metricsDone)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		subCh := taskMonitor.Subscribe()
		defer taskMonitor.Unsubscribe(subCh)

		updateMetrics := func() {
			taskStats := taskMonitor.GetStats()
			running := bootstrap.ToInt(taskStats["active_tasks"])
			completed := bootstrap.ToInt(taskStats["completed_tasks"])
			total := bootstrap.ToInt(taskStats["total_tasks"])
			metricsCollector.SetTaskStats(total, 0, running, completed)

			if taskQueue != nil {
				pending, _ := taskQueue.GetQueueLength()
				processing, _ := taskQueue.GetProcessingLength()
				delayed, _ := taskQueue.GetDelayedLength()
				dead, _ := taskQueue.GetDeadLetterLength()
				metricsCollector.SetQueueStats(pending, processing, delayed, dead)
			}

			windowEnd := time.Now()
			windowStart := windowEnd.Add(-24 * time.Hour)
			if success, failed, err := exchangeService.GetRecordStats(0, windowStart, windowEnd); err == nil {
				failureReasons := map[string]int64{}
				if reasons, err := exchangeService.GetFailureReasonStats(0, windowStart, windowEnd, 10); err == nil {
					failureReasons = reasons
				}
				metricsCollector.SetExchangeRecentStats(success, failed, failureReasons)
			}

			tokenStats := tokenManager.GetTokenStats()
			metricsCollector.SetTokenStats(
				bootstrap.ToInt(tokenStats["total"]),
				bootstrap.ToInt(tokenStats["healthy"]),
				bootstrap.ToInt(tokenStats["error"]),
			)
			metricsCollector.SetAuditDropped(middleware.AuditDroppedCount())
		}

		updateMetrics()
		for {
			select {
			case <-metricsCtx.Done():
				return
			case <-ticker.C:
				updateMetrics()
			case _, ok := <-subCh:
				if !ok {
					return
				}
				// 收到任务状态变更后立即刷新一次指标。
				updateMetrics()
			}
		}
	}()

	// 初始化处理器。
	authHandler := handlers.NewAuthHandler(authService, jwtManager)
	accountHandler := handlers.NewAccountHandler(accountService, taskService, core.Redis)
	taskHandler := handlers.NewTaskHandler(taskService, cloudService, accountService)
	taskHandler.SetRedisCache(core.Redis)
	queueStatusHandler := handlers.NewQueueStatusHandler(taskQueue)
	adminHandler := handlers.NewAdminHandler(adminService)
	exchangeHandler := handlers.NewExchangeHandler(exchangeService, productService)
	announcementHandler := handlers.NewAnnouncementHandler(announcementService)
	operationHandler := handlers.NewOperationHandler(operationService)
	accountHandler.SetOperationService(operationService)
	taskHandler.SetOperationService(operationService)
	exchangeHandler.SetOperationService(operationService)
	auditFilter := middleware.NewAuditLogFilter()
	// 初始化全局共享的审计 writer（避免每个请求新建 worker goroutine）。
	middleware.SetAuditDroppedMetrics(metricsCollector)
	defer middleware.SetAuditDroppedMetrics(nil)
	middleware.InitGlobalAuditWriter(repos.AuditLog)
	defer middleware.StopGlobalAuditWriter()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	if err := configureTrustedProxies(r, bootstrap.GetEnv("TRUSTED_PROXIES", "")); err != nil {
		return err
	}
	r.MaxMultipartMemory = 8 << 20 // 8 MiB
	r.Use(middleware.RequestIDMiddleware())
	r.Use(middleware.RecoveryWithLogger())
	requestTimeout := bootstrap.GetDurationEnv("REQUEST_TIMEOUT", 30*time.Second)
	r.Use(middleware.TimeoutMiddleware(requestTimeout))
	r.Use(middleware.BodySizeLimitMiddleware(10 << 20)) // 10 MiB
	r.Use(middleware.HTTPMetricsMiddleware(metricsCollector))
	r.Use(middleware.CORSMiddleware())
	// 使用高级限流中间件保护接口；保留实例引用以便优雅关闭。
	rateLimitConfig := middleware.DefaultRateLimitConfig()
	if err := middleware.ValidateRateLimitConfig(rateLimitConfig); err != nil {
		return fmt.Errorf("限流配置无效: %w", err)
	}
	preAuthLimiter := middleware.NewAdvancedRateLimitMiddleware(rateLimitConfig)
	postAuthLimiter := middleware.NewAuthenticatedRateLimitMiddleware(rateLimitConfig)
	preAuthLimiter.SetRedisStore(core.Redis)
	postAuthLimiter.SetRedisStore(core.Redis)
	preAuthLimiter.SetMetrics(metricsCollector)
	postAuthLimiter.SetMetrics(metricsCollector)
	r.Use(preAuthLimiter.HandlerFunc())
	defer preAuthLimiter.Stop()
	defer postAuthLimiter.Stop()
	// 释放 SMS 限流器后台协程。
	defer accountHandler.Close()

	// registerRoutes exposes the global WebSocket hub. Own its lifecycle here so
	// bind failures, graceful shutdown errors and normal exits all stop the hub
	// before repositories and the database are closed.
	wsHub := ws.GetHub()
	wsHub.SetWSMessageRepository(repos.WSMessage)
	if err := wsHub.ConfigureEventBus(ctx, core.Redis, bootstrap.GetEnv("WS_EVENT_CHANNEL", "caiyun:ws:events"), bootstrap.GetEnv("INSTANCE_ID", "api")); err != nil {
		return err
	}
	defer wsHub.Stop()

	registerRoutes(r, routeDependencies{
		jwtManager:       jwtManager,
		repos:            repos,
		rateLimitConfig:  rateLimitConfig,
		postAuthRateMw:   postAuthLimiter,
		auditFilter:      auditFilter,
		metricsCollector: metricsCollector,
		readinessCheck:   coreReadinessCheck(core, taskQueue),
		handlers: routeHandlers{
			auth:         authHandler,
			account:      accountHandler,
			task:         taskHandler,
			queueStatus:  queueStatusHandler,
			admin:        adminHandler,
			exchange:     exchangeHandler,
			announcement: announcementHandler,
			operation:    operationHandler,
		},
	})

	port := bootstrap.GetEnv("PORT", "8080")
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("API 服务启动在端口 %s", port)
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErr <- err
	}()

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("API 服务运行失败: %w", err)
		}
		return nil
	case <-ctx.Done():
		log.Println("正在关闭 API 服务...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("关闭 API 服务失败: %w", err)
	}
	// HTTP 已停止接收请求。后续 defer 会依次停止 WebSocket Hub、
	// drain 审计 writer，再关闭 Core/DB。
	log.Println("API 服务已停止")
	return nil
}

func newJWTManagerFromEnv() (*jwt.Manager, error) {
	algorithm := strings.ToUpper(strings.TrimSpace(bootstrap.GetEnv("JWT_ALGORITHM", "HS256")))
	var manager *jwt.Manager
	switch algorithm {
	case "RS256":
		var err error
		manager, err = jwt.NewRS256Manager(
			bootstrap.GetSecretEnv("JWT_PRIVATE_KEY", ""),
			bootstrap.GetSecretEnv("JWT_PUBLIC_KEY", ""),
		)
		if err != nil {
			return nil, fmt.Errorf("初始化 RS256 JWT 失败: %w", err)
		}
	case "HS256", "":
		manager = jwt.NewManager(bootstrap.GetSecretEnv("JWT_SECRET", ""))
	default:
		return nil, fmt.Errorf("不支持的 JWT_ALGORITHM: %s，仅支持 HS256 或 RS256", algorithm)
	}
	return manager.SetIssuerAudience(
		bootstrap.GetEnv("JWT_ISSUER", "caiyun-api"),
		bootstrap.GetEnv("JWT_AUDIENCE", "caiyun-web"),
	), nil
}
