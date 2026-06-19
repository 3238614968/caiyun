package bootstrap

import (
	"log"
	"path/filepath"
	"strings"

	applogger "caiyun/pkg/logger"
)

// ConfigureStandardLogger routes standard-library log output through the
// project logger so legacy log.Printf call sites share the same level, format
// and optional file rotation configuration.
func ConfigureStandardLogger(serviceName string) func() {
	cfg := &applogger.Config{
		Level:      parseLogLevel(GetEnv("LOG_LEVEL", "info")),
		OutputPath: resolveLogOutputPath(serviceName),
		MaxSize:    int64(GetIntEnv("LOG_MAX_SIZE", 10)),
		MaxBackups: GetIntEnv("LOG_MAX_BACKUPS", 3),
		MaxAge:     GetIntEnv("LOG_MAX_AGE", 30),
		Compress:   GetBoolEnv("LOG_COMPRESS", true),
		JSONFormat: GetBoolEnv("LOG_JSON_FORMAT", false),
	}

	logger, err := applogger.New(cfg)
	if err != nil {
		log.Printf("初始化结构化日志失败，回退到标准输出: %v", err)
		return func() {}
	}
	applogger.SetStandard(logger)

	writer := logger.Writer()
	log.SetFlags(0)
	log.SetOutput(writer)

	return func() {
		_ = writer.Close()
		_ = logger.Close()
	}
}

func parseLogLevel(level string) applogger.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "panic":
		return applogger.PanicLevel
	case "fatal":
		return applogger.FatalLevel
	case "error":
		return applogger.ErrorLevel
	case "warn", "warning":
		return applogger.WarnLevel
	case "debug":
		return applogger.DebugLevel
	case "trace":
		return applogger.TraceLevel
	default:
		return applogger.InfoLevel
	}
}

func resolveLogOutputPath(serviceName string) string {
	rawPath := strings.TrimSpace(GetEnv("LOG_FILE_PATH", GetEnv("LOG_PATH", "")))
	if rawPath == "" {
		return ""
	}

	ext := strings.ToLower(filepath.Ext(rawPath))
	if ext == ".log" {
		return rawPath
	}

	fileName := strings.TrimSpace(serviceName)
	if fileName == "" {
		fileName = "app"
	}
	if !strings.HasSuffix(fileName, ".log") {
		fileName += ".log"
	}
	return filepath.Join(rawPath, fileName)
}
