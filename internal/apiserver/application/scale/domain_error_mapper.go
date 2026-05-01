package scale

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func wrapScaleDomainError(err error, fallbackCode int, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return errors.WrapC(err, scaleDomainErrorCode(err, fallbackCode), format, args...)
}

func scaleDomainErrorCode(err error, fallbackCode int) int {
	kind, ok := domainScale.ErrorKindOf(err)
	if !ok {
		return fallbackCode
	}
	switch kind {
	case domainScale.ErrorKindInvalidArgument:
		return errorCode.ErrInvalidArgument
	default:
		return fallbackCode
	}
}
