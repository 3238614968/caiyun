package services

import (
	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
	"errors"
)

func (s *AccountService) saveAccountLoginContext(ctx context.Context, userID uint, req *CreateAccountRequest) (*models.Account, error) {
	base := models.Account{UserID: userID, Phone: req.Phone, Auth: req.Auth, Remark: req.Remark, Platform: "pc", IsActive: true}
	if info, err := auth.ParseToken(req.Auth); err == nil && info != nil {
		base.Token, base.ExpireAt = info.Token, info.Expire
		if info.Platform != "" {
			base.Platform = info.Platform
		}
	}
	var account models.Account
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		account = base
		err = s.unitOfWork.WithinTransaction(ctx, func(repos repository.TransactionRepositories) error {
			if err := repos.Account.SaveLogin(&account); err != nil {
				return err
			}
			return repos.ExchangeAccount.ReplaceLoginByAccountID(account.ID, account.Auth, account.Token, account.JWTToken)
		})
		if !errors.Is(err, repository.ErrDuplicateAccountIdentity) {
			break
		}
	}
	if errors.Is(err, repository.ErrDuplicateAccountIdentity) {
		return nil, ErrAccountExists
	}
	if err != nil {
		return nil, err
	}
	if clearer, ok := s.tokenProvider.(interface{ ClearToken(uint) }); ok {
		clearer.ClearToken(account.ID)
	}
	return &account, nil
}
