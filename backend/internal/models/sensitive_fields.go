package models

import (
	"caiyun/internal/security"

	"gorm.io/gorm"
)

func protectSensitiveCredentialFields(fields ...*string) error {
	for _, field := range fields {
		if field == nil {
			continue
		}
		encrypted, err := security.EncryptString(*field)
		if err != nil {
			return err
		}
		*field = encrypted
	}
	return nil
}

func revealSensitiveCredentialFields(fields ...*string) error {
	for _, field := range fields {
		if field == nil {
			continue
		}
		decrypted, err := security.DecryptString(*field)
		if err != nil {
			return err
		}
		*field = decrypted
	}
	return nil
}

func (a *Account) BeforeSave(tx *gorm.DB) error {
	return protectSensitiveCredentialFields(&a.Auth, &a.Token, &a.JWTToken)
}

func (a *Account) AfterFind(tx *gorm.DB) error {
	return revealSensitiveCredentialFields(&a.Auth, &a.Token, &a.JWTToken)
}

func (a *Account) AfterSave(tx *gorm.DB) error {
	return revealSensitiveCredentialFields(&a.Auth, &a.Token, &a.JWTToken)
}

func (r *ExchangeRule) BeforeSave(tx *gorm.DB) error {
	return protectSensitiveCredentialFields(&r.Auth, &r.Token, &r.JWTToken)
}

func (r *ExchangeRule) AfterFind(tx *gorm.DB) error {
	return revealSensitiveCredentialFields(&r.Auth, &r.Token, &r.JWTToken)
}

func (r *ExchangeRule) AfterSave(tx *gorm.DB) error {
	return revealSensitiveCredentialFields(&r.Auth, &r.Token, &r.JWTToken)
}
