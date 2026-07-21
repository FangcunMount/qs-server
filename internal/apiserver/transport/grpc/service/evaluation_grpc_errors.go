package service

import (
	"context"
	"errors"

	pkgerrors "github.com/FangcunMount/component-base/pkg/errors"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// toEvaluationGRPCError maps Evaluation Ensure/Execute application errors to
// gRPC status codes (EV-R017). Public messages stay safe; the stable class is
// returned via status message prefix "evaluation:<class>: ..." only for
// InvalidArgument/FailedPrecondition/NotFound where the message is already
// operator-facing. Internal/Unavailable never leak SQL or full error chains.
func toEvaluationGRPCError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	}
	if errors.Is(err, evalrun.ErrInvalidClaim) ||
		errors.Is(err, evalrun.ErrInvalidTransition) ||
		errors.Is(err, evalrun.ErrInputSnapshotConflict) {
		return status.Error(codes.Aborted, "evaluation run claim conflict")
	}

	coder := pkgerrors.ParseCoder(err)
	switch coder.Code() {
	case errorCode.ErrInvalidArgument, errorCode.ErrBind, errorCode.ErrAssessmentInvalidArgument:
		return status.Error(codes.InvalidArgument, coder.String())
	case errorCode.ErrAssessmentInvalidStatus,
		errorCode.ErrAssessmentNoScale,
		errorCode.ErrAssessmentQuestionnaireNotPublished,
		errorCode.ErrAssessmentAnswerSheetMismatch,
		errorCode.ErrAssessmentScaleNotLinked,
		errorCode.ErrAssessmentModelValidationFailed,
		errorCode.ErrModuleInitializationFailed:
		return status.Error(codes.FailedPrecondition, coder.String())
	case errorCode.ErrAssessmentNotFound,
		errorCode.ErrAssessmentTesteeNotFound,
		errorCode.ErrAssessmentQuestionnaireNotFound,
		errorCode.ErrAssessmentAnswerSheetNotFound,
		errorCode.ErrAssessmentScaleNotFound,
		errorCode.ErrAssessmentScoreNotFound,
		errorCode.ErrMedicalScaleNotFound,
		errorCode.ErrAnswerSheetNotFound,
		errorCode.ErrQuestionnaireNotFound,
		errorCode.ErrInterpretReportNotFound:
		return status.Error(codes.NotFound, coder.String())
	case errorCode.ErrAssessmentDuplicate:
		return status.Error(codes.AlreadyExists, coder.String())
	case errorCode.ErrPermissionDenied, errorCode.ErrForbidden:
		return status.Error(codes.PermissionDenied, coder.String())
	case errorCode.ErrDatabase:
		return status.Error(codes.Unavailable, "dependency unavailable")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
