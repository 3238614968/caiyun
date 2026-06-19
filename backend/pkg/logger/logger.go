package logger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Level 日志级别
type Level uint32

const (
	PanicLevel Level = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
	TraceLevel
)

// Config 日志配置
type Config struct {
	Level      Level
	OutputPath string
	MaxSize    int64 // MB
	MaxBackups int   // 最大备份数
	MaxAge     int   // 最大保存天数
	Compress   bool  // 是否压缩
	JSONFormat bool  // JSON 格式输出
}

// Logger 结构化日志器
type Logger struct {
	*logrus.Logger
	mu     sync.RWMutex
	config *Config
	hook   *rotateHook
	fields logrus.Fields
}

// rotateHook 日志轮转 hook
type rotateHook struct {
	maxSize    int64
	maxBackups int
	maxAge     int
	compress   bool
	filename   string
	file       *os.File
	mu         sync.Mutex
	size       int64
}

// Levels 实现 logrus.Hook 接口
func (hook *rotateHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

// Fire 实现 logrus.Hook 接口
func (hook *rotateHook) Fire(entry *logrus.Entry) error {
	hook.mu.Lock()
	defer hook.mu.Unlock()

	// 检查是否需要轮转
	if hook.size >= hook.maxSize*1024*1024 {
		if err := hook.rotate(); err != nil {
			return err
		}
	}

	// 写入日志
	line, err := entry.String()
	if err != nil {
		return err
	}

	n, err := hook.file.WriteString(line)
	if err != nil {
		return err
	}

	hook.size += int64(n)
	return nil
}

// rotate 执行日志轮转
func (hook *rotateHook) rotate() error {
	if err := hook.file.Close(); err != nil {
		return err
	}

	// 生成新文件名（带时间戳）
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	backupFile := fmt.Sprintf("%s.%s.log", strings.TrimSuffix(hook.filename, ".log"), timestamp)
	if err := os.Rename(hook.filename, backupFile); err != nil {
		return err
	}

	// 创建新文件
	newFile, err := os.OpenFile(hook.filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	hook.file = newFile
	hook.size = 0

	// 清理旧备份
	go hook.cleanOldBackups()

	return nil
}

// cleanOldBackups 清理旧的备份文件
func (hook *rotateHook) cleanOldBackups() {
	dir := filepath.Dir(hook.filename)
	prefix := filepath.Base(strings.TrimSuffix(hook.filename, ".log"))

	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type backupFile struct {
		path    string
		modTime time.Time
	}
	var backups []backupFile
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".log") {
			info, err := file.Info()
			if err != nil {
				continue
			}
			backups = append(backups, backupFile{path: filepath.Join(dir, name), modTime: info.ModTime()})
		}
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.Before(backups[j].modTime)
	})

	// 按修改时间排序，删除最旧的
	if len(backups) > hook.maxBackups {
		for i := 0; i < len(backups)-hook.maxBackups; i++ {
			os.Remove(backups[i].path)
		}
	}
}

// Fields 日志字段
type Fields map[string]interface{}

// New 创建新的日志实例
func New(config *Config) (*Logger, error) {
	logger := &Logger{
		Logger: logrus.New(),
		config: config,
		fields: make(logrus.Fields),
	}

	// 设置日志级别
	logger.SetLevel(logrus.Level(config.Level))

	// 设置输出格式
	if config.JSONFormat {
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		logger.SetFormatter(&logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
			FullTimestamp:   true,
		})
	}
	logger.SetOutput(os.Stdout)

	// 如果配置了输出路径，添加轮转 hook
	if config.OutputPath != "" {
		if err := os.MkdirAll(filepath.Dir(config.OutputPath), 0755); err != nil {
			return nil, fmt.Errorf("创建日志目录失败：%w", err)
		}

		file, err := os.OpenFile(config.OutputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("打开日志文件失败：%w", err)
		}

		info, _ := file.Stat()
		hook := &rotateHook{
			maxSize:    config.MaxSize,
			maxBackups: config.MaxBackups,
			maxAge:     config.MaxAge,
			compress:   config.Compress,
			filename:   config.OutputPath,
			file:       file,
			size:       info.Size(),
		}

		logger.AddHook(hook)
		logger.hook = hook
	}

	return logger, nil
}

// WithField 添加单个字段
func (l *Logger) WithField(key string, value interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newLogger := &Logger{
		Logger: l.Logger.WithField(key, value).Logger,
		config: l.config,
		fields: make(logrus.Fields),
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	newLogger.fields[key] = value
	return newLogger
}

// WithFields 添加多个字段
func (l *Logger) WithFields(fields Fields) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newLogger := &Logger{
		Logger: l.Logger.WithFields(logrus.Fields(fields)).Logger,
		config: l.config,
		fields: make(logrus.Fields),
	}
	for k, v := range l.fields {
		newLogger.fields[k] = v
	}
	for k, v := range fields {
		newLogger.fields[k] = v
	}
	return newLogger
}

// WithError 添加错误字段
func (l *Logger) WithError(err error) *Logger {
	return l.WithField("error", err)
}

// Debug 输出 DEBUG 级别日志
func (l *Logger) Debug(args ...interface{}) {
	l.Logger.Debug(args...)
}

// Debugf 格式化输出 DEBUG 级别日志
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Logger.Debugf(format, args...)
}

// Debugln 输出 DEBUG 级别日志（换行）
func (l *Logger) Debugln(args ...interface{}) {
	l.Logger.Debugln(args...)
}

// Info 输出 INFO 级别日志
func (l *Logger) Info(args ...interface{}) {
	l.Logger.Info(args...)
}

// Infof 格式化输出 INFO 级别日志
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Logger.Infof(format, args...)
}

// Infoln 输出 INFO 级别日志（换行）
func (l *Logger) Infoln(args ...interface{}) {
	l.Logger.Infoln(args...)
}

// Warn 输出 WARN 级别日志
func (l *Logger) Warn(args ...interface{}) {
	l.Logger.Warn(args...)
}

// Warnf 格式化输出 WARN 级别日志
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Logger.Warnf(format, args...)
}

// Warnln 输出 WARN 级别日志（换行）
func (l *Logger) Warnln(args ...interface{}) {
	l.Logger.Warnln(args...)
}

// Error 输出 ERROR 级别日志
func (l *Logger) Error(args ...interface{}) {
	l.Logger.Error(args...)
}

// Errorf 格式化输出 ERROR 级别日志
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Logger.Errorf(format, args...)
}

// Errorln 输出 ERROR 级别日志（换行）
func (l *Logger) Errorln(args ...interface{}) {
	l.Logger.Errorln(args...)
}

// Fatal 输出 FATAL 级别日志并退出
func (l *Logger) Fatal(args ...interface{}) {
	l.Logger.Fatal(args...)
}

// Fatalf 格式化输出 FATAL 级别日志并退出
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Logger.Fatalf(format, args...)
}

// Fatalln 输出 FATAL 级别日志（换行）并退出
func (l *Logger) Fatalln(args ...interface{}) {
	l.Logger.Fatalln(args...)
}

// Panic 输出 PANIC 级别日志并 panic
func (l *Logger) Panic(args ...interface{}) {
	l.Logger.Panic(args...)
}

// Panicf 格式化输出 PANIC 级别日志并 panic
func (l *Logger) Panicf(format string, args ...interface{}) {
	l.Logger.Panicf(format, args...)
}

// Panicln 输出 PANIC 级别日志（换行）并 panic
func (l *Logger) Panicln(args ...interface{}) {
	l.Logger.Panicln(args...)
}

// GetCallerInfo 获取调用者信息
func GetCallerInfo(skip int) (file string, line int, function string) {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown", 0, "unknown"
	}

	function = runtime.FuncForPC(pc).Name()
	if idx := strings.LastIndex(function, "/"); idx != -1 {
		function = function[idx+1:]
	}

	return filepath.Base(file), line, function
}

// Standard 返回标准日志实例
var standard *Logger

func init() {
	var err error
	standard, err = New(&Config{
		Level:      InfoLevel,
		OutputPath: "",
		MaxSize:    100,
		MaxBackups: 5,
		MaxAge:     30,
		JSONFormat: false,
	})
	if err != nil {
		panic(err)
	}
}

// Standard 返回标准日志器
func Standard() *Logger {
	return standard
}

// SetStandard 设置标准日志器
func SetStandard(logger *Logger) {
	standard = logger
}

// ContextKey 用于从 context 中获取 logger
type ContextKey string

const loggerKey ContextKey = "logger"

// NewContext 将 logger 添加到 context
func NewContext(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// FromContext 从 context 中获取 logger
func FromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(loggerKey).(*Logger); ok {
		return logger
	}
	return standard
}

// Close 关闭日志器
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.hook != nil && l.hook.file != nil {
		return l.hook.file.Close()
	}
	return nil
}
