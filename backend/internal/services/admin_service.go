package services

import "caiyun/internal/repository"

// AdminService coordinates administrator use cases. Detailed use cases are
// separated by responsibility in admin_user_management.go,
// admin_account_management.go and admin_reporting.go.
type AdminService struct {
	userRepo       *repository.UserRepository
	accountRepo    *repository.AccountRepository
	taskLogRepo    *repository.TaskLogRepository
	taskConfigRepo *repository.TaskConfigRepository
	unitOfWork     repository.UnitOfWork
}

// NewAdminService creates an administrator service.
func NewAdminService(
	userRepo *repository.UserRepository,
	accountRepo *repository.AccountRepository,
	taskLogRepo *repository.TaskLogRepository,
	taskConfigRepo *repository.TaskConfigRepository,
	unitOfWorks ...repository.UnitOfWork,
) *AdminService {
	unitOfWork := repository.NewUnitOfWorkFromUserRepository(userRepo)
	if len(unitOfWorks) > 0 && unitOfWorks[0] != nil {
		unitOfWork = unitOfWorks[0]
	}
	return &AdminService{
		userRepo:       userRepo,
		accountRepo:    accountRepo,
		taskLogRepo:    taskLogRepo,
		taskConfigRepo: taskConfigRepo,
		unitOfWork:     unitOfWork,
	}
}
