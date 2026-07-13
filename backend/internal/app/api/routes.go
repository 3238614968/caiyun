package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/handlers"
	"caiyun/internal/middleware"
	"caiyun/internal/monitor"
	"caiyun/internal/version"
	"caiyun/internal/ws"
	"caiyun/pkg/jwt"
	apiresponse "caiyun/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type routeHandlers struct {
	auth         *handlers.AuthHandler
	account      *handlers.AccountHandler
	task         *handlers.TaskHandler
	queueStatus  *handlers.QueueStatusHandler
	admin        *handlers.AdminHandler
	exchange     *handlers.ExchangeHandler
	announcement *handlers.AnnouncementHandler
	operation    *handlers.OperationHandler
}

type routeDependencies struct {
	jwtManager       *jwt.Manager
	repos            bootstrap.Repositories
	rateLimitConfig  *middleware.RateLimitConfig
	postAuthRateMw   *middleware.RateLimitMiddlewareInstance
	auditFilter      *middleware.AuditLogFilter
	metricsCollector *monitor.Metrics
	readinessCheck   func(context.Context) error
	handlers         routeHandlers
}

func registerRoutes(r *gin.Engine, deps routeDependencies) {
	registerHealthAndMetricsRoutes(r, deps)
	registerAuthRoutes(r, deps.handlers.auth)
	registerProtectedRoutes(r, deps)
	registerAdminRoutes(r, deps)
	registerWebSocketRoute(r, deps)
}

func registerHealthAndMetricsRoutes(r *gin.Engine, deps routeDependencies) {
	liveHandler := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "caiyun-api",
			"version": version.Get(),
		})
	}
	readyHandler := func(c *gin.Context) {
		if deps.readinessCheck != nil {
			if err := deps.readinessCheck(c.Request.Context()); err != nil {
				log.Printf("API readiness check failed: %v", err)
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status":  "not_ready",
					"service": "caiyun-api",
				})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  "ready",
			"service": "caiyun-api",
			"version": version.Get(),
		})
	}
	// 根路径和 /livez 仅反映进程存活；/readyz 与兼容 /health 校验外部依赖。
	r.GET("/", liveHandler)
	r.GET("/livez", liveHandler)
	r.GET("/startupz", liveHandler)
	r.GET("/readyz", readyHandler)
	r.GET("/health", readyHandler)
	r.GET(
		"/metrics",
		middleware.OptionalAuthMiddlewareWithUserAndSession(deps.jwtManager, deps.repos.User, deps.repos.RefreshSession),
		middleware.MonitorOrAdminMiddleware(),
		gin.WrapH(promhttp.HandlerFor(deps.metricsCollector.Registry(), promhttp.HandlerOpts{})),
	)
}

func registerAuthRoutes(r *gin.Engine, authHandler *handlers.AuthHandler) {
	registerAuthRoutesAt(r, "/api/auth", authHandler)
	// 企业级 API 版本化入口：新增 /api/v1/auth，旧 /api/auth 继续兼容。
	registerAuthRoutesAt(r, "/api/v1/auth", authHandler)
}

func registerAuthRoutesAt(r *gin.Engine, basePath string, authHandler *handlers.AuthHandler) {
	public := r.Group(basePath)
	{
		public.POST("/register", authHandler.Register)
		public.POST("/login", authHandler.Login)
		public.POST("/refresh", authHandler.RefreshToken)
		public.POST("/password/reset-code/send", authHandler.SendPasswordResetCode)
		public.POST("/password/reset", authHandler.ResetPassword)
	}
}

func registerProtectedRoutes(r *gin.Engine, deps routeDependencies) {
	registerProtectedRoutesAt(r, "/api", deps)
	// 企业级 API 版本化入口：保留旧 /api 路由兼容现有前端，同时新增 /api/v1 供新客户端和 OpenAPI 契约使用。
	registerProtectedRoutesAt(r, "/api/v1", deps)
}

func registerProtectedRoutesAt(r *gin.Engine, basePath string, deps routeDependencies) {
	h := deps.handlers
	protected := r.Group(basePath)
	protected.Use(middleware.AuthMiddlewareWithUserAndSession(deps.jwtManager, deps.repos.User, deps.repos.RefreshSession))
	protected.Use(middleware.CSRFMiddleware())
	protected.Use(deps.postAuthRateMw.HandlerFunc())
	protected.Use(middleware.AuditMiddlewareWithFilter(deps.repos.AuditLog, deps.auditFilter))
	{
		protected.GET("/auth/me", h.auth.GetCurrentUser)
		protected.POST("/auth/logout", h.auth.Logout)
		protected.POST("/auth/logout-all", h.auth.LogoutAll)

		registerAccountRoutes(protected, h.account)
		registerTaskRoutes(protected, h.task, h.queueStatus)
		registerStatsRoutes(protected, h.task)
		registerExchangeRoutes(protected, h.exchange)
		registerProductRoutes(protected, h.exchange)
		protected.GET("/operations/:id", h.operation.Get)
		protected.POST("/operations/:id/cancel", h.operation.Cancel)

		// 公告路由（公开）
		protected.GET("/announcements", h.announcement.GetPublishedAnnouncements)
		protected.GET("/announcements/popup", h.announcement.GetPopupAnnouncement)

		// 抢兑配置（公开，普通用户可访问）
		protected.GET("/exchange/config", h.exchange.GetExchangeConfigPublic)
	}
}

func registerAccountRoutes(parent *gin.RouterGroup, accountHandler *handlers.AccountHandler) {
	accounts := parent.Group("/accounts")
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
}

func registerTaskRoutes(parent *gin.RouterGroup, taskHandler *handlers.TaskHandler, queueStatusHandler *handlers.QueueStatusHandler) {
	tasks := parent.Group("/tasks")
	{
		tasks.GET("/logs", taskHandler.GetTaskLogs)
		tasks.POST("/trigger-all", taskHandler.TriggerAllTasks)
		tasks.GET("/queue-status", middleware.AdminMiddleware(), queueStatusHandler.GetQueueStatus)
		tasks.GET("/status", taskHandler.GetTaskStatus)
	}
}

func registerStatsRoutes(parent *gin.RouterGroup, taskHandler *handlers.TaskHandler) {
	stats := parent.Group("/stats")
	{
		stats.GET("/dashboard", taskHandler.GetDashboard)
		stats.GET("/cloud", taskHandler.GetCloudStats)
		stats.GET("/trend", taskHandler.GetTrendData)
		stats.POST("/calculate", taskHandler.CalculateStats)
		stats.GET("/total-cloud", taskHandler.GetTotalCloudCount)
	}
}

func registerExchangeRoutes(parent *gin.RouterGroup, exchangeHandler *handlers.ExchangeHandler) {
	exchange := parent.Group("/exchange")
	{
		// 新语义路由：账号规则。旧 /accounts 路由保留兼容一到两个版本。
		exchange.POST("/rules", exchangeHandler.AddExchangeRule)
		exchange.GET("/rules", exchangeHandler.GetExchangeRules)
		exchange.PUT("/rules/:id", exchangeHandler.UpdateExchangeRule)
		exchange.DELETE("/rules/:id", exchangeHandler.DeleteExchangeRule)

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
}

func registerProductRoutes(parent *gin.RouterGroup, exchangeHandler *handlers.ExchangeHandler) {
	products := parent.Group("/products")
	{
		products.GET("/search", exchangeHandler.SearchProducts)
		products.GET("/categories", exchangeHandler.GetCategories)
		products.POST("/update", exchangeHandler.UpdateProducts)
	}
}

func registerAdminRoutes(r *gin.Engine, deps routeDependencies) {
	registerAdminRoutesAt(r, "/api/admin", deps)
	// 企业级 API 版本化入口：新增 /api/v1/admin，旧 /api/admin 继续兼容。
	registerAdminRoutesAt(r, "/api/v1/admin", deps)
}

func registerAdminRoutesAt(r *gin.Engine, basePath string, deps routeDependencies) {
	h := deps.handlers
	admin := r.Group(basePath)
	admin.Use(
		middleware.AuthMiddlewareWithUserAndSession(deps.jwtManager, deps.repos.User, deps.repos.RefreshSession),
		middleware.CSRFMiddleware(),
		deps.postAuthRateMw.HandlerFunc(),
		middleware.AdminMiddleware(),
		middleware.AuditMiddlewareWithFilter(deps.repos.AuditLog, deps.auditFilter),
	)
	{
		admin.GET("/users", h.admin.GetAllUsers)
		admin.GET("/accounts", h.admin.GetAllAccounts)
		admin.GET("/accounts/search", h.admin.SearchAllAccounts)
		admin.GET("/accounts/summaries", h.admin.GetAccountSummaries)
		admin.GET("/dashboard", h.admin.GetAdminDashboard)
		admin.PUT("/users/:id/role", h.admin.UpdateUserRole)
		admin.PUT("/users/:id/password", h.admin.ResetUserPassword)
		admin.PUT("/accounts/:id/status", h.admin.UpdateAccountStatus)
		admin.DELETE("/users/:id", h.admin.DeleteUser)
		admin.DELETE("/accounts/:id", h.admin.DeleteAccount)
		admin.GET("/stats/overview", h.admin.GetStatsOverview)
		admin.GET("/task-configs", h.admin.GetTaskConfigs)
		admin.PUT("/task-configs/:task_type", h.admin.UpdateTaskConfig)
		admin.GET("/tasks/queue-status", h.queueStatus.GetQueueStatus)

		// 抢兑配置管理。
		admin.GET("/exchange/config", h.exchange.GetExchangeConfig)
		admin.PUT("/exchange/config", h.exchange.UpdateExchangeConfig)
		admin.POST("/exchange/execute-monthly", h.exchange.ExecuteMonthlyExchange)
		admin.GET("/exchange/rules", h.exchange.GetAdminExchangeAccounts)
		admin.PUT("/exchange/rules/:id", h.exchange.UpdateAdminExchangeAccount)
		admin.GET("/exchange/accounts", h.exchange.GetAdminExchangeAccounts)
		admin.PUT("/exchange/accounts/:id", h.exchange.UpdateAdminExchangeAccount)
		admin.GET("/exchange/tasks", h.exchange.GetAdminExchangeTasks)

		// 公告管理。
		admin.GET("/announcements", h.announcement.GetAllAnnouncements)
		admin.POST("/announcements", h.announcement.CreateAnnouncement)
		admin.PUT("/announcements/:id", h.announcement.UpdateAnnouncement)
		admin.DELETE("/announcements/:id", h.announcement.DeleteAnnouncement)
		admin.GET("/announcements/:id", h.announcement.GetAnnouncement)
	}
}

func registerWebSocketRoute(r *gin.Engine, deps routeDependencies) {
	wsHub := ws.GetHub()
	wsHub.SetWSMessageRepository(deps.repos.WSMessage)

	r.GET("/ws", func(c *gin.Context) {
		// 浏览器发起的 WebSocket 必然携带 Origin 头，且不可被 JS 跨站伪造。
		// 拒绝缺失 Origin 的请求，作为 CSWSH 的纵深防御（upgrader 内部仍会做白名单二次校验）。
		if c.GetHeader("Origin") == "" {
			apiresponse.Forbidden(c, "missing origin")
			return
		}

		token := ""
		if cookieToken, err := c.Cookie("auth_token"); err == nil {
			token = cookieToken
		}
		if token == "" {
			apiresponse.Unauthorized(c, "missing token")
			return
		}

		claims, err := deps.jwtManager.ValidateToken(token)
		if err != nil {
			apiresponse.Unauthorized(c, "invalid token")
			return
		}
		user, err := deps.repos.User.WithContext(c.Request.Context()).FindByID(claims.UserID)
		if err != nil {
			apiresponse.Unauthorized(c, "user not found")
			return
		}
		if claims.TokenVersion != user.TokenVersion || claims.SessionID == "" {
			apiresponse.Unauthorized(c, "session revoked")
			return
		}
		active, err := deps.repos.RefreshSession.IsActive(c.Request.Context(), claims.SessionID, claims.UserID, time.Now())
		if err != nil || !active {
			apiresponse.Unauthorized(c, "session revoked")
			return
		}

		wsHub.HandleWebSocket(c.Writer, c.Request, user.ID)
	})

	r.GET("/events", func(c *gin.Context) {
		token, err := c.Cookie("auth_token")
		if err != nil || token == "" {
			apiresponse.Unauthorized(c, "missing token")
			return
		}
		claims, err := deps.jwtManager.ValidateToken(token)
		if err != nil {
			apiresponse.Unauthorized(c, "invalid token")
			return
		}
		user, err := deps.repos.User.WithContext(c.Request.Context()).FindByID(claims.UserID)
		if err != nil || claims.TokenVersion != user.TokenVersion || claims.SessionID == "" {
			apiresponse.Unauthorized(c, "session revoked")
			return
		}
		active, err := deps.repos.RefreshSession.IsActive(c.Request.Context(), claims.SessionID, claims.UserID, time.Now())
		if err != nil || !active {
			apiresponse.Unauthorized(c, "session revoked")
			return
		}
		wsHub.HandleSSE(c.Writer, c.Request, user.ID)
	})
}
