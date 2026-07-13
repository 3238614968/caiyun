package errors

// BusinessCode 是稳定的业务错误码；HTTP status 只表达传输语义，业务码用于前端和调用方精确分支处理。
type BusinessCode string

const (
	BusinessCodeAuthInvalidCredentials BusinessCode = "AUTH_INVALID_CREDENTIALS"
	BusinessCodeAuthSessionExpired     BusinessCode = "AUTH_SESSION_EXPIRED"
	BusinessCodeAuthPermissionDenied   BusinessCode = "AUTH_PERMISSION_DENIED"
	BusinessCodeIdentityUsernameExists BusinessCode = "IDENTITY_USERNAME_EXISTS"
	BusinessCodeIdentityEmailExists    BusinessCode = "IDENTITY_EMAIL_EXISTS"

	BusinessCodeAccountNotFound     BusinessCode = "ACCOUNT_NOT_FOUND"
	BusinessCodeAccountDisabled     BusinessCode = "ACCOUNT_DISABLED"
	BusinessCodeAccountTokenExpired BusinessCode = "ACCOUNT_TOKEN_EXPIRED"

	BusinessCodeTaskAlreadyRunning BusinessCode = "TASK_ALREADY_RUNNING"
	BusinessCodeTaskTimeout        BusinessCode = "TASK_TIMEOUT"
	BusinessCodeTaskRetryExceeded  BusinessCode = "TASK_RETRY_EXCEEDED"

	BusinessCodeExchangeProductNotFound  BusinessCode = "EXCHANGE_PRODUCT_NOT_FOUND"
	BusinessCodeExchangeStockUnavailable BusinessCode = "EXCHANGE_STOCK_UNAVAILABLE"
	BusinessCodeExchangeDuplicate        BusinessCode = "EXCHANGE_DUPLICATE"
	BusinessCodeExchangeRuleDisabled     BusinessCode = "EXCHANGE_RULE_DISABLED"
	BusinessCodeExchangeTaskConflict     BusinessCode = "EXCHANGE_TASK_CONFLICT"
)
