package service

import (
	"context"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	runqueryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
)

func evaluateFailureResponse(
	ctx context.Context,
	runQuery runqueryApp.Service,
	assessmentID uint64,
	message string,
) *pb.EvaluateAssessmentResponse {
	resp := &pb.EvaluateAssessmentResponse{
		Success: false,
		Status:  "failed",
		Message: message,
	}
	applyLatestRunFailureMetadata(ctx, runQuery, assessmentID, func(retryable bool, runID, failureKind string) {
		resp.Retryable = retryable
		resp.RunId = runID
		resp.FailureKind = failureKind
	})
	return resp
}

func generateReportFailureResponse(
	ctx context.Context,
	runQuery runqueryApp.Service,
	assessmentID uint64,
	message string,
) *pb.GenerateReportFromAssessmentResponse {
	resp := &pb.GenerateReportFromAssessmentResponse{
		Success: false,
		Status:  "failed",
		Message: message,
	}
	applyLatestRunFailureMetadata(ctx, runQuery, assessmentID, func(retryable bool, runID, failureKind string) {
		resp.Retryable = retryable
		resp.RunId = runID
		resp.FailureKind = failureKind
	})
	return resp
}

func applyLatestRunFailureMetadata(
	ctx context.Context,
	runQuery runqueryApp.Service,
	assessmentID uint64,
	apply func(retryable bool, runID, failureKind string),
) {
	if runQuery == nil || assessmentID == 0 || apply == nil {
		return
	}
	run, err := runQuery.FindLatestByAssessmentID(ctx, assessmentID)
	if err != nil || run == nil {
		return
	}
	apply(run.Retryable, run.RunID, run.ErrorCode)
}
