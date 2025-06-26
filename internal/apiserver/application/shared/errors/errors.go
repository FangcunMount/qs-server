package errors

import (
	"errors"
	"fmt"
)

// 预定义的应用层错误
var (
	// 通用错误
	ErrValidationFailed    = errors.New("validation failed")
	ErrUnauthorized        = errors.New("unauthorized")
	ErrPermissionDenied    = errors.New("permission denied")
	ErrResourceNotFound    = errors.New("resource not found")
	ErrResourceExists      = errors.New("resource already exists")
	ErrConcurrencyConflict = errors.New("concurrency conflict")

	// 业务逻辑错误
	ErrInvalidOperation      = errors.New("invalid operation")
	ErrBusinessRuleViolation = errors.New("business rule violation")
	ErrDataInconsistency     = errors.New("data inconsistency")

	// 系统错误
	ErrInternalServer     = errors.New("internal server error")
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrTimeout            = errors.New("operation timeout")
)

// ErrorType 错误类型
type ErrorType string

const (
	ValidationError    ErrorType = "VALIDATION_ERROR"
	BusinessError      ErrorType = "BUSINESS_ERROR"
	AuthorizationError ErrorType = "AUTHORIZATION_ERROR"
	NotFoundError      ErrorType = "NOT_FOUND_ERROR"
	ConflictError      ErrorType = "CONFLICT_ERROR"
	SystemError        ErrorType = "SYSTEM_ERROR"
)

// ApplicationError 应用层错误
type ApplicationError struct {
	Type    ErrorType              `json:"type"`
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
	Cause   error                  `json:"-"`
}

// Error 实现 error 接口
func (e *ApplicationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap 支持 errors.Unwrap
func (e *ApplicationError) Unwrap() error {
	return e.Cause
}

// NewApplicationError 创建应用层错误
func NewApplicationError(errorType ErrorType, code, message string) *ApplicationError {
	return &ApplicationError{
		Type:    errorType,
		Code:    code,
		Message: message,
		Details: make(map[string]interface{}),
	}
}

// WithCause 添加原因错误
func (e *ApplicationError) WithCause(cause error) *ApplicationError {
	e.Cause = cause
	return e
}

// WithDetail 添加详细信息
func (e *ApplicationError) WithDetail(key string, value interface{}) *ApplicationError {
	if e.Details == nil {
		e.Details = make(map[string]interface{})
	}
	e.Details[key] = value
	return e
}

// 常用错误构造函数

// NewValidationError 创建验证错误
func NewValidationError(field, message string) *ApplicationError {
	return NewApplicationError(ValidationError, "VALIDATION_FAILED", "Validation failed").
		WithDetail("field", field).
		WithDetail("message", message)
}

// NewBusinessError 创建业务错误
func NewBusinessError(code, message string) *ApplicationError {
	return NewApplicationError(BusinessError, code, message)
}

// NewNotFoundError 创建资源不存在错误
func NewNotFoundError(resource, identifier string) *ApplicationError {
	return NewApplicationError(NotFoundError, "RESOURCE_NOT_FOUND",
		fmt.Sprintf("%s not found", resource)).
		WithDetail("resource", resource).
		WithDetail("identifier", identifier)
}

// NewConflictError 创建冲突错误
func NewConflictError(resource, message string) *ApplicationError {
	return NewApplicationError(ConflictError, "RESOURCE_CONFLICT", message).
		WithDetail("resource", resource)
}

// NewAuthorizationError 创建授权错误
func NewAuthorizationError(operation string) *ApplicationError {
	return NewApplicationError(AuthorizationError, "UNAUTHORIZED",
		fmt.Sprintf("Not authorized to %s", operation)).
		WithDetail("operation", operation)
}

// NewSystemError 创建系统错误
func NewSystemError(message string, cause error) *ApplicationError {
	return NewApplicationError(SystemError, "SYSTEM_ERROR", message).
		WithCause(cause)
}

// ValidationErrors 验证错误集合
type ValidationErrors struct {
	Errors []*ApplicationError `json:"errors"`
}

// Error 实现 error 接口
func (ve *ValidationErrors) Error() string {
	if len(ve.Errors) == 0 {
		return "validation failed"
	}
	return ve.Errors[0].Error()
}

// Add 添加验证错误
func (ve *ValidationErrors) Add(field, message string) {
	ve.Errors = append(ve.Errors, NewValidationError(field, message))
}

// HasErrors 是否有错误
func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

// NewValidationErrors 创建验证错误集合
func NewValidationErrors() *ValidationErrors {
	return &ValidationErrors{
		Errors: make([]*ApplicationError, 0),
	}
}

// ErrorMatcher 错误匹配器
type ErrorMatcher struct {
	matchers map[error]ErrorType
}

// NewErrorMatcher 创建错误匹配器
func NewErrorMatcher() *ErrorMatcher {
	return &ErrorMatcher{
		matchers: make(map[error]ErrorType),
	}
}

// Register 注册错误映射
func (em *ErrorMatcher) Register(err error, errorType ErrorType) *ErrorMatcher {
	em.matchers[err] = errorType
	return em
}

// Match 匹配错误类型
func (em *ErrorMatcher) Match(err error) ErrorType {
	for registeredErr, errorType := range em.matchers {
		if errors.Is(err, registeredErr) {
			return errorType
		}
	}
	return SystemError // 默认为系统错误
}

// ToApplicationError 将普通错误转换为应用层错误
func (em *ErrorMatcher) ToApplicationError(err error) *ApplicationError {
	if appErr, ok := err.(*ApplicationError); ok {
		return appErr
	}

	errorType := em.Match(err)
	switch errorType {
	case NotFoundError:
		return NewApplicationError(NotFoundError, "RESOURCE_NOT_FOUND", err.Error()).WithCause(err)
	case ConflictError:
		return NewApplicationError(ConflictError, "RESOURCE_CONFLICT", err.Error()).WithCause(err)
	case ValidationError:
		return NewApplicationError(ValidationError, "VALIDATION_FAILED", err.Error()).WithCause(err)
	case BusinessError:
		return NewApplicationError(BusinessError, "BUSINESS_ERROR", err.Error()).WithCause(err)
	case AuthorizationError:
		return NewApplicationError(AuthorizationError, "UNAUTHORIZED", err.Error()).WithCause(err)
	default:
		return NewApplicationError(SystemError, "SYSTEM_ERROR", "An internal error occurred").WithCause(err)
	}
}
