package services

import (
	"context"
	"fmt"
	"time"

	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/repository"
)

// GetToken 获取账号 Token（优先委托 TokenManager，未注入时回退到数据库+按需刷新路径）。
func (s *AccountService) GetToken(accountID uint) (string, error) {
	if s.tokenProvider != nil {
		tokenInfo, err := s.tokenProvider.GetToken(accountID)
		if err != nil {
			return "", err
		}
		if tokenInfo == nil {
			return "", fmt.Errorf("账号 %d Token 为空", accountID)
		}
		if tokenInfo.SSOToken != "" {
			return tokenInfo.SSOToken, nil
		}
		if tokenInfo.JWTToken != "" {
			return tokenInfo.JWTToken, nil
		}
		return "", fmt.Errorf("账号 %d Token 为空", accountID)
	}

	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return "", err
	}

	if account.Token == "" {
		if err := s.RefreshToken(account); err != nil {
			return "", err
		}
	}
	if account.Token == "" {
		return "", fmt.Errorf("账号 %d Token 为空", accountID)
	}
	return account.Token, nil
}

// RefreshToken 刷新账号Token
func (s *AccountService) RefreshToken(account *models.Account) error {
	return s.RefreshTokenContext(context.Background(), account)
}

// RefreshTokenContext 刷新账号 Token，并将取消传播到认证重试、上游 HTTP 与数据库写入。
func (s *AccountService) RefreshTokenContext(ctx context.Context, account *models.Account) error {
	ctx = accountServiceContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if account == nil {
		return fmt.Errorf("账号为空")
	}

	authClient := corehttp.NewClient()
	if authStr := sanitizeAuthValue(account.Auth); authStr != "" {
		authClient.SetAuth(authStr)
	}
	authForAccount := auth.NewAuth(authClient)

	userDomainID := ""
	jwtToken := account.JWTToken
	if err := ctx.Err(); err != nil {
		return err
	}
	if token, _, err := authForAccount.GetJWTTokenWithSSOTokenContext(ctx, account.Phone); err == nil && token != "" {
		jwtToken = token
		userDomainID = jwtUserDomainID(token)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	refreshed, err := authForAccount.RefreshAuthorizationContext(ctx, account.Auth, account.Phone, userDomainID)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if refreshed.SSOToken != "" {
		if token, err := authForAccount.TyrzLoginContext(ctx, refreshed.SSOToken); err == nil && token != "" {
			jwtToken = token
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	applyAuthorizationRefreshToAccount(account, refreshed, jwtToken)

	return s.persistAuthorizationRefreshContext(ctx, account)
}

func (s *AccountService) persistAuthorizationRefreshContext(ctx context.Context, account *models.Account) error {
	if account == nil {
		return fmt.Errorf("账号为空")
	}
	if s.unitOfWork != nil {
		return s.unitOfWork.WithinTransaction(ctx, func(repos repository.TransactionRepositories) error {
			if err := repos.Account.UpdateAuthorizationFields(account.ID, account.Auth, account.Token, account.JWTToken, account.Platform, account.ExpireAt); err != nil {
				return err
			}
			if s.exchangeRepo != nil {
				if err := repos.ExchangeAccount.UpdateAuthByAccountID(account.ID, account.Auth, account.Token, account.JWTToken); err != nil {
					return fmt.Errorf("同步抢兑账号鉴权失败: %w", err)
				}
			}
			return nil
		})
	}

	accountRepo := s.accountRepositoryWithContext(ctx)
	if err := accountRepo.UpdateAuthorizationFields(account.ID, account.Auth, account.Token, account.JWTToken, account.Platform, account.ExpireAt); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if exchangeRepo := s.exchangeRepositoryWithContext(ctx); exchangeRepo != nil {
		if err := exchangeRepo.UpdateAuthByAccountID(account.ID, account.Auth, account.Token, account.JWTToken); err != nil {
			return fmt.Errorf("同步抢兑账号鉴权失败: %w", err)
		}
	}
	return ctx.Err()
}

// RefreshTokenIfNeeded 根据需要刷新Token
func (s *AccountService) RefreshTokenIfNeeded(account *models.Account) error {
	now := time.Now()
	expireAt := accountAuthorizationExpireAt(account)
	if !authorizationShouldRefresh(expireAt, now) {
		return nil
	}

	if err := s.RefreshToken(account); err != nil {
		// 提前 5 天预刷新失败时，不覆盖数据库，也不阻断仍未过期账号的正常任务。
		if expireAt > now.UnixMilli() {
			return nil
		}
		return err
	}
	return nil
}
