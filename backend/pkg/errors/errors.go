package errors

import (
	"errors"
	"fmt"
)

// 通用业务错误
var (
	ErrNotFound           = errors.New("资源不存在")
	ErrUnauthorized       = errors.New("未授权访问")
	ErrForbidden          = errors.New("权限不足")
	ErrInvalidParams      = errors.New("参数错误")
	ErrInternalServer     = errors.New("服务器内部错误")
	ErrResourceExists     = errors.New("资源已存在")
	ErrOperationFailed    = errors.New("操作失败")
	ErrTimeout            = errors.New("操作超时")
	ErrServiceUnavailable = errors.New("服务不可用")
)

// AppError 应用错误接口
type AppError interface {
	error
	Code() int
	Message() string
	Details() string
	Wrap(err error) AppError
	Is(target error) bool
}

// appError 应用错误实现
type appError struct {
	code    int
	message string
	details string
	err     error
}

// Error 实现 error 接口
func (e *appError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.message, e.err)
	}
	return e.message
}

// Code 返回错误码
func (e *appError) Code() int {
	return e.code
}

// Message 返回错误消息
func (e *appError) Message() string {
	return e.message
}

// Details 返回错误详情
func (e *appError) Details() string {
	return e.details
}

// Wrap 包装错误
func (e *appError) Wrap(err error) AppError {
	if e == nil {
		return Wrap(err, CodeInternalError, ErrInternalServer.Error())
	}
	return &appError{
		code:    e.code,
		message: e.message,
		details: e.details,
		err:     err,
	}
}

// Is 判断错误类型
func (e *appError) Is(target error) bool {
	if target == e.err {
		return true
	}
	if t, ok := target.(*appError); ok {
		return e.code == t.code
	}
	return false
}

// Unwrap 返回包装的错误
func (e *appError) Unwrap() error {
	return e.err
}

// 错误码常量
const (
	CodeBadRequest    = 400
	CodeUnauthorized  = 401
	CodeForbidden     = 403
	CodeNotFound      = 404
	CodeInternalError = 500
)

// New 创建应用错误
func New(code int, message string) AppError {
	return &appError{
		code:    code,
		message: message,
	}
}

// NewWithDetails 创建带详情的应用错误
func NewWithDetails(code int, message, details string) AppError {
	return &appError{
		code:    code,
		message: message,
		details: details,
	}
}

// Wrap 包装错误
func Wrap(err error, code int, message string) AppError {
	return &appError{
		code:    code,
		message: message,
		err:     err,
	}
}

// WrapWithDetails 包装带详情的错误
func WrapWithDetails(err error, code int, message, details string) AppError {
	return &appError{
		code:    code,
		message: message,
		details: details,
		err:     err,
	}
}

// IsNotFound 判断是否未找到错误
func IsNotFound(err error) bool {
	var appErr *appError
	if errors.As(err, &appErr) {
		return appErr.code == CodeNotFound
	}
	return errors.Is(err, ErrNotFound)
}

// IsUnauthorized 判断是否未授权错误
func IsUnauthorized(err error) bool {
	var appErr *appError
	if errors.As(err, &appErr) {
		return appErr.code == CodeUnauthorized
	}
	return errors.Is(err, ErrUnauthorized)
}

// IsForbidden 判断是否禁止访问错误
func IsForbidden(err error) bool {
	var appErr *appError
	if errors.As(err, &appErr) {
		return appErr.code == CodeForbidden
	}
	return errors.Is(err, ErrForbidden)
}

// IsInvalidParams 判断是否参数错误
func IsInvalidParams(err error) bool {
	var appErr *appError
	if errors.As(err, &appErr) {
		return appErr.code == CodeBadRequest
	}
	return errors.Is(err, ErrInvalidParams)
}

// IsInternalError 判断是否内部错误
func IsInternalError(err error) bool {
	var appErr *appError
	if errors.As(err, &appErr) {
		return appErr.code == CodeInternalError
	}
	return errors.Is(err, ErrInternalServer)
}
