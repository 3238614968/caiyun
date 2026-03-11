package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/core/auth"
	"caiyun/internal/handlers"
	"caiyun/internal/middleware"
	"caiyun/internal/monitor"
	"caiyun/internal/repository"
	"caiyun/internal/services"
	"caiyun/internal/ws"
	"caiyun/pkg/database"
	"caiyun/pkg/jwt"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// 标准库 log 默认写 stderr，会导致运行日志全部落到错误日志文件。
	log.SetOutput(os.Stdout)

	// 加载环境变量文件，缺失时继续使用环境变量和默认值。
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件，使用环境变量和默认配置")
	}

	// 初始化 MySQL。
	dbConfig := database.Config{
		Host:     getEnv("DB_HOST", "localhost"),
		Port:     getEnv("DB_PORT", "3306"),
		User:     getEnv("DB_USER", "root"),
		Password: getEnv("DB_PASSWORD", "root123"),
		DBName:   getEnv("DB_NAME", "caiyun"),
	}
	db, err := database.NewMySQL(dbConfig)
	if err != nil {
		log.Printf("数据库连接失败: %v", err)
		os.Exit(1)
	}

	// 初始化 Redis。
	redisConfig := cache.RedisConfig{
		Host:     getEnv("REDIS_HOST", "localhost"),
		Port:     getEnv("REDIS_PORT", "6379"),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	}
	redisCache, err := cache.NewRedisCache(redisConfig)
	if err != nil {
		log.Printf("Redis 连接失败: %v", err)
		os.Exit(1)
	}

	// 初始化认证与仓储依赖。
	jwtSecret := getEnv("JWT_SECRET", "your-secret-key-change-in-production")
	jwtExpiry := 7 * 24 * time.Hour
	jwtManager := jwt.NewManager(jwtSecret)
	authMgr := auth.NewAuth(nil)

	userRepo := repository.NewUserRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	taskLogRepo := repository.NewTaskLogRepository(db)
	cloudStatsRepo := repository.NewCloudStatsRepository(db)
	taskConfigRepo := repository.NewTaskConfigRepository(db)
	productRepo := repository.NewProductRepository(db)
	exchangeAccountRepo := repository.NewExchangeAccountRepository(db)
	exchangeTaskRepo := repository.NewExchangeTaskRepository(db)
	exchangeRecordRepo := repository.NewExchangeRecordRepository(db)
	configRepo := repository.NewSystemConfigRepository(db)
	auditLogRepo := repository.NewAuditLogRepository(db)
	redisStorage := cache.NewRedisStorage(redisCache, "caiyun:task")

	// 同步任务注册表到数据库，便于后续新增任务时自动入库。
	if err := taskConfigRepo.AutoMigrate(); err != nil {
		log.Printf("任务配置表迁移失败: %v", err)
		os.Exit(1)
	}
	if err := taskConfigRepo.SyncDefinitions(services.DefaultTaskConfigs()); err != nil {
		log.Printf("任务配置同步失败: %v", err)
		os.Exit(1)
	}

	// 初始化服务层。
	authService := services.NewAuthService(userRepo, jwtManager, jwtExpiry)
	accountService := services.NewAccountService(accountRepo, userRepo, redisCache, authMgr)
	taskService := services.NewTaskService(accountRepo, taskLogRepo, redisStorage, authMgr, taskConfigRepo)
	cloudService := services.NewCloudService(accountRepo, cloudStatsRepo, taskLogRepo)
	adminService := services.NewAdminService(userRepo, accountRepo, taskLogRepo, taskConfigRepo)
	tokenManager := services.NewTokenManager(accountRepo, exchangeAccountRepo, authMgr)
	exchangeService := services.NewExchangeService(productRepo, exchangeAccountRepo, exchangeTaskRepo, accountRepo, configRepo, exchangeRecordRepo, authMgr, tokenManager)
	productService := services.NewProductService(productRepo, accountRepo)
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
			running := toInt(taskStats["active_tasks"])
			completed := toInt(taskStats["completed_tasks"])
			total := toInt(taskStats["total_tasks"])
			metricsCollector.SetTaskStats(total, 0, running, completed)

			tokenStats := tokenManager.GetTokenStats()
			metricsCollector.SetTokenStats(
				toInt(tokenStats["total"]),
				toInt(tokenStats["healthy"]),
				toInt(tokenStats["error"]),
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
	taskHandler.SetRedisCache(redisCache)
	queueStatusHandler := handlers.NewQueueStatusHandler(redisCache)
	adminHandler := handlers.NewAdminHandler(adminService)
	exchangeHandler := handlers.NewExchangeHandler(exchangeService, productService)
	auditFilter := middleware.NewAuditLogFilter()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.CORSMiddleware())
	// 使用高级限流中间件保护接口。
	r.Use(middleware.AdvancedRateLimitMiddleware(middleware.DefaultRateLimitConfig()))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(metricsCollector.Registry(), promhttp.HandlerOpts{})))

	public := r.Group("/api/auth")
	{
		public.POST("/register", authHandler.Register)
		public.POST("/login", authHandler.Login)
		public.POST("/refresh", authHandler.RefreshToken)
	}

	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddleware(jwtManager))
	{
		protected.GET("/auth/me", authHandler.GetCurrentUser)

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
		}

		// 商品中心路由。
		products := protected.Group("/products")
		{
			products.GET("/search", exchangeHandler.SearchProducts)
			products.GET("/categories", exchangeHandler.GetCategories)
			products.POST("/update", exchangeHandler.UpdateProducts)
		}
	}

	admin := r.Group("/api/admin")
	admin.Use(
		middleware.AuthMiddleware(jwtManager),
		middleware.AdminMiddleware(),
		middleware.AuditMiddlewareWithFilter(auditLogRepo, auditFilter),
	)
	{
		admin.GET("/users", adminHandler.GetAllUsers)
		admin.GET("/accounts", adminHandler.GetAllAccounts)
		admin.GET("/accounts/summaries", adminHandler.GetAccountSummaries)
		admin.GET("/dashboard", adminHandler.GetAdminDashboard)
		admin.PUT("/users/:id/role", adminHandler.UpdateUserRole)
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
	}

	// 初始化 WebSocket Hub 与离线消息存储。
	wsHub := ws.GetHub()
	wsMessageRepo := repository.NewWSMessageRepository(db)
	wsHub.SetWSMessageRepository(wsMessageRepo)

	r.GET("/ws", func(c *gin.Context) {
		token := c.Query("token")
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		wsHub.HandleWebSocket(c.Writer, c.Request, claims.UserID)
	})

	port := getEnv("PORT", "8080")
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
	if err := redisCache.Close(); err != nil {
		log.Printf("关闭 Redis 连接失败: %v", err)
	}
	taskMonitor.Stop()
	log.Println("服务已停止")
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// toInt 将常见数值类型安全转换为 int。
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
