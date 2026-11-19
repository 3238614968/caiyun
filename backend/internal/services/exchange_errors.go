package services

import (
	"errors"
)

var (
	ErrExchangeInvalidInput        = errors.New("invalid exchange input")
	ErrExchangeCloudAccountMissing = errors.New("exchange cloud account not found")
	ErrExchangeRuleNotFound        = errors.New("exchange rule not found")
	ErrExchangeTaskNotFound        = errors.New("exchange task not found")
	ErrExchangeProductNotFound     = errors.New("exchange product not found")
	ErrExchangePermissionDenied    = errors.New("exchange permission denied")
	ErrExchangeAccountDisabled     = errors.New("exchange account disabled")
	ErrExchangeCredentialsMissing  = errors.New("exchange credentials missing")
	ErrExchangeProductInactive     = errors.New("exchange product inactive")
	ErrExchangeTaskConflict        = errors.New("exchange task conflict")
	ErrExchangeTaskAlreadyExists   = errors.New("exchange task already exists")
	ErrExchangeMonthlyLimitReached = errors.New("exchange monthly limit reached")
	ErrExchangeExecutionFailed     = errors.New("exchange execution failed")
	ErrExchangeOutcomeUnknown      = errors.New("exchange outcome requires reconciliation")
	ErrExchangeBatchPartialFailure = errors.New("exchange batch partially failed")
	ErrExchangeBatchFailed         = errors.New("exchange batch failed")
)

type exchangeOutcomeUnknownError struct{ message string }

func (e *exchangeOutcomeUnknownError) Error() string {
	return e.message
}
func (e *exchangeOutcomeUnknownError) PublicMessage() string {
	return "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"
}
func (e *exchangeOutcomeUnknownError) Is(target error) bool {
	return target == ErrExchangeOutcomeUnknown
}

func newExchangeOutcomeUnknownError(message string) error {
	return &exchangeOutcomeUnknownError{message: message}
}

// exchangeErrorPublicMessage is used in per-item batch results. It must never
// return err.Error(), because unknown errors may contain SQL or upstream data.
func exchangeErrorPublicMessage(err error) string {
	switch {
	case errors.Is(err, ErrExchangeInvalidInput):
		return "请求参数错误"
	case errors.Is(err, ErrExchangeCloudAccountMissing):
		return "云盘账号不存在"
	case errors.Is(err, ErrExchangeRuleNotFound):
		return "账号规则不存在"
	case errors.Is(err, ErrExchangeTaskNotFound):
		return "抢兑任务不存在"
	case errors.Is(err, ErrExchangeProductNotFound):
		return "商品不存在"
	case errors.Is(err, ErrExchangePermissionDenied):
		return "无权执行该操作"
	case errors.Is(err, ErrExchangeAccountDisabled):
		return "云盘账号未启用"
	case errors.Is(err, ErrExchangeCredentialsMissing):
		return "云盘账号鉴权信息缺失"
	case errors.Is(err, ErrExchangeProductInactive):
		return "商品已下架，无法抢兑"
	case errors.Is(err, ErrExchangeTaskAlreadyExists):
		return "该账号在相同兑换配置下已存在此商品的抢兑任务"
	case errors.Is(err, ErrExchangeMonthlyLimitReached):
		return "本月已兑换同系列商品，已触发月度保护"
	case errors.Is(err, ErrExchangeTaskConflict):
		return "抢兑任务冲突，请刷新后重试"
	case errors.Is(err, ErrExchangeOutcomeUnknown):
		return "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"
	case errors.Is(err, ErrExchangeExecutionFailed):
		return "抢兑失败，请查看执行结果"
	case errors.Is(err, ErrExchangeBatchPartialFailure):
		return "批量抢兑部分成功，请查看各任务执行结果"
	case errors.Is(err, ErrExchangeBatchFailed):
		return "批量抢兑失败，请查看各任务执行结果"
	default:
		return "创建失败，请稍后重试"
	}
}
