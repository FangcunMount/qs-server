package interpretation

import (
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func queryModuleNotConfigured(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrModuleInitializationFailed, format, args...)
}

func queryDatabase(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrDatabase, format, args...)
}

func queryInvalidArgument(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrInvalidArgument, format, args...)
}

func queryReportNotFound(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrInterpretReportNotFound, format, args...)
}

// IsReportNotFound identifies the stable report-query not-found contract.
func IsReportNotFound(err error) bool {
	return cberrors.IsCode(err, errorCode.ErrInterpretReportNotFound)
}
