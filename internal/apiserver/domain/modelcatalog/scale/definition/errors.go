package definition

import (
	stderrors "errors"
	"fmt"
)

// ErrorKind 划分scale 领域失败 不使用 取决 依赖 API 错误码。
type ErrorKind string

const (
	ErrorKindInvalidArgument ErrorKind = "invalid_argument"
	// ErrorKindRuleFrozen 表示规则已冻结：发布态或归档态量表不允许变更规则。
	ErrorKindRuleFrozen ErrorKind = "rule_frozen"
)

// DomainError 是领域-native error that 应用服务s 映射到 API 编码。
type DomainError struct {
	kind    ErrorKind
	message string
	cause   error
}

func newError(kind ErrorKind, format string, args ...interface{}) error {
	return &DomainError{kind: kind, message: fmt.Sprintf(format, args...)}
}

func (e *DomainError) Error() string {
	if e == nil {
		return ""
	}
	if e.cause == nil {
		return e.message
	}
	return fmt.Sprintf("%s: %v", e.message, e.cause)
}

func (e *DomainError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *DomainError) Kind() ErrorKind {
	if e == nil {
		return ""
	}
	return e.kind
}

// ErrorKindOf 返回首个 scale 领域 error 类型 在错误链中。
func ErrorKindOf(err error) (ErrorKind, bool) {
	var domainErr *DomainError
	if !stderrors.As(err, &domainErr) {
		return "", false
	}
	return domainErr.kind, true
}
