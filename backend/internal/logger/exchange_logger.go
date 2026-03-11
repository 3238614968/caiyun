package logger

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// ExchangeLogger 兑换中心专用日志
type ExchangeLogger struct {
	logger     *log.Logger
	errorLog   *log.Logger
	logLevel   LogLevel
	logDir     string
	maxAge     int // 日志保留天数
	enableFile bool
}

// LogEntry 日志条目结构
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	TaskID    uint                   `json:"task_id,omitempty"`
	AccountID uint                   `json:"account_id,omitempty"`
	ProductID uint                   `json:"product_id,omitempty"`
	PrizeID   string                 `json:"prize_id,omitempty"`
	Duration  int64                  `json:"duration_ms,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Extra     map[string]interface{} `json:"extra,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Function  string                 `json:"function,omitempty"`
}

// NewExchangeLogger 创建兑换中心日志
func NewExchangeLogger(logDir string, maxAge int) *ExchangeLogger {
	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("创建日志目录失败：%v", err)
	}

	// 创建标准日志
	stdLog := log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)

	// 创建错误日志文件
	errorLogFile := filepath.Join(logDir, "exchange_error.log")
	errorFile, err := os.OpenFile(errorLogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("打开错误日志文件失败：%v", err)
	}
	errorLogger := log.New(errorFile, "", log.LstdFlags|log.Lshortfile)

	return &ExchangeLogger{
		logger:     stdLog,
		errorLog:   errorLogger,
		logLevel:   INFO,
		logDir:     logDir,
		maxAge:     maxAge,
		enableFile: true,
	}
}

// SetLogLevel 设置日志级别
func (l *ExchangeLogger) SetLogLevel(level LogLevel) {
	l.logLevel = level
}

// formatMessage 格式化日志消息
func (l *ExchangeLogger) formatMessage(level string, msg string, entry *LogEntry) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	return fmt.Sprintf("[%s] [%s] %s", timestamp, level, msg)
}

// getCallerInfo 获取调用者信息
func (l *ExchangeLogger) getCallerInfo() (file string, line int, function string) {
	pc, file, line, ok := runtime.Caller(3)
	if !ok {
		return "", 0, ""
	}

	// 只保留文件名
	file = filepath.Base(file)

	// 获取函数名
	function = runtime.FuncForPC(pc).Name()
	parts := strings.Split(function, ".")
	if len(parts) > 0 {
		function = parts[len(parts)-1]
	}

	return file, line, function
}

// Info 输出 INFO 级别日志
func (l *ExchangeLogger) Info(msg string, entry *LogEntry) {
	if l.logLevel <= INFO {
		formattedMsg := l.formatMessage("INFO", msg, entry)
		l.logger.Println(formattedMsg)

		if l.enableFile {
			l.writeToFile("info", formattedMsg)
		}
	}
}

// Warn 输出 WARN 级别日志
func (l *ExchangeLogger) Warn(msg string, entry *LogEntry) {
	if l.logLevel <= WARN {
		formattedMsg := l.formatMessage("WARN", msg, entry)
		l.logger.Println(formattedMsg)

		if l.enableFile {
			l.writeToFile("warn", formattedMsg)
		}
	}
}

// Error 输出 ERROR 级别日志
func (l *ExchangeLogger) Error(msg string, entry *LogEntry) {
	if l.logLevel <= ERROR {
		formattedMsg := l.formatMessage("ERROR", msg, entry)
		l.errorLog.Println(formattedMsg)

		if l.enableFile {
			l.writeToFile("error", formattedMsg)
		}

		// 触发告警（如果配置了告警系统）
		l.triggerAlert(msg, entry)
	}
}

// Debug 输出 DEBUG 级别日志
func (l *ExchangeLogger) Debug(msg string, entry *LogEntry) {
	if l.logLevel <= DEBUG {
		formattedMsg := l.formatMessage("DEBUG", msg, entry)
		l.logger.Println(formattedMsg)
	}
}

// ExchangeLog 记录抢兑专用日志
func (l *ExchangeLogger) ExchangeLog(taskID, accountID, productID uint, prizeID, action, result string, duration int64, err error) {
	entry := &LogEntry{
		Timestamp: time.Now(),
		TaskID:    taskID,
		AccountID: accountID,
		ProductID: productID,
		PrizeID:   prizeID,
		Duration:  duration,
	}

	msg := fmt.Sprintf("抢兑执行 - 任务 ID:%d, 账号 ID:%d, 商品 ID:%d, PrizeID:%s, 动作:%s, 结果:%s, 耗时:%dms",
		taskID, accountID, productID, prizeID, action, result, duration)

	if err != nil {
		entry.Error = err.Error()
		entry.Extra = map[string]interface{}{
			"action": action,
			"result": result,
		}
		l.Error(msg, entry)
	} else {
		l.Info(msg, entry)
	}
}

// writeToFile 写入日志文件
func (l *ExchangeLogger) writeToFile(level, msg string) {
	filename := filepath.Join(l.logDir, fmt.Sprintf("exchange_%s_%s.log", level, time.Now().Format("20060102")))
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	f.WriteString(msg + "\n")
}

// triggerAlert 触发告警（简化实现，后续集成告警系统）
func (l *ExchangeLogger) triggerAlert(msg string, entry *LogEntry) {
	// 当前为简化告警实现，后续可扩展为邮件、短信、Webhook 等通知渠道。
	_ = entry
	log.Printf("[ALERT] %s\n", msg)
}

// RotateLogs 日志归档（每天调用一次）
func (l *ExchangeLogger) RotateLogs() {
	if !l.enableFile {
		return
	}

	// 删除超过保留天数的日志文件
	cutoff := time.Now().AddDate(0, 0, -l.maxAge)

	filepath.Walk(l.logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// 检查是否是过期的日志文件
		if info.ModTime().Before(cutoff) && strings.HasSuffix(info.Name(), ".log") {
			os.Remove(path)
			log.Printf("已删除过期日志文件：%s\n", path)
		}

		return nil
	})
}

// StartAutoRotate 启动自动日志归档（每天凌晨 2 点执行）
func (l *ExchangeLogger) StartAutoRotate(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				now := time.Now()
				nextRun := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())

				if now.After(nextRun) {
					nextRun = nextRun.Add(24 * time.Hour)
				}

				time.Sleep(nextRun.Sub(now))
				l.RotateLogs()
			}
		}
	}()
}

// GetStats 获取日志统计信息
func (l *ExchangeLogger) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"log_dir":    l.logDir,
		"max_age":    l.maxAge,
		"log_level":  l.logLevel.String(),
		"file_count": 0,
		"total_size": int64(0),
	}

	// 统计日志文件数量和大小
	filepath.Walk(l.logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".log") {
			stats["file_count"] = stats["file_count"].(int) + 1
			stats["total_size"] = stats["total_size"].(int64) + info.Size()
		}

		return nil
	})

	return stats
}

// String 日志级别转字符串
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}
