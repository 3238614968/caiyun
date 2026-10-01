package handlers

import (
	"caiyun/internal/security"
	"caiyun/internal/services"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

func respondAccountWriteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidPhone):
		respondError(c, http.StatusBadRequest, "手机号格式不正确")
	case errors.Is(err, services.ErrInvalidAuthorization):
		respondError(c, http.StatusBadRequest, "账号认证信息不能为空")
	case errors.Is(err, services.ErrAccountExists):
		respondError(c, http.StatusConflict, "该用户下此手机号已存在，请刷新账号列表后更新登录信息")
	case errors.Is(err, security.ErrCredentialUnreadable):
		respondError(c, http.StatusConflict, "旧账号凭据无法解密，请重新登录更新认证信息")
	default:
		// Preserve the cause for trace_id diagnostics; do not expose credentials
		// or database details in the HTTP response.
		respondInternalServer(c, err)
	}
}
