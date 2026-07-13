package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// AddExchangeAccount 添加兑换账号
func (s *ExchangeService) AddExchangeAccount(userID uint, accountID uint, remark string, exchangeTime1, exchangeTime2 string, productID *uint) (*models.ExchangeAccount, error) {
	return s.AddExchangeAccountContext(context.Background(), userID, accountID, remark, exchangeTime1, exchangeTime2, productID)
}

func (s *ExchangeService) AddExchangeAccountContext(ctx context.Context, userID uint, accountID uint, remark string, exchangeTime1, exchangeTime2 string, productID *uint) (*models.ExchangeAccount, error) {
	var created *models.ExchangeAccount
	err := s.withinTransaction(ctx, func(txService *ExchangeService) error {
		account, err := txService.accountRepo.GetByID(accountID)
		if err != nil || account.UserID != userID {
			return ErrExchangeCloudAccountMissing
		}
		existing, err := txService.exchangeAccountRepo.FindByAccountID(accountID)
		if err != nil {
			return fmt.Errorf("query exchange rule: %w", err)
		}
		if existing != nil {
			return ErrExchangeTaskConflict
		}

		created = &models.ExchangeAccount{
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
		if err := txService.exchangeAccountRepo.Create(created); err != nil {
			if errors.Is(err, repository.ErrDuplicateExchangeRule) {
				return ErrExchangeTaskConflict
			}
			return fmt.Errorf("create exchange rule: %w", err)
		}
		if productID != nil && *productID > 0 {
			if err := txService.syncScheduledTaskForAccount(created, *productID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetExchangeAccounts 获取用户的兑换账号列表
func (s *ExchangeService) GetExchangeAccounts(userID uint, isAdmin bool) ([]*models.ExchangeAccount, error) {
	return s.GetExchangeAccountsContext(context.Background(), userID, isAdmin)
}

func (s *ExchangeService) GetExchangeAccountsContext(ctx context.Context, userID uint, isAdmin bool) ([]*models.ExchangeAccount, error) {
	repo := s.exchangeAccountRepo.WithContext(ctx)
	if isAdmin {
		return repo.GetAll()
	}
	return repo.GetByUserID(userID)
}
func (s *ExchangeService) syncScheduledTaskForAccount(exchangeAccount *models.ExchangeAccount, productID uint) error {
	product, err := s.productRepo.GetByID(productID)
	if err != nil {
		return ErrExchangeProductNotFound
	}

	tasks, err := s.exchangeTaskRepo.GetByExchangeAccountID(exchangeAccount.ID)
	if err != nil {
		return fmt.Errorf("load scheduled tasks: %w", err)
	}

	if existingTask := findActiveScheduledTask(tasks); existingTask != nil {
		existingTask.ProductID = product.ID
		existingTask.PrizeID = product.PrizeID
		existingTask.PrizeName = product.PrizeName
		existingTask.TaskType = string(models.ExchangeTaskLongTerm)
		if existingTask.MaxAttempts <= 0 {
			existingTask.MaxAttempts = 10
		}
		if existingTask.Status == "" {
			existingTask.Status = string(models.ExchangeTaskPending)
		}
		if err := s.exchangeTaskRepo.UpdateTaskDefinition(
			existingTask.ID,
			product.ID,
			product.PrizeID,
			product.PrizeName,
			string(models.ExchangeTaskLongTerm),
			existingTask.MaxAttempts,
			existingTask.Status,
		); err != nil {
			return fmt.Errorf("update scheduled task: %w", err)
		}
		return nil
	}

	task := &models.ExchangeTask{
		UserID:            exchangeAccount.UserID,
		ExchangeAccountID: exchangeAccount.ID,
		ProductID:         product.ID,
		PrizeID:           product.PrizeID,
		PrizeName:         product.PrizeName,
		TaskType:          string(models.ExchangeTaskLongTerm),
		MaxAttempts:       10,
		AttemptedCount:    0,
		Status:            string(models.ExchangeTaskPending),
		SuccessCount:      0,
		FailCount:         0,
	}

	if err := s.exchangeTaskRepo.Create(task); err != nil {
		if errors.Is(err, repository.ErrDuplicateActiveExchangeTask) {
			return ErrExchangeTaskConflict
		}
		return fmt.Errorf("create scheduled task: %w", err)
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
	return s.UpdateExchangeAccountContext(context.Background(), id, userID, isAdmin, remark, exchangeTime1, exchangeTime2, isActive, productID)
}

func (s *ExchangeService) UpdateExchangeAccountContext(ctx context.Context, id uint, userID uint, isAdmin bool, remark string, exchangeTime1, exchangeTime2 string, isActive bool, productID *uint) error {
	return s.withinTransaction(ctx, func(txService *ExchangeService) error {
		account, err := txService.exchangeAccountRepo.GetByID(id)
		if err != nil {
			return ErrExchangeRuleNotFound
		}
		if !isAdmin && account.UserID != userID {
			return ErrExchangePermissionDenied
		}
		account.Remark = remark
		account.ExchangeTime1 = exchangeTime1
		account.ExchangeTime2 = exchangeTime2
		account.IsActive = isActive
		if productID != nil && *productID > 0 {
			if err := txService.syncScheduledTaskForAccount(account, *productID); err != nil {
				return err
			}
		}
		if err := txService.exchangeAccountRepo.Update(account); err != nil {
			return fmt.Errorf("update exchange rule: %w", err)
		}
		return nil
	})
}

// DeleteExchangeAccount 删除兑换账号
func (s *ExchangeService) DeleteExchangeAccount(id uint, userID uint) error {
	return s.DeleteExchangeAccountContext(context.Background(), id, userID)
}

func (s *ExchangeService) DeleteExchangeAccountContext(ctx context.Context, id uint, userID uint) error {
	return s.withinTransaction(ctx, func(txService *ExchangeService) error {
		account, err := txService.exchangeAccountRepo.GetByID(id)
		if err != nil {
			return ErrExchangeRuleNotFound
		}
		if account.UserID != userID {
			return ErrExchangePermissionDenied
		}
		tasks, err := txService.exchangeTaskRepo.GetByExchangeAccountID(account.ID)
		if err != nil {
			return fmt.Errorf("load exchange-rule tasks: %w", err)
		}
		for _, task := range tasks {
			if err := txService.exchangeTaskRepo.Delete(task.ID); err != nil {
				return fmt.Errorf("delete exchange-rule task: %w", err)
			}
		}
		if err := txService.exchangeAccountRepo.Delete(id); err != nil {
			return fmt.Errorf("delete exchange rule: %w", err)
		}
		return nil
	})
}

// ExchangeTaskCreateOptions 描述创建抢兑任务时可覆盖的任务级配置。
type ExchangeTaskCreateOptions struct {
	SourceOperationID     string
	TaskType              string
	MaxAttempts           int
	ScheduledExchangeTime string
	RestockCycle          string
	RestockWeekday        *int
	RestockDayOfMonth     *int
	RestockTimes          string
	CustomCron            string
	CalendarPolicy        string
	HolidayDates          string
	WorkdayDates          string
}

// CreateExchangeTaskItemResult 描述批量创建中单个账号/规则的创建结果。
type CreateExchangeTaskItemResult struct {
	TargetType string               `json:"target_type"`
	TargetID   uint                 `json:"target_id"`
	Success    bool                 `json:"success"`
	Message    string               `json:"message"`
	Task       *models.ExchangeTask `json:"task,omitempty"`
}

type CreateExchangeTasksResult struct {
	Tasks   []*models.ExchangeTask
	Errors  []string
	Results []CreateExchangeTaskItemResult
}

func newExchangeTaskCreateOptions(taskType string, maxAttempts int) ExchangeTaskCreateOptions {
	return ExchangeTaskCreateOptions{TaskType: taskType, MaxAttempts: maxAttempts, RestockCycle: "daily"}
}

func (options ExchangeTaskCreateOptions) withDefaults() ExchangeTaskCreateOptions {
	if strings.TrimSpace(options.TaskType) == "" {
		options.TaskType = string(models.ExchangeTaskFixed)
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = 1
	}
	if strings.TrimSpace(options.RestockCycle) == "" {
		options.RestockCycle = "daily"
	}
	if strings.TrimSpace(options.CalendarPolicy) == "" {
		options.CalendarPolicy = "all"
	}
	options.ScheduledExchangeTime = strings.TrimSpace(options.ScheduledExchangeTime)
	options.RestockCycle = strings.TrimSpace(strings.ToLower(options.RestockCycle))
	options.RestockTimes = strings.TrimSpace(options.RestockTimes)
	options.CustomCron = strings.TrimSpace(options.CustomCron)
	options.CalendarPolicy = strings.TrimSpace(strings.ToLower(options.CalendarPolicy))
	options.HolidayDates = strings.TrimSpace(options.HolidayDates)
	options.WorkdayDates = strings.TrimSpace(options.WorkdayDates)
	return options
}

// CreateExchangeTaskByAccountID 使用云盘账号创建抢兑任务。
// 如该云盘账号尚未添加账号规则，会自动创建一条默认规则，避免用户必须先进入“账号规则”模块。
func (s *ExchangeService) CreateExchangeTaskByAccountID(userID uint, accountID uint, productID uint, taskType string, maxAttempts int) (*models.ExchangeTask, error) {
	return s.CreateExchangeTaskByAccountIDWithOptions(userID, accountID, productID, newExchangeTaskCreateOptions(taskType, maxAttempts))
}

func (s *ExchangeService) CreateExchangeTaskByAccountIDWithOptions(userID uint, accountID uint, productID uint, options ExchangeTaskCreateOptions) (*models.ExchangeTask, error) {
	return s.CreateExchangeTaskByAccountIDWithOptionsContext(context.Background(), userID, accountID, productID, options)
}

func (s *ExchangeService) CreateExchangeTaskByAccountIDWithOptionsContext(ctx context.Context, userID uint, accountID uint, productID uint, options ExchangeTaskCreateOptions) (*models.ExchangeTask, error) {
	var created *models.ExchangeTask
	err := s.withinTransaction(ctx, func(txService *ExchangeService) error {
		exchangeAccount, err := txService.ensureExchangeAccountForCloudAccount(userID, accountID)
		if err != nil {
			return err
		}
		created, err = txService.createExchangeTaskWithOptions(userID, exchangeAccount.ID, productID, options)
		return err
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *ExchangeService) ensureExchangeAccountForCloudAccount(userID uint, accountID uint) (*models.ExchangeAccount, error) {
	if accountID == 0 {
		return nil, ErrExchangeInvalidInput
	}

	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return nil, ErrExchangeCloudAccountMissing
	}
	if account.UserID != userID {
		return nil, ErrExchangePermissionDenied
	}
	if !account.IsActive {
		return nil, ErrExchangeAccountDisabled
	}
	if strings.TrimSpace(account.Auth) == "" {
		return nil, ErrExchangeCredentialsMissing
	}

	exchangeAccount, err := s.exchangeAccountRepo.FindByAccountID(accountID)
	if err != nil {
		return nil, fmt.Errorf("query exchange rule: %w", err)
	}
	if exchangeAccount != nil {
		if exchangeAccount.UserID != userID {
			return nil, ErrExchangePermissionDenied
		}
		exchangeAccount.Phone = account.Phone
		exchangeAccount.Auth = account.Auth
		exchangeAccount.Token = account.Token
		exchangeAccount.JWTToken = account.JWTToken
		if strings.TrimSpace(exchangeAccount.Remark) == "" {
			exchangeAccount.Remark = account.Remark
		}
		if strings.TrimSpace(exchangeAccount.ExchangeTime1) == "" {
			exchangeAccount.ExchangeTime1 = "10:00:00"
		}
		if strings.TrimSpace(exchangeAccount.ExchangeTime2) == "" {
			exchangeAccount.ExchangeTime2 = "16:00:00"
		}
		if !exchangeAccount.IsActive {
			exchangeAccount.IsActive = true
		}
		if err := s.exchangeAccountRepo.Update(exchangeAccount); err != nil {
			return nil, fmt.Errorf("sync exchange rule: %w", err)
		}
		return exchangeAccount, nil
	}

	remark := strings.TrimSpace(account.Remark)
	if remark == "" {
		remark = account.Phone
	}
	exchangeAccount = &models.ExchangeAccount{
		UserID:        userID,
		AccountID:     account.ID,
		Phone:         account.Phone,
		Auth:          account.Auth,
		Token:         account.Token,
		JWTToken:      account.JWTToken,
		Remark:        remark,
		ExchangeTime1: "10:00:00",
		ExchangeTime2: "16:00:00",
		IsActive:      true,
	}
	if err := s.exchangeAccountRepo.Create(exchangeAccount); err != nil {
		if errors.Is(err, repository.ErrDuplicateExchangeRule) {
			return nil, ErrExchangeTaskConflict
		}
		return nil, fmt.Errorf("create default exchange rule: %w", err)
	}
	return exchangeAccount, nil
}

func uniqueUintIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// CreateExchangeTasks 批量创建抢兑任务，支持同时传入账号规则 ID 和云盘账号 ID。
func (s *ExchangeService) CreateExchangeTasks(userID uint, exchangeAccountIDs []uint, accountIDs []uint, productID uint, options ExchangeTaskCreateOptions) CreateExchangeTasksResult {
	result := CreateExchangeTasksResult{Tasks: []*models.ExchangeTask{}, Errors: []string{}, Results: []CreateExchangeTaskItemResult{}}
	for _, id := range uniqueUintIDs(exchangeAccountIDs) {
		task, err := s.CreateExchangeTaskWithOptions(userID, id, productID, options)
		item := CreateExchangeTaskItemResult{TargetType: "exchange_rule", TargetID: id}
		if err != nil {
			item.Success = false
			item.Message = exchangeErrorPublicMessage(err)
			result.Errors = append(result.Errors, fmt.Sprintf("账号规则 %d: %s", id, item.Message))
			result.Results = append(result.Results, item)
			continue
		}
		item.Success = true
		item.Message = "创建成功"
		item.Task = task
		result.Tasks = append(result.Tasks, task)
		result.Results = append(result.Results, item)
	}
	for _, id := range uniqueUintIDs(accountIDs) {
		task, err := s.CreateExchangeTaskByAccountIDWithOptions(userID, id, productID, options)
		item := CreateExchangeTaskItemResult{TargetType: "cloud_account", TargetID: id}
		if err != nil {
			item.Success = false
			item.Message = exchangeErrorPublicMessage(err)
			result.Errors = append(result.Errors, fmt.Sprintf("云盘账号 %d: %s", id, item.Message))
			result.Results = append(result.Results, item)
			continue
		}
		item.Success = true
		item.Message = "创建成功"
		item.Task = task
		result.Tasks = append(result.Tasks, task)
		result.Results = append(result.Results, item)
	}
	return result
}

// CreateExchangeTask 创建抢兑任务
func (s *ExchangeService) CreateExchangeTask(userID uint, exchangeAccountID uint, productID uint, taskType string, maxAttempts int) (*models.ExchangeTask, error) {
	return s.CreateExchangeTaskWithOptions(userID, exchangeAccountID, productID, newExchangeTaskCreateOptions(taskType, maxAttempts))
}

func (s *ExchangeService) CreateExchangeTaskWithOptions(userID uint, exchangeAccountID uint, productID uint, options ExchangeTaskCreateOptions) (*models.ExchangeTask, error) {
	return s.CreateExchangeTaskWithOptionsContext(context.Background(), userID, exchangeAccountID, productID, options)
}

func (s *ExchangeService) CreateExchangeTaskWithOptionsContext(ctx context.Context, userID uint, exchangeAccountID uint, productID uint, options ExchangeTaskCreateOptions) (*models.ExchangeTask, error) {
	var created *models.ExchangeTask
	err := s.withinTransaction(ctx, func(txService *ExchangeService) error {
		var err error
		created, err = txService.createExchangeTaskWithOptions(userID, exchangeAccountID, productID, options)
		return err
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *ExchangeService) createExchangeTaskWithOptions(userID uint, exchangeAccountID uint, productID uint, options ExchangeTaskCreateOptions) (*models.ExchangeTask, error) {
	options.SourceOperationID = strings.TrimSpace(options.SourceOperationID)
	if options.SourceOperationID != "" {
		existing, err := s.exchangeTaskRepo.FindBySourceOperationID(options.SourceOperationID)
		if err != nil {
			return nil, fmt.Errorf("query source operation task: %w", err)
		}
		if existing != nil {
			if existing.UserID != userID {
				return nil, ErrExchangePermissionDenied
			}
			return existing, nil
		}
	}
	options = options.withDefaults()
	product, err := s.productRepo.GetByID(productID)
	if err != nil {
		return nil, ErrExchangeProductNotFound
	}
	if !product.IsActive {
		return nil, ErrExchangeProductInactive
	}
	exchangeAccount, err := s.exchangeAccountRepo.GetByID(exchangeAccountID)
	if err != nil {
		return nil, ErrExchangeRuleNotFound
	}
	if exchangeAccount.UserID != userID {
		return nil, ErrExchangePermissionDenied
	}

	candidateTask := &models.ExchangeTask{
		UserID:            userID,
		ExchangeAccountID: exchangeAccountID,
		ProductID:         productID,
		PrizeID:           product.PrizeID,
		PrizeName:         product.PrizeName,
		Product:           *product,
	}
	if skip, _, err := shouldSkipExchangeMonthlySeries(s.exchangeRecordRepo, s.productRepo, candidateTask, time.Now()); err != nil {
		log.Printf("【抢兑月度保护】创建任务时查询本月同系列记录失败，继续创建: %v", err)
	} else if skip {
		return nil, ErrExchangeMonthlyLimitReached
	}

	var sourceOperationID *string
	if options.SourceOperationID != "" {
		sourceOperationID = &options.SourceOperationID
	}
	task := &models.ExchangeTask{
		SourceOperationID:     sourceOperationID,
		UserID:                userID,
		ExchangeAccountID:     exchangeAccountID,
		ProductID:             productID,
		PrizeID:               product.PrizeID,
		PrizeName:             product.PrizeName,
		TaskType:              options.TaskType,
		MaxAttempts:           options.MaxAttempts,
		ScheduledExchangeTime: options.ScheduledExchangeTime,
		RestockCycle:          options.RestockCycle,
		RestockWeekday:        options.RestockWeekday,
		RestockDayOfMonth:     options.RestockDayOfMonth,
		RestockTimes:          options.RestockTimes,
		CustomCron:            options.CustomCron,
		CalendarPolicy:        options.CalendarPolicy,
		HolidayDates:          options.HolidayDates,
		WorkdayDates:          options.WorkdayDates,
		AttemptedCount:        0,
		Status:                string(models.ExchangeTaskPending),
		SuccessCount:          0,
		FailCount:             0,
	}
	if err := s.exchangeTaskRepo.Create(task); err != nil {
		if options.SourceOperationID != "" {
			if existing, lookupErr := s.exchangeTaskRepo.FindBySourceOperationID(options.SourceOperationID); lookupErr == nil && existing != nil {
				if existing.UserID != userID {
					return nil, ErrExchangePermissionDenied
				}
				return existing, nil
			}
		}
		if errors.Is(err, repository.ErrDuplicateActiveExchangeTask) {
			return nil, ErrExchangeTaskAlreadyExists
		}
		return nil, fmt.Errorf("create exchange task: %w", err)
	}
	return task, nil
}

// GetExchangeTasks 获取用户的抢兑任务列表
func (s *ExchangeService) GetExchangeTasks(userID uint, isAdmin bool) ([]*models.ExchangeTask, error) {
	return s.GetExchangeTasksWithFilter(userID, isAdmin, repository.ExchangeTaskFilter{})
}

// GetExchangeTasksWithFilter 获取用户的抢兑任务列表并填充只读调度预览字段。
func (s *ExchangeService) GetExchangeTasksWithFilter(userID uint, isAdmin bool, filter repository.ExchangeTaskFilter) ([]*models.ExchangeTask, error) {
	var (
		tasks []*models.ExchangeTask
		err   error
	)
	if isAdmin {
		tasks, err = s.exchangeTaskRepo.GetAllWithFilter(filter)
	} else {
		tasks, err = s.exchangeTaskRepo.GetByUserIDWithFilter(userID, filter)
	}
	if err != nil {
		return nil, err
	}
	s.fillExchangeTaskRuntimePreview(tasks, time.Now())
	return tasks, nil
}

func (s *ExchangeService) fillExchangeTaskRuntimePreview(tasks []*models.ExchangeTask, now time.Time) {
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if s != nil && s.exchangeTaskRepo != nil {
			task.NextRunAt = s.exchangeTaskRepo.CalculateNextRun(task, now)
		} else {
			task.NextRunAt = repository.CalculateExchangeTaskNextRun(task, now)
		}
	}
}

// UpdateExchangeTask 更新抢兑任务
func (s *ExchangeService) UpdateExchangeTask(id uint, userID uint, maxAttempts int) error {
	return s.UpdateExchangeTaskContext(context.Background(), id, userID, maxAttempts)
}

func (s *ExchangeService) UpdateExchangeTaskContext(ctx context.Context, id uint, userID uint, maxAttempts int) error {
	return s.withinTransaction(ctx, func(txService *ExchangeService) error {
		task, err := txService.exchangeTaskRepo.GetByID(id)
		if err != nil {
			return ErrExchangeTaskNotFound
		}
		if task.UserID != userID {
			return ErrExchangePermissionDenied
		}
		if err := txService.exchangeTaskRepo.UpdateMaxAttempts(task.ID, maxAttempts); err != nil {
			return fmt.Errorf("update exchange task: %w", err)
		}
		return nil
	})
}

// DeleteExchangeTask 删除抢兑任务
func (s *ExchangeService) DeleteExchangeTask(id uint, userID uint) error {
	return s.DeleteExchangeTaskContext(context.Background(), id, userID)
}

func (s *ExchangeService) DeleteExchangeTaskContext(ctx context.Context, id uint, userID uint) error {
	return s.withinTransaction(ctx, func(txService *ExchangeService) error {
		task, err := txService.exchangeTaskRepo.GetByID(id)
		if err != nil {
			return ErrExchangeTaskNotFound
		}
		if task.UserID != userID {
			return ErrExchangePermissionDenied
		}
		if err := txService.exchangeTaskRepo.Delete(id); err != nil {
			return fmt.Errorf("delete exchange task: %w", err)
		}
		return nil
	})
}
