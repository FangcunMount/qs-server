package errors

import (
	"net/http"

	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// ErrCode 实现 pkg/errors.Coder 接口
type ErrCode struct {
	// C 错误码
	C int
	// HTTP HTTP状态码
	HTTP int
	// Ext 外部错误信息
	Ext string
	// Ref 参考文档
	Ref string
}

// Code 返回错误码
func (e ErrCode) Code() int {
	return e.C
}

// String 返回外部错误信息
func (e ErrCode) String() string {
	return e.Ext
}

// HTTPStatus 返回HTTP状态码
func (e ErrCode) HTTPStatus() int {
	return e.HTTP
}

// Reference 返回参考文档
func (e ErrCode) Reference() string {
	return e.Ref
}

// register 注册错误码到全局错误码注册表
func register(code int, httpStatus int, message string, refs string) {
	coder := &ErrCode{
		C:    code,
		HTTP: httpStatus,
		Ext:  message,
		Ref:  refs,
	}

	errors.MustRegister(coder)
}

// NewWithCode 创建带错误码的错误
func NewWithCode(code int, format string, args ...interface{}) error {
	return errors.WithCode(code, format, args...)
}

// WrapWithCode 包装错误并添加错误码
func WrapWithCode(err error, code int, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return errors.WrapC(err, code, format, args...)
}

// ParseCoder 解析错误获取错误码信息
func ParseCoder(err error) errors.Coder {
	return errors.ParseCoder(err)
}

// IsCode 检查错误是否包含指定错误码
func IsCode(err error, code int) bool {
	return errors.IsCode(err, code)
}

// HTTPStatusFromError 从错误中获取HTTP状态码
func HTTPStatusFromError(err error) int {
	if err == nil {
		return http.StatusOK
	}

	coder := ParseCoder(err)
	if coder != nil {
		return coder.HTTPStatus()
	}

	return http.StatusInternalServerError
}

// MessageFromError 从错误中获取用户友好的错误消息
func MessageFromError(err error) string {
	if err == nil {
		return "OK"
	}

	coder := ParseCoder(err)
	if coder != nil {
		return coder.String()
	}

	return "Internal server error"
}

// CodeFromError 从错误中获取错误码
func CodeFromError(err error) int {
	if err == nil {
		return 0
	}

	coder := ParseCoder(err)
	if coder != nil {
		return coder.Code()
	}

	return 1 // 默认未知错误码
}
