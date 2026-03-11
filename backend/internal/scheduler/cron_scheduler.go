package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Job 任务接口
type Job interface {
	Execute() error
	GetName() string
	GetDescription() string
}

// JobResult 任务执行结果
type JobResult struct {
	JobName     string
	Status      string
	Message     string
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration
	Error       error
}

// Scheduler 定时任务调度器
type Scheduler struct {
	cron          *cron.Cron
	jobs          map[string]Job
	jobResults    map[string][]*JobResult
	maxResults    int
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *log.Logger
	notifications chan *JobResult
}

// Config 调度器配置
type Config struct {
	MaxResults    int
	Logger        *log.Logger
	EnableHistory bool
}

// NewScheduler 创建调度器
func NewScheduler(config ...Config) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	// 解析配置
	var cfg Config
	if len(config) > 0 {
		cfg = config[0]
	}

	// 设置默认值
	if cfg.MaxResults == 0 {
		cfg.MaxResults = 100
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	// 创建cron实例
	c := cron.New(cron.WithLogger(cron.VerbosePrintfLogger(cfg.Logger)))

	s := &Scheduler{
		cron:          c,
		jobs:          make(map[string]Job),
		jobResults:    make(map[string][]*JobResult),
		maxResults:    cfg.MaxResults,
		ctx:           ctx,
		cancel:        cancel,
		logger:        cfg.Logger,
		notifications: make(chan *JobResult, 100),
	}

	return s
}

// AddJob 添加定时任务
func (s *Scheduler) AddJob(schedule string, job Job, options ...cron.Option) (cron.EntryID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 验证schedule格式
	if err := s.validateSchedule(schedule); err != nil {
		return 0, err
	}

	// 添加任务到cron
	entryID, err := s.cron.AddFunc(schedule, func() { s.wrapJob(job).Run() })
	if err != nil {
		return 0, err
	}

	// 保存任务信息
	s.jobs[job.GetName()] = job
	s.jobResults[job.GetName()] = make([]*JobResult, 0, s.maxResults)

	s.logger.Printf("添加定时任务: %s, 计划: %s", job.GetName(), schedule)
	return entryID, nil
}

// AddJobWithName 添加带名称的定时任务
func (s *Scheduler) AddJobWithName(name, schedule string, jobFunc func() error, description string) (cron.EntryID, error) {
	job := &namedJob{
		name:        name,
		description: description,
		jobFunc:     jobFunc,
	}
	return s.AddJob(schedule, job)
}

// RemoveJob 移除定时任务
func (s *Scheduler) RemoveJob(jobName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 查找所有相关entry
	entries := s.cron.Entries()
	for _, entry := range entries {
		if job, ok := entry.Job.(*wrappedJob); ok {
			if job.GetName() == jobName {
				s.cron.Remove(entry.ID)
				delete(s.jobs, jobName)
				delete(s.jobResults, jobName)
				s.logger.Printf("移除定时任务: %s", jobName)
				return nil
			}
		}
	}

	return fmt.Errorf("任务不存在: %s", jobName)
}

// ExecuteJob 立即执行任务
func (s *Scheduler) ExecuteJob(jobName string) (*JobResult, error) {
	s.mu.RLock()
	job, exists := s.jobs[jobName]
	s.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", jobName)
	}

	result := s.executeJob(job)
	return result, nil
}

// Start 启动调度器
func (s *Scheduler) Start() {
	s.cron.Start()
	s.logger.Println("定时任务调度器已启动")

	// 启动通知处理器
	go s.notificationHandler()
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
	s.logger.Println("正在停止定时任务调度器...")
	s.cancel()
	s.cron.Stop()
	s.logger.Println("定时任务调度器已停止")
}

// GetJobStatus 获取任务状态
func (s *Scheduler) GetJobStatus(jobName string) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, exists := s.jobs[jobName]
	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", jobName)
	}

	results := s.jobResults[jobName]
	status := map[string]interface{}{
		"name":        job.GetName(),
		"description": job.GetDescription(),
		"total_runs":  len(results),
	}

	if len(results) > 0 {
		lastResult := results[len(results)-1]
		status["last_run"] = map[string]interface{}{
			"status":       lastResult.Status,
			"started_at":   lastResult.StartedAt,
			"completed_at": lastResult.CompletedAt,
			"duration":     lastResult.Duration.String(),
			"message":      lastResult.Message,
		}
	}

	// 获取下次执行时间
	entries := s.cron.Entries()
	for _, entry := range entries {
		if job, ok := entry.Job.(*wrappedJob); ok {
			if job.GetName() == jobName {
				status["next_run"] = entry.Next
				break
			}
		}
	}

	return status, nil
}

// GetJobs 获取所有任务列表
func (s *Scheduler) GetJobs() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var jobs []map[string]interface{}
	for name, job := range s.jobs {
		results := s.jobResults[name]

		jobInfo := map[string]interface{}{
			"name":        job.GetName(),
			"description": job.GetDescription(),
			"total_runs":  len(results),
		}

		if len(results) > 0 {
			lastResult := results[len(results)-1]
			jobInfo["last_run"] = map[string]interface{}{
				"status":       lastResult.Status,
				"started_at":   lastResult.StartedAt,
				"completed_at": lastResult.CompletedAt,
				"duration":     lastResult.Duration.String(),
			}
		}

		jobs = append(jobs, jobInfo)
	}

	return jobs
}

// GetJobHistory 获取任务执行历史
func (s *Scheduler) GetJobHistory(jobName string, limit int) ([]*JobResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results, exists := s.jobResults[jobName]
	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", jobName)
	}

	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}

	// 返回最新的结果
	start := len(results) - limit
	if start < 0 {
		start = 0
	}

	return results[start:], nil
}

// wrapJob 包装任务
func (s *Scheduler) wrapJob(job Job) cron.Job {
	return &wrappedJob{
		job:     job,
		sched:   s,
		context: s.ctx,
	}
}

// executeJob 执行任务并记录结果
func (s *Scheduler) executeJob(job Job) *JobResult {
	result := &JobResult{
		JobName:   job.GetName(),
		StartedAt: time.Now(),
	}

	defer func() {
		result.CompletedAt = time.Now()
		result.Duration = result.CompletedAt.Sub(result.StartedAt)

		// 记录结果
		s.recordResult(result)

		// 发送通知
		select {
		case s.notifications <- result:
		default:
			s.logger.Printf("通知队列已满，丢弃任务结果: %s", job.GetName())
		}
	}()

	s.logger.Printf("开始执行任务: %s", job.GetName())

	if err := job.Execute(); err != nil {
		result.Status = "failed"
		result.Message = err.Error()
		result.Error = err
		s.logger.Printf("任务执行失败: %s, 错误: %v", job.GetName(), err)
	} else {
		result.Status = "success"
		result.Message = "任务执行成功"
		s.logger.Printf("任务执行成功: %s, 耗时: %v", job.GetName(), result.Duration)
	}

	return result
}

// recordResult 记录任务结果
func (s *Scheduler) recordResult(result *JobResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	results := s.jobResults[result.JobName]

	// 限制历史记录数量
	if len(results) >= s.maxResults {
		results = results[1:]
	}

	s.jobResults[result.JobName] = append(results, result)
}

// validateSchedule 验证cron表达式格式
func (s *Scheduler) validateSchedule(schedule string) error {
	// 检查基本的cron表达式格式
	parts := strings.Fields(schedule)
	if len(parts) != 5 && len(parts) != 6 {
		return fmt.Errorf("无效的cron表达式格式: %s", schedule)
	}

	// 验证每个字段
	validators := []func(string) bool{
		validateMinute,
		validateHour,
		validateDayOfMonth,
		validateMonth,
		validateDayOfWeek,
	}

	for i, part := range parts[:5] {
		if !validators[i](part) {
			return fmt.Errorf("cron表达式字段 %d 无效: %s", i+1, part)
		}
	}

	return nil
}

// notificationHandler 处理通知
func (s *Scheduler) notificationHandler() {
	for {
		select {
		case result := <-s.notifications:
			// 这里可以添加通知逻辑，比如发送邮件、Slack消息等
			s.logger.Printf("任务通知 - %s: %s (%v)", result.JobName, result.Status, result.Duration)

		case <-s.ctx.Done():
			return
		}
	}
}

// wrappedJob 包装的任务类型
type wrappedJob struct {
	job     Job
	sched   *Scheduler
	context context.Context
}

func (w *wrappedJob) Run() {
	select {
	case <-w.context.Done():
		return
	default:
		w.sched.executeJob(w.job)
	}
}

func (w *wrappedJob) GetName() string {
	return w.job.GetName()
}

// namedJob 命名任务
type namedJob struct {
	name        string
	description string
	jobFunc     func() error
}

func (n *namedJob) Execute() error {
	return n.jobFunc()
}

func (n *namedJob) GetName() string {
	return n.name
}

func (n *namedJob) GetDescription() string {
	return n.description
}

// 验证函数
func validateMinute(s string) bool {
	return validateCronField(s, 0, 59)
}

func validateHour(s string) bool {
	return validateCronField(s, 0, 23)
}

func validateDayOfMonth(s string) bool {
	return validateCronField(s, 1, 31)
}

func validateMonth(s string) bool {
	return validateCronField(s, 1, 12)
}

func validateDayOfWeek(s string) bool {
	return validateCronField(s, 0, 7)
}

func validateCronField(s string, min, max int) bool {
	if s == "*" {
		return true
	}

	// 检查范围表达式
	if strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		if len(parts) != 2 {
			return false
		}

		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return false
		}

		return start >= min && start <= max && end >= min && end <= max && start <= end
	}

	// 检查步长表达式
	if strings.Contains(s, "/") {
		parts := strings.Split(s, "/")
		if len(parts) != 2 {
			return false
		}

		step, err := strconv.Atoi(parts[1])
		if err != nil || step <= 0 {
			return false
		}

		return validateCronField(parts[0], min, max)
	}

	// 检查列表表达式
	if strings.Contains(s, ",") {
		parts := strings.Split(s, ",")
		for _, part := range parts {
			if !validateCronField(part, min, max) {
				return false
			}
		}
		return true
	}

	// 检查单个数字
	val, err := strconv.Atoi(s)
	if err != nil {
		return false
	}

	return val >= min && val <= max
}
