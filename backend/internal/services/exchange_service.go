package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/utils"
	"caiyun/internal/ws"
	"encoding/json"
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
	authMgr             *auth.Auth
	tokenMgr            *TokenManager
	hub                 *ws.Hub
}

func NewExchangeService(
	productRepo *repository.ProductRepository,
	exchangeAccountRepo *repository.ExchangeAccountRepository,
	exchangeTaskRepo *repository.ExchangeTaskRepository,
	accountRepo *repository.AccountRepository,
	configRepo *repository.SystemConfigRepository,
	exchangeRecordRepo *repository.ExchangeRecordRepository,
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
		authMgr:             authMgr,
		tokenMgr:            tokenMgr,
		hub:                 ws.GetHub(),
	}
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
func (s *ExchangeService) AddExchangeAccount(userID uint, accountID uint, remark string, exchangeTime1, exchangeTime2 string) (*models.ExchangeAccount, error) {
	// 检查云盘账号是否存在
	account, err := s.accountRepo.GetByID(accountID)
	if err != nil {
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

	return exchangeAccount, nil
}

// GetExchangeAccounts 获取用户的兑换账号列表
func (s *ExchangeService) GetExchangeAccounts(userID uint) ([]*models.ExchangeAccount, error) {
	return s.exchangeAccountRepo.GetByUserID(userID)
}

// UpdateExchangeAccount 更新兑换账号配置
func (s *ExchangeService) UpdateExchangeAccount(id uint, userID uint, remark string, exchangeTime1, exchangeTime2 string, isActive bool) error {
	account, err := s.exchangeAccountRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("兑换账号不存在")
	}

	if account.UserID != userID {
		return fmt.Errorf("无权操作该账号")
	}

	account.Remark = remark
	account.ExchangeTime1 = exchangeTime1
	account.ExchangeTime2 = exchangeTime2
	account.IsActive = isActive

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

	return s.exchangeAccountRepo.Delete(id)
}

// CreateExchangeTask 创建抢兑任务
func (s *ExchangeService) CreateExchangeTask(userID uint, exchangeAccountID uint, productID uint, taskType string, maxAttempts int) (*models.ExchangeTask, error) {
	// 获取商品信息
	product, err := s.productRepo.GetByID(productID)
	if err != nil {
		return nil, fmt.Errorf("商品不存在")
	}

	// 获取兑换账号信息
	exchangeAccount, err := s.exchangeAccountRepo.GetByID(exchangeAccountID)
	if err != nil {
		return nil, fmt.Errorf("兑换账号不存在")
	}

	if exchangeAccount.UserID != userID {
		return nil, fmt.Errorf("无权操作该账号")
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
func (s *ExchangeService) GetExchangeTasks(userID uint) ([]*models.ExchangeTask, error) {
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
	// 更新任务状态为运行中
	s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskRunning))

	// 获取兑换账号
	account, err := s.exchangeAccountRepo.GetByID(task.ExchangeAccountID)
	if err != nil {
		s.recordExchangeResult(task, false, "获取兑换账号失败", 0)
		s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskFailed))
		return
	}

	accountName := account.Remark
	if accountName == "" {
		accountName = account.Phone
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

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("【抢兑任务】任务 %d 第 %d 次重试...", task.ID, attempt)
			// 更新重试次数
			now := time.Now()
			s.exchangeTaskRepo.UpdateRetryCount(task.ID, attempt, &now)
			// 重试间隔：指数退避
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		success, message, execTime = s.doExchange(account, task.PrizeID)

		// 如果成功，或者错误不需要重试，则退出循环
		if success || !s.shouldRetry(message) {
			break
		}
	}

	// 记录结果
	s.recordExchangeResult(task, success, message, execTime)

	// 更新任务状态
	if success {
		log.Printf("【抢兑任务】任务 %d 执行成功，账号: %s", task.ID, accountName)
		// 抢兑成功
		if task.TaskType == string(models.ExchangeTaskFixed) {
			// 固定次数任务，检查是否达到最大次数
			if task.AttemptedCount+1 >= task.MaxAttempts {
				s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskCompleted))
			} else {
				s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
			}
		} else {
			// 长期任务，保持待执行状态
			s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
		}
	} else {
		log.Printf("【抢兑任务】任务 %d 执行失败，账号: %s，原因: %s", task.ID, accountName, message)
		// 抢兑失败
		if strings.Contains(message, "奖品单日已耗尽") || strings.Contains(message, "奖品已兑完") {
			// 奖品已抽完，停止任务
			s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskCompleted))
		} else if task.RetryCount >= maxRetries {
			// 重试次数用尽，标记为失败
			s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskFailed))
			log.Printf("【抢兑任务】任务 %d 重试次数已用尽，标记为失败", task.ID)
		} else {
			// 其他错误，保持待执行状态
			s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
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

// shouldRetry 判断是否需要重试（使用公共函数）
func (s *ExchangeService) shouldRetry(message string) bool {
	return utils.IsRetryableError(message)
}

// doExchange 执行兑换请求
func (s *ExchangeService) doExchange(account *models.ExchangeAccount, prizeID string) (bool, string, int) {
	startTime := time.Now()

	// 创建带认证的HTTP客户端
	client, err := s.tokenMgr.CreateAuthenticatedClient(account.AccountID, account.Auth)
	if err != nil {
		return false, fmt.Sprintf("获取账号 Token 失败：%v", err), int(time.Since(startTime).Milliseconds())
	}

	// 调用兑换 API
	url := utils.BuildExchangeURL(prizeID)
	resp, err := client.Get(url, nil)
	if err != nil {
		return false, fmt.Sprintf("请求失败：%v", err), int(time.Since(startTime).Milliseconds())
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return false, fmt.Sprintf("读取响应失败：%v", err), int(time.Since(startTime).Milliseconds())
	}

	execTime := int(time.Since(startTime).Milliseconds())

	// 解析响应
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return false, fmt.Sprintf("解析响应失败：%v", err), execTime
	}

	msg, ok := response["msg"].(string)
	if !ok {
		return false, "响应格式错误", execTime
	}

	if msg != "success" {
		return false, msg, execTime
	}

	return true, "兑换成功", execTime
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

	s.exchangeRecordRepo.Create(record)
	s.exchangeTaskRepo.UpdateAttempt(task.ID, success, message)
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
