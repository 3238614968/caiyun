package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/handlers"
	"caiyun/internal/middleware"
	"caiyun/internal/monitor"
	"caiyun/internal/services"
	"caiyun/internal/ws"
	"caiyun/pkg/jwt"

	"github.com/gin-gonic/gin"
)

func main() {
	bootstrap.LoadEnvFile()
	closeLogger := bootstrap.ConfigureStandardLogger("api")
	defer closeLogger()

	core, err := bootstrap.InitCore()
	if err != nil {
		log.Printf("基础依赖初始化失败: %v", err)
		os.Exit(1)
	}

	// 初始化认证与仓储依赖。
	jwtExpiry := 7 * 24 * time.Hour
	jwtManager := newJWTManagerFromEnv()
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
	authService := services.NewAuthServiceWithPasswordResetCache(repos.User, jwtManager, jwtExpiry, passwordResetConfig, core.Redis)

	sharedServices, err := bootstrap.InitSharedServices(core)
	if err != nil {
		log.Printf("初始化共享业务服务失败: %v", err)
		os.Exit(1)
	}
	accountService := sharedServices.Account
	taskService := sharedServices.Task
	cloudService := sharedServices.Cloud
	tokenManager := sharedServices.TokenManager
	exchangeService := sharedServices.Exchange
	taskQueue := sharedServices.TaskQueue
	log.Printf("任务队列后端: %s", bootstrap.TaskQueueBackendName())

	adminService := services.NewAdminService(repos.User, repos.Account, repos.TaskLog, repos.TaskConfig)
	productService := services.NewProductService(repos.Product, repos.Account)
	announcementService := services.NewAnnouncementService(repos.Announcement)

	// 初始化任务监控器，并注册为 API 进程可见的全局实例。
	taskMonitor := monitor.NewTaskMonitor(monitor.Config{Logger: log.Default(), MaxHistory: 1000})
	taskMonitor.StartCleanupJob(5*time.Minute, 30*time.Minute)
	monitor.SetGlobalTaskMonitor(taskMonitor)
	metricsCollector := monitor.NewMetrics()

	// 定时同步基础监控指标到 Prometheus。
	go func() {
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
	auditFilter := middleware.NewAuditLogFilter()
	// 初始化全局共享的审计 writer（避免每个请求新建 worker goroutine）。
	middleware.InitGlobalAuditWriter(repos.AuditLog)
	defer middleware.StopGlobalAuditWriter()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.MaxMultipartMemory = 8 << 20 // 8 MiB
	r.Use(middleware.RecoveryWithLogger())
	r.Use(middleware.BodySizeLimitMiddleware(10 << 20)) // 10 MiB
	r.Use(middleware.CORSMiddleware())
	// 使用高级限流中间件保护接口；保留实例引用以便优雅关闭。
	rateLimitConfig := middleware.DefaultRateLimitConfig()
	preAuthLimiter := middleware.NewAdvancedRateLimitMiddleware(rateLimitConfig)
	postAuthLimiter := middleware.NewAuthenticatedRateLimitMiddleware(rateLimitConfig)
	preAuthLimiter.SetRedisStore(core.Redis)
	postAuthLimiter.SetRedisStore(core.Redis)
	r.Use(preAuthLimiter.HandlerFunc())
	defer preAuthLimiter.Stop()
	defer postAuthLimiter.Stop()
	// 释放 SMS 限流器后台协程。
	defer accountHandler.Close()

	registerRoutes(r, routeDependencies{
		jwtManager:       jwtManager,
		repos:            repos,
		rateLimitConfig:  rateLimitConfig,
		postAuthRateMw:   postAuthLimiter,
		auditFilter:      auditFilter,
		metricsCollector: metricsCollector,
		handlers: routeHandlers{
			auth:         authHandler,
			account:      accountHandler,
			task:         taskHandler,
			queueStatus:  queueStatusHandler,
			admin:        adminHandler,
			exchange:     exchangeHandler,
			announcement: announcementHandler,
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

	go func() {
		log.Printf("API 服务启动在端口 %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("服务运行失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("API收到停止信号: %s", sig)
	log.Println("正在关闭服务...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("强制关闭服务失败: %v", err)
	}
	tokenManager.Stop()
	ws.GetHub().Stop()
	if err := core.Close(); err != nil {
		log.Printf("关闭基础依赖失败: %v", err)
	}
	taskMonitor.Stop()
	log.Println("服务已停止")
}

func newJWTManagerFromEnv() *jwt.Manager {
	algorithm := strings.ToUpper(strings.TrimSpace(bootstrap.GetEnv("JWT_ALGORITHM", "HS256")))
	switch algorithm {
	case "RS256":
		manager, err := jwt.NewRS256Manager(
			bootstrap.GetSecretEnv("JWT_PRIVATE_KEY", ""),
			bootstrap.GetSecretEnv("JWT_PUBLIC_KEY", ""),
		)
		if err != nil {
			log.Fatalf("初始化 RS256 JWT 失败: %v", err)
		}
		return manager
	case "HS256", "":
		return jwt.NewManager(bootstrap.GetSecretEnv("JWT_SECRET", ""))
	default:
		log.Fatalf("不支持的 JWT_ALGORITHM: %s，仅支持 HS256 或 RS256", algorithm)
		return nil
	}
}
