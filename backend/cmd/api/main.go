package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/handlers"
	"caiyun/internal/middleware"
	"caiyun/internal/monitor"
	"caiyun/internal/queue"
	"caiyun/internal/services"
	"caiyun/internal/ws"
	"caiyun/pkg/jwt"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// 标准库 log 默认写 stderr，会导致运行日志全部落到错误日志文件。
	log.SetOutput(os.Stdout)

	bootstrap.LoadEnvFile()

	core, err := bootstrap.InitCore()
	if err != nil {
		log.Printf("基础依赖初始化失败: %v", err)
		os.Exit(1)
	}

	// 初始化认证与仓储依赖。
	jwtSecret := bootstrap.GetSecretEnv("JWT_SECRET", "your-secret-key-change-in-production")
	jwtExpiry := 7 * 24 * time.Hour
	jwtManager := jwt.NewManager(jwtSecret)
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
	accountService := services.NewAccountService(repos.Account, repos.User, core.Redis, core.Auth, repos.ExchangeAccount)
	taskQueue, err := queue.NewConfiguredTaskQueue(core.Redis)
	if err != nil {
		log.Printf("初始化任务队列失败: %v", err)
		os.Exit(1)
	}
	accountService.SetTaskQueue(taskQueue)
	log.Printf("任务队列后端: %s", queue.TaskQueueBackendFromEnv())
	taskService := services.NewTaskService(repos.Account, repos.TaskLog, core.TaskStore, core.Auth, repos.TaskConfig, repos.CloudStats)
	cloudService := services.NewCloudService(repos.Account, repos.CloudStats, repos.TaskLog)
	adminService := services.NewAdminService(repos.User, repos.Account, repos.TaskLog, repos.TaskConfig)
	tokenManager := services.NewTokenManager(repos.Account, repos.ExchangeAccount, core.Auth)
	tokenManager.SetDistributedLockCache(core.Redis)
	exchangeService := services.NewExchangeService(repos.Product, repos.ExchangeAccount, repos.ExchangeTask, repos.Account, repos.SystemConfig, repos.ExchangeRecord, repos.TaskLog, core.Auth, tokenManager)
	productService := services.NewProductService(repos.Product, repos.Account)
	announcementService := services.NewAnnouncementService(repos.Announcement)
	taskService.SetTokenManager(tokenManager)

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
	accountHandler := handlers.NewAccountHandler(accountService, taskService)
	taskHandler := handlers.NewTaskHandler(taskService, cloudService, accountService)
	taskHandler.SetRedisCache(core.Redis)
	queueStatusHandler := handlers.NewQueueStatusHandler(taskQueue)
	adminHandler := handlers.NewAdminHandler(adminService)
	exchangeHandler := handlers.NewExchangeHandler(exchangeService, productService)
	announcementHandler := handlers.NewAnnouncementHandler(announcementService)
	auditFilter := middleware.NewAuditLogFilter()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.MaxMultipartMemory = 8 << 20 // 8 MiB
	r.Use(gin.Recovery())
	r.Use(middleware.BodySizeLimitMiddleware(10 << 20)) // 10 MiB
	r.Use(middleware.CORSMiddleware())
	// 使用高级限流中间件保护接口。
	rateLimitConfig := middleware.DefaultRateLimitConfig()
	r.Use(middleware.AdvancedRateLimitMiddleware(rateLimitConfig))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", middleware.AuthMiddlewareWithUser(jwtManager, repos.User), middleware.AdminMiddleware(), gin.WrapH(promhttp.HandlerFor(metricsCollector.Registry(), promhttp.HandlerOpts{})))

	public := r.Group("/api/auth")
	{
		public.POST("/register", authHandler.Register)
		public.POST("/login", authHandler.Login)
		public.POST("/password/reset-code/send", authHandler.SendPasswordResetCode)
		public.POST("/password/reset", authHandler.ResetPassword)
	}

	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddlewareWithUser(jwtManager, repos.User))
	protected.Use(middleware.CSRFMiddleware())
	protected.Use(middleware.AuthenticatedRateLimitMiddleware(rateLimitConfig))
	{
		protected.GET("/auth/me", authHandler.GetCurrentUser)
		protected.POST("/auth/refresh", authHandler.RefreshToken)
		protected.POST("/auth/logout", authHandler.Logout)

		accounts := protected.Group("/accounts")
		{
			accounts.GET("", accountHandler.ListAccounts)
			accounts.POST("", accountHandler.CreateAccount)
			accounts.POST("/sms/send", accountHandler.SendSmsCode)
			accounts.GET("/sms/status/:phone", accountHandler.GetSmsStatus)
			accounts.POST("/sms/verify", accountHandler.SmsLogin)
			accounts.GET("/:id", accountHandler.GetAccount)
			accounts.PUT("/:id", accountHandler.UpdateAccount)
			accounts.DELETE("/:id", accountHandler.DeleteAccount)
			accounts.PUT("/:id/status", accountHandler.SetAccountStatus)
			accounts.POST("/:id/refresh", accountHandler.RefreshToken)
			accounts.POST("/:id/trigger", accountHandler.TriggerTask)
		}

		tasks := protected.Group("/tasks")
		{
			tasks.GET("/logs", taskHandler.GetTaskLogs)
			tasks.POST("/trigger-all", taskHandler.TriggerAllTasks)
			tasks.GET("/queue-status", queueStatusHandler.GetQueueStatus)
			tasks.GET("/status", taskHandler.GetTaskStatus)
		}

		stats := protected.Group("/stats")
		{
			stats.GET("/dashboard", taskHandler.GetDashboard)
			stats.GET("/cloud", taskHandler.GetCloudStats)
			stats.GET("/trend", taskHandler.GetTrendData)
			stats.POST("/calculate", taskHandler.CalculateStats)
			stats.GET("/total-cloud", taskHandler.GetTotalCloudCount)
		}

		// 兑换中心路由。
		exchange := protected.Group("/exchange")
		{
			exchange.POST("/accounts", exchangeHandler.AddExchangeAccount)
			exchange.GET("/accounts", exchangeHandler.GetExchangeAccounts)
			exchange.PUT("/accounts/:id", exchangeHandler.UpdateExchangeAccount)
			exchange.DELETE("/accounts/:id", exchangeHandler.DeleteExchangeAccount)
			exchange.POST("/tasks", exchangeHandler.CreateExchangeTask)
			exchange.GET("/tasks", exchangeHandler.GetExchangeTasks)
			exchange.PUT("/tasks/:id", exchangeHandler.UpdateExchangeTask)
			exchange.DELETE("/tasks/:id", exchangeHandler.DeleteExchangeTask)
			exchange.POST("/tasks/:id/execute", exchangeHandler.ExecuteExchangeTask)
			exchange.POST("/tasks/batch-execute", exchangeHandler.BatchExecuteExchangeTasks)
			exchange.GET("/records", exchangeHandler.GetExchangeRecords)
			exchange.GET("/records/export", exchangeHandler.ExportExchangeRecords)
			exchange.POST("/immediate", exchangeHandler.ImmediateExchange)
		}

		// 商品中心路由。
		products := protected.Group("/products")
		{
			products.GET("/search", exchangeHandler.SearchProducts)
			products.GET("/categories", exchangeHandler.GetCategories)
			products.POST("/update", exchangeHandler.UpdateProducts)
		}

		// 公告路由（公开）
		protected.GET("/announcements", announcementHandler.GetPublishedAnnouncements)
		protected.GET("/announcements/popup", announcementHandler.GetPopupAnnouncement)

		// 抢兑配置（公开，普通用户可访问）
		protected.GET("/exchange/config", exchangeHandler.GetExchangeConfigPublic)
	}

	admin := r.Group("/api/admin")
	admin.Use(
		middleware.AuthMiddlewareWithUser(jwtManager, repos.User),
		middleware.CSRFMiddleware(),
		middleware.AuthenticatedRateLimitMiddleware(rateLimitConfig),
		middleware.AdminMiddleware(),
		middleware.AuditMiddlewareWithFilter(repos.AuditLog, auditFilter),
	)
	{
		admin.GET("/users", adminHandler.GetAllUsers)
		admin.GET("/accounts", adminHandler.GetAllAccounts)
		admin.GET("/accounts/search", adminHandler.SearchAllAccounts)
		admin.GET("/accounts/summaries", adminHandler.GetAccountSummaries)
		admin.GET("/dashboard", adminHandler.GetAdminDashboard)
		admin.PUT("/users/:id/role", adminHandler.UpdateUserRole)
		admin.PUT("/users/:id/password", adminHandler.ResetUserPassword)
		admin.PUT("/accounts/:id/status", adminHandler.UpdateAccountStatus)
		admin.DELETE("/users/:id", adminHandler.DeleteUser)
		admin.DELETE("/accounts/:id", adminHandler.DeleteAccount)
		admin.GET("/stats/overview", adminHandler.GetStatsOverview)
		admin.GET("/task-configs", adminHandler.GetTaskConfigs)
		admin.PUT("/task-configs/:task_type", adminHandler.UpdateTaskConfig)

		// 抢兑配置管理。
		admin.GET("/exchange/config", exchangeHandler.GetExchangeConfig)
		admin.PUT("/exchange/config", exchangeHandler.UpdateExchangeConfig)
		admin.POST("/exchange/execute-monthly", exchangeHandler.ExecuteMonthlyExchange)

		// 公告管理。
		admin.GET("/announcements", announcementHandler.GetAllAnnouncements)
		admin.POST("/announcements", announcementHandler.CreateAnnouncement)
		admin.PUT("/announcements/:id", announcementHandler.UpdateAnnouncement)
		admin.DELETE("/announcements/:id", announcementHandler.DeleteAnnouncement)
		admin.GET("/announcements/:id", announcementHandler.GetAnnouncement)
	}

	// 初始化 WebSocket Hub 与离线消息存储。
	wsHub := ws.GetHub()
	wsHub.SetWSMessageRepository(repos.WSMessage)

	r.GET("/ws", func(c *gin.Context) {
		token := ""
		if cookieToken, err := c.Cookie("auth_token"); err == nil {
			token = cookieToken
		}
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		user, err := repos.User.FindByID(claims.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			return
		}
		if claims.TokenVersion != user.TokenVersion {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session revoked"})
			return
		}

		wsHub.HandleWebSocket(c.Writer, c.Request, user.ID)
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
	<-quit
	log.Println("正在关闭服务...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("强制关闭服务失败: %v", err)
	}
	tokenManager.Stop()
	if err := core.Redis.Close(); err != nil {
		log.Printf("关闭 Redis 连接失败: %v", err)
	}
	taskMonitor.Stop()
	log.Println("服务已停止")
}
