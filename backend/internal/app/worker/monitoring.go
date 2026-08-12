package worker

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/monitor"
	"caiyun/internal/queue"
	"caiyun/internal/version"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// startMonitoringAPI 启动监控API（可选）
func startMonitoringAPI(worker *Worker, core *bootstrap.Core, config MonitoringConfig) {
	host := config.Host
	port := config.Port
	if !isLoopbackHost(host) && !config.AllowPlaintext {
		log.Printf("监控API未启动：WORKER_MONITOR_HOST=%s 非本机地址。若已由 HTTPS 反代保护，请显式设置 WORKER_MONITOR_ALLOW_PLAINTEXT=true", host)
		return
	}

	monitorCtx, cancelMonitoring := context.WithCancel(worker.ctx)
	var background sync.WaitGroup
	defer func() {
		// ListenAndServe can also return because binding failed while the Worker
		// is still alive. Cancel and join both helper goroutines on every path.
		cancelMonitoring()
		background.Wait()
	}()

	metricsCollector := worker.metrics
	if metricsCollector == nil {
		metricsCollector = monitor.NewMetrics()
	}
	metricsHandler := promhttp.HandlerFor(metricsCollector.Registry(), promhttp.HandlerOpts{})

	updateMetrics := func() {
		taskMonitorStats := worker.taskMonitor.GetStats()
		taskManagerStats := worker.taskManager.GetStatus()

		total := bootstrap.ToInt(taskMonitorStats["total_tasks"])
		running := bootstrap.ToInt(taskMonitorStats["active_tasks"])
		completed := bootstrap.ToInt(taskMonitorStats["completed_tasks"])
		pending := bootstrap.ToInt(taskManagerStats["pending_tasks"])

		metricsCollector.SetTaskStats(total, pending, running, completed)
		metricsCollector.TouchWorkerHeartbeat(time.Now())
		if worker.taskQueue != nil {
			queuePending, _ := worker.taskQueue.GetQueueLength()
			processing, _ := worker.taskQueue.GetProcessingLength()
			delayed, _ := worker.taskQueue.GetDelayedLength()
			dead, _ := worker.taskQueue.GetDeadLetterLength()
			metricsCollector.SetQueueStats(queuePending, processing, delayed, dead)
		}
	}

	mux := http.NewServeMux()
	liveHandler := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"service": "caiyun-worker",
			"time":    time.Now().Format("2006-01-02 15:04:05"),
			"version": version.Get(),
		})
	}
	readyHandler := func(w http.ResponseWriter, r *http.Request) {
		if err := checkWorkerReadiness(r.Context(), worker, core); err != nil {
			log.Printf("Worker readiness check failed: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
				"status":  "not_ready",
				"service": "caiyun-worker",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "ready",
			"service": "caiyun-worker",
			"version": version.Get(),
		})
	}
	// 根路径和 /livez 仅反映进程存活；/readyz 与兼容 /health 校验依赖。
	mux.HandleFunc("/", liveHandler)
	mux.HandleFunc("/livez", liveHandler)
	mux.HandleFunc("/startupz", liveHandler)
	mux.HandleFunc("/readyz", readyHandler)
	mux.HandleFunc("/health", readyHandler)
	mux.HandleFunc("/status", requireMonitorAuth(config.Token, func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		metricsCollector.TouchWorkerHeartbeat(now)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"task_monitor": worker.taskMonitor.GetStats(),
			"task_manager": worker.taskManager.GetStatus(),
			"worker": map[string]interface{}{
				"concurrency":    worker.concurrency,
				"heartbeat_at":   now.Format(time.RFC3339),
				"queue_metadata": queue.MetadataOf(worker.taskQueue),
			},
		})
	}))
	mux.Handle("/metrics", requireMonitorAuth(config.Token, func(w http.ResponseWriter, r *http.Request) {
		updateMetrics()
		metricsHandler.ServeHTTP(w, r)
	}))

	srv := &http.Server{
		Addr:              net.JoinHostPort(host, port),
		Handler:           otelhttp.NewHandler(mux, "caiyun.worker.monitor"),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// 周期性打印摘要，方便不接入监控系统时观察运行状态。
	background.Add(1)
	go func() {
		defer background.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				updateMetrics()
				log.Printf("任务监控统计: %+v", worker.taskMonitor.GetStats())
				log.Printf("任务管理器状态: %+v", worker.taskManager.GetStatus())
			case <-monitorCtx.Done():
				return
			}
		}
	}()

	background.Add(1)
	go func() {
		defer background.Done()
		<-monitorCtx.Done()
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

func checkWorkerReadiness(parent context.Context, worker *Worker, core *bootstrap.Core) error {
	if worker == nil || core == nil || core.DB == nil || core.Redis == nil || worker.taskQueue == nil {
		return fmt.Errorf("Worker 核心依赖未初始化")
	}
	if err := worker.ctx.Err(); err != nil {
		return fmt.Errorf("Worker 正在停止: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	return bootstrap.CheckReadiness(ctx, core, worker.taskQueue)
}

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("监控API响应编码失败: %v", err)
	}
}

func requireMonitorAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.Error(w, "monitor token is required", http.StatusUnauthorized)
			return
		}

		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == "" {
			provided = r.Header.Get("X-Monitor-Token")
		}
		// 常量时间比较，避免时序攻击逐字节推断 token。
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
