package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/monitor"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

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
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "ok",
			"service": "caiyun-worker",
			"time":    time.Now().Format("2006-01-02 15:04:05"),
		})
	}
	// 宝塔 Go 项目管理会探测项目端口根路径；根路径返回 200，避免误判启动失败。
	mux.HandleFunc("/", healthHandler)
	mux.HandleFunc("/health", healthHandler)
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
		// 常量时间比较，避免时序攻击逐字节推断 token。
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
