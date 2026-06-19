package response

import (
	stderrors "errors"
	"net/http"

	appErrors "caiyun/pkg/errors"
	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PageData 分页数据
type PageData struct {
	List     interface{} `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// SuccessWithMessage 带消息的成功响应
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

// SuccessCreated 创建成功的响应 (201)
func SuccessCreated(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Response{
		Code:    0,
		Message: "创建成功",
		Data:    data,
	})
}

// SuccessNoContent 删除成功无内容返回 (204)
func SuccessNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// genericInternalMessage 是非 AppError 时返回给客户端的通用消息，
// 避免把 GORM / SQL / 上游响应体等内部细节泄漏给调用方。
const genericInternalMessage = "服务器内部错误，请稍后再试"

// Error 错误响应
func Error(c *gin.Context, err error) {
	// 检查是否是 AppError
	var appErr appErrors.AppError
	if stderrors.As(err, &appErr) {
		c.JSON(appErr.Code(), Response{
			Code:    appErr.Code(),
			Message: appErr.Message(),
		})
		return
	}

	// 普通 error：客户端只看到通用消息，详细信息写入 gin context 供审计中间件记录。
	recordInternalError(c, err)
	c.JSON(http.StatusInternalServerError, Response{
		Code:    http.StatusInternalServerError,
		Message: genericInternalMessage,
	})
}

// ErrorWithCode 指定错误码的错误响应
func ErrorWithCode(c *gin.Context, code int, message string) {
	c.JSON(code, Response{
		Code:    code,
		Message: message,
	})
}

// ErrorWithData 指定错误码并附带结构化错误上下文。
func ErrorWithData(c *gin.Context, code int, message string, data interface{}) {
	c.JSON(code, Response{
		Code:    code,
		Message: message,
		Data:    data,
	})
}

// BadRequest 400 错误
func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, Response{
		Code:    http.StatusBadRequest,
		Message: message,
	})
}

// Unauthorized 401 错误
func Unauthorized(c *gin.Context, message string) {
	c.JSON(http.StatusUnauthorized, Response{
		Code:    http.StatusUnauthorized,
		Message: message,
	})
}

// Forbidden 403 错误
func Forbidden(c *gin.Context, message string) {
	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Message: message,
	})
}

// NotFound 404 错误
func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, Response{
		Code:    http.StatusNotFound,
		Message: message,
	})
}

// Conflict 409 错误
func Conflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, Response{
		Code:    http.StatusConflict,
		Message: message,
	})
}

// InternalServer 500 错误
func InternalServer(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, Response{
		Code:    http.StatusInternalServerError,
		Message: message,
	})
}

// ServiceUnavailable 503 错误
func ServiceUnavailable(c *gin.Context, message string) {
	c.JSON(http.StatusServiceUnavailable, Response{
		Code:    http.StatusServiceUnavailable,
		Message: message,
	})
}

// Timeout 504 错误
func Timeout(c *gin.Context, message string) {
	c.JSON(http.StatusGatewayTimeout, Response{
		Code:    http.StatusGatewayTimeout,
		Message: message,
	})
}

// Pagination 分页响应
func Pagination(c *gin.Context, list interface{}, total int64, page, pageSize int) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: PageData{
			List:     list,
			Total:    total,
			Page:     page,
			PageSize: pageSize,
		},
	})
}

// List 列表响应（不带分页信息）
func List(c *gin.Context, list interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    list,
	})
}

// Count 数量响应
func Count(c *gin.Context, count int64) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    gin.H{"count": count},
	})
}

// ID 返回资源 ID
func ID(c *gin.Context, id uint) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    gin.H{"id": id},
	})
}

// Message 只返回消息
func Message(c *gin.Context, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: message,
	})
}

// WrapData 包装数据
func WrapData(c *gin.Context, data interface{}, message string) {
	if message == "" {
		message = "success"
	}
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

// HandleAppError 处理应用错误并返回响应
func HandleAppError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	var appErr appErrors.AppError
	if stderrors.As(err, &appErr) {
		c.JSON(appErr.Code(), Response{
			Code:    appErr.Code(),
			Message: appErr.Message(),
		})
		return
	}

	// 非 AppError：客户端只看到通用消息，详细信息记录到 gin context。
	recordInternalError(c, err)
	c.JSON(http.StatusInternalServerError, Response{
		Code:    http.StatusInternalServerError,
		Message: genericInternalMessage,
	})
}

// internalErrorLogKey 用于把内部错误详情写入 gin context，供审计中间件记录到 ErrorMsg。
const internalErrorLogKey = "_internal_error_detail"

// recordInternalError 把 err 的完整信息记录到 gin context 与标准库 log，
// 但不写入 HTTP 响应。审计中间件会读取 c.Errors 输出到审计日志。
func recordInternalError(c *gin.Context, err error) {
	if err == nil || c == nil {
		return
	}
	_ = c.Error(err)
	c.Set(internalErrorLogKey, err.Error())
}

// WithDataAndMeta 带元数据的响应
func WithDataAndMeta(c *gin.Context, data interface{}, meta map[string]interface{}) {
	response := gin.H{
		"code":    0,
		"message": "success",
		"data":    data,
	}
	if meta != nil {
		response["meta"] = meta
	}
	c.JSON(http.StatusOK, response)
}
