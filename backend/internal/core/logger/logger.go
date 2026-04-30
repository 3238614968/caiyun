package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-colorable"
)

// 日志级别
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelSuccess
	LevelWarn
	LevelError
	LevelFatal
	LevelTrace
)

// Logger 日志记录器
type Logger struct {
	level  LogLevel
	output io.Writer
	mu     sync.RWMutex

	errorCount int
	lastError  string
}

// NewLogger 创建日志记录器
func NewLogger(level LogLevel) *Logger {
	return &Logger{
		level:  level,
		output: colorable.NewColorableStdout(),
	}
}

// log 内部日志方法
func (l *Logger) log(level LogLevel, color string, args ...interface{}) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("%s [%s] %s", color, timestamp, formatArgs(args...))

	fmt.Fprintln(l.output, message)
}

// formatArgs 格式化参数
func formatArgs(args ...interface{}) string {
	var parts []string
	for _, arg := range args {
		parts = append(parts, fmt.Sprintf("%v", arg))
	}
	return strings.Join(parts, " ")
}

// color codes
const (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorPurple = "\033[35m"
)

// Fatal 致命错误（红色，退出程序）
func (l *Logger) Fatal(args ...interface{}) {
	l.recordError(formatArgs(args...))
	l.log(LevelFatal, colorRed, args...)
	osExit(1)
}

// Error 错误（红色）
func (l *Logger) Error(args ...interface{}) {
	if l.level <= LevelError {
		l.recordError(formatArgs(args...))
		l.log(LevelError, colorRed, args...)
	}
}

// Warn 警告（黄色）
func (l *Logger) Warn(args ...interface{}) {
	if l.level <= LevelWarn {
		l.log(LevelWarn, colorYellow, args...)
	}
}

// Info 信息（绿色）
func (l *Logger) Info(args ...interface{}) {
	if l.level <= LevelInfo {
		l.log(LevelInfo, colorGreen, args...)
	}
}

// Success 成功（绿色）
func (l *Logger) Success(args ...interface{}) {
	if l.level <= LevelSuccess {
		l.log(LevelSuccess, colorGreen, args...)
	}
}

// Fail 失败（红色）
func (l *Logger) Fail(args ...interface{}) {
	if l.level <= LevelError {
		l.recordError(formatArgs(args...))
		l.log(LevelError, colorRed, args...)
	}
}

// Start 开始（青色）
func (l *Logger) Start(args ...interface{}) {
	if l.level <= LevelInfo {
		l.log(LevelInfo, colorCyan, args...)
	}
}

// Debug 调试（青色）
func (l *Logger) Debug(args ...interface{}) {
	if l.level <= LevelDebug {
		l.log(LevelDebug, colorCyan, args...)
	}
}

// Trace 跟踪（紫色）
func (l *Logger) Trace(args ...interface{}) {
	if l.level <= LevelTrace {
		l.log(LevelTrace, colorPurple, args...)
	}
}

// Box 框式日志
func (l *Logger) Box(title, content string) {
	lines := strings.Split(content, "\n")
	maxLen := len(title)
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	maxLen += 4 // padding

	// 顶部边框
	border := strings.Repeat("=", maxLen)
	l.log(LevelInfo, colorGreen, "")
	l.log(LevelInfo, colorGreen, border)
	l.log(LevelInfo, colorGreen, fmt.Sprintf("  %s", title))

	// 内容行
	for _, line := range lines {
		padding := strings.Repeat(" ", maxLen-len(line)-2)
		l.log(LevelInfo, colorGreen, fmt.Sprintf("  %s%s", line, padding))
	}

	// 底部边框
	l.log(LevelInfo, colorGreen, border)
	l.log(LevelInfo, colorGreen, "")
}

// osExit 用于测试的可替换退出函数
type exitFunc func(int)

var osExit exitFunc = func(code int) {
	os.Exit(code)
}

type ErrorSnapshot struct {
	ErrorCount int
}

func (l *Logger) Snapshot() ErrorSnapshot {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return ErrorSnapshot{ErrorCount: l.errorCount}
}

func (l *Logger) ErrorsSince(snapshot ErrorSnapshot) (bool, string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.errorCount <= snapshot.ErrorCount {
		return false, ""
	}
	return true, strings.TrimSpace(l.lastError)
}

func (l *Logger) recordError(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errorCount++
	l.lastError = strings.TrimSpace(message)
}
