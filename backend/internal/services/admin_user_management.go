package services

import (
	"caiyun/internal/repository"
	"caiyun/internal/security/authcache"
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound          = errors.New("用户不存在")
	ErrCannotDeleteSelf      = errors.New("不能删除自己")
	ErrCannotDemoteSelf      = errors.New("不能将自己的管理员角色降级")
	ErrCannotRemoveLastAdmin = errors.New("不能移除最后一个管理员")
)

// UserListItem 用户列表项
type UserListItem struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

// GetAllUsers 获取所有用户
func (s *AdminService) GetAllUsers(page, size int, keywords ...string) ([]*UserListItem, int64, error) {
	offset := (page - 1) * size
	keyword := ""
	if len(keywords) > 0 {
		keyword = strings.TrimSpace(keywords[0])
	}
	users, total, err := s.userRepo.Search(keyword, offset, size)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*UserListItem, len(users))
	for i, user := range users {
		result[i] = &UserListItem{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return result, total, nil
}

// UpdateUserRoleRequest 更新用户角色请求
type UpdateUserRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=user admin"`
}

// ResetUserPasswordRequest 管理员重置用户密码请求
type ResetUserPasswordRequest struct {
	Password string `json:"password" binding:"required,min=8"`
}

// UpdateUserRole updates a role and revokes the target user's existing
// sessions. All administrative role changes first lock the complete current
// administrator set in a stable order. This makes the "at least one admin"
// invariant serializable across concurrent demotions and deletions instead of
// relying on a count read made outside the write transaction.
func (s *AdminService) UpdateUserRole(userID, currentUserID uint, req *UpdateUserRoleRequest) error {
	if req == nil {
		return errors.New("update user role request is nil")
	}
	if s.unitOfWork == nil {
		return errors.New("unit of work is not configured")
	}

	roleChanged := false
	err := s.unitOfWork.WithinTransaction(context.Background(), func(repos repository.TransactionRepositories) error {
		// Take the shared guard first for every role mutation. Taking the target
		// row first would let two demotions lock different rows and deadlock when
		// each later tries to lock the rest of the administrator set.
		admins, err := repos.User.LockByRoleForUpdate("admin")
		if err != nil {
			return err
		}
		user, err := repos.User.FindByIDForUpdate(userID)
		if err != nil {
			return ErrUserNotFound
		}
		if user.Role == req.Role {
			return nil
		}
		if userID == currentUserID && user.Role == "admin" && req.Role != "admin" {
			return ErrCannotDemoteSelf
		}
		if user.Role == "admin" && req.Role != "admin" && len(admins) <= 1 {
			return ErrCannotRemoveLastAdmin
		}

		// Keep a compare-and-set fence even while the rows are locked. It protects
		// the invariant if a caller is ever wired to a database/transaction mode
		// where row-lock semantics are weaker than InnoDB's FOR UPDATE.
		updated, err := repos.User.UpdateRoleAndRevokeSessionsIfCurrentRole(user.ID, user.Role, req.Role)
		if err != nil {
			return err
		}
		if !updated {
			return ErrUserNotFound
		}
		roleChanged = true
		return nil
	})
	if err != nil {
		return err
	}
	if roleChanged {
		// Cache invalidation is deliberately after commit so a rolled-back role
		// update cannot evict a still-valid authorization snapshot.
		authcache.Delete(userID)
	}
	return nil
}

// ResetUserPassword 管理员重置用户密码
func (s *AdminService) ResetUserPassword(userID uint, req *ResetUserPasswordRequest) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	if err := validatePasswordStrength(user.Username, req.Password); err != nil {
		return err
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.userRepo.UpdatePasswordAndRevokeSessions(user.ID, string(hashedPassword)); err != nil {
		return err
	}
	// 管理员重置密码后立即失效该用户的认证缓存。
	authcache.Delete(user.ID)
	return nil
}

// DeleteUser 删除用户。禁止删除自己和最后一个管理员，避免管理面锁死。
func (s *AdminService) DeleteUser(userID, currentUserID uint) error {
	return s.DeleteUserContext(context.Background(), userID, currentUserID)
}

// DeleteUserContext atomically erases dependent business data, anonymizes audit
// evidence and soft-deletes the anonymized user identity.
func (s *AdminService) DeleteUserContext(ctx context.Context, userID, currentUserID uint) error {
	if userID == currentUserID {
		return ErrCannotDeleteSelf
	}
	if s.unitOfWork == nil {
		return errors.New("unit of work is not configured")
	}
	err := s.unitOfWork.WithinTransaction(ctx, func(repos repository.TransactionRepositories) error {
		// See UpdateUserRole: lock the entire administrator set before the target
		// row. Delete and role-update paths must share the same lock order so two
		// concurrent operations cannot each conclude that another admin remains.
		admins, err := repos.User.LockByRoleForUpdate("admin")
		if err != nil {
			return err
		}
		user, err := repos.User.FindByIDForUpdate(userID)
		if err != nil {
			return ErrUserNotFound
		}
		if user.Role == "admin" {
			if len(admins) <= 1 {
				return ErrCannotRemoveLastAdmin
			}
		}

		cleanup := []func(uint) error{
			repos.Operation.DeleteByUserID,
			repos.RefreshSession.DeleteByUserID,
			repos.ExchangeRecord.DeleteByUserID,
			repos.ExchangeTask.DeleteByUserID,
			repos.ExchangeAccount.DeleteByUserID,
			repos.TaskLog.DeleteByUserID,
			repos.CloudStats.DeleteByUserID,
			repos.Account.DeleteByUserID,
			repos.WSMessage.DeleteByUserID,
		}
		for _, erase := range cleanup {
			if err := erase(userID); err != nil {
				return err
			}
		}
		if err := repos.AuditLog.AnonymizeByUserID(userID); err != nil {
			return err
		}
		return repos.User.AnonymizeAndDelete(userID)
	})
	if err != nil {
		return err
	}
	// Cache invalidation is deliberately after commit; a rolled-back deletion
	// must not evict a still-valid user snapshot.
	authcache.Delete(userID)
	return nil
}
