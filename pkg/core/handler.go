package core

import (
	"net/http"
	"strconv"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/gin-gonic/gin"
)

// BaseHandler 基础Handler结构
// 提供通用的HTTP响应方法
type BaseHandler struct{}

// NewBaseHandler 创建基础Handler
func NewBaseHandler() *BaseHandler {
	return &BaseHandler{}
}

// SuccessResponse 成功响应
func (h *BaseHandler) SuccessResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// SuccessResponseWithMessage 带消息的成功响应
func (h *BaseHandler) SuccessResponseWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

// ErrorResponse 智能错误响应 - 根据错误类型自动选择合适的HTTP状态码和错误码
func (h *BaseHandler) ErrorResponse(c *gin.Context, err error) {
	if err == nil {
		h.Success(c, nil)
		return
	}

	// 记录错误日志
	logger.L(c.Request.Context()).Errorw("HTTP Handler Error",
		"action", "http_error",
		"error", err.Error(),
	)

	var httpStatus int
	var errorCode int
	var message string
	var reference string

	// 尝试解析为内部错误码
	if coder := errors.ParseCoder(err); coder != nil {
		httpStatus = coder.HTTPStatus()
		errorCode = coder.Code()
		message = coder.String()
		reference = coder.Reference()
	} else {
		// 处理未知错误
		httpStatus = http.StatusInternalServerError
		errorCode = 100101 // ErrDatabase 默认值
		message = "内部服务器错误"
	}

	// 发送响应
	c.JSON(httpStatus, Response{
		Code:      errorCode,
		Message:   message,
		Reference: reference,
	})
}

// ErrorResponseWithCode 直接使用错误码的错误响应
func (h *BaseHandler) ErrorResponseWithCode(c *gin.Context, code int, format string, args ...interface{}) {
	err := errors.WithCode(code, format, args...)
	h.Error(c, err)
}

// BadRequestResponse 400错误响应
func (h *BaseHandler) BadRequestResponse(c *gin.Context, message string, err error) {
	if err != nil {
		h.Error(c, errors.Wrap(err, message))
	} else {
		h.ErrorResponseWithCode(c, 100003, "%s", message) // ErrBind
	}
}

// NotFoundResponse 404错误响应
func (h *BaseHandler) NotFoundResponse(c *gin.Context, message string, err error) {
	if err != nil {
		h.Error(c, errors.Wrap(err, message))
	} else {
		h.ErrorResponseWithCode(c, 100102, "%s", message) // ErrPageNotFound
	}
}

// InternalErrorResponse 500错误响应
func (h *BaseHandler) InternalErrorResponse(c *gin.Context, message string, err error) {
	if err != nil {
		h.Error(c, errors.Wrap(err, message))
	} else {
		h.ErrorResponseWithCode(c, 100101, "%s", message) // ErrDatabase
	}
}

// ValidationErrorResponse 参数验证错误响应
func (h *BaseHandler) ValidationErrorResponse(c *gin.Context, field, message string) {
	h.ErrorResponseWithCode(c, 100003, "参数验证失败: %s %s", field, message) // ErrValidation
}

// UnauthorizedResponse 401错误响应
func (h *BaseHandler) UnauthorizedResponse(c *gin.Context, message string) {
	h.ErrorResponseWithCode(c, 100004, "%s", message) // ErrTokenInvalid
}

// ForbiddenResponse 403错误响应
func (h *BaseHandler) ForbiddenResponse(c *gin.Context, message string) {
	h.ErrorResponseWithCode(c, 100010, "%s", message) // ErrPermissionDenied
}

// ConflictResponse 409错误响应
func (h *BaseHandler) ConflictResponse(c *gin.Context, message string, err error) {
	if err != nil {
		h.Error(c, errors.Wrap(err, message))
	} else {
		h.ErrorResponseWithCode(c, 100201, "%s", message) // ErrUserAlreadyExists
	}
}

// BindJSON 绑定JSON参数
func (h *BaseHandler) BindJSON(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		logger.L(c.Request.Context()).Errorw("BindJSON failed",
			"action", "bind_json",
			"path", c.Request.URL.Path,
			"method", c.Request.Method,
			"content_type", c.ContentType(),
			"error", err.Error(),
		)
		h.BadRequestResponse(c, "JSON参数绑定失败", err)
		return err
	}
	return nil
}

// BindQuery 绑定查询参数
func (h *BaseHandler) BindQuery(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindQuery(obj); err != nil {
		h.BadRequestResponse(c, "查询参数绑定失败", err)
		return err
	}
	return nil
}

// BindUri 绑定URI参数
func (h *BaseHandler) BindUri(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindUri(obj); err != nil {
		h.BadRequestResponse(c, "URI参数绑定失败", err)
		return err
	}
	return nil
}

// GetPathParam 获取路径参数
func (h *BaseHandler) GetPathParam(c *gin.Context, key string) string {
	return c.Param(key)
}

// GetQueryParam 获取查询参数
func (h *BaseHandler) GetQueryParam(c *gin.Context, key string) string {
	return c.Query(key)
}

// GetQueryParamInt 获取整数查询参数
func (h *BaseHandler) GetQueryParamInt(c *gin.Context, key string, defaultValue int) int {
	value := c.Query(key)
	if value == "" {
		return defaultValue
	}

	if intValue, err := strconv.Atoi(value); err == nil {
		return intValue
	}

	return defaultValue
}

// ====================== 简化方法名别名 ======================

// Success 成功响应的简化别名
func (h *BaseHandler) Success(c *gin.Context, data interface{}) {
	h.SuccessResponse(c, data)
}

// Error 错误响应的简化别名
func (h *BaseHandler) Error(c *gin.Context, err error) {
	h.ErrorResponse(c, err)
}

// ErrorWithCode 错误码响应的简化别名
func (h *BaseHandler) ErrorWithCode(c *gin.Context, code int, format string, args ...interface{}) {
	h.ErrorResponseWithCode(c, code, format, args...)
}

// BindURI URI参数绑定的别名（统一命名风格）
func (h *BaseHandler) BindURI(c *gin.Context, obj interface{}) error {
	return h.BindUri(c, obj)
}
