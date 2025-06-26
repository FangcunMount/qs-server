package errors

import "net/http"

// 错误码范围分配：
// 10xxxx - 通用错误
// 11xxxx - 用户相关错误
// 12xxxx - 问卷相关错误
// 13xxxx - 认证授权错误
// 14xxxx - 验证错误
// 15xxxx - 数据库错误
// 16xxxx - 外部服务错误

// 通用错误码定义 (10xxxx)
const (
	// ErrSuccess - 成功
	ErrSuccess int = iota + 100000

	// ErrUnknown - 内部服务器错误
	ErrUnknown
	// ErrBind - 请求参数绑定错误
	ErrBind
	// ErrValidation - 参数验证错误
	ErrValidation
	// ErrTokenInvalid - Token 无效
	ErrTokenInvalid
	// ErrPageNotFound - 页面未找到
	ErrPageNotFound
	// ErrInternalServerError - 内部服务器错误
	ErrInternalServerError
	// ErrRequestTimeout - 请求超时
	ErrRequestTimeout
	// ErrTooManyRequests - 请求过于频繁
	ErrTooManyRequests
	// ErrMethodNotAllowed - 方法不允许
	ErrMethodNotAllowed
	// ErrUnsupportedMediaType - 不支持的媒体类型
	ErrUnsupportedMediaType
	// ErrInvalidJSON - JSON格式错误
	ErrInvalidJSON
	// ErrMissingHeader - 缺少请求头
	ErrMissingHeader
	// ErrInvalidHeader - 请求头无效
	ErrInvalidHeader
	// ErrServiceUnavailable - 服务不可用
	ErrServiceUnavailable
	// ErrBadGateway - 网关错误
	ErrBadGateway
	// ErrGatewayTimeout - 网关超时
	ErrGatewayTimeout
)

// 通用错误码注册
func init() {
	register(ErrSuccess, http.StatusOK, "操作成功", "")
	register(ErrUnknown, http.StatusInternalServerError, "内部服务器错误", "")
	register(ErrBind, http.StatusBadRequest, "请求参数绑定失败", "")
	register(ErrValidation, http.StatusBadRequest, "参数验证失败", "")
	register(ErrTokenInvalid, http.StatusUnauthorized, "Token无效", "")
	register(ErrPageNotFound, http.StatusNotFound, "页面未找到", "")
	register(ErrInternalServerError, http.StatusInternalServerError, "内部服务器错误", "")
	register(ErrRequestTimeout, http.StatusRequestTimeout, "请求超时", "")
	register(ErrTooManyRequests, http.StatusTooManyRequests, "请求过于频繁", "")
	register(ErrMethodNotAllowed, http.StatusMethodNotAllowed, "请求方法不允许", "")
	register(ErrUnsupportedMediaType, http.StatusUnsupportedMediaType, "不支持的媒体类型", "")
	register(ErrInvalidJSON, http.StatusBadRequest, "JSON格式错误", "")
	register(ErrMissingHeader, http.StatusBadRequest, "缺少必要的请求头", "")
	register(ErrInvalidHeader, http.StatusBadRequest, "请求头格式无效", "")
	register(ErrServiceUnavailable, http.StatusServiceUnavailable, "服务暂时不可用", "")
	register(ErrBadGateway, http.StatusBadGateway, "网关错误", "")
	register(ErrGatewayTimeout, http.StatusGatewayTimeout, "网关超时", "")
}
