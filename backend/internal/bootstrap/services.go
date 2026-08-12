package bootstrap

import (
	"fmt"

	"caiyun/internal/queue"
	"caiyun/internal/services"
	"caiyun/internal/ws"
)

// SharedServices 汇总 API 与 Worker 都会使用的核心业务服务。
//
// 统一在这里装配可以避免两个进程各自复制一份初始化逻辑，尤其是：
//   - 任务队列后端选择与 AccountService 注入；
//   - TokenManager 分布式锁配置；
//   - TaskService 与 TokenManager 的关联；
//   - ExchangeService 的完整依赖图。
type SharedServices struct {
	Account      *services.AccountService
	Task         *services.TaskService
	Cloud        *services.CloudService
	TokenManager *services.TokenManager
	Exchange     *services.ExchangeService
	Operation    *services.OperationService
	TaskQueue    queue.ReliableTaskQueue
}

func InitSharedServices(core *Core, eventHubs ...*ws.Hub) (*SharedServices, error) {
	if core == nil {
		return nil, fmt.Errorf("core is nil")
	}
	repos := core.Repository
	var eventHub *ws.Hub
	if len(eventHubs) > 0 {
		eventHub = eventHubs[0]
	}

	taskQueue, err := queue.NewConfiguredTaskQueue(core.Redis)
	if err != nil {
		return nil, fmt.Errorf("初始化任务队列失败: %w", err)
	}

	accountService := services.NewAccountService(repos.Account, repos.User, core.Redis, core.Auth, repos.ExchangeAccount)
	accountService.SetTaskQueue(taskQueue)

	taskService := services.NewTaskService(repos.Account, repos.TaskLog, core.TaskStore, core.Auth, repos.TaskConfig, repos.CloudStats)
	taskService.SetEventHub(eventHub)
	cloudService := services.NewCloudService(repos.Account, repos.CloudStats, repos.TaskLog)

	tokenManager := services.NewTokenManager(repos.Account, repos.ExchangeAccount, core.Auth)
	tokenManager.SetDistributedLockCache(core.Redis)
	accountService.SetTokenProvider(tokenManager)
	taskService.SetTokenManager(tokenManager)

	exchangeService := services.NewExchangeService(
		repos.Product,
		repos.ExchangeAccount,
		repos.ExchangeTask,
		repos.Account,
		repos.SystemConfig,
		repos.ExchangeRecord,
		repos.TaskLog,
		core.Auth,
		tokenManager,
		eventHub,
	)
	exchangeService.SetLockStore(core.Redis)
	operationService := services.NewOperationService(repos.Operation, taskQueue)

	return &SharedServices{
		Account:      accountService,
		Task:         taskService,
		Cloud:        cloudService,
		TokenManager: tokenManager,
		Exchange:     exchangeService,
		Operation:    operationService,
		TaskQueue:    taskQueue,
	}, nil
}

func TaskQueueBackendName() string {
	return queue.TaskQueueBackendFromEnv()
}
