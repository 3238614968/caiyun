package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/utils"
	"caiyun/internal/ws"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ExchangeService 抢兑服务
type ExchangeService struct {
	productRepo         *repository.ProductRepository
	exchangeAccountRepo *repository.ExchangeAccountRepository
	exchangeTaskRepo    *repository.ExchangeTaskRepository
	accountRepo         *repository.AccountRepository
	configRepo          *repository.SystemConfigRepository
	exchangeRecordRepo  *repository.ExchangeRecordRepository
	taskLogRepo         *repository.TaskLogRepository
	authMgr             *auth.Auth
	tokenMgr            *TokenManager
	hub                 *ws.Hub
	lockStore           exchangeLockStore
}

type exchangeLockStore interface {
	SetNX(key string, value interface{}, expiration time.Duration) (bool, error)
	Del(keys ...string) error
}

func NewExchangeService(
	productRepo *repository.ProductRepository,
	exchangeAccountRepo *repository.ExchangeAccountRepository,
	exchangeTaskRepo *repository.ExchangeTaskRepository,
	accountRepo *repository.AccountRepository,
	configRepo *repository.SystemConfigRepository,
	exchangeRecordRepo *repository.ExchangeRecordRepository,
	taskLogRepo *repository.TaskLogRepository,
	authMgr *auth.Auth,
	tokenMgr *TokenManager,
) *ExchangeService {
	return &ExchangeService{
		productRepo:         productRepo,
		exchangeAccountRepo: exchangeAccountRepo,
		exchangeTaskRepo:    exchangeTaskRepo,
		accountRepo:         accountRepo,
		configRepo:          configRepo,
		exchangeRecordRepo:  exchangeRecordRepo,
		taskLogRepo:         taskLogRepo,
		authMgr:             authMgr,
		tokenMgr:            tokenMgr,
		hub:                 ws.GetHub(),
	}
}

func (s *ExchangeService) SetLockStore(lockStore exchangeLockStore) {
	s.lockStore = lockStore
}

// UpdateProducts 更新商品信息 (从云盘 API 获取)
func (s *ExchangeService) UpdateProducts(accountID uint) error {
	_, err := syncProductsFromCloud(s.productRepo, s.accountRepo, accountID)
	return err
}

// SearchProducts 搜索商品
func (s *ExchangeService) SearchProducts(keyword string, limit int) ([]*models.Product, error) {
	if keyword == "" {
		return s.productRepo.FindActive()
	}
	return s.productRepo.Search(keyword, limit)
}

// GetProductCategories 获取商品分类
func (s *ExchangeService) GetProductCategories() ([]string, error) {
	return s.productRepo.GetCategories()
}

// AddExchangeAccount 添加兑换账号
func (s *ExchangeService) AddExchangeAccount(userID uint, accountID uint, remark string, exchangeTime1, exchangeTime2 string, productID *uint) (*models.ExchangeAccount, error) {
	// 检查云盘账号是否存在
	account, err := s.accountRepo.GetByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("云盘账号不存在")
	}
	if account.UserID != userID {
		return nil, fmt.Errorf("云盘账号不存在")
	}

	// 检查是否已添加为兑换账号
	if s.exchangeAccountRepo.ExistsByAccountID(accountID) {
		return nil, fmt.Errorf("该账号已添加为兑换账号")
	}

	// 创建兑换账号
	exchangeAccount := &models.ExchangeAccount{
		UserID:        userID,
		AccountID:     accountID,
		Phone:         account.Phone,
		Auth:          account.Auth,
		Token:         account.Token,
		JWTToken:      account.JWTToken,
		Remark:        remark,
		ExchangeTime1: exchangeTime1,
		ExchangeTime2: exchangeTime2,
		IsActive:      true,
	}

	if err := s.exchangeAccountRepo.Create(exchangeAccount); err != nil {
		return nil, fmt.Errorf("创建兑换账号失败：%w", err)
	}

	if productID != nil && *productID > 0 {
		if err := s.syncScheduledTaskForAccount(exchangeAccount, *productID); err != nil {
			_ = s.exchangeAccountRepo.Delete(exchangeAccount.ID)
			return nil, err
		}
	}

	return exchangeAccount, nil
}

// GetExchangeAccounts 获取用户的兑换账号列表
func (s *ExchangeService) GetExchangeAccounts(userID uint, isAdmin bool) ([]*models.ExchangeAccount, error) {
	if isAdmin {
		return s.exchangeAccountRepo.GetAll()
	}
	return s.exchangeAccountRepo.GetByUserID(userID)
}
func (s *ExchangeService) syncScheduledTaskForAccount(exchangeAccount *models.ExchangeAccount, productID uint) error {
	product, err := s.productRepo.GetByID(productID)
	if err != nil {
		return fmt.Errorf("商品不存在")
	}

	tasks, err := s.exchangeTaskRepo.GetByExchangeAccountID(exchangeAccount.ID)
	if err != nil {
		return fmt.Errorf("获取任务失败: %v", err)
	}

	if existingTask := findActiveScheduledTask(tasks); existingTask != nil {
		existingTask.ProductID = product.ID
		existingTask.PrizeID = product.PrizeID
		existingTask.PrizeName = product.PrizedName
		existingTask.TaskType = string(models.ExchangeTaskLongTerm)
		if existingTask.MaxAttempts <= 0 {
			existingTask.MaxAttempts = 10
		}
		if existingTask.Status == "" {
			existingTask.Status = string(models.ExchangeTaskPending)
		}
		if err := s.exchangeTaskRepo.Update(existingTask); err != nil {
			return fmt.Errorf("更新任务失败: %v", err)
		}
		return nil
	}

	task := &models.ExchangeTask{
		UserID:            exchangeAccount.UserID,
		ExchangeAccountID: exchangeAccount.ID,
		ProductID:         product.ID,
		PrizeID:           product.PrizeID,
		PrizeName:         product.PrizedName,
		TaskType:          string(models.ExchangeTaskLongTerm),
		MaxAttempts:       10,
		AttemptedCount:    0,
		Status:            string(models.ExchangeTaskPending),
		SuccessCount:      0,
		FailCount:         0,
	}

	if err := s.exchangeTaskRepo.Create(task); err != nil {
		return fmt.Errorf("创建预定任务失败: %v", err)
	}

	return nil
}

func findActiveScheduledTask(tasks []*models.ExchangeTask) *models.ExchangeTask {
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if task.TaskType != string(models.ExchangeTaskLongTerm) {
			continue
		}
		if task.Status == string(models.ExchangeTaskPending) || task.Status == string(models.ExchangeTaskRunning) {
			return task
		}
	}
	return nil
}

// UpdateExchangeAccount 更新兑换账号配置
func (s *ExchangeService) UpdateExchangeAccount(id uint, userID uint, isAdmin bool, remark string, exchangeTime1, exchangeTime2 string, isActive bool, productID *uint) error {
	account, err := s.exchangeAccountRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("兑换账号不存在")
	}

	// 检查权限：管理员可以修改所有账号，普通用户只能修改自己的账号
	if !isAdmin && account.UserID != userID {
		return fmt.Errorf("无权操作该账号")
	}

	account.Remark = remark
	account.ExchangeTime1 = exchangeTime1
	account.ExchangeTime2 = exchangeTime2
	account.IsActive = isActive

	// 如果提供了新的商品ID，更新或创建对应的抢兑任务（预定模式，不检查库存）
	if productID != nil && *productID > 0 {
		if err := s.syncScheduledTaskForAccount(account, *productID); err != nil {
			return err
		}
	}

	return s.exchangeAccountRepo.Update(account)
}

// DeleteExchangeAccount 删除兑换账号
func (s *ExchangeService) DeleteExchangeAccount(id uint, userID uint) error {
	account, err := s.exchangeAccountRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("兑换账号不存在")
	}

	if account.UserID != userID {
		return fmt.Errorf("无权操作该账号")
	}

	tasks, err := s.exchangeTaskRepo.GetByExchangeAccountID(account.ID)
	if err != nil {
		return fmt.Errorf("获取关联抢兑任务失败: %v", err)
	}
	for _, task := range tasks {
		if err := s.exchangeTaskRepo.Delete(task.ID); err != nil {
			return fmt.Errorf("删除关联抢兑任务失败: %v", err)
		}
	}

	return s.exchangeAccountRepo.Delete(id)
}

// CreateExchangeTask 创建抢兑任务
func (s *ExchangeService) CreateExchangeTask(userID uint, exchangeAccountID uint, productID uint, taskType string, maxAttempts int) (*models.ExchangeTask, error) {
	// 获取商品信息
	product, err := s.productRepo.GetByID(productID)
	if err != nil {
		return nil, fmt.Errorf("商品不存在")
	}

	// 检查商品是否可抢兑
	if !product.IsActive {
		return nil, fmt.Errorf("商品已下架，无法抢兑")
	}
	if product.StockStatus != "available" || product.DailyRemainderCount <= 0 {
		return nil, fmt.Errorf("商品已售罄，无法抢兑")
	}

	// 获取兑换账号信息
	exchangeAccount, err := s.exchangeAccountRepo.GetByID(exchangeAccountID)
	if err != nil {
		return nil, fmt.Errorf("兑换账号不存在")
	}

	if exchangeAccount.UserID != userID {
		return nil, fmt.Errorf("无权操作该账号")
	}

	candidateTask := &models.ExchangeTask{
		UserID:            userID,
		ExchangeAccountID: exchangeAccountID,
		ProductID:         productID,
		PrizeID:           product.PrizeID,
		PrizeName:         product.PrizedName,
		Product:           *product,
	}
	if skip, reason, err := shouldSkipExchangeMonthlySeries(s.exchangeRecordRepo, s.productRepo, candidateTask, time.Now()); err != nil {
		log.Printf("【抢兑月度保护】创建任务时查询本月同系列记录失败，继续创建: %v", err)
	} else if skip {
		return nil, fmt.Errorf("%s", reason)
	}

	// 检查任务是否已存在
	if s.exchangeTaskRepo.CheckTaskExists(userID, exchangeAccountID, product.PrizeID) {
		return nil, fmt.Errorf("该账号已存在该商品的抢兑任务")
	}

	// 创建抢兑任务
	task := &models.ExchangeTask{
		UserID:            userID,
		ExchangeAccountID: exchangeAccountID,
		ProductID:         productID,
		PrizeID:           product.PrizeID,
		PrizeName:         product.PrizedName,
		TaskType:          taskType,
		MaxAttempts:       maxAttempts,
		AttemptedCount:    0,
		Status:            string(models.ExchangeTaskPending),
		SuccessCount:      0,
		FailCount:         0,
	}

	if err := s.exchangeTaskRepo.Create(task); err != nil {
		return nil, fmt.Errorf("创建抢兑任务失败：%w", err)
	}

	return task, nil
}

// GetExchangeTasks 获取用户的抢兑任务列表
func (s *ExchangeService) GetExchangeTasks(userID uint, isAdmin bool) ([]*models.ExchangeTask, error) {
	if isAdmin {
		return s.exchangeTaskRepo.GetAll()
	}
	return s.exchangeTaskRepo.GetByUserID(userID)
}

// UpdateExchangeTask 更新抢兑任务
func (s *ExchangeService) UpdateExchangeTask(id uint, userID uint, maxAttempts int) error {
	task, err := s.exchangeTaskRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("任务不存在")
	}

	if task.UserID != userID {
		return fmt.Errorf("无权操作该任务")
	}

	task.MaxAttempts = maxAttempts
	return s.exchangeTaskRepo.Update(task)
}

// DeleteExchangeTask 删除抢兑任务
func (s *ExchangeService) DeleteExchangeTask(id uint, userID uint) error {
	task, err := s.exchangeTaskRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("任务不存在")
	}

	if task.UserID != userID {
		return fmt.Errorf("无权操作该任务")
	}

	return s.exchangeTaskRepo.Delete(id)
}

// ExecuteExchangeTask 执行抢兑任务 (立即执行)
func (s *ExchangeService) ExecuteExchangeTask(taskID uint, userID uint) error {
	task, err := s.exchangeTaskRepo.GetByID(taskID)
	if err != nil {
		return fmt.Errorf("任务不存在")
	}

	if task.UserID != userID {
		return fmt.Errorf("无权操作该任务")
	}

	// 异步执行抢兑
	go s.executeSingleTask(task)

	return nil
}

// BatchExecuteResult 批量执行结果
type BatchExecuteResult struct {
	TaskID  uint   `json:"task_id"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// BatchExecuteExchangeTasks 批量执行抢兑任务（使用工作池模式优化）
func (s *ExchangeService) BatchExecuteExchangeTasks(taskIDs []uint, userID uint) []BatchExecuteResult {
	results := make([]BatchExecuteResult, len(taskIDs))

	// 获取并发配置
	concurrency := 5
	if config, err := s.GetExchangeConcurrency(); err == nil && config > 0 {
		concurrency = config
	}

	// 使用工作池模式控制并发
	executor := utils.NewConcurrentExecutor(concurrency)

	for i, taskID := range taskIDs {
		index := i // 捕获索引
		id := taskID

		executor.Execute(func() {
			result := BatchExecuteResult{
				TaskID: id,
			}

			// 验证任务归属
			task, err := s.exchangeTaskRepo.GetByID(id)
			if err != nil {
				result.Success = false
				result.Message = "任务不存在"
				results[index] = result
				return
			}

			if task.UserID != userID {
				result.Success = false
				result.Message = "无权操作该任务"
				results[index] = result
				return
			}

			// 执行任务
			s.executeSingleTask(task)

			result.Success = true
			result.Message = "任务已开始执行"
			results[index] = result
		})
	}

	executor.Wait()
	return results
}

// executeSingleTask 执行单个抢兑任务（带重试机制）
func (s *ExchangeService) executeSingleTask(task *models.ExchangeTask) {
	started, err := s.exchangeTaskRepo.TryMarkRunning(task.ID)
	if err != nil {
		log.Printf("【抢兑任务】任务 %d 抢占执行权失败: %v", task.ID, err)
		return
	}
	if !started {
		log.Printf("【抢兑任务】任务 %d 已被其他进程执行或状态不可运行，跳过", task.ID)
		return
	}

	if skip, reason := s.monthlySeriesSkipReason(task); skip {
		s.updateExchangeTaskLastResult(task.ID, reason)
		s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskPending))
		log.Printf("【抢兑月度保护】任务 %d 跳过执行: %s", task.ID, reason)
		s.hub.SendToUser(task.UserID, ws.Message{
			Type: "exchange_skipped",
			Data: map[string]interface{}{
				"task_id":      task.ID,
				"account_name": exchangeAccountName(&task.ExchangeAccount),
				"product_name": task.PrizeName,
				"success":      false,
				"message":      reason,
			},
		})
		return
	}

	// 获取兑换账号
	account, err := s.exchangeAccountRepo.GetByID(task.ExchangeAccountID)
	if err != nil {
		s.recordExchangeResult(task, false, "获取兑换账号失败", 0)
		s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskFailed))
		return
	}

	accountName := account.Remark
	if accountName == "" {
		accountName = account.Phone
	}

	if s.accountRepo != nil && account.AccountID > 0 {
		cloudAccount, err := s.accountRepo.GetByID(account.AccountID)
		if err != nil {
			s.recordExchangeResult(task, false, "云盘账号不存在或已删除", 0)
			s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskFailed))
			return
		}
		if !cloudAccount.IsActive {
			s.recordExchangeResult(task, false, "云盘账号已失效，请重新登录后再启用任务", 0)
			s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskPending))
			return
		}
		if cloudAccount.Auth == "" {
			s.recordExchangeResult(task, false, "云盘账号认证为空，请重新登录", 0)
			s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskPending))
			return
		}
	}

	log.Printf("【抢兑任务】开始执行任务 %d，账号: %s，商品: %s", task.ID, accountName, task.PrizeName)

	// 执行抢兑（带重试）
	maxRetries := task.MaxRetries
	if maxRetries <= 0 {
		maxRetries = constants.DefaultMaxRetries // 使用常量：默认重试3次
	}

	var success bool
	var message string
	var execTime int
	attemptsUsed := 0
	releaseSeriesLockOnFailure := func() {}

	if locked, release, reason := s.acquireMonthlySeriesLock(task, time.Now()); !locked {
		success, message, execTime = false, reason, 0
	} else {
		releaseSeriesLockOnFailure = release

		for attempt := 0; attempt <= maxRetries; attempt++ {
			attemptsUsed = attempt
			if attempt > 0 {
				log.Printf("【抢兑任务】任务 %d 第 %d 次重试...", task.ID, attempt)
				// 更新重试次数
				now := time.Now()
				s.updateExchangeTaskRetryCount(task.ID, attempt, &now)
				// 重试间隔：指数退避
				time.Sleep(time.Duration(attempt*2) * time.Second)
			}

			prizeID := s.resolveTaskPrizeID(task)
			if !isUsableExchangePrizeID(prizeID) {
				success, message, execTime = false, "商品已下架或不存在，请更新商品列表后重新创建抢兑任务", 0
			} else {
				success, message, execTime = s.doExchange(account, prizeID)
			}

			// 如果成功，或者错误不需要重试，则退出循环
			if success || !s.shouldRetry(message) {
				break
			}
		}
	}

	// 记录结果
	s.recordExchangeResult(task, success, message, execTime)
	if !success {
		releaseSeriesLockOnFailure()
	}

	// 更新任务状态
	if success {
		log.Printf("【抢兑任务】任务 %d 执行成功，账号: %s", task.ID, accountName)
		// 抢兑成功
		if isSingleRunExchangeTask(task.TaskType) {
			latestTask := task
			if latest, err := s.exchangeTaskRepo.GetByID(task.ID); err == nil && latest != nil {
				latestTask = latest
			} else if err != nil {
				log.Printf("【抢兑任务】任务 %d 读取最新尝试次数失败，使用本地快照: %v", task.ID, err)
			}
			// 固定次数任务，检查是否达到最大次数
			if latestTask.MaxAttempts > 0 && latestTask.AttemptedCount >= latestTask.MaxAttempts {
				s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskCompleted))
			} else {
				s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskPending))
			}
		} else {
			// 长期任务，保持待执行状态
			s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskPending))
		}
	} else {
		log.Printf("【抢兑任务】任务 %d 执行失败，账号: %s，原因: %s", task.ID, accountName, message)
		// 抢兑失败
		if strings.Contains(message, "奖品单日已耗尽") || strings.Contains(message, "奖品已兑完") || strings.Contains(message, "商品已下架或不存在") || strings.Contains(message, "商品ID不是可兑换 prizeId") {
			// 奖品已抽完，停止任务
			s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskCompleted))
		} else if attemptsUsed >= maxRetries {
			// 重试次数用尽，标记为失败
			s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskFailed))
			log.Printf("【抢兑任务】任务 %d 重试次数已用尽，标记为失败", task.ID)
		} else {
			// 其他错误，保持待执行状态
			s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskPending))
		}
	}

	// 推送 WebSocket 通知
	s.hub.SendToUser(task.UserID, ws.Message{
		Type: "exchange_complete",
		Data: map[string]interface{}{
			"task_id":      task.ID,
			"account_name": accountName,
			"product_name": task.PrizeName,
			"success":      success,
			"message":      message,
			"retry_count":  task.RetryCount,
		},
	})
}

func (s *ExchangeService) updateExchangeTaskStatus(taskID uint, status string) {
	if err := s.exchangeTaskRepo.UpdateStatus(taskID, status); err != nil {
		log.Printf("【抢兑任务】更新任务状态失败: task_id=%d status=%s err=%v", taskID, status, err)
	}
}

func (s *ExchangeService) updateExchangeTaskRetryCount(taskID uint, retryCount int, lastRetryAt *time.Time) {
	if err := s.exchangeTaskRepo.UpdateRetryCount(taskID, retryCount, lastRetryAt); err != nil {
		log.Printf("【抢兑任务】更新任务重试次数失败: task_id=%d retry=%d err=%v", taskID, retryCount, err)
	}
}

func (s *ExchangeService) updateExchangeTaskLastResult(taskID uint, result string) {
	if err := s.exchangeTaskRepo.UpdateLastResult(taskID, result); err != nil {
		log.Printf("【抢兑任务】更新任务最后结果失败: task_id=%d err=%v", taskID, err)
	}
}

// shouldRetry 判断是否需要重试（使用公共函数）
func (s *ExchangeService) shouldRetry(message string) bool {
	return utils.IsRetryableError(message)
}

// doExchange 执行兑换请求
func (s *ExchangeService) doExchange(account *models.ExchangeAccount, prizeID string) (bool, string, int) {
	return performExchange(account, prizeID, s.tokenMgr)
}

func (s *ExchangeService) resolveTaskPrizeID(task *models.ExchangeTask) string {
	prizeID := taskExchangePrizeID(task)
	if isUsableExchangePrizeID(prizeID) {
		return prizeID
	}
	if s.productRepo == nil || task == nil || task.PrizeName == "" {
		return prizeID
	}
	product, err := s.productRepo.FindExchangeableReplacement(task.PrizeName, task.PrizeID)
	if err != nil {
		log.Printf("【抢兑任务】任务 %d 查询商品替换失败: %v", task.ID, err)
		return prizeID
	}
	if product == nil || !isUsableExchangePrizeID(product.PrizeID) {
		return prizeID
	}
	if task.PrizeID != product.PrizeID || task.ProductID != product.ID {
		task.PrizeID = product.PrizeID
		task.ProductID = product.ID
		task.Product = *product
		if err := s.exchangeTaskRepo.Update(task); err != nil {
			log.Printf("【抢兑任务】任务 %d 更新商品快照失败: %v", task.ID, err)
		}
		log.Printf("【抢兑任务】任务 %d 已自动修正商品ID为 %s，避免使用历史 memo 导致 404", task.ID, product.PrizeID)
	}
	return product.PrizeID
}

// recordExchangeResult 记录抢兑结果
func (s *ExchangeService) recordExchangeResult(task *models.ExchangeTask, success bool, message string, execTimeMs int) {
	record := &models.ExchangeRecord{
		UserID:            task.UserID,
		ExchangeAccountID: task.ExchangeAccountID,
		ExchangeTaskID:    &task.ID,
		ProductID:         task.ProductID,
		PrizeID:           task.PrizeID,
		PrizeName:         task.PrizeName,
		Status:            string(models.ExchangeRecordSuccess),
		Message:           message,
		ExecutionTimeMs:   execTimeMs,
	}

	if !success {
		record.Status = string(models.ExchangeRecordFailed)
	}

	if err := s.exchangeRecordRepo.Create(record); err != nil {
		log.Printf("【抢兑任务】创建兑换记录失败: task_id=%d account_id=%d prize=%s err=%v", task.ID, task.ExchangeAccountID, task.PrizeName, err)
	}
	if err := s.exchangeTaskRepo.UpdateAttempt(task.ID, success, message); err != nil {
		log.Printf("【抢兑任务】更新任务尝试结果失败: task_id=%d success=%t err=%v", task.ID, success, err)
	}
	createExchangeSystemLog(
		s.taskLogRepo,
		task.UserID,
		task.ExchangeAccount.AccountID,
		task.PrizeName,
		exchangeAccountName(&task.ExchangeAccount),
		success,
		message,
		execTimeMs,
	)
}

// GetExchangeConcurrency 获取抢兑并发数
func (s *ExchangeService) GetExchangeConcurrency() (int, error) {
	config, err := s.configRepo.GetByKey("exchange_concurrency")
	if err != nil {
		return constants.DefaultConcurrency, nil // 使用常量：默认 10 并发
	}

	// 使用 strconv 代替 fmt.Sscanf，避免乱码问题
	if config.KeyValue == "" {
		return 10, nil
	}

	concurrency, err := strconv.Atoi(config.KeyValue)
	if err != nil {
		// 解析失败返回默认值
		return 10, nil
	}

	// 验证范围
	if concurrency <= 0 {
		return 10, nil
	}
	if concurrency > 1000 {
		concurrency = 1000 // 限制最大值
	}

	return concurrency, nil
}

// SetExchangeConcurrency 设置抢兑并发数
func (s *ExchangeService) SetExchangeConcurrency(concurrency int) error {
	// 验证参数
	if concurrency <= 0 {
		return fmt.Errorf("并发数必须大于 0")
	}
	if concurrency > 1000 {
		return fmt.Errorf("并发数不能超过 1000")
	}

	return s.configRepo.UpdateByKey("exchange_concurrency", fmt.Sprintf("%d", concurrency), "抢兑任务并发数量")
}

// GetSystemConfig 获取系统配置
func (s *ExchangeService) GetSystemConfig(key string) (*models.SystemConfig, error) {
	return s.configRepo.GetByKey(key)
}

// SetSystemConfig 设置系统配置
func (s *ExchangeService) SetSystemConfig(key, value, description string) error {
	return s.configRepo.UpdateByKey(key, value, description)
}

// GetExchangeRecords 获取抢兑记录列表
func (s *ExchangeService) GetExchangeRecords(userID uint, accountID uint, productName string, status string, startDate string, endDate string, page int, limit int) ([]*models.ExchangeRecord, int64, error) {
	return s.exchangeTaskRepo.GetRecordsWithFilter(userID, accountID, productName, status, startDate, endDate, page, limit)
}

// GetRecordStats 获取抢兑记录统计信息
func (s *ExchangeService) GetRecordStats(userID uint, startTime, endTime time.Time) (successCount, failCount int64, err error) {
	return s.exchangeRecordRepo.GetStats(userID, startTime, endTime)
}

// ExecuteMonthlyExchange 执行月卡兑换任务（所有账号）
func (s *ExchangeService) ExecuteMonthlyExchange() {
	if !s.acquireMonthlyExchangeRunLock() {
		return
	}

	// 获取所有兑换账号
	accounts, err := s.exchangeAccountRepo.GetAllActive()
	if err != nil {
		// 记录错误日志并发送通知
		log.Printf("【月卡兑换】获取兑换账号列表失败: %v", err)
		s.hub.Broadcast(ws.Message{
			Type: "exchange_error",
			Data: map[string]interface{}{
				"task":    "monthly_exchange",
				"error":   "获取兑换账号列表失败",
				"details": err.Error(),
			},
		})
		return
	}

	if len(accounts) == 0 {
		log.Println("【月卡兑换】没有活跃的兑换账号，跳过执行")
		return
	}

	log.Printf("【月卡兑换】开始执行，共 %d 个账号", len(accounts))

	// 从系统配置获取月卡商品ID，使用常量作为默认值
	monthlyCardPrizeID := constants.DefaultMonthlyCardPrizeID
	if config, err := s.GetSystemConfig("exchange_monthly_prize_id"); err == nil && config.KeyValue != "" {
		monthlyCardPrizeID = config.KeyValue
	}

	// 并发执行兑换
	concurrency := 10
	if config, err := s.GetExchangeConcurrency(); err == nil && config > 0 {
		concurrency = config
	}

	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, account := range accounts {
		wg.Add(1)
		go func(acc *models.ExchangeAccount) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// 执行月卡兑换
			s.executeMonthlyExchangeForAccount(acc, monthlyCardPrizeID)
		}(account)
	}

	wg.Wait()
}

// executeMonthlyExchangeForAccount 为单个账号执行月卡兑换
func (s *ExchangeService) executeMonthlyExchangeForAccount(account *models.ExchangeAccount, prizeID string) {
	if !s.acquireMonthlyExchangeAccountLock(account.ID) {
		return
	}

	startTime := time.Now()
	accountName := account.Remark
	if accountName == "" {
		accountName = account.Phone
	}

	log.Printf("【月卡兑换】开始为账号 %s 执行兑换", accountName)

	// 执行兑换
	success, message, execTime := s.doExchange(account, prizeID)

	// 查找或创建月卡商品记录
	product, err := s.productRepo.GetByPrizeID(prizeID)
	if err != nil {
		// 如果商品不存在，创建一个临时记录
		product = &models.Product{
			PrizeID:    prizeID,
			PrizedName: "移动云盘月卡",
			POrder:     0,
			Category:   "会员权益",
		}
		log.Printf("【月卡兑换】商品 %s 不存在，使用临时记录", prizeID)
	}

	// 记录兑换结果
	record := &models.ExchangeRecord{
		UserID:            account.UserID,
		ExchangeAccountID: account.ID,
		ProductID:         product.ID,
		PrizeID:           prizeID,
		PrizeName:         product.PrizedName,
		Status:            string(models.ExchangeRecordSuccess),
		Message:           message,
		ExecutionTimeMs:   execTime,
	}

	if !success {
		record.Status = string(models.ExchangeRecordFailed)
		log.Printf("【月卡兑换】账号 %s 兑换失败: %s", accountName, message)
	} else {
		log.Printf("【月卡兑换】账号 %s 兑换成功，耗时 %d ms", accountName, execTime)
	}

	if err := s.exchangeRecordRepo.Create(record); err != nil {
		log.Printf("【月卡兑换】记录兑换结果失败: %v", err)
	}

	// 推送 WebSocket 通知
	s.hub.SendToUser(account.UserID, ws.Message{
		Type: "monthly_exchange_complete",
		Data: map[string]interface{}{
			"account_name": accountName,
			"product_name": product.PrizedName,
			"success":      success,
			"message":      message,
			"exec_time":    time.Since(startTime).Milliseconds(),
		},
	})
}

func (s *ExchangeService) acquireMonthlyExchangeRunLock() bool {
	if s.lockStore == nil {
		log.Println("【月卡兑换】未配置分布式锁，继续执行（仅建议单实例本地环境）")
		return true
	}
	key := "exchange:monthly:run:" + time.Now().Format("2006-01-02")
	locked, err := s.lockStore.SetNX(key, "1", 24*time.Hour)
	if err != nil {
		log.Printf("【月卡兑换】获取全局日级锁失败: %v", err)
		return false
	}
	if !locked {
		log.Println("【月卡兑换】今日已执行过，跳过")
		return false
	}
	return true
}

func (s *ExchangeService) acquireMonthlyExchangeAccountLock(accountID uint) bool {
	if s.lockStore == nil {
		return true
	}
	key := fmt.Sprintf("exchange:monthly:acc:%d:%s", accountID, time.Now().Format("2006-01-02"))
	locked, err := s.lockStore.SetNX(key, "1", 25*time.Hour)
	if err != nil {
		log.Printf("【月卡兑换】账号 %d 获取日级锁失败: %v", accountID, err)
		return false
	}
	if !locked {
		log.Printf("【月卡兑换】账号 %d 今日已处理，跳过", accountID)
		return false
	}
	return true
}
