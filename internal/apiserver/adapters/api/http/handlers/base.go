package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler 通用Handler接口
// 所有业务Handler都应该实现这个接口以支持自动注册
type Handler interface {
	// GetName 获取Handler名称（用于注册）
	GetName() string
}

// BaseHandler 基础Handler结构
// 提供通用的HTTP响应方法
type BaseHandler struct{}

// NewBaseHandler 创建基础Handler
func NewBaseHandler() *BaseHandler {
	return &BaseHandler{}
}

// SuccessResponse 成功响应
func (h *BaseHandler) SuccessResponse(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "success",
		"data":    data,
	})
}

// SuccessResponseWithMessage 带消息的成功响应
func (h *BaseHandler) SuccessResponseWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": message,
		"data":    data,
	})
}

// ErrorResponse 错误响应
func (h *BaseHandler) ErrorResponse(c *gin.Context, code int, message string, err error) {
	response := gin.H{
		"code":    code,
		"message": message,
	}

	if err != nil {
		response["error"] = err.Error()
	}

	c.JSON(code, response)
}

// BadRequestResponse 400错误响应
func (h *BaseHandler) BadRequestResponse(c *gin.Context, message string, err error) {
	h.ErrorResponse(c, http.StatusBadRequest, message, err)
}

// NotFoundResponse 404错误响应
func (h *BaseHandler) NotFoundResponse(c *gin.Context, message string, err error) {
	h.ErrorResponse(c, http.StatusNotFound, message, err)
}

// InternalErrorResponse 500错误响应
func (h *BaseHandler) InternalErrorResponse(c *gin.Context, message string, err error) {
	h.ErrorResponse(c, http.StatusInternalServerError, message, err)
}

// BindJSON 绑定JSON参数
func (h *BaseHandler) BindJSON(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindJSON(obj); err != nil {
		h.BadRequestResponse(c, "参数错误", err)
		return err
	}
	return nil
}

// BindQuery 绑定查询参数
func (h *BaseHandler) BindQuery(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindQuery(obj); err != nil {
		h.BadRequestResponse(c, "参数错误", err)
		return err
	}
	return nil
}

// BindURI 绑定URI参数
func (h *BaseHandler) BindURI(c *gin.Context, obj interface{}) error {
	if err := c.ShouldBindUri(obj); err != nil {
		h.BadRequestResponse(c, "参数错误", err)
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

// GetQueryParamWithDefault 获取查询参数（带默认值）
func (h *BaseHandler) GetQueryParamWithDefault(c *gin.Context, key, defaultValue string) string {
	value := c.Query(key)
	if value == "" {
		return defaultValue
	}
	return value
}
