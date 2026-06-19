package monitor

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics 封装 Prometheus 指标注册器与项目指标。
type Metrics struct {
	registry *prometheus.Registry

	// 抢兑相关指标。
	exchangeTotal    prometheus.Counter
	exchangeSuccess  prometheus.Counter
	exchangeFailed   prometheus.Counter
	exchangeDuration prometheus.Histogram

	// Token 相关指标。
	tokenTotal   prometheus.Gauge
	tokenHealthy prometheus.Gauge
	tokenExpired prometheus.Gauge

	// 任务相关指标。
	taskTotal     prometheus.Gauge
	taskPending   prometheus.Gauge
	taskRunning   prometheus.Gauge
	taskCompleted prometheus.Gauge

	// 缓存相关指标。
	cacheHits   prometheus.Counter
	cacheMisses prometheus.Counter

	// 审计相关指标。
	auditDropped prometheus.Gauge
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
	m.auditDropped = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "caiyun",
		Subsystem: "audit",
		Name:      "dropped_total",
		Help:      "因审计日志异步队列满而丢弃的日志累计数量",
	})

	m.registry.MustRegister(m.exchangeTotal)
	m.registry.MustRegister(m.exchangeSuccess)
	m.registry.MustRegister(m.exchangeFailed)
	m.registry.MustRegister(m.exchangeDuration)
	m.registry.MustRegister(m.tokenTotal)
	m.registry.MustRegister(m.tokenHealthy)
	m.registry.MustRegister(m.tokenExpired)
	m.registry.MustRegister(m.taskTotal)
	m.registry.MustRegister(m.taskPending)
	m.registry.MustRegister(m.taskRunning)
	m.registry.MustRegister(m.taskCompleted)
	m.registry.MustRegister(m.cacheHits)
	m.registry.MustRegister(m.cacheMisses)
	m.registry.MustRegister(m.auditDropped)

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

// IncCacheHits 增加缓存命中次数。
func (m *Metrics) IncCacheHits() {
	m.cacheHits.Inc()
}

// IncCacheMisses 增加缓存未命中次数。
func (m *Metrics) IncCacheMisses() {
	m.cacheMisses.Inc()
}

// SetAuditDropped 设置审计日志丢弃累计数量。
func (m *Metrics) SetAuditDropped(count int64) {
	m.auditDropped.Set(float64(count))
}
