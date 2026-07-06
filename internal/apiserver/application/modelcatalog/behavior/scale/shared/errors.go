package shared

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/definition"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// WrapScaleDomainError maps domain scale errors to application-layer error codes.
func WrapScaleDomainError(err error, fallbackCode int, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return errors.WrapC(err, ScaleDomainErrorCode(err, fallbackCode), format, args...)
}

func ScaleDomainErrorCode(err error, fallbackCode int) int {
	kind, ok := scaledefinition.ErrorKindOf(err)
	if !ok {
		return fallbackCode
	}
	switch kind {
	case scaledefinition.ErrorKindInvalidArgument, scaledefinition.ErrorKindRuleFrozen:
		return errorCode.ErrInvalidArgument
	default:
		return fallbackCode
	}
}
