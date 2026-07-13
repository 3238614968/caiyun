package monitor

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics 封装 Prometheus 指标注册器与项目指标。
type Metrics struct {
	registry *prometheus.Registry

	// 抢兑相关指标。
	exchangeTotal         prometheus.Counter
	exchangeSuccess       prometheus.Counter
	exchangeFailed        prometheus.Counter
	exchangeDuration      prometheus.Histogram
	exchangeAttempts      *prometheus.CounterVec
	exchangeRecentTotal   prometheus.Gauge
	exchangeSuccessRate   prometheus.Gauge
	exchangeFailReasons   *prometheus.GaugeVec
	exchangeScheduleSkips *prometheus.CounterVec
	exchangeScheduleTasks *prometheus.CounterVec

	// HTTP 请求指标。
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec

	// Token 相关指标。
	tokenTotal   prometheus.Gauge
	tokenHealthy prometheus.Gauge
	tokenExpired prometheus.Gauge

	// 任务相关指标。
	taskTotal     prometheus.Gauge
	taskPending   prometheus.Gauge
	taskRunning   prometheus.Gauge
	taskCompleted prometheus.Gauge

	// 队列相关指标。
	queuePending    prometheus.Gauge
	queueProcessing prometheus.Gauge
	queueDelayed    prometheus.Gauge
	queueDead       prometheus.Gauge

	// Worker 相关指标。
	workerUp            prometheus.Gauge
	workerHeartbeatUnix prometheus.Gauge

	// 缓存相关指标。
	cacheHits   prometheus.Counter
	cacheMisses prometheus.Counter

	// 审计相关指标。
	auditDropped      prometheus.Counter
	auditDroppedLast  atomic.Int64
	rateLimitRejected *prometheus.CounterVec

	// 历史归档指标。
	historyArchiveRuns            *prometheus.CounterVec
	historyArchiveDuration        prometheus.Histogram
	historyArchiveMoved           *prometheus.CounterVec
	historyArchiveBatches         *prometheus.CounterVec
	historyArchiveBatchDuration   *prometheus.HistogramVec
	historyArchiveHitLimits       prometheus.Counter
	historyArchiveLastRunUnix     prometheus.Gauge
	historyArchiveLastSuccessUnix prometheus.Gauge
}

// NewMetrics 创建指标收集器并注册指标。
func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
	}

	// 注册 Go 运行时与进程指标。
	m.registry.MustRegister(collectors.NewGoCollector())
	m.registry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// 抢兑指标。
	m.exchangeTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "exchange",
		Name:      "total",
		Help:      "抢兑总次数",
	})
	m.exchangeSuccess = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "exchange",
		Name:      "success_total",
		Help:      "抢兑成功次数",
	})
	m.exchangeFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "exchange",
		Name:      "failed_total",
		Help:      "抢兑失败次数",
	})
	m.exchangeDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "caiyun",
		Subsystem: "exchange",
		Name:      "duration_seconds",
		Help:      "抢兑耗时分布（秒）",
		Buckets:   []float64{0.1, 0.5, 1, 2, 3, 5, 10},
	})
	m.exchangeAttempts = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "exchange",
		Name:      "attempts_total",
		Help:      "按结果与失败原因分类的抢兑累计次数",
	}, []string{"result", "reason"})
	m.exchangeRecentTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "exchange",
		Name:      "recent_total",
		Help:      "最近统计窗口内的抢兑总次数",
	})
	m.exchangeSuccessRate = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "exchange",
		Name:      "success_rate",
		Help:      "最近统计窗口内的抢兑成功率，范围 0-1",
	})
	m.exchangeFailReasons = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "exchange",
		Name:      "failure_reason_total",
		Help:      "最近统计窗口内按归类失败原因统计的抢兑失败次数",
	}, []string{"reason"})
	m.exchangeScheduleSkips = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "exchange_scheduler",
		Name:      "skipped_total",
		Help:      "调度器按补货周期、日历策略或时间策略跳过任务的累计次数",
	}, []string{"reason", "restock_cycle", "calendar_policy"})
	m.exchangeScheduleTasks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "exchange_scheduler",
		Name:      "matched_tasks_total",
		Help:      "调度器按任务级时间和补货周期命中的任务累计次数",
	}, []string{"slot", "restock_cycle", "calendar_policy", "has_task_time", "has_custom_cron"})

	// HTTP 请求指标。
	m.httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "HTTP 请求总数",
	}, []string{"method", "route", "status"})
	m.httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "caiyun",
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "HTTP 请求耗时分布（秒）",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
	}, []string{"method", "route", "status"})

	// Token 指标。
	m.tokenTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "token",
		Name:      "total",
		Help:      "Token 总数",
	})
	m.tokenHealthy = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "token",
		Name:      "healthy",
		Help:      "健康 Token 数量",
	})
	m.tokenExpired = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "token",
		Name:      "expired",
		Help:      "异常或过期 Token 数量",
	})

	// 任务指标。
	m.taskTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "task",
		Name:      "total",
		Help:      "任务总数（运行中+已完成）",
	})
	m.taskPending = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "task",
		Name:      "pending",
		Help:      "待执行任务数量",
	})
	m.taskRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "task",
		Name:      "running",
		Help:      "运行中任务数量",
	})
	m.taskCompleted = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "task",
		Name:      "completed",
		Help:      "已完成任务数量",
	})

	// 队列指标。
	m.queuePending = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "queue",
		Name:      "pending",
		Help:      "待消费队列长度",
	})
	m.queueProcessing = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "queue",
		Name:      "processing",
		Help:      "处理中队列长度",
	})
	m.queueDelayed = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "queue",
		Name:      "delayed",
		Help:      "延迟队列长度",
	})
	m.queueDead = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "queue",
		Name:      "dead_letter",
		Help:      "死信队列长度",
	})

	// Worker 指标。
	m.workerUp = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "worker",
		Name:      "up",
		Help:      "Worker 存活状态：1=存活，0=未存活",
	})
	m.workerHeartbeatUnix = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "worker",
		Name:      "heartbeat_unix",
		Help:      "Worker 最近一次心跳时间（Unix 时间戳）",
	})

	// 缓存指标。
	m.cacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "cache",
		Name:      "hits_total",
		Help:      "缓存命中次数",
	})
	m.cacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "cache",
		Name:      "misses_total",
		Help:      "缓存未命中次数",
	})
	m.auditDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "audit",
		Name:      "dropped_total",
		Help:      "因审计日志异步队列满而丢弃的日志累计数量",
	})
	m.rateLimitRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "security",
		Name:      "rate_limit_rejected_total",
		Help:      "被限流中间件拒绝的请求累计数量",
	}, []string{"route", "reason", "dimension"})
	m.historyArchiveRuns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "history_archive",
		Name:      "runs_total",
		Help:      "历史归档任务执行次数",
	}, []string{"status"})
	m.historyArchiveDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "caiyun",
		Subsystem: "history_archive",
		Name:      "duration_seconds",
		Help:      "历史归档任务耗时分布（秒）",
		Buckets:   []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
	})
	m.historyArchiveMoved = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "history_archive",
		Name:      "moved_rows_total",
		Help:      "历史归档累计搬移的记录行数",
	}, []string{"table"})
	m.historyArchiveBatches = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "history_archive",
		Name:      "batches_total",
		Help:      "历史归档内部批次执行次数",
	}, []string{"table"})
	m.historyArchiveBatchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "caiyun",
		Subsystem: "history_archive",
		Name:      "batch_duration_seconds",
		Help:      "历史归档单批次耗时分布（秒）",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
	}, []string{"table"})
	m.historyArchiveHitLimits = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "caiyun",
		Subsystem: "history_archive",
		Name:      "hit_batch_limit_total",
		Help:      "历史归档因触发单轮最大批次数上限而提前结束的累计次数",
	})
	m.historyArchiveLastRunUnix = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "history_archive",
		Name:      "last_run_unix",
		Help:      "历史归档最近一次执行完成时间（Unix 时间戳）",
	})
	m.historyArchiveLastSuccessUnix = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "history_archive",
		Name:      "last_success_unix",
		Help:      "历史归档最近一次成功完成时间（Unix 时间戳）",
	})

	m.registry.MustRegister(m.exchangeTotal)
	m.registry.MustRegister(m.exchangeSuccess)
	m.registry.MustRegister(m.exchangeFailed)
	m.registry.MustRegister(m.exchangeDuration)
	m.registry.MustRegister(m.exchangeAttempts)
	m.registry.MustRegister(m.exchangeRecentTotal)
	m.registry.MustRegister(m.exchangeSuccessRate)
	m.registry.MustRegister(m.exchangeFailReasons)
	m.registry.MustRegister(m.exchangeScheduleSkips)
	m.registry.MustRegister(m.exchangeScheduleTasks)
	m.registry.MustRegister(m.httpRequests)
	m.registry.MustRegister(m.httpDuration)
	m.registry.MustRegister(m.tokenTotal)
	m.registry.MustRegister(m.tokenHealthy)
	m.registry.MustRegister(m.tokenExpired)
	m.registry.MustRegister(m.taskTotal)
	m.registry.MustRegister(m.taskPending)
	m.registry.MustRegister(m.taskRunning)
	m.registry.MustRegister(m.taskCompleted)
	m.registry.MustRegister(m.queuePending)
	m.registry.MustRegister(m.queueProcessing)
	m.registry.MustRegister(m.queueDelayed)
	m.registry.MustRegister(m.queueDead)
	m.registry.MustRegister(m.workerUp)
	m.registry.MustRegister(m.workerHeartbeatUnix)
	m.registry.MustRegister(m.cacheHits)
	m.registry.MustRegister(m.cacheMisses)
	m.registry.MustRegister(m.auditDropped)
	m.registry.MustRegister(m.rateLimitRejected)
	m.registry.MustRegister(m.historyArchiveRuns)
	m.registry.MustRegister(m.historyArchiveDuration)
	m.registry.MustRegister(m.historyArchiveMoved)
	m.registry.MustRegister(m.historyArchiveBatches)
	m.registry.MustRegister(m.historyArchiveBatchDuration)
	m.registry.MustRegister(m.historyArchiveHitLimits)
	m.registry.MustRegister(m.historyArchiveLastRunUnix)
	m.registry.MustRegister(m.historyArchiveLastSuccessUnix)

	return m
}

// Registry 返回指标注册器。
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// IncExchangeTotal 增加抢兑总次数。
func (m *Metrics) IncExchangeTotal() {
	m.exchangeTotal.Inc()
}

// IncExchangeSuccess 增加抢兑成功次数。
func (m *Metrics) IncExchangeSuccess() {
	m.exchangeSuccess.Inc()
}

// IncExchangeFailed 增加抢兑失败次数。
func (m *Metrics) IncExchangeFailed() {
	m.exchangeFailed.Inc()
}

// ObserveExchangeDuration 记录抢兑耗时（秒）。
func (m *Metrics) ObserveExchangeDuration(duration float64) {
	m.exchangeDuration.Observe(duration)
}

// RecordExchangeAttempt 记录单次抢兑结果、失败原因与耗时。
func (m *Metrics) RecordExchangeAttempt(success bool, reason string, duration time.Duration) {
	if m == nil {
		return
	}
	m.IncExchangeTotal()
	if success {
		m.IncExchangeSuccess()
		reason = "success"
	} else {
		m.IncExchangeFailed()
		if reason == "" || reason == "unknown" {
			reason = "other"
		}
	}
	if duration < 0 {
		duration = 0
	}
	m.ObserveExchangeDuration(duration.Seconds())
	m.exchangeAttempts.WithLabelValues(exchangeResultLabel(success), reason).Inc()
}

// SetTokenStats 设置 Token 统计指标。
func (m *Metrics) SetTokenStats(total, healthy, expired int) {
	m.tokenTotal.Set(float64(total))
	m.tokenHealthy.Set(float64(healthy))
	m.tokenExpired.Set(float64(expired))
}

// SetTaskStats 设置任务统计指标。
func (m *Metrics) SetTaskStats(total, pending, running, completed int) {
	m.taskTotal.Set(float64(total))
	m.taskPending.Set(float64(pending))
	m.taskRunning.Set(float64(running))
	m.taskCompleted.Set(float64(completed))
}

// SetWorkerState 设置 Worker 存活状态。
func (m *Metrics) SetWorkerState(up bool) {
	if m == nil {
		return
	}
	if up {
		m.workerUp.Set(1)
		return
	}
	m.workerUp.Set(0)
}

// TouchWorkerHeartbeat 更新时间戳心跳，并自动标记 Worker 为存活。
func (m *Metrics) TouchWorkerHeartbeat(now time.Time) {
	if m == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	m.workerUp.Set(1)
	m.workerHeartbeatUnix.Set(float64(now.Unix()))
}

// IncCacheHits 增加缓存命中次数。
func (m *Metrics) IncCacheHits() {
	m.cacheHits.Inc()
}

// IncCacheMisses 增加缓存未命中次数。
func (m *Metrics) IncCacheMisses() {
	m.cacheMisses.Inc()
}

// IncAuditDropped 实时增加一条审计日志丢弃计数，并同步内部累计值避免后续轮询重复记数。
func (m *Metrics) IncAuditDropped() {
	if m == nil {
		return
	}
	m.auditDropped.Inc()
	m.auditDroppedLast.Add(1)
}

// SetAuditDropped 同步审计日志丢弃累计数量到 Prometheus Counter。
func (m *Metrics) SetAuditDropped(count int64) {
	if m == nil {
		return
	}
	if count < 0 {
		count = 0
	}
	for {
		prev := m.auditDroppedLast.Load()
		if count <= prev {
			return
		}
		if m.auditDroppedLast.CompareAndSwap(prev, count) {
			m.auditDropped.Add(float64(count - prev))
			return
		}
	}
}

// RecordRateLimitRejection 记录被限流拦截的请求。
func (m *Metrics) RecordRateLimitRejection(route, reason, dimension string) {
	if m == nil {
		return
	}
	if route == "" {
		route = "unknown"
	}
	if reason == "" {
		reason = "UNKNOWN"
	}
	if dimension == "" {
		dimension = "unknown"
	}
	m.rateLimitRejected.WithLabelValues(route, reason, dimension).Inc()
}

// ObserveHTTPRequest 记录 HTTP 请求数量与耗时。
func (m *Metrics) ObserveHTTPRequest(method, route, status string, durationSeconds float64) {
	if m == nil {
		return
	}
	m.httpRequests.WithLabelValues(method, route, status).Inc()
	m.httpDuration.WithLabelValues(method, route, status).Observe(durationSeconds)
}

// SetQueueStats 设置队列长度统计指标。
func (m *Metrics) SetQueueStats(pending, processing, delayed, dead int64) {
	if m == nil {
		return
	}
	m.queuePending.Set(float64(pending))
	m.queueProcessing.Set(float64(processing))
	m.queueDelayed.Set(float64(delayed))
	m.queueDead.Set(float64(dead))
}

// SetExchangeRecentStats 设置最近窗口内抢兑统计与失败原因分布。
func (m *Metrics) SetExchangeRecentStats(success, failed int64, failureReasons map[string]int64) {
	if m == nil {
		return
	}
	total := success + failed
	m.exchangeRecentTotal.Set(float64(total))
	if total > 0 {
		m.exchangeSuccessRate.Set(float64(success) / float64(total))
	} else {
		m.exchangeSuccessRate.Set(0)
	}
	m.exchangeFailReasons.Reset()
	for reason, count := range failureReasons {
		m.exchangeFailReasons.WithLabelValues(reason).Set(float64(count))
	}
}

// IncExchangeScheduleSkip 记录调度器因策略不匹配而跳过任务。
func (m *Metrics) IncExchangeScheduleSkip(reason, restockCycle, calendarPolicy string) {
	if m == nil {
		return
	}
	m.exchangeScheduleSkips.WithLabelValues(reason, restockCycle, calendarPolicy).Inc()
}

// IncExchangeScheduleMatchedTask 记录调度器命中的可执行任务。
func (m *Metrics) IncExchangeScheduleMatchedTask(slot, restockCycle, calendarPolicy string, hasTaskTime, hasCustomCron bool) {
	if m == nil {
		return
	}
	m.exchangeScheduleTasks.WithLabelValues(slot, restockCycle, calendarPolicy, boolLabel(hasTaskTime), boolLabel(hasCustomCron)).Inc()
}

// RecordHistoryArchiveRun 记录历史归档任务的执行结果、耗时与搬移量。
func (m *Metrics) RecordHistoryArchiveRun(status string, duration time.Duration, taskLogsMoved, exchangeRecordsMoved int64, hitBatchLimit bool) {
	m.RecordHistoryArchiveRunAt(status, time.Now(), duration, taskLogsMoved, exchangeRecordsMoved, hitBatchLimit)
}

func (m *Metrics) RecordHistoryArchiveBatch(table string, moved int64, duration time.Duration) {
	if m == nil || strings.TrimSpace(table) == "" {
		return
	}
	m.historyArchiveBatches.WithLabelValues(table).Inc()
	m.historyArchiveBatchDuration.WithLabelValues(table).Observe(duration.Seconds())
	if moved > 0 {
		m.historyArchiveMoved.WithLabelValues(table).Add(float64(moved))
	}
}

// RecordHistoryArchiveRunAt 记录历史归档任务的执行结果、耗时、搬移量与最后一次执行时间。
func (m *Metrics) RecordHistoryArchiveRunAt(status string, finishedAt time.Time, duration time.Duration, taskLogsMoved, exchangeRecordsMoved int64, hitBatchLimit bool) {
	if m == nil {
		return
	}
	if status == "" {
		status = "success"
	}
	if finishedAt.IsZero() {
		finishedAt = time.Now()
	}
	m.historyArchiveRuns.WithLabelValues(status).Inc()
	m.historyArchiveDuration.Observe(duration.Seconds())
	m.historyArchiveLastRunUnix.Set(float64(finishedAt.Unix()))
	if status == "success" {
		m.historyArchiveLastSuccessUnix.Set(float64(finishedAt.Unix()))
	}
	if hitBatchLimit {
		m.historyArchiveHitLimits.Inc()
	}
}

func boolLabel(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func exchangeResultLabel(success bool) string {
	if success {
		return "success"
	}
	return "failed"
}
