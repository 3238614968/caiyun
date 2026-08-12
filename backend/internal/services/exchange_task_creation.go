package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/repository"
)

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
