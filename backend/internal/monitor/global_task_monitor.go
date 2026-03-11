package monitor

import "sync"

var (
	globalTaskMonitor   *TaskMonitor
	globalTaskMonitorMu sync.RWMutex
)

// SetGlobalTaskMonitor 设置全局任务监控器实例。
func SetGlobalTaskMonitor(tm *TaskMonitor) {
	globalTaskMonitorMu.Lock()
	defer globalTaskMonitorMu.Unlock()
	globalTaskMonitor = tm
}

// GetGlobalTaskMonitor 获取全局任务监控器实例。
func GetGlobalTaskMonitor() *TaskMonitor {
	globalTaskMonitorMu.RLock()
	defer globalTaskMonitorMu.RUnlock()
	return globalTaskMonitor
}
