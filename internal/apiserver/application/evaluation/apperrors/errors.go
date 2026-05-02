package apperrors

import (
	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func InvalidArgument(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrInvalidArgument, format, args...)
}

func ModuleNotConfigured(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrModuleInitializationFailed, format, args...)
}

func Database(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrDatabase, format, args...)
}

func DatabaseMessage(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrDatabase, format, args...)
}

func AssessmentNotFound(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrAssessmentNotFound, format, args...)
}

func MedicalScaleNotFound(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrMedicalScaleNotFound, format, args...)
}

func AnswerSheetNotFound(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrAnswerSheetNotFound, format, args...)
}

func QuestionnaireNotFound(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrQuestionnaireNotFound, format, args...)
}

func AssessmentInvalidStatus(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrAssessmentInvalidStatus, format, args...)
}

func WrapAssessmentInvalidStatus(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrAssessmentInvalidStatus, format, args...)
}

func AssessmentCreateFailed(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrAssessmentCreateFailed, format, args...)
}

func AssessmentSubmitFailed(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrAssessmentSubmitFailed, format, args...)
}

func AssessmentInterpretFailed(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrAssessmentInterpretFailed, format, args...)
}

func AssessmentScoreNotFound(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrAssessmentScoreNotFound, format, args...)
}

func IsAssessmentScoreNotFound(err error) bool {
	return cberrors.ParseCoder(err).Code() == errorCode.ErrAssessmentScoreNotFound
}

func InterpretReportNotFound(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrInterpretReportNotFound, format, args...)
}

func InterpretReportGenerationFailed(err error, format string, args ...interface{}) error {
	return cberrors.WrapC(err, errorCode.ErrInterpretReportGenerationFailed, format, args...)
}

func PermissionDenied(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrPermissionDenied, format, args...)
}

func Forbidden(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrForbidden, format, args...)
}

func Bind(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrBind, format, args...)
}

func UnsupportedOperation(format string, args ...interface{}) error {
	return cberrors.WithCode(errorCode.ErrUnsupportedOperation, format, args...)
}

func IsUnsupportedOperation(err error) bool {
	return cberrors.IsCode(err, errorCode.ErrUnsupportedOperation)
}
