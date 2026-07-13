package response

import (
	stderrors "errors"
	"net/http"

	appErrors "caiyun/pkg/errors"
	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code         int         `json:"code"`
	BusinessCode string      `json:"business_code,omitempty"`
	Message      string      `json:"message"`
	Data         interface{} `json:"data,omitempty"`
	TraceID      string      `json:"trace_id,omitempty"`
}

// PageData 分页数据
type PageData struct {
	List     interface{} `json:"list"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
}

func traceIDFromContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if traceID := c.Writer.Header().Get("X-Trace-ID"); traceID != "" {
		return traceID
	}
	if traceID := c.Writer.Header().Get("X-Request-ID"); traceID != "" {
		return traceID
	}
	if traceID := c.GetString("request_id"); traceID != "" {
		return traceID
	}
	return ""
}

func responseBody(c *gin.Context, code int, message string, data interface{}) Response {
	return Response{
		Code:    code,
		Message: message,
		Data:    data,
		TraceID: traceIDFromContext(c),
	}
}

func responseBodyWithBusinessCode(c *gin.Context, code int, businessCode string, message string, data interface{}) Response {
	body := responseBody(c, code, message, data)
	body.BusinessCode = businessCode
	return body
}

func jsonResponse(c *gin.Context, httpStatus, code int, message string, data interface{}) {
	c.JSON(httpStatus, responseBody(c, code, message, data))
}

func jsonResponseWithBusinessCode(c *gin.Context, httpStatus, code int, businessCode string, message string, data interface{}) {
	c.JSON(httpStatus, responseBodyWithBusinessCode(c, code, businessCode, message, data))
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	jsonResponse(c, http.StatusOK, 0, "success", data)
}

// SuccessWithMessage 带消息的成功响应
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	jsonResponse(c, http.StatusOK, 0, message, data)
}

// Accepted returns a durable asynchronous operation accepted response (202).
func Accepted(c *gin.Context, data interface{}) {
	jsonResponse(c, http.StatusAccepted, 0, "accepted", data)
}

// SuccessCreated 创建成功的响应 (201)
func SuccessCreated(c *gin.Context, data interface{}) {
	jsonResponse(c, http.StatusCreated, 0, "创建成功", data)
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
		jsonResponse(c, appErr.Code(), appErr.Code(), appErr.Message(), nil)
		return
	}

	// 普通 error：客户端只看到通用消息，详细信息写入 gin context 供审计中间件记录。
	recordInternalError(c, err)
	jsonResponse(c, http.StatusInternalServerError, http.StatusInternalServerError, genericInternalMessage, nil)
}

// ErrorWithCode 指定错误码的错误响应
func ErrorWithCode(c *gin.Context, code int, message string) {
	jsonResponse(c, code, code, message, nil)
}

// ErrorWithData 指定错误码并附带结构化错误上下文。
func ErrorWithData(c *gin.Context, code int, message string, data interface{}) {
	jsonResponse(c, code, code, message, data)
}

// ErrorWithBusinessCode 返回 HTTP 状态码与稳定业务错误码分离的错误响应。
func ErrorWithBusinessCode(c *gin.Context, httpStatus int, businessCode string, message string) {
	jsonResponseWithBusinessCode(c, httpStatus, httpStatus, businessCode, message, nil)
}

// BadRequest 400 错误
func BadRequest(c *gin.Context, message string) {
	jsonResponse(c, http.StatusBadRequest, http.StatusBadRequest, message, nil)
}

// Unauthorized 401 错误
func Unauthorized(c *gin.Context, message string) {
	jsonResponse(c, http.StatusUnauthorized, http.StatusUnauthorized, message, nil)
}

// Forbidden 403 错误
func Forbidden(c *gin.Context, message string) {
	jsonResponse(c, http.StatusForbidden, http.StatusForbidden, message, nil)
}

// NotFound 404 错误
func NotFound(c *gin.Context, message string) {
	jsonResponse(c, http.StatusNotFound, http.StatusNotFound, message, nil)
}

// Conflict 409 错误
func Conflict(c *gin.Context, message string) {
	jsonResponse(c, http.StatusConflict, http.StatusConflict, message, nil)
}

// InternalServer 500 错误
func InternalServer(c *gin.Context, message string) {
	jsonResponse(c, http.StatusInternalServerError, http.StatusInternalServerError, message, nil)
}

// ServiceUnavailable 503 错误
func ServiceUnavailable(c *gin.Context, message string) {
	jsonResponse(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, message, nil)
}

// Timeout 504 错误
func Timeout(c *gin.Context, message string) {
	jsonResponse(c, http.StatusGatewayTimeout, http.StatusGatewayTimeout, message, nil)
}

// Pagination 分页响应
func Pagination(c *gin.Context, list interface{}, total int64, page, pageSize int) {
	jsonResponse(c, http.StatusOK, 0, "success", PageData{
		List:     list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// List 列表响应（不带分页信息）
func List(c *gin.Context, list interface{}) {
	jsonResponse(c, http.StatusOK, 0, "success", list)
}

// Count 数量响应
func Count(c *gin.Context, count int64) {
	jsonResponse(c, http.StatusOK, 0, "success", gin.H{"count": count})
}

// ID 返回资源 ID
func ID(c *gin.Context, id uint) {
	jsonResponse(c, http.StatusOK, 0, "success", gin.H{"id": id})
}

// Message 只返回消息
func Message(c *gin.Context, message string) {
	jsonResponse(c, http.StatusOK, 0, message, nil)
}

// WrapData 包装数据
func WrapData(c *gin.Context, data interface{}, message string) {
	if message == "" {
		message = "success"
	}
	jsonResponse(c, http.StatusOK, 0, message, data)
}

// HandleAppError 处理应用错误并返回响应
func HandleAppError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	var appErr appErrors.AppError
	if stderrors.As(err, &appErr) {
		jsonResponse(c, appErr.Code(), appErr.Code(), appErr.Message(), nil)
		return
	}

	// 非 AppError：客户端只看到通用消息，详细信息记录到 gin context。
	recordInternalError(c, err)
	jsonResponse(c, http.StatusInternalServerError, http.StatusInternalServerError, genericInternalMessage, nil)
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
	if traceID := traceIDFromContext(c); traceID != "" {
		response["trace_id"] = traceID
	}
	c.JSON(http.StatusOK, response)
}
