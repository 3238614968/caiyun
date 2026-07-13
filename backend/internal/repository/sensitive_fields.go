package repository

import "caiyun/internal/security"

func encryptCredentialValue(value string) (string, error) {
	return security.EncryptString(value)
}
