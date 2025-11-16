package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/errors"
)

// BaseHandler 基础Handler结构
// 提供通用的HTTP响应方法
type BaseHandler struct{}

// NewBaseHandler 创建基础Handler
func NewBaseHandler() *BaseHandler {
	return &BaseHandler{}
}

// Response 统一响应结构
type Response struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
	Reference string      `json:"reference,omitempty"`
}

// SuccessResponse 成功响应
func (h *BaseHandler) SuccessResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    code.ErrSuccess,
		Message: "操作成功",
		Data:    data,
	})
}

// SuccessResponseWithMessage 带消息的成功响应
func (h *BaseHandler) SuccessResponseWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    code.ErrSuccess,
		Message: message,
		Data:    data,
	})
}

// ErrorResponse 智能错误响应 - 根据错误类型自动选择合适的HTTP状态码和错误码
func (h *BaseHandler) ErrorResponse(c *gin.Context, err error) {
	if err == nil {
		h.SuccessResponse(c, nil)
		return
	}

	// 记录错误日志
	log.Errorf("HTTP Handler Error: %+v", err)

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
		errorCode = code.ErrDatabase
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
	h.ErrorResponse(c, err)
}

// BadRequestResponse 400错误响应
func (h *BaseHandler) BadRequestResponse(c *gin.Context, message string, err error) {
	if err != nil {
		h.ErrorResponse(c, errors.Wrap(err, message))
	} else {
		h.ErrorResponseWithCode(c, code.ErrBind, "%s", message)
	}
}

// NotFoundResponse 404错误响应
func (h *BaseHandler) NotFoundResponse(c *gin.Context, message string, err error) {
	if err != nil {
		h.ErrorResponse(c, errors.Wrap(err, message))
	} else {
		h.ErrorResponseWithCode(c, code.ErrPageNotFound, "%s", message)
	}
}

// InternalErrorResponse 500错误响应
func (h *BaseHandler) InternalErrorResponse(c *gin.Context, message string, err error) {
	if err != nil {
		h.ErrorResponse(c, errors.Wrap(err, message))
	} else {
		h.ErrorResponseWithCode(c, code.ErrDatabase, "%s", message)
	}
}

// ValidationErrorResponse 参数验证错误响应
func (h *BaseHandler) ValidationErrorResponse(c *gin.Context, field, message string) {
	h.ErrorResponseWithCode(c, code.ErrValidation, "参数验证失败: %s %s", field, message)
}

// UnauthorizedResponse 401错误响应
func (h *BaseHandler) UnauthorizedResponse(c *gin.Context, message string) {
	h.ErrorResponseWithCode(c, code.ErrTokenInvalid, "%s", message)
}

// ForbiddenResponse 403错误响应
func (h *BaseHandler) ForbiddenResponse(c *gin.Context, message string) {
	h.ErrorResponseWithCode(c, code.ErrPermissionDenied, "%s", message)
}

// ConflictResponse 409错误响应
func (h *BaseHandler) ConflictResponse(c *gin.Context, message string, err error) {
	if err != nil {
		h.ErrorResponse(c, errors.Wrap(err, message))
	} else {
		h.ErrorResponseWithCode(c, code.ErrUserAlreadyExists, "%s", message)
	}
}

// BindJSON 绑定JSON参数
func (h *BaseHandler) BindJSON(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindJSON(obj); err != nil {
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

// GetUserID 从上下文获取当前用户ID（需要认证中间件设置）
func (h *BaseHandler) GetUserID(c *gin.Context) (string, bool) {
	userID, exists := c.Get("user_id")
	if !exists || userID == nil {
		return "", false
	}

	if id, ok := userID.(string); ok {
		return id, true
	}

	return "", false
}
