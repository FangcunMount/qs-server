package shared

import (
	stderrors "errors"

	"github.com/FangcunMount/component-base/pkg/errors"
	assessmentmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/assessmentmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// WrapScaleDomainError 映射领域 scale errors 到 application-layer error 编码。
func WrapScaleDomainError(err error, fallbackCode int, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return errors.WrapC(err, ScaleDomainErrorCode(err, fallbackCode), format, args...)
}

func ScaleDomainErrorCode(err error, fallbackCode int) int {
	return fallbackCode
}

// WrapAssessmentModelError maps assessment model domain errors to application codes.
func WrapAssessmentModelError(err error, fallbackCode int, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	code := fallbackCode
	if stderrors.Is(err, assessmentmodel.ErrInvalidArgument) || stderrors.Is(err, assessmentmodel.ErrInvalidState) {
		code = errorCode.ErrInvalidArgument
	}
	return errors.WrapC(err, code, format, args...)
}
